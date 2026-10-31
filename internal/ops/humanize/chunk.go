package humanize

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// protector replaces code, links, quotes, and structural markers with stable
// tokens before a chunk goes to a model. The model can rewrite the prose without
// being able to damage the parts the rules say must stay verbatim.
type protector struct {
	values []string
	kinds  []string
}

func (p *protector) add(kind, value string) string {
	p.values = append(p.values, value)
	p.kinds = append(p.kinds, kind)
	return fmt.Sprintf("[[WB%d]]", len(p.values)-1)
}

func (p *protector) kind(token string) string {
	var index int
	if _, err := fmt.Sscanf(token, "[[WB%d]]", &index); err != nil {
		return ""
	}
	if index < 0 || index >= len(p.kinds) {
		return ""
	}
	return p.kinds[index]
}

func (p *protector) isHeading(block string) bool {
	fields := strings.Fields(block)
	if len(fields) == 0 {
		return false
	}
	return p.kind(fields[0]) == "heading"
}

// PromptNote explains every placeholder to the model. Without this, models
// treat a leading placeholder as clutter and delete it while "cleaning up" the
// document.
func (p *protector) PromptNote() string {
	if p == nil || len(p.values) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("PROTECTED TOKENS (these are data, not formatting to clean up):\n")
	for i, kind := range p.kinds {
		description := "keep it exactly where it is"
		switch kind {
		case "heading":
			description = "heading marker; keep it at the start of its line"
		case "marker":
			description = "list marker; keep it at the start of its line"
		case "code":
			description = "code block; keep it on its own line, unchanged"
		case "inline-code":
			description = "inline code; keep it exactly as written"
		case "table":
			description = "table; keep it exactly as written"
		case "quote":
			description = "quotation; keep it exactly as written"
		case "frontmatter":
			description = "front matter; keep it exactly as written"
		case "math":
			description = "equation; keep it exactly as written"
		case "rule":
			description = "horizontal rule; keep it on its own line"
		case "footnote":
			description = "footnote definition; keep it exactly as written"
		case "link":
			description = "link target; keep it exactly as written"
		case "url":
			description = "URL; keep it exactly as written"
		case "html":
			description = "HTML tag; keep it exactly as written"
		}
		fmt.Fprintf(&b, "- [[WB%d]]: %s\n", i, description)
	}
	return strings.TrimSpace(b.String())
}

// restore puts the protected values back. It returns the tokens that the model
// dropped, so the caller can retry instead of silently losing a code block.
func (p *protector) restore(text string) (string, []string) {
	var missing []string
	pairs := make([]string, 0, len(p.values)*2)
	for i, value := range p.values {
		token := fmt.Sprintf("[[WB%d]]", i)
		if !strings.Contains(text, token) {
			missing = append(missing, token)
		}
		pairs = append(pairs, token, value)
	}
	return strings.NewReplacer(pairs...).Replace(text), missing
}

var (
	inlineCodePattern  = regexp.MustCompile("`[^`\\n]+`")
	inlineQuotePattern = regexp.MustCompile("\"[^\"\\n]{1,300}\"")
	markdownLinkURL    = regexp.MustCompile(`(\[[^\]\n]*\]\()([^)\n]+)(\))`)
	bareURLPattern     = regexp.MustCompile(`https?://[^\s<>()\[\]]+`)
	htmlTagPattern     = regexp.MustCompile(`</?[A-Za-z][^>\n]*>`)
	inlineMathPattern  = regexp.MustCompile(`\$[^$\n]+\$`)
	headingMarker      = regexp.MustCompile(`^(\s{0,3}#{1,6}\s+)`)
	taskMarker         = regexp.MustCompile(`^(\s*[-*+]\s+\[[ xX]\]\s+)`)
	listMarker         = regexp.MustCompile(`^(\s*[-*+]\s+)`)
	orderedMarker      = regexp.MustCompile(`^(\s*\d+[.)]\s+)`)
	blankLines         = regexp.MustCompile(`\n[ \t]*\n`)
	footnoteDef        = regexp.MustCompile(`^\[\^[^\]]+\]:`)
	tableSeparator     = regexp.MustCompile(`^[ \t]*\|?[ \t]*:?-{1,}:?[ \t]*(\|[ \t]*:?-{1,}:?[ \t]*)+\|?[ \t]*$`)
	frontMatterKey     = regexp.MustCompile(`^[A-Za-z0-9_-]+\s*[:=]`)
)

