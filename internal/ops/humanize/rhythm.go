package humanize

import (
	"strings"
	"unicode"
)

type rhythmStats struct {
	sentences        int
	words            int
	avgSentence      float64
	short            int
	long             int
	veryLong         int
	maxRun           int
	casualStarters   int
	contractions     int
	formalConnectors []string
	functionRatio    float64
	avgWordLength    float64
	emDashes         int
	paragraphs       int
	oneSentenceParas int
}

var functionWords = func() map[string]bool {
	words := `the a an and or but so to of in on at for with by from up about into over after before between out against during without is are was were be been being have has had do does did will would can could should may might must it its it's this that that's these those i i'm you you're he she we we're they they're me him her us them my your his our their not no as if then than when where which who whom whose what how why all any both each few more most other some such only own same too very just also now here there don't doesn't didn't isn't aren't wasn't weren't can't cannot won't wouldn't shouldn't couldn't haven't hasn't hadn't`
	return func() map[string]bool {
		out := map[string]bool{}
		for _, word := range strings.Fields(words) {
			out[word] = true
		}
		return out
	}()
}()

func analyzeRhythm(text string) rhythmStats {
	clean := stripPlaceholders(text)
	var stats rhythmStats
	stats.emDashes = strings.Count(clean, "—")

	paragraphs := blankLines.Split(strings.TrimSpace(clean), -1)
	for _, paragraph := range paragraphs {
		if strings.TrimSpace(paragraph) == "" {
			continue
		}
		stats.paragraphs++
		if len(splitSentences(paragraph)) == 1 {
			stats.oneSentenceParas++
		}
	}

	sentences := splitSentences(clean)
	run := 1
	previous := 0
	for _, sentence := range sentences {
		words := wordFields(sentence)
		if len(words) == 0 {
			continue
		}
		stats.sentences++
		stats.words += len(words)
		if len(words) < 6 {
			stats.short++
		}
		if len(words) > 25 {
			stats.long++
		}
		if len(words) > 40 {
			stats.veryLong++
		}
		first := strings.ToLower(strings.Trim(words[0], `"'“”‘’()[]{}.,;:!?`))
		switch first {
		case "and", "but", "so", "then", "still":
			stats.casualStarters++
		}
		switch first {
		case "moreover", "furthermore", "additionally", "however", "therefore", "thus", "consequently", "nevertheless", "nonetheless":
			stats.formalConnectors = append(stats.formalConnectors, first)
		}
		for _, word := range words {
			if strings.ContainsAny(word, "'’") {
				stats.contractions++
			}
		}
		if previous > 0 {
			diff := len(words) - previous
			if diff < 0 {
				diff = -diff
			}
			if diff <= 3 {
				run++
				if run > stats.maxRun {
					stats.maxRun = run
				}
			} else {
				run = 1
			}
		}
		previous = len(words)
	}
	if stats.sentences > 0 {
		stats.avgSentence = float64(stats.words) / float64(stats.sentences)
	}

	allWords := wordFields(clean)
	if len(allWords) > 0 {
		functionCount := 0
		letterCount := 0
		for _, word := range allWords {
			lower := strings.ToLower(strings.Trim(word, `"'“”‘’()[]{}.,;:!?`))
			if functionWords[lower] {
				functionCount++
			}
			for _, r := range word {
				if unicode.IsLetter(r) {
					letterCount++
				}
			}
		}
		stats.functionRatio = float64(functionCount) / float64(len(allWords))
		stats.avgWordLength = float64(letterCount) / float64(len(allWords))
	}
	return stats
}

func wordFields(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r)
	})
}

func rhythmIssues(stats rhythmStats, allowContractions bool) []string {
	if stats.sentences < 3 || stats.words < 45 {
		return nil
	}
	var issues []string
	add := func(issue string) { issues = append(issues, issue) }

	avgLimit := 21.0
	funcFloor := 0.32
	wordCeiling := 4.9
	longShareLimit := 0.15
	if !allowContractions {
		avgLimit = 24
		funcFloor = 0.31
		wordCeiling = 5.3
		longShareLimit = 0.20
	}
	if stats.avgSentence > avgLimit {
		add("the rhythm is too even and lecture-like; average sentence length should be 12-18 words, so split long sentences and add short ones")
	}
	if stats.short == 0 {
		add("add at least one sentence under 6 words")
	}
	if stats.sentences > 0 && float64(stats.long)/float64(stats.sentences) > longShareLimit {
		add("split some sentences over 25 words")
	}
	if stats.veryLong > 0 {
		add("no sentence should run past 40 words")
	}
	if stats.maxRun >= 4 {
		add("several sentences in a row have similar lengths; vary the rhythm")
	}
	if allowContractions && stats.casualStarters == 0 {
		add("start some sentences with And, But, So, Then, or Still")
	}
	if allowContractions && stats.contractions == 0 {
		add("use contractions such as don't, it's, we're, that's")
	}
	if stats.functionRatio > 0 && stats.functionRatio < funcFloor {
		add("the text is too noun-heavy; use more small joining words and pronouns (the, a, and, to, it, that, you, we)")
	}
	if stats.avgWordLength > wordCeiling {
		add("the words are long and formal; use shorter, plainer words where you can")
	}
	if len(stats.formalConnectors) > 0 {
		add("remove formal connectors such as " + strings.Join(uniqueStrings(stats.formalConnectors), ", "))
	}
	if stats.paragraphs >= 3 && stats.oneSentenceParas == 0 {
		add("make one paragraph a single sentence so the page has some air")
	}
	return issues
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range in {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
