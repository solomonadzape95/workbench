package humanize

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// bannedWords are the vocabulary entries from the rules. Word matching is
// case-insensitive and respects word boundaries.
var bannedWords = []string{
	"delve", "tapestry", "landscape", "realm", "testament", "pivotal",
	"crucial", "robust", "seamless", "leverage", "utilize", "foster",
	"underscore", "multifaceted", "intricate", "embark", "unlock", "elevate",
	"game-changer", "facilitate", "commence",
	"leveraging", "leveraged", "utilizing", "utilized", "fostering",
	"fostered", "unlocking", "elevating", "navigate", "navigating",
	"navigated", "seamlessly", "robustly", "crucially", "pivotally",
	"intricately", "fosters", "facilitates", "facilitated", "facilitating",
	"leverages", "utilizes", "underscores", "unlocks", "elevates", "embarks",
	"delves", "commences", "navigates", "landscapes", "realms", "game-changers",
}

var bannedPhrases = []string{
	"in today's fast-paced world", "in today’s fast-paced world",
	"fast-paced world", "in the realm of", "it's important to note",
	"it is important to note", "it's worth noting", "it is worth noting",
	"let's dive in", "let's delve into", "in conclusion", "in summary",
	"at the end of the day", "may potentially", "can often help to",
	"could potentially", "not just", "not about", "isn't just", "is not just",
	"isn't about", "is not about", "wasn't just", "was not just",
	"wasn't about", "was not about",
}

var (
	notJustPattern = regexp.MustCompile(`(?i)\bnot\s+just\b[^.!?\n]{0,80}\bbut\b`)
	overallOpener  = regexp.MustCompile(`(?im)^\s*overall\s*,`)
)

var bannedWordPatterns = func() []struct {
	word    string
	pattern *regexp.Regexp
} {
	out := make([]struct {
		word    string
		pattern *regexp.Regexp
	}, 0, len(bannedWords))
	for _, word := range bannedWords {
		out = append(out, struct {
			word    string
			pattern *regexp.Regexp
		}{word, regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)})
	}
	return out
}()

// LintIssues checks a rewritten chunk against the hard rules. It returns short,
// model-readable notes, not user-facing prose. Empty means the chunk is clean.
func LintIssues(text string) []string {
	return LintIssuesStyle(text, true)
}

// LintIssuesStyle is LintIssues with the register taken into account. Academic
// prose intentionally avoids contractions, and its vocabulary is allowed to be
// a little denser.
func LintIssuesStyle(text string, allowContractions bool) []string {
	clean := stripPlaceholders(text)
	lower := strings.ToLower(clean)
	seen := map[string]bool{}
	var issues []string
	add := func(issue string) {
		if issue == "" || seen[issue] {
			return
		}
		seen[issue] = true
		issues = append(issues, issue)
	}

	for _, entry := range bannedWordPatterns {
		if entry.pattern.MatchString(clean) {
			add(fmt.Sprintf("remove the word %q", entry.word))
		}
	}
	for _, phrase := range bannedPhrases {
		if strings.Contains(lower, phrase) {
			add(fmt.Sprintf("remove the phrase %q", phrase))
		}
	}
	if notJustPattern.MatchString(clean) {
		add(`remove the "not just X, but Y" construction`)
	}
	if overallOpener.MatchString(clean) {
		add(`do not open a paragraph with "Overall,"`)
	}
	if strings.Contains(clean, ";") {
		add("replace semicolons with periods or commas unless the original voice uses them")
	}

	words := len(strings.Fields(clean))
	if words > 0 {
		dashes := strings.Count(clean, "—")
		allowed := words/300 + 1
		if dashes > allowed {
			add(fmt.Sprintf("use at most %d em dash(es) in %d words", allowed, words))
		}
	}
	for _, issue := range rhythmIssues(analyzeRhythm(clean), allowContractions) {
		add(issue)
	}
	return issues
}

func stripPlaceholders(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "[[WB") {
			if end := strings.Index(text[i:], "]]"); end >= 0 {
				i += end + 2
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

func wordCount(text string) int {
	return len(strings.Fields(stripPlaceholders(text)))
}

func lengthRatio(original, rewritten string) float64 {
	before := wordCount(original)
	after := wordCount(rewritten)
	if before == 0 {
		return 1
	}
	return float64(after) / float64(before)
}

func lengthIssue(original, rewritten string) string {
	if wordCount(original) < 30 {
		return ""
	}
	ratio := lengthRatio(original, rewritten)
	if ratio < 0.65 {
		return fmt.Sprintf("the rewrite is %.0f%% of the original length; keep it within 15%% of the original without adding new facts", ratio*100)
	}
	if ratio > 1.35 {
		return fmt.Sprintf("the rewrite is %.0f%% of the original length; keep it within 15%% of the original without adding new facts", ratio*100)
	}
	return ""
}

func cleanOutput(text string) string {
	text = strings.TrimSpace(text)
	text = stripWrappingFence(text)
	lines := strings.Split(text, "\n")
	if len(lines) > 1 {
		first := strings.TrimSpace(lines[0])
		lower := strings.ToLower(first)
		if len(first) < 160 && strings.HasSuffix(first, ":") {
			for _, opener := range []string{"here is", "here's", "sure", "certainly", "of course", "rewritten text", "humanized text", "here is the rewrite"} {
				if strings.HasPrefix(lower, opener) {
					text = strings.TrimSpace(strings.Join(lines[1:], "\n"))
					break
				}
			}
		}
	}
	return strings.TrimSpace(text)
}

func stripWrappingFence(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		return text
	}
	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(first, "```") || last != "```" {
		return text
	}
	for _, line := range lines[1 : len(lines)-1] {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			return text
		}
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}