// ProtectDocument normalizes the input and returns the protected text plus the
// protector needed to restore it.
func ProtectDocument(text string) (string, *protector) {
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	p := &protector{}
	var out []string

	addBlock := func(kind, value string) {
		token := p.add(kind, value)
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, token, "")
	}

	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// YAML front matter, only at the top of the file.
		if i == 0 && (trimmed == "---" || trimmed == "+++") && i+1 < len(lines) && frontMatterKey.MatchString(lines[1]) {
			marker := trimmed
			j := i + 1
			for ; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == marker || strings.TrimSpace(lines[j]) == "..." {
					break
				}
			}
			if j < len(lines) {
				addBlock("frontmatter", strings.Join(lines[i:j+1], "\n"))
				i = j + 1
				continue
			}
		}

		if char, count, ok := fenceStart(line); ok {
			j := i + 1
			for ; j < len(lines); j++ {
				if fenceEnd(lines[j], char, count) {
					break
				}
			}
			if j >= len(lines) {
				j = len(lines) - 1
			}
			addBlock("code", strings.Join(lines[i:j+1], "\n"))
			i = j + 1
			continue
		}

		if strings.TrimSpace(line) == "$$" {
			j := i + 1
			for ; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == "$$" {
					break
				}
			}
			if j >= len(lines) {
				j = len(lines) - 1
			}
			addBlock("math", strings.Join(lines[i:j+1], "\n"))
			i = j + 1
			continue
		}

		if isIndentedCode(line) {
			j := i + 1
			for j < len(lines) && (isIndentedCode(lines[j]) || strings.TrimSpace(lines[j]) == "") {
				j++
			}
			for j > i && strings.TrimSpace(lines[j-1]) == "" {
				j--
			}
			addBlock("code", strings.Join(lines[i:j], "\n"))
			i = j
			continue
		}

		if strings.Contains(line, "|") && i+1 < len(lines) && tableSeparator.MatchString(lines[i+1]) {
			j := i + 2
			for j < len(lines) && strings.Contains(lines[j], "|") && strings.TrimSpace(lines[j]) != "" {
				j++
			}
			addBlock("table", strings.Join(lines[i:j], "\n"))
			i = j
			continue
		}

		if strings.HasPrefix(trimmed, ">") {
			j := i + 1
			for j < len(lines) {
				next := strings.TrimSpace(lines[j])
				if strings.HasPrefix(next, ">") {
					j++
					continue
				}
				if next == "" && j+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j+1]), ">") {
					j++
					continue
				}
				break
			}
			addBlock("quote", strings.Join(lines[i:j], "\n"))
			i = j
			continue
		}

		if isHorizontalRule(line) {
			addBlock("rule", line)
			i++
			continue
		}

		if footnoteDef.MatchString(line) {
			addBlock("footnote", line)
			i++
			continue
		}

		out = append(out, protectInlineLine(line, p))
		i++
	}

	joined := strings.Join(out, "\n")
	joined = blankLines.ReplaceAllString(joined, "\n\n")
	return strings.TrimSpace(joined), p
}

func protectInlineLine(line string, p *protector) string {
	line = inlineCodePattern.ReplaceAllStringFunc(line, func(value string) string {
		return p.add("inline-code", value)
	})
	line = markdownLinkURL.ReplaceAllStringFunc(line, func(value string) string {
		match := markdownLinkURL.FindStringSubmatch(value)
		if len(match) != 4 {
			return value
		}
		return match[1] + p.add("link", match[2]) + match[3]
	})
	line = inlineQuotePattern.ReplaceAllStringFunc(line, func(value string) string {
		return p.add("quote", value)
	})
	line = htmlTagPattern.ReplaceAllStringFunc(line, func(value string) string {
		return p.add("html", value)
	})
	line = bareURLPattern.ReplaceAllStringFunc(line, func(value string) string {
		return p.add("url", value)
	})
	line = inlineMathPattern.ReplaceAllStringFunc(line, func(value string) string {
		return p.add("math", value)
	})

	switch {
	case taskMarker.MatchString(line):
		loc := taskMarker.FindStringIndex(line)
		line = p.add("marker", line[loc[0]:loc[1]]) + line[loc[1]:]
	case headingMarker.MatchString(line):
		loc := headingMarker.FindStringIndex(line)
		line = p.add("heading", line[loc[0]:loc[1]]) + line[loc[1]:]
	case orderedMarker.MatchString(line):
		loc := orderedMarker.FindStringIndex(line)
		line = p.add("marker", line[loc[0]:loc[1]]) + line[loc[1]:]
	case listMarker.MatchString(line):
		loc := listMarker.FindStringIndex(line)
		line = p.add("marker", line[loc[0]:loc[1]]) + line[loc[1]:]
	}
	return line
}

func fenceStart(line string) (byte, int, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || len(trimmed) < 3 {
		return 0, 0, false
	}
	char := trimmed[0]
	if char != '`' && char != '~' {
		return 0, 0, false
	}
	count := 0
	for count < len(trimmed) && trimmed[count] == char {
		count++
	}
	if count < 3 {
		return 0, 0, false
	}
	return char, count, true
}

