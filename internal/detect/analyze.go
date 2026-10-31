package detect

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	digitStartPattern  = regexp.MustCompile(`^\d`)
	contractionPattern = regexp.MustCompile(`(?i)\b\w+['’](t|s|re|ve|ll|d|m)\b`)
	phraseMu           sync.Mutex
)

// Options tunes the sentence threshold. The default is deliberately near the
// middle: the report should show a mix rather than forcing every sentence into
// one camp.
type Options struct {
	SentenceThreshold float64
	MinWords          int
	// LogProbScore is an optional 0-1 AI-likelihood from an external
	// log-probability scorer (DetectGPT, Fast-DetectGPT, Binoculars, ...).
	LogProbScore  *float64
	LogProbWeight float64
	LogProbDetail string
}

func DefaultOptions() Options {
	return Options{SentenceThreshold: 0.55, MinWords: 1}
}

type scoredSentence struct {
	Text       string
	Paragraph  int
	Words      int
	Length     int
	Local      float64
	AI         float64
	PhraseHits []string
	Reasons    []string
}

type scoredParagraph struct {
	Index     int
	Text      string
	Words     int
	Sentences []*scoredSentence
	AI        float64
	Flagged   int
}

// Analyze turns extracted prose into a full diagnostic report.
func Analyze(doc Document, opts Options) (Report, error) {
	if opts.SentenceThreshold <= 0 || opts.SentenceThreshold >= 1 {
		opts.SentenceThreshold = 0.55
	}
	if opts.MinWords <= 0 {
		opts.MinWords = 1
	}

	paragraphs := make([]*scoredParagraph, 0, len(doc.Paragraphs))
	var sentences []*scoredSentence
	var allTokens []string
	var lens []int
	var paragraphWords []int

	for _, input := range doc.Paragraphs {
		text := strings.TrimSpace(collapseSpace(input.Text))
		if text == "" {
			continue
		}
		parts := proseSentences(text)
		if len(parts) == 0 {
			continue
		}
		para := &scoredParagraph{Index: input.Index, Text: text}
		for _, part := range parts {
			tokens := wordTokens(part)
			if len(tokens) < opts.MinWords {
				continue
			}
			s := &scoredSentence{
				Text:      part,
				Paragraph: len(paragraphs),
				Words:     len(tokens),
				Length:    len(tokens),
			}
			para.Sentences = append(para.Sentences, s)
			para.Words += len(tokens)
			sentences = append(sentences, s)
			allTokens = append(allTokens, tokens...)
			lens = append(lens, len(tokens))
		}
		if len(para.Sentences) == 0 {
			continue
		}
		paragraphs = append(paragraphs, para)
		paragraphWords = append(paragraphWords, para.Words)
	}
	if len(sentences) == 0 {
		return Report{}, fmt.Errorf("no prose sentences found")
	}

	stats := documentStats(allTokens, lens, paragraphWords, sentences)
	formal := formalProfile(stats)
	stats.Profile = "conversational"
	if formal {
		stats.Profile = "formal / academic"
	}
	docRaw, signals := scoreDocument(stats, formal, opts)
	docProb := sigmoid((docRaw - 0.46) * 7.0)
	if formal {
		docProb = sigmoid((docRaw - 0.37) * 8.0)
	}

	// Sentence-level scoring uses both local evidence and the document prior.
	meanLen := stats.AvgSentenceWords
	for i, sentence := range sentences {
		sentence.PhraseHits = matchedPhrases(sentence.Text, aiPhrases)
		phraseWeight := phraseWeightIn(sentence.Text, aiPhrases)
		localPhrase := clamp01(phraseWeight / 5.0)
		localUniform := 1 - clamp01(math.Abs(float64(sentence.Length)-meanLen)/math.Max(meanLen, 1)/0.65)
		firstWord := firstToken(sentence.Text)
		localFormal := 0.0
		if formalStarters[firstWord] {
			localFormal = 1
		}
		localFunction := 1 - clamp01((sentenceFunctionRate(sentence.Text)-0.35)/0.20)
		localLong := clamp01((sentenceLongWordRate(sentence.Text) - 0.06) / 0.25)
		localHedge := clamp01(float64(countSetWords(sentence.Text, hedges)) / 2)
		localContraction := 0.0
		if hasContraction(sentence.Text) {
			localContraction = 1
		}
		neighbor := neighbourUniformity(lens, i)
		local := 0.30*localPhrase + 0.20*localUniform + 0.15*localFormal +
			0.10*localFunction + 0.10*localLong + 0.05*localHedge +
			0.10*neighbor - 0.08*localContraction
		local = clamp01(local)
		sentence.Local = local
		sentence.AI = clamp01(0.58*docProb + 0.42*local + 0.10*localPhrase)
		sentence.Reasons = sentenceReasons(sentence, localUniform, localFormal, localFunction, localLong, neighbor, localPhrase, stats)
	}

	var sentenceBuckets [10]int
	for _, sentence := range sentences {
		bucket := int(sentence.AI * 10)
		if bucket > 9 {
			bucket = 9
		}
		sentenceBuckets[bucket]++
	}
	stats.SentenceBuckets = sentenceBuckets

	flagged := make([]*scoredSentence, 0)
	for _, sentence := range sentences {
		if sentence.AI >= opts.SentenceThreshold {
			flagged = append(flagged, sentence)
		}
	}
	flaggedTotal := len(flagged)
	sentenceShare := float64(flaggedTotal) / float64(len(sentences))
	ai := 100 * (0.55*docProb + 0.45*sentenceShare)
	if len(sentences) < 3 {
		ai = 100 * docProb
	}
	ai = clamp(ai, 1, 99)
	human := 100 - ai
	reliability := reliabilityScore(stats, ai, signals, len(sentences))
	confidence := confidenceLabel(reliability, stats.Words)
	verdict := verdictFor(ai, reliability, confidence)

	// Paragraph rollups.
	for _, para := range paragraphs {
		total := 0.0
		flag := 0
		for _, sentence := range para.Sentences {
			total += sentence.AI
			if sentence.AI >= opts.SentenceThreshold {
				flag++
			}
		}
		para.AI = 100 * total / float64(len(para.Sentences))
		para.Flagged = flag
	}

	// The report only needs the strongest flagged sentences.
	sort.SliceStable(flagged, func(i, j int) bool { return flagged[i].AI > flagged[j].AI })
	if len(flagged) > 100 {
		flagged = flagged[:100]
	}
	flaggedOut := make([]Sentence, 0, len(flagged))
	for i, sentence := range flagged {
		flaggedOut = append(flaggedOut, Sentence{
			Index:     i,
			Paragraph: sentence.Paragraph,
			Text:      sentence.Text,
			Words:     sentence.Words,
			AI:        round1(sentence.AI * 100),
			Reasons:   sentence.Reasons,
		})
	}

	paragraphOut := make([]Paragraph, 0, len(paragraphs))
	for _, para := range paragraphs {
		preview := para.Text
		if len([]rune(preview)) > 180 {
			preview = string([]rune(preview)[:177]) + "…"
		}
		paragraphOut = append(paragraphOut, Paragraph{
			Index:     para.Index,
			Words:     para.Words,
			Sentences: len(para.Sentences),
			AI:        round1(para.AI),
			Flagged:   para.Flagged,
			Preview:   preview,
		})
	}

	attribution := attribute(doc, stats, ai, sentences)
	top := ""
	note := ""
	if len(attribution) > 0 {
		top = attribution[0].Label
		note = attribution[0].Notes
	}

	summary := Summary{
		AI:              round1(ai),
		Human:           round1(human),
		Verdict:         verdict,
		Confidence:      confidence,
		Reliability:     round1(reliability),
		SentenceAI:      round1(sentenceShare * 100),
		Flagged:         flaggedTotal,
		Words:           stats.Words,
		Sentences:       stats.Sentences,
		Paragraphs:      stats.Paragraphs,
		AvgSentence:     round1(stats.AvgSentenceWords),
		SentenceCV:      round2(stats.SentenceCV),
		Profile:         stats.Profile,
		AttributionTop:  top,
		AttributionNote: note,
	}

	warnings := buildWarnings(doc, stats, ai, reliability, signals)
	report := Report{
		Version:     Version,
		GeneratedAt: time.Now().UTC(),
		Source:      doc.Source,
		Summary:     summary,
		Signals:     signals,
		Paragraphs:  paragraphOut,
		Flagged:     flaggedOut,
		Attribution: attribution,
		Warnings:    warnings,
		Methodology: detectorMethodology,
		References:  detectorReferences,
		Stats:       stats,
	}
	return report, nil
}

