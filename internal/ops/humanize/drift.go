package humanize

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// A free model can answer a bad prompt with a meta-comment, a Markdown
// heading, a completely different paragraph, or a new statistic. Those are
// hard failures: the caller keeps the original text instead of shipping the
// model's mistake. Softer drift (a missing name, for example) goes into the
// revision prompt first.
var (
	metaReplyPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bthe text (?:you|that you) (?:provided|gave|sent)\b`),
		regexp.MustCompile(`(?i)\bplease (?:provide|send|share) (?:the|a|more) (?:body|text|passage|paragraph|full)\b`),
		regexp.MustCompile(`(?i)\bnot a (?:full|complete) (?:passage|paragraph|text|document)\b`),
		regexp.MustCompile(`(?i)\b(?:i|we) (?:can't|cannot|can not|won't|will not) (?:rewrite|humanize|help|do that)\b`),
		regexp.MustCompile(`(?i)\bas an ai\b`),
		regexp.MustCompile(`(?i)\b(?:i|we) (?:need|require) (?:the )?(?:body|full|original|source) text\b`),
		regexp.MustCompile(`(?i)\bTEXT TO REWRITE\b`),
		regexp.MustCompile(`(?i)\bPREVIOUS PARAGRAPH\b`),
		regexp.MustCompile(`(?i)\bOUTPUT CONTRACT\b`),
		regexp.MustCompile(`(?i)\baccording to the style rules\b`),
		regexp.MustCompile(`(?i)^\s*(?:here is|here's) the (?:rewritten|humanized|revised)\b`),
		regexp.MustCompile(`(?i)^\s*(?:sure|certainly|of course)\s*[,:]`),
	}

	numberPattern      = regexp.MustCompile(`\b\d[\d,./%-]*\b`)
	acronymPattern     = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,}(?:s)?\b`)
	properNounPattern  = regexp.MustCompile(`\b[A-Z][a-z]+(?:[A-Z][a-z]+)*\b`)
	sentenceSplitter   = regexp.MustCompile(`[.!?]\s+`)
	abbreviationDot    = regexp.MustCompile(`(?i)\b(Dr|Prof|Mr|Mrs|Ms|Jr|Sr|St|vs|etc|e\.g|i\.e|Fig|No|Vol|pp|cf|al)\.\s+`)
	properNounStoplist = func() map[string]bool {
		out := map[string]bool{}
		for _, word := range strings.Fields(`
			The This That These Those It We They He She His Her Their Our Your My
			A An In On At By For From With Without And But Or So If When While
			Although Because Since As After Before During However Therefore Thus
			Moreover Furthermore Additionally Finally First Second Third Fourth
			Next Then There Here What Which Who Whom Whose Why How All Some Many
			Most More Less Few Both Each Every Either Neither No Not Only Also
			Just Very Such Other Another Same New Old Good Bad Best Better High
			Low Large Small One Two Three Four Five Six Seven Eight Nine Ten
			Chapter Section Figure Table University Department Faculty College
			School Institute Centre Center Committee Board Office Report Study
			Project System Platform Research Data Results Method Methods
			Dr Prof Mr Mrs Ms Jr Sr
			Introduction Background Conclusion Abstract Declaration
			References Appendix Summary Acknowledgement
		`) {
			out[strings.ToLower(word)] = true
		}
		return out
	}()

	contentStopwords = func() map[string]bool {
		out := map[string]bool{}
		for _, word := range strings.Fields(`
			the a an and or but if then than that this these those it its is are
			was were be been being have has had do does did not no nor so as at
			by for from in into of on onto or over under to up with within without
			we you he she they them their our your his her i me my mine ours yours
			can could may might must shall should will would about above after
			again against all also any because before below between both during
			each few further here how more most other some such there through
			under until very what when where which while who whom why
		`) {
			out[word] = true
		}
		return out
	}()
)

func metaReplyIssue(text string) string {
	for _, pattern := range metaReplyPatterns {
		if pattern.MatchString(text) {
			return "return the rewritten passage only, not a note or question about the text"
		}
	}
	return ""
}

func addedMarkdownStructure(original, rewritten string) string {
	o := strings.TrimSpace(original)
	r := strings.TrimSpace(rewritten)
	if strings.HasPrefix(r, "#") && !strings.HasPrefix(o, "#") {
		return "do not add a Markdown heading"
	}
	if (strings.HasPrefix(r, "- ") || strings.HasPrefix(r, "* ") || strings.HasPrefix(r, "+ ")) &&
		!(strings.HasPrefix(o, "- ") || strings.HasPrefix(o, "* ") || strings.HasPrefix(o, "+ ")) {
		return "do not add a bullet list"
	}
	if strings.Contains(r, "**") && !strings.Contains(o, "**") {
		return "do not add Markdown bold markers"
	}
	return ""
}

func trimLeadingMarker(text string) string {
	text = strings.TrimSpace(text)
	if marker, space := docxListMarker(text); marker != "" {
		return strings.TrimSpace(strings.TrimPrefix(text, marker+space))
	}
	return text
}

func numbersIn(text string) map[string]bool {
	out := map[string]bool{}
	for _, match := range numberPattern.FindAllString(trimLeadingMarker(text), -1) {
		out[match] = true
	}
	return out
}

func missingNumber(original, rewritten string) string {
	before := numbersIn(original)
	if len(before) == 0 {
		return ""
	}
	after := numbersIn(rewritten)
	for number := range before {
		if !after[number] {
			return number
		}
	}
	return ""
}

func addedNumber(original, rewritten string) string {
	before := numbersIn(original)
	after := numbersIn(rewritten)
	for number := range after {
		if before[number] || !significantNumber(number) {
			continue
		}
		return number
	}
	return ""
}

// significantNumber ignores a one-digit number that may simply be the model
// spelling out "one" or "two". Years, decimals, percentages and multi-digit
// numbers are facts and must not be invented.
func significantNumber(number string) bool {
	if len(number) >= 2 {
		return true
	}
	return strings.ContainsAny(number, ".%/")
}

func acronymsIn(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, acronym := range acronymPattern.FindAllString(text, -1) {
		if len(acronym) < 2 || seen[acronym] {
			continue
		}
		seen[acronym] = true
		out = append(out, acronym)
	}
	return out
}

func properNouns(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, sentence := range splitSentencesForAnchors(text) {
		words := strings.Fields(sentence)
		for i, word := range words {
			clean := strings.Trim(word, `.,;:!?()[]{}"'“”‘’`)
			if clean == "" {
				continue
			}
			if i == 0 {
				continue // sentence-initial capital is not an entity
			}
			match := properNounPattern.FindString(clean)
			if match == "" {
				continue
			}
			at := strings.Index(clean, match)
			if at > 0 {
				prev := rune(clean[at-1])
				if !unicode.IsSpace(prev) && !strings.ContainsRune(`([{"'“”‘’`, prev) {
					continue // e.g. a missing space in "votes.Wood"
				}
			}
			if properNounStoplist[strings.ToLower(match)] {
				continue
			}
			if seen[match] {
				continue
			}
			seen[match] = true
			out = append(out, match)
		}
	}
	return out
}

