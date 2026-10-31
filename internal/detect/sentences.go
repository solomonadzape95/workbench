package detect

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	refSentencePattern = regexp.MustCompile(`^[A-Z][A-Za-z'’\-]+,\s+[A-Z]\.`)
	initialPattern     = regexp.MustCompile(`(?:^|\s)[A-Z]\.$`)
	acronymDotPattern  = regexp.MustCompile(`(?:[A-Z]\.){2,}$`)
)

var abbreviations = []string{
	"mr.", "mrs.", "ms.", "dr.", "prof.", "sr.", "jr.", "st.", "vs.", "etc.",
	"e.g.", "i.e.", "fig.", "no.", "vol.", "pp.", "cf.", "al.", "eq.", "ref.",
	"sec.", "ch.", "dept.", "univ.", "inc.", "ltd.", "co.", "corp.", "approx.",
	"est.", "jan.", "feb.", "mar.", "apr.", "jun.", "jul.", "aug.", "sep.",
	"sept.", "oct.", "nov.", "dec.",
}

// splitSentences is deliberately conservative. Academic text is full of
// abbreviations, decimals, initials, and citation years; splitting those into
// fake sentences makes every rhythm signal noisy.
func splitSentences(text string) []string {
	text = collapseSpace(text)
	runes := []rune(text)
	var out []string
	start := 0
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '.', '!', '?':
		default:
			continue
		}
		if runes[i] == '.' && i > 0 && i+1 < len(runes) &&
			unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1]) {
			continue // decimal number
		}
		if runes[i] == '.' && i+1 < len(runes) && runes[i+1] == '.' {
			continue // ellipsis
		}
		prefix := strings.ToLower(strings.TrimSpace(string(runes[start : i+1])))
		if strings.HasSuffix(prefix, ".") && isAbbreviation(prefix) {
			continue
		}
		if initialPattern.MatchString(string(runes[start:i+1])) || acronymDotPattern.MatchString(string(runes[start:i+1])) {
			continue
		}

		end := i + 1
		for end < len(runes) && unicode.IsSpace(runes[end]) {
			end++
		}
		if end >= len(runes) {
			break
		}
		next := runes[end]
		if !unicode.IsUpper(next) && !unicode.IsDigit(next) && !strings.ContainsRune(`"'“”‘’([`, next) {
			continue
		}
		// Include closing quotes and brackets in the sentence.
		pieceEnd := i + 1
		for pieceEnd < len(runes) && strings.ContainsRune(`"'”’)]}`, runes[pieceEnd]) {
			pieceEnd++
		}
		piece := strings.TrimSpace(string(runes[start:pieceEnd]))
		if piece != "" {
			out = append(out, piece)
		}
		start = pieceEnd
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}
		i = start - 1
	}
	if rest := strings.TrimSpace(string(runes[start:])); rest != "" {
		out = append(out, rest)
	}
	return out
}

func isAbbreviation(prefix string) bool {
	for _, abbreviation := range abbreviations {
		if strings.HasSuffix(prefix, abbreviation) {
			return true
		}
	}
	return false
}

func isReferenceSentence(text string) bool {
	return refSentencePattern.MatchString(strings.TrimSpace(text)) && regexp.MustCompile(`\b(19|20)\d{2}\b`).MatchString(text)
}

func isMostlyQuote(text string) bool {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, `"`) && !strings.HasPrefix(text, "“") {
		return false
	}
	inside := 0
	open := false
	for _, r := range text {
		switch r {
		case '"', '“', '”':
			open = !open
		default:
			if open {
				inside++
			}
		}
	}
	return float64(inside)/float64(max(len([]rune(text)), 1)) > 0.65
}

// proseSentences filters a paragraph down to sentence-like text.
func proseSentences(text string) []string {
	var out []string
	for _, sentence := range splitSentences(text) {
		sentence = strings.TrimSpace(sentence)
		words := strings.Fields(sentence)
		if len(words) < 5 || !hasLetter(sentence) {
			continue
		}
		if looksLikeHeading(sentence) || isReferenceSentence(sentence) || isMostlyQuote(sentence) {
			continue
		}
		if strings.Count(sentence, " ") < 3 {
			continue
		}
		out = append(out, sentence)
	}
	return out
}

func wordTokens(text string) []string {
	var out []string
	for _, field := range strings.Fields(strings.ToLower(text)) {
		word := strings.Trim(field, `.,;:!?()[]{}<>"'“”‘’—–-/\\|@#$%^&*+=~`)
		word = strings.TrimSpace(word)
		if word != "" {
			out = append(out, word)
		}
	}
	return out
}

func contentTokens(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if len(token) < 3 || functionWords[token] {
			continue
		}
		out = append(out, token)
	}
	return out
}

func simpleStem(word string) string {
	word = strings.ToLower(word)
	for _, suffix := range []string{"ingly", "edly", "ation", "ments", "ment", "ness", "ities", "ity", "ings", "ing", "ies", "ied", "ed", "es", "s"} {
		if strings.HasSuffix(word, suffix) && len(word) > len(suffix)+3 {
			return strings.TrimSuffix(word, suffix)
		}
	}
	return word
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