func documentStats(tokens []string, lens, paragraphWords []int, sentences []*scoredSentence) Stats {
	stats := Stats{
		Words:        len(tokens),
		Sentences:    len(lens),
		Paragraphs:   len(paragraphWords),
		ContentWords: len(contentTokens(tokens)),
	}
	if stats.Sentences > 0 {
		mean, std := meanStd(lens)
		stats.AvgSentenceWords = mean
		if mean > 0 {
			stats.SentenceCV = std / mean
		}
		short, long := 0, 0
		for _, n := range lens {
			if n <= 7 {
				short++
			}
			if n >= 30 {
				long++
			}
		}
		stats.ShortSentenceRate = float64(short) / float64(len(lens))
		stats.LongSentenceRate = float64(long) / float64(len(lens))
		stats.UniformRunRate = uniformRunShare(lens)
	}
	if len(paragraphWords) > 0 {
		mean, std := meanStd(paragraphWords)
		if mean > 0 {
			stats.ParagraphCV = std / mean
		}
	}
	functions, pronounCount, contractions, longWords, totalLen, hedgesCount := 0, 0, 0, 0, 0, 0
	for _, token := range tokens {
		if functionWords[token] {
			functions++
		}
		if pronouns[token] {
			pronounCount++
		}
		if hasContraction(token) {
			contractions++
		}
		if len([]rune(token)) >= 8 {
			longWords++
		}
		if hedges[token] {
			hedgesCount++
		}
		totalLen += len([]rune(token))
	}
	rawText := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		rawText = append(rawText, sentence.Text)
	}
	joinedText := strings.Join(rawText, " ")
	if len(tokens) > 0 {
		stats.FunctionWordRate = float64(functions) / float64(len(tokens))
		stats.PronounRate = float64(pronounCount) / float64(len(tokens))
		stats.ContractionRate = float64(contractions) / float64(len(tokens)) * 1000
		stats.LongWordRate = float64(longWords) / float64(len(tokens))
		stats.AvgWordLength = float64(totalLen) / float64(len(tokens))
		stats.HedgeRate = float64(hedgesCount) / float64(len(tokens)) * 1000
	}
	phraseResult := phraseWeightInWithCounts(joinedText, aiPhrases)
	phraseHits := 0
	for _, match := range phraseResult.matches {
		phraseHits += match.count
	}
	stats.AIPhraseHits = phraseHits
	stats.AIPhraseRate = float64(phraseHits) / math.Max(float64(len(tokens)), 1) * 1000
	stats.AIPhraseWeight = phraseResult.weight / math.Max(float64(len(tokens)), 1) * 1000
	stats.FormalConnector = float64(countSetWords(joinedText, formalConnectors)) / math.Max(float64(len(tokens)), 1) * 1000
	stats.ResearchRegister = float64(countSetWords(joinedText, researchRegister)) / math.Max(float64(len(tokens)), 1) * 1000
	stats.EmDashRate = float64(strings.Count(joinedText, "—")) / math.Max(float64(len(tokens)), 1) * 1000
	stats.SemicolonRate = float64(strings.Count(joinedText, ";")) / math.Max(float64(len(tokens)), 1) * 1000
	stats.RepeatedTrigrams = repeatedTrigramRate(tokens)

	starters := map[string]bool{}
	formalStarts := 0
	capitalized, digits := 0, 0
	for _, sentence := range sentences {
		words := strings.Fields(sentence.Text)
		if len(words) == 0 {
			continue
		}
		first := strings.ToLower(trimWord(words[0]))
		starters[first] = true
		if formalStarters[first] {
			formalStarts++
		}
		for i, word := range words {
			clean := trimWord(word)
			if clean == "" {
				continue
			}
			if i > 0 && unicode.IsUpper([]rune(clean)[0]) {
				capitalized++
			}
			if digitStartPattern.MatchString(clean) {
				digits++
			}
		}
	}
	if stats.Sentences > 0 {
		stats.StarterDiversity = float64(len(starters)) / float64(stats.Sentences)
		stats.FormalStarterRate = float64(formalStarts) / float64(stats.Sentences)
	}
	if stats.Words > 0 {
		stats.SpecificityRate = float64(capitalized+digits) / float64(stats.Words)
	}
	return stats
}