func splitSentencesForAnchors(text string) []string {
	text = abbreviationDot.ReplaceAllString(text, "$1<dot> ")
	parts := sentenceSplitter.Split(strings.TrimSpace(text), -1)
	for i := range parts {
		parts[i] = strings.ReplaceAll(parts[i], "<dot>", ".")
	}
	return parts
}

func containsWord(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func missingAnchors(original, rewritten string) []string {
	var out []string
	for _, noun := range properNouns(original) {
		if !containsWord(rewritten, noun) {
			out = append(out, noun)
		}
	}
	return out
}

func newAnchors(original, rewritten string) []string {
	var out []string
	for _, noun := range properNouns(rewritten) {
		if !containsWord(original, noun) {
			out = append(out, noun)
		}
	}
	return out
}

func contentTokens(text string) map[string]bool {
	out := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(word) < 3 || contentStopwords[word] {
			continue
		}
		out[word] = true
	}
	return out
}

func contentOverlap(original, rewritten string) float64 {
	before := contentTokens(original)
	if len(before) == 0 {
		return 1
	}
	after := contentTokens(rewritten)
	hits := 0
	for word := range before {
		if after[word] {
			hits++
		}
	}
	return float64(hits) / float64(len(before))
}

// driftIssues are soft problems. The revision loop gets a chance to fix them.
func driftIssues(original, rewritten string) []string {
	var issues []string
	if issue := metaReplyIssue(rewritten); issue != "" {
		issues = append(issues, issue)
	}
	if issue := addedMarkdownStructure(original, rewritten); issue != "" {
		issues = append(issues, issue)
	}
	if number := missingNumber(original, rewritten); number != "" {
		issues = append(issues, fmt.Sprintf("keep the number %q exactly as it appears", number))
	}
	if number := addedNumber(original, rewritten); number != "" {
		issues = append(issues, fmt.Sprintf("remove the number %q; do not add facts", number))
	}
	for _, acronym := range acronymsIn(original) {
		if !strings.Contains(rewritten, acronym) {
			issues = append(issues, fmt.Sprintf("keep the acronym %q exactly as it appears", acronym))
		}
	}
	missing := missingAnchors(original, rewritten)
	for i, noun := range missing {
		if i >= 5 {
			break
		}
		issues = append(issues, fmt.Sprintf("keep the name or term %q exactly as it appears", noun))
	}
	for i, noun := range newAnchors(original, rewritten) {
		if i >= 5 {
			break
		}
		issues = append(issues, fmt.Sprintf("do not add the name or place %q; it is not in the input", noun))
	}
	if len(contentTokens(original)) >= 12 && contentOverlap(original, rewritten) < 0.20 {
		issues = append(issues, "keep the original subject and claims; the draft shares too little wording with the input")
	}
	return issues
}

// FatalRewriteIssue rejects the classes of failure that must never reach a
// document: meta replies, invented structure, changed numbers, replaced names,
// and paragraphs about a different subject.
func FatalRewriteIssue(original, rewritten string) string {
	if issue := metaReplyIssue(rewritten); issue != "" {
		return issue
	}
	if issue := addedMarkdownStructure(original, rewritten); issue != "" {
		return issue
	}
	if number := missingNumber(original, rewritten); number != "" {
		return fmt.Sprintf("dropped or changed the number %q", number)
	}
	if number := addedNumber(original, rewritten); number != "" {
		return fmt.Sprintf("added the number %q", number)
	}

	before := properNouns(original)
	if len(before) >= 2 {
		found := 0
		for _, noun := range before {
			if containsWord(rewritten, noun) {
				found++
			}
		}
		if float64(found)/float64(len(before)) < 0.5 {
			return "replaced the names or terms in the input"
		}
	}
	if len(before) >= 2 {
		added := 0
		for _, noun := range newAnchors(original, rewritten) {
			if len(noun) > 0 {
				added++
			}
		}
		if added >= 2 {
			return "added names or places that were not in the input"
		}
	}
	if len(contentTokens(original)) >= 12 && contentOverlap(original, rewritten) < 0.10 {
		return "wrote content unrelated to the input"
	}
	return ""
}
