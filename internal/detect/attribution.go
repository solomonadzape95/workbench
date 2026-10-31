package detect

import (
	"fmt"
	"sort"
	"strings"
)

// attribute is a fingerprint estimate. It is intentionally weaker than the
// main score: models copy each other, and a humanizer removes most fingerprints.
func attribute(doc Document, stats Stats, ai float64, sentences []*scoredSentence) []Attribution {
	if ai < 35 {
		return []Attribution{{
			Family:     "human",
			Label:      "No strong AI fingerprint",
			Score:      0,
			Confidence: "medium",
			Evidence:   humanEvidence(stats),
			Notes:      "The rhythm and vocabulary look more human than machine. This is not proof of authorship.",
		}}
	}

	text := make([]string, 0, len(doc.Paragraphs))
	for _, paragraph := range doc.Paragraphs {
		text = append(text, paragraph.Text)
	}
	joined := strings.Join(text, " ")

	type ranked struct {
		family familyFingerprint
		raw    float64
		hits   []phraseMatch
	}
	var rankings []ranked
	for _, family := range familyFingerprints {
		result := phraseWeightInWithCounts(joined, family.Phrases)
		raw := result.weight
		// A few structural clues are shared by the assistant families. They
		// help the score but do not decide between them.
		if stats.EmDashRate > 1.0 && (family.Key == "chatgpt" || family.Key == "claude") {
			raw += 0.4
		}
		if stats.FormalStarterRate > 0.35 && family.Key == "chatgpt" {
			raw += 0.3
		}
		rankings = append(rankings, ranked{family: family, raw: raw, hits: result.matches})
	}
	sort.SliceStable(rankings, func(i, j int) bool { return rankings[i].raw > rankings[j].raw })

	top := rankings[0]
	second := rankings[1]
	if top.raw < 2.8 || top.raw < second.raw*1.15 {
		evidence := genericEvidence(sentences)
		return []Attribution{{
			Family:     "generic",
			Label:      "Generic LLM style / unknown",
			Score:      round1(clamp(ai*0.55, 20, 60)),
			Confidence: "low",
			Evidence:   evidence,
			Notes:      "The prose has machine-like signals, but no model-family fingerprint is strong enough to attribute it.",
		}}
	}

	confidence := "low"
	if top.raw >= 6 && top.raw >= second.raw*1.6 {
		confidence = "high"
	} else if top.raw >= 3 && top.raw >= second.raw*1.25 {
		confidence = "medium"
	}

	out := make([]Attribution, 0, 4)
	for i, entry := range rankings {
		if i >= 4 {
			break
		}
		score := 100 * entry.raw / (entry.raw + 2.5)
		evidence := make([]string, 0, 5)
		for _, hit := range entry.hits {
			if len(evidence) >= 5 {
				break
			}
			evidence = append(evidence, hit.phrase)
		}
		if len(evidence) == 0 {
			evidence = genericEvidence(sentences)
		}
		notes := entry.family.Notes
		if i == 0 {
			notes += " Fingerprint only; models share vocabulary, so treat this as a lead rather than an identification."
		} else if i == 1 && top.raw < second.raw*1.5 {
			notes += " Close to the top family, so the ranking is uncertain."
		}
		familyConfidence := confidence
		if i > 0 {
			familyConfidence = "low"
			if entry.raw >= top.raw*0.85 {
				familyConfidence = "medium"
			}
		}
		out = append(out, Attribution{
			Family:     entry.family.Key,
			Label:      entry.family.Label,
			Score:      round1(score),
			Confidence: familyConfidence,
			Evidence:   evidence,
			Notes:      notes,
		})
	}
	return out
}

func humanEvidence(stats Stats) []string {
	var evidence []string
	if stats.SentenceCV >= 0.45 {
		evidence = append(evidence, fmt.Sprintf("sentence-length CV %.2f", stats.SentenceCV))
	}
	if stats.ShortSentenceRate >= 0.10 {
		evidence = append(evidence, fmt.Sprintf("%.0f%% short sentences", stats.ShortSentenceRate*100))
	}
	if stats.ContractionRate >= 1 {
		evidence = append(evidence, fmt.Sprintf("%.1f contractions per 1k words", stats.ContractionRate))
	}
	if stats.AIPhraseRate < 1 {
		evidence = append(evidence, "few assistant phrases")
	}
	if len(evidence) == 0 {
		evidence = append(evidence, "no strong assistant fingerprint")
	}
	return evidence
}

func genericEvidence(sentences []*scoredSentence) []string {
	seen := map[string]bool{}
	var evidence []string
	for _, sentence := range sentences {
		for _, phrase := range sentence.PhraseHits {
			if seen[phrase] {
				continue
			}
			seen[phrase] = true
			evidence = append(evidence, phrase)
			if len(evidence) >= 5 {
				return evidence
			}
		}
	}
	if len(evidence) == 0 {
		evidence = append(evidence, "uniform sentence rhythm")
	}
	return evidence
}