type signalSpec struct {
	key     string
	label   string
	group   string
	weight  float64
	raw     float64
	display string
	score   float64
	detail  string
}

func formalProfile(stats Stats) bool {
	return stats.Words >= 120 && stats.ContractionRate < 2 && stats.PronounRate < 0.025
}

func scoreDocument(stats Stats, formal bool, opts Options) (float64, []Signal) {
	specs := []signalSpec{
		{"sentence_cv", "Sentence rhythm", "rhythm", 0.14, stats.SentenceCV, fmt.Sprintf("CV %.2f", stats.SentenceCV), normLow(stats.SentenceCV, 0.30, 0.75), "How much sentence lengths vary; AI often clusters around a narrow range."},
		{"uniform_runs", "Uniform runs", "rhythm", 0.12, stats.UniformRunRate, fmt.Sprintf("%.0f%%", stats.UniformRunRate*100), normHigh(stats.UniformRunRate, 0.10, 0.55), "Share of sentences sitting in runs of three or more similar lengths."},
		{"short_sentences", "Short sentences", "rhythm", 0.09, stats.ShortSentenceRate, fmt.Sprintf("%.0f%%", stats.ShortSentenceRate*100), normLow(stats.ShortSentenceRate, 0.03, 0.25), "Human drafts usually contain more very short sentences and asides."},
		{"paragraph_cv", "Paragraph variance", "rhythm", 0.04, stats.ParagraphCV, fmt.Sprintf("CV %.2f", stats.ParagraphCV), normLow(stats.ParagraphCV, 0.20, 0.70), "Uniform paragraph sizes are a weak machine-like signal."},
		{"ai_phrases", "AI phrases", "vocabulary", 0.16, stats.AIPhraseWeight, phraseDisplay(stats), normHigh(stats.AIPhraseWeight, 0.5, 30.0), "Assistant vocabulary and template phrases per 1,000 words, weighted by strength."},
		{"formal_connectors", "Formal connectors", "vocabulary", 0.08, stats.FormalConnector, fmt.Sprintf("%.1f/1k", stats.FormalConnector), normHigh(stats.FormalConnector, 1.0, 80.0), "Moreover, furthermore, additionally, however, therefore, in conclusion."},
		{"function_words", "Function words", "vocabulary", 0.04, stats.FunctionWordRate, fmt.Sprintf("%.0f%%", stats.FunctionWordRate*100), normLow(stats.FunctionWordRate, 0.30, 0.55), "Articles, pronouns, prepositions, and conjunctions; human prose tends to use more."},
		{"long_words", "Long words", "vocabulary", 0.03, stats.LongWordRate, fmt.Sprintf("%.0f%%", stats.LongWordRate*100), normHigh(stats.LongWordRate, 0.10, 0.30), "Share of words with eight or more characters."},
		{"contractions", "Contractions", "vocabulary", 0.02, stats.ContractionRate, fmt.Sprintf("%.1f/1k", stats.ContractionRate), normLow(stats.ContractionRate, 1.0, 30.0), "Contractions per 1,000 words; formal human writing can legitimately have none."},
		{"hedging", "Hedging", "vocabulary", 0.03, stats.HedgeRate, fmt.Sprintf("%.1f/1k", stats.HedgeRate), normHigh(stats.HedgeRate, 2.0, 30.0), "May, might, often, typically, generally, suggests."},
		{"pronouns", "Pronouns", "vocabulary", 0.02, stats.PronounRate, fmt.Sprintf("%.0f%%", stats.PronounRate*100), normLow(stats.PronounRate, 0.01, 0.08), "First- and second-person pronouns; human prose often uses more."},
		{"starter_diversity", "Starter diversity", "structure", 0.06, stats.StarterDiversity, fmt.Sprintf("%.0f%%", stats.StarterDiversity*100), normLow(stats.StarterDiversity, 0.45, 0.80), "Unique sentence-opening words divided by sentence count."},
		{"formal_starters", "Formal starters", "structure", 0.03, stats.FormalStarterRate, fmt.Sprintf("%.0f%%", stats.FormalStarterRate*100), normHigh(stats.FormalStarterRate, 0.08, 0.45), "Discourse-marker openers such as However, Moreover, Therefore, and In conclusion."},
		{"repetition", "Repeated trigrams", "structure", 0.04, stats.RepeatedTrigrams, fmt.Sprintf("%.1f%%", stats.RepeatedTrigrams*100), normHigh(stats.RepeatedTrigrams, 0.005, 0.06), "Repeated three-word content sequences; templates repeat more."},
		{"specificity", "Specificity", "structure", 0.03, stats.SpecificityRate, fmt.Sprintf("%.1f%%", stats.SpecificityRate*100), normLow(stats.SpecificityRate, 0.02, 0.12), "Names, places, and numbers; specific human writing usually carries more."},
		{"research_register", "Research register", "vocabulary", 0.20, stats.ResearchRegister, fmt.Sprintf("%.1f/1k", stats.ResearchRegister), normLow(stats.ResearchRegister, 1.0, 60.0), "Study, survey, interview, sample, method, analysis, and findings; academic human writing usually carries more."},
		{"em_dash", "Em dashes", "punctuation", 0.03, stats.EmDashRate, fmt.Sprintf("%.1f/1k", stats.EmDashRate), normHigh(stats.EmDashRate, 0.2, 3.0), "Em dashes per 1,000 words; assistant prose often overuses them."},
		{"semicolons", "Semicolons", "punctuation", 0.02, stats.SemicolonRate, fmt.Sprintf("%.1f/1k", stats.SemicolonRate), normHigh(stats.SemicolonRate, 0.2, 4.0), "Semicolons per 1,000 words; weak signal, style-dependent."},
	}
	if opts.LogProbScore != nil {
		weight := opts.LogProbWeight
		if weight <= 0 || weight > 1 {
			weight = 0.15
		}
		score := clamp01(*opts.LogProbScore)
		detail := opts.LogProbDetail
		if detail == "" {
			detail = "External local log-probability scorer."
		}
		specs = append(specs, signalSpec{
			key: "logprob", label: "Log-probability", group: "model", weight: weight,
			raw: score * 100, display: fmt.Sprintf("%.0f%%", score*100), score: score, detail: detail,
		})
	}
	if stats.Paragraphs < 2 {
		for i := range specs {
			if specs[i].key == "paragraph_cv" {
				specs[i].score = 0
				specs[i].display = "n/a"
				specs[i].detail = "Need at least two paragraphs to measure paragraph uniformity."
			}
		}
	}
	if formal {
		multipliers := map[string]float64{
			"sentence_cv":       1.45,
			"uniform_runs":      1.35,
			"short_sentences":   1.30,
			"paragraph_cv":      1.20,
			"ai_phrases":        0.55,
			"formal_connectors": 1.25,
			"function_words":    0.70,
			"long_words":        0.80,
			"contractions":      0.50,
			"hedging":           1.00,
			"pronouns":          0.50,
			"starter_diversity": 1.50,
			"formal_starters":   0.80,
			"repetition":        1.00,
			"specificity":       0.80,
			"research_register": 1.00,
			"em_dash":           1.00,
			"semicolons":        1.00,
		}
		for i := range specs {
			if multiplier, ok := multipliers[specs[i].key]; ok {
				specs[i].weight *= multiplier
			}
		}
	}
	totalWeight := 0.0
	raw := 0.0
	signals := make([]Signal, 0, len(specs))
	for _, spec := range specs {
		totalWeight += spec.weight
		raw += spec.weight * spec.score
	}
	if totalWeight == 0 {
		return 0, nil
	}
	raw /= totalWeight
	for _, spec := range specs {
		direction := "human-like"
		if spec.score >= 0.5 {
			direction = "AI-like"
		}
		signals = append(signals, Signal{
			Key:          spec.key,
			Label:        spec.label,
			Group:        spec.group,
			Raw:          spec.raw,
			Display:      spec.display,
			AIScore:      round1(spec.score * 100),
			Weight:       spec.weight / totalWeight,
			Contribution: round1(spec.weight / totalWeight * spec.score * 100),
			Direction:    direction,
			Detail:       spec.detail,
		})
	}
	sort.SliceStable(signals, func(i, j int) bool {
		return signals[i].Contribution > signals[j].Contribution
	})
	return raw, signals
}