func fenceEnd(line string, char byte, count int) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < count {
		return false
	}
	for i := 0; i < count; i++ {
		if trimmed[i] != char {
			return false
		}
	}
	return strings.TrimSpace(trimmed[count:]) == ""
}

func isIndentedCode(line string) bool {
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
}

func isHorizontalRule(line string) bool {
	trimmed := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	trimmed = strings.ReplaceAll(trimmed, "\t", "")
	if len(trimmed) < 3 {
		return false
	}
	char := trimmed[0]
	if char != '-' && char != '*' && char != '_' {
		return false
	}
	for i := 1; i < len(trimmed); i++ {
		if trimmed[i] != char {
			return false
		}
	}
	return true
}

type chunkUnit struct {
	text  string
	sep   string
	words int
}

// ChunkText groups protected blocks into chunks of at most maxWords words. It
// only splits a block when that block alone is more than twice the limit, and
// then it splits on sentence boundaries. Normal blocks keep their paragraph
// breaks, and headings stay with the text that follows them.
func ChunkText(protected string, maxWords int, p *protector) []string {
	if maxWords < 100 {
		maxWords = 100
	}
	blocks := blankLines.Split(protected, -1)
	units := make([]chunkUnit, 0, len(blocks))
	for i := 0; i < len(blocks); i++ {
		block := strings.TrimSpace(blocks[i])
		if block == "" {
			continue
		}
		if p != nil && p.isHeading(block) && i+1 < len(blocks) {
			next := strings.TrimSpace(blocks[i+1])
			if next != "" {
				block = block + "\n\n" + next
				i++
			}
		}
		units = append(units, splitUnit(block, maxWords)...)
	}

	var chunks []string
	var current []chunkUnit
	words := 0
	for _, unit := range units {
		if len(current) > 0 && words+unit.words > maxWords {
			chunks = append(chunks, joinUnits(current))
			current = nil
			words = 0
		}
		current = append(current, unit)
		words += unit.words
	}
	if len(current) > 0 {
		chunks = append(chunks, joinUnits(current))
	}
	return chunks
}

func splitUnit(block string, maxWords int) []chunkUnit {
	words := len(strings.Fields(block))
	if words <= maxWords*2 {
		return []chunkUnit{{text: block, sep: "\n\n", words: words}}
	}
	sentences := splitSentences(block)
	if len(sentences) < 2 {
		return []chunkUnit{{text: block, sep: "\n\n", words: words}}
	}
	var out []chunkUnit
	var current []string
	currentWords := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		text := strings.Join(current, " ")
		sep := " "
		if len(out) == 0 {
			sep = "\n\n"
		}
		out = append(out, chunkUnit{text: text, sep: sep, words: len(strings.Fields(text))})
		current = nil
		currentWords = 0
	}
	for _, sentence := range sentences {
		n := len(strings.Fields(sentence))
		if len(current) > 0 && currentWords+n > maxWords {
			flush()
		}
		current = append(current, sentence)
		currentWords += n
	}
	flush()
	return out
}

func joinUnits(units []chunkUnit) string {
	var b strings.Builder
	for i, unit := range units {
		if i > 0 {
			b.WriteString(unit.sep)
		}
		b.WriteString(unit.text)
	}
	return strings.TrimSpace(b.String())
}

func splitSentences(text string) []string {
	var out []string
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '.', '!', '?':
			end := i + 1
			for end < len(text) {
				switch text[end] {
				case '"', '\'', ')', ']', '}':
					end++
					continue
				}
				if strings.HasPrefix(text[end:], "”") || strings.HasPrefix(text[end:], "’") {
					end += 3
					continue
				}
				break
			}
			next := end
			for next < len(text) {
				r, size := utf8.DecodeRuneInString(text[next:])
				if !unicode.IsSpace(r) {
					break
				}
				next += size
			}
			if next > end && next < len(text) {
				piece := strings.TrimSpace(text[start:end])
				if piece != "" {
					out = append(out, piece)
				}
				start = next
				i = next - 1
			}
		}
	}
	if rest := strings.TrimSpace(text[start:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

// PreviousContext returns the last few words of a chunk for continuity. It is
// context only; the model is told not to repeat or rewrite it.
func PreviousContext(chunk string) string {
	words := strings.Fields(chunk)
	if len(words) <= 80 {
		return ""
	}
	return strings.Join(words[len(words)-80:], " ")
}

func placeholderTokens(text string) []string {
	return placeholderPattern.FindAllString(text, -1)
}

var placeholderPattern = regexp.MustCompile(`\[\[WB\d+\]\]`)

func missingPlaceholders(original, rewritten string) []string {
	var missing []string
	for _, token := range placeholderTokens(original) {
		if !strings.Contains(rewritten, token) {
			missing = append(missing, token)
		}
	}
	return missing
}