func sentenceReasons(s *scoredSentence, uniform, formal, functionLow, long, neighbor, phrase float64, stats Stats) []string {
	var reasons []string
	if len(s.PhraseHits) > 0 {
		evidence := s.PhraseHits
		if len(evidence) > 2 {
			evidence = evidence[:2]
		}
		reasons = append(reasons, "assistant phrase: "+strings.Join(evidence, ", "))
	}
	if uniform > 0.72 {
		reasons = append(reasons, "length close to the document average")
	}
	if formal > 0.5 {
		reasons = append(reasons, "formal sentence opener")
	}
	if functionLow > 0.65 {
		reasons = append(reasons, "few function words")
	}
	if long > 0.55 {
		reasons = append(reasons, "long-word heavy")
	}
	if neighbor > 0.8 {
		reasons = append(reasons, "similar length to neighbouring sentences")
	}
	if phrase > 0.5 {
		reasons = append(reasons, "template-like phrasing")
	}
	if len(reasons) == 0 {
		if stats.SentenceCV < 0.45 {
			reasons = append(reasons, "document-level rhythm is uniform")
		} else {
			reasons = append(reasons, "high document-level stylometric score")
		}
	}
	return reasons
}

func buildWarnings(doc Document, stats Stats, ai, reliability float64, signals []Signal) []string {
	warnings := append([]string(nil), doc.Warnings...)
	if stats.Words < 300 {
		warnings = append(warnings, fmt.Sprintf("Short sample: %d words. Detectors are much less reliable below 300 words; treat this score as a weak signal.", stats.Words))
	}
	if stats.Sentences < 5 {
		warnings = append(warnings, "Very few prose sentences were found; the sentence-share component is unstable.")
	}
	if stats.Profile == "formal / academic" {
		warnings = append(warnings, "Formal academic prose can look AI-like. Contractions and first person are not required for human writing; use the sentence evidence, not the headline alone.")
	}
	if ai >= 35 && ai <= 65 {
		warnings = append(warnings, "The document sits in the uncertain middle. Check the flagged sentences and the signal table before acting on the number.")
	}
	if stats.AIPhraseRate > 1.0 {
		warnings = append(warnings, "Assistant-style phrases were detected. That is stronger evidence than rhythm alone, but still not proof.")
	}
	if reliability < 40 {
		warnings = append(warnings, "Low reliability: the text is short, mixed, or the signals disagree.")
	}
	// Keep the most useful first; the UI truncates.
	if len(warnings) > 6 {
		warnings = warnings[:6]
	}
	return warnings
}

func phraseDisplay(stats Stats) string {
	if stats.AIPhraseRate > 100 {
		return fmt.Sprintf("%d phrases", stats.AIPhraseHits)
	}
	return fmt.Sprintf("%.1f/1k", stats.AIPhraseRate)
}

func reliabilityScore(stats Stats, ai float64, signals []Signal, sentences int) float64 {
	wordScore := math.Min(float64(stats.Words)/500, 1) * 40
	sentenceScore := math.Min(float64(sentences)/20, 1) * 25
	var values []float64
	for _, signal := range signals {
		values = append(values, signal.AIScore/100)
	}
	agreement := 0.0
	if len(values) > 1 {
		mean, std := meanStdFloat(values)
		_ = mean
		agreement = 1 - clamp01(std/0.30)
	}
	agreement *= 20
	margin := clamp01(math.Abs(ai-50)/40) * 15
	score := wordScore + sentenceScore + agreement + margin
	if stats.Words < 150 {
		score = math.Min(score, 35)
	} else if stats.Words < 300 {
		score = math.Min(score, 65)
	}
	return clamp(score, 0, 100)
}

func confidenceLabel(reliability float64, words int) string {
	switch {
	case words < 150 || reliability < 40:
		return "low"
	case words < 300 || reliability < 70:
		return "medium"
	default:
		return "high"
	}
}

func verdictFor(ai, reliability float64, confidence string) string {
	var verdict string
	switch {
	case ai >= 80:
		verdict = "Likely AI-generated"
	case ai >= 60:
		verdict = "Likely AI-assisted"
	case ai >= 40:
		verdict = "Mixed or uncertain"
	case ai >= 20:
		verdict = "Mostly human, with AI-like passages"
	default:
		verdict = "Likely human-written"
	}
	if confidence == "low" && ai >= 55 {
		return "Leaning AI, but low confidence"
	}
	if confidence == "low" && ai <= 45 {
		return "Leaning human, but low confidence"
	}
	return verdict
}

func meanStd(values []int) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	total := 0.0
	for _, value := range values {
		total += float64(value)
	}
	mean := total / float64(len(values))
	variance := 0.0
	for _, value := range values {
		diff := float64(value) - mean
		variance += diff * diff
	}
	return mean, math.Sqrt(variance / float64(len(values)))
}

func meanStdFloat(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	mean := total / float64(len(values))
	variance := 0.0
	for _, value := range values {
		diff := value - mean
		variance += diff * diff
	}
	return mean, math.Sqrt(variance / float64(len(values)))
}

func uniformRunShare(lens []int) float64 {
	if len(lens) < 3 {
		return 0
	}
	inRun := make([]bool, len(lens))
	for i := 0; i+2 < len(lens); i++ {
		low, high := lens[i], lens[i]
		for j := i; j < i+3; j++ {
			if lens[j] < low {
				low = lens[j]
			}
			if lens[j] > high {
				high = lens[j]
			}
		}
		if high-low <= 2 {
			for j := i; j < i+3; j++ {
				inRun[j] = true
			}
		}
	}
	count := 0
	for _, yes := range inRun {
		if yes {
			count++
		}
	}
	return float64(count) / float64(len(lens))
}

func neighbourUniformity(lens []int, i int) float64 {
	score := 0.0
	checks := 0.0
	for _, j := range []int{i - 1, i + 1} {
		if j < 0 || j >= len(lens) {
			continue
		}
		checks++
		if absInt(lens[j]-lens[i]) <= 2 {
			score++
		}
	}
	if checks == 0 {
		return 0
	}
	return score / checks
}

func repeatedTrigramRate(tokens []string) float64 {
	if len(tokens) < 6 {
		return 0
	}
	content := contentTokens(tokens)
	if len(content) < 6 {
		return 0
	}
	counts := map[string]int{}
	total := 0
	for i := 0; i+2 < len(content); i++ {
		key := simpleStem(content[i]) + " " + simpleStem(content[i+1]) + " " + simpleStem(content[i+2])
		counts[key]++
		total++
	}
	if total == 0 {
		return 0
	}
	repeated := 0
	for _, count := range counts {
		if count > 1 {
			repeated += count - 1
		}
	}
	return float64(repeated) / float64(total)
}

func sentenceFunctionRate(text string) float64 {
	tokens := wordTokens(text)
	if len(tokens) == 0 {
		return 0
	}
	count := 0
	for _, token := range tokens {
		if functionWords[token] {
			count++
		}
	}
	return float64(count) / float64(len(tokens))
}

func sentenceLongWordRate(text string) float64 {
	tokens := wordTokens(text)
	if len(tokens) == 0 {
		return 0
	}
	long := 0
	for _, token := range tokens {
		if len([]rune(token)) >= 8 {
			long++
		}
	}
	return float64(long) / float64(len(tokens))
}

func firstToken(text string) string {
	words := strings.Fields(strings.TrimSpace(text))
	if len(words) == 0 {
		return ""
	}
	return strings.ToLower(trimWord(words[0]))
}

func hasContraction(text string) bool {
	return strings.ContainsAny(text, "'’") && contractionPattern.MatchString(text)
}

func trimWord(word string) string {
	return strings.Trim(word, `.,;:!?()[]{}<>"'“”‘’—–-/\\|@#$%^&*+=~`)
}

func countSetWords(text string, set map[string]bool) int {
	count := 0
	for _, token := range wordTokens(text) {
		if set[token] {
			count++
		}
	}
	return count
}

func phraseWeightIn(text string, phrases []weightedPhrase) float64 {
	return phraseWeightInWithCounts(text, phrases).weight
}

type phraseMatch struct {
	phrase string
	count  int
	weight float64
}

type phraseResult struct {
	weight  float64
	matches []phraseMatch
}

func phraseWeightInWithCounts(text string, phrases []weightedPhrase) phraseResult {
	lower := strings.ToLower(text)
	result := phraseResult{}
	for _, phrase := range phrases {
		count := phraseCount(lower, phrase.phrase)
		if count == 0 {
			continue
		}
		// Cap repeated occurrences so one tic cannot dominate the score.
		capped := count
		if capped > 3 {
			capped = 3
		}
		result.weight += phrase.weight * float64(capped)
		result.matches = append(result.matches, phraseMatch{phrase: phrase.phrase, count: count, weight: phrase.weight})
	}
	sort.SliceStable(result.matches, func(i, j int) bool {
		return result.matches[i].weight*float64(result.matches[i].count) > result.matches[j].weight*float64(result.matches[j].count)
	})
	return result
}

func phraseCount(lower, phrase string) int {
	if phrase == "" {
		return 0
	}
	if strings.ContainsAny(phrase, " -'’") || strings.ContainsAny(phrase, ".,;:!?") {
		return strings.Count(lower, phrase)
	}
	phraseMu.Lock()
	re, ok := phraseRegexes[phrase]
	if !ok {
		re = regexp.MustCompile(`\b` + regexp.QuoteMeta(phrase) + `\b`)
		phraseRegexes[phrase] = re
	}
	phraseMu.Unlock()
	return len(re.FindAllStringIndex(lower, -1))
}

var phraseRegexes = map[string]*regexp.Regexp{}

func matchedPhrases(text string, phrases []weightedPhrase) []string {
	result := phraseWeightInWithCounts(text, phrases)
	out := make([]string, 0, len(result.matches))
	for _, match := range result.matches {
		out = append(out, match.phrase)
	}
	return out
}

func sigmoid(x float64) float64 {
	return 1 / (1 + math.Exp(-x))
}

func normHigh(value, low, high float64) float64 {
	return clamp01((value - low) / (high - low))
}

func normLow(value, low, high float64) float64 {
	return 1 - clamp01((value-low)/(high-low))
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func round1(value float64) float64 {
	return math.Round(value*10) / 10
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}
