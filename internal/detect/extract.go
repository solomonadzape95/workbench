package detect

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/beevik/etree"
)

const maxInputBytes = 128 << 20

// Document is the extracted prose the scorer sees.
type Document struct {
	Source     Source
	Paragraphs []ParagraphInput
	Warnings   []string
}

// ParagraphInput is one prose paragraph. Style is kept for diagnostics.
type ParagraphInput struct {
	Index int
	Text  string
	Style string
}

var (
	docxCaptionPattern = regexp.MustCompile(`(?i)^(figure|table)\s+\d+(?:\.\d+)*\s*(?::|\.\s+|[-–—]\s*)`)
	docxSectionPattern = regexp.MustCompile(`(?i)^(chapter|section|part|appendix|appendices|table of contents|list of (tables|figures)|abstract|declaration|certification|dedication|acknowledg(e)?ments?|references|bibliography|glossary|abbreviations|definitions?)\b`)
	docxNumbered       = regexp.MustCompile(`^\d+(?:\.\d+)*[.)]?\s+\S`)
	docxMarkdownHead   = regexp.MustCompile(`^#{1,6}\s`)
)

// Extract reads TXT, Markdown, DOCX, or PDF and returns prose paragraphs.
func Extract(ctx context.Context, path string) (Document, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Document{}, err
	}
	if info.IsDir() {
		return Document{}, fmt.Errorf("%s is a directory", path)
	}
	if info.Size() > maxInputBytes {
		return Document{}, fmt.Errorf("file is larger than %d MB", maxInputBytes>>20)
	}
	format := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	doc := Document{
		Source: Source{
			Path:   path,
			Name:   filepath.Base(path),
			Format: format,
			Bytes:  info.Size(),
		},
	}

	switch format {
	case "txt":
		data, err := os.ReadFile(path)
		if err != nil {
			return Document{}, err
		}
		if !utf8.Valid(data) {
			return Document{}, fmt.Errorf("%s is not valid UTF-8 text", path)
		}
		doc.Paragraphs = paragraphsFromPlain(string(data), true)
	case "md", "markdown":
		data, err := os.ReadFile(path)
		if err != nil {
			return Document{}, err
		}
		if !utf8.Valid(data) {
			return Document{}, fmt.Errorf("%s is not valid UTF-8 text", path)
		}
		doc.Paragraphs = paragraphsFromPlain(cleanMarkdown(string(data)), false)
	case "docx":
		paras, warnings, err := paragraphsFromDOCX(path)
		if err != nil {
			return Document{}, err
		}
		doc.Paragraphs = paras
		doc.Warnings = append(doc.Warnings, warnings...)
	case "pdf":
		paras, warnings, err := paragraphsFromPDF(ctx, path)
		if err != nil {
			return Document{}, err
		}
		doc.Paragraphs = paras
		doc.Warnings = append(doc.Warnings, warnings...)
	default:
		return Document{}, fmt.Errorf("unsupported format %q; use TXT, Markdown, DOCX, or PDF", format)
	}

	// Number the prose paragraphs in reading order and drop empty blocks.
	clean := doc.Paragraphs[:0]
	for _, p := range doc.Paragraphs {
		text := strings.TrimSpace(collapseSpace(p.Text))
		if text == "" || !hasLetter(text) {
			continue
		}
		p.Text = text
		p.Index = len(clean)
		clean = append(clean, p)
	}
	doc.Paragraphs = trimReferences(clean)
	if len(doc.Paragraphs) == 0 {
		return Document{}, fmt.Errorf("no prose text found in %s", path)
	}
	return doc, nil
}

func paragraphsFromDOCX(path string) ([]ParagraphInput, []string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open DOCX: %w", err)
	}
	defer reader.Close()

	var documentXML []byte
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, nil, err
		}
		documentXML, err = io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, nil, err
		}
		break
	}
	if len(documentXML) == 0 {
		return nil, nil, fmt.Errorf("DOCX has no word/document.xml")
	}
	tree := etree.NewDocument()
	if err := tree.ReadFromBytes(documentXML); err != nil {
		return nil, nil, fmt.Errorf("parse DOCX XML: %w", err)
	}
	body := docxDirectChild(tree.Root(), "body")
	if body == nil {
		return nil, nil, fmt.Errorf("DOCX has no document body")
	}

	var (
		out          []ParagraphInput
		warnings     []string
		inReferences bool
		seenBody     bool
	)
	bodyParagraphs := make([]*etree.Element, 0)
	for _, element := range body.ChildElements() {
		if element.Tag == "p" {
			bodyParagraphs = append(bodyParagraphs, element)
		}
	}
	start := 0
	for i, element := range bodyParagraphs {
		if isBodyStart(collapseSpace(docxText(element))) {
			start = i + 1
			break
		}
	}
	for _, element := range bodyParagraphs[start:] {
		text := strings.TrimSpace(collapseSpace(docxText(element)))
		if text == "" {
			continue
		}
		style := docxStyle(element)
		headingStyle := strings.HasPrefix(strings.ToLower(style), "heading") || strings.EqualFold(style, "Title")
		if headingStyle {
			if strings.Contains(strings.ToUpper(text), "REFERENCES") || strings.Contains(strings.ToUpper(text), "BIBLIOGRAPHY") {
				inReferences = true
			}
			if looksLikeHeading(text) || !endsSentence(text) || len(strings.Fields(text)) < 10 {
				continue
			}
			// A body paragraph mislabelled as a heading still counts as prose.
		}
		if inReferences {
			continue
		}
		if isReferenceHeading(text) {
			inReferences = true
			continue
		}
		if docxCaptionPattern.MatchString(text) {
			continue
		}
		if !hasLetter(text) || len(text) < 4 {
			continue
		}
		if looksLikeHeading(text) {
			continue
		}
		words := len(strings.Fields(text))
		if text == strings.ToUpper(text) && len(text) < 120 && words <= 16 && !strings.ContainsAny(text, "!?") {
			continue
		}
		if docxHasDescendant(element, "drawing", "pict", "object", "oMath", "fldSimple", "instrText") {
			continue
		}
		// Short name lines and signature blocks are front matter, not prose.
		if !seenBody && words <= 8 {
			continue
		}
		seenBody = true
		out = append(out, ParagraphInput{Text: text, Style: style})
	}
	if len(out) == 0 {
		warnings = append(warnings, "the DOCX contains no body prose after removing headings, tables, figures, and references")
	}
	return out, warnings, nil
}

func paragraphsFromPDF(ctx context.Context, path string) ([]ParagraphInput, []string, error) {
	binary, err := exec.LookPath("pdftotext")
	if err != nil {
		return nil, nil, fmt.Errorf("PDF extraction needs pdftotext; install poppler (brew install poppler) or convert the PDF to DOCX first")
	}
	cmd := exec.CommandContext(ctx, binary, "-layout", "-enc", "UTF-8", path, "-")
	data, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, fmt.Errorf("pdftotext: %w", err)
	}
	text := cleanPDFText(string(data))
	var warnings []string
	warnings = append(warnings, "PDF text extraction is best-effort; convert to DOCX for a cleaner, table-free score")
	if len(strings.Fields(text)) < 30 {
		warnings = append(warnings, "the PDF produced very little text; it may be a scan and need OCR")
	}
	paras := paragraphsFromPlain(text, false)
	paras = trimFrontMatter(paras)
	paras = filterFrontSections(paras)
	return paras, warnings, nil
}

var pdfPageNumber = regexp.MustCompile(`(?i)^(page\s+)?\d+(\s*(of|/)\s*\d+)?$`)

func cleanPDFText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	pages := strings.Split(text, "\f")
	lineCount := map[string]int{}
	for _, page := range pages {
		for _, line := range strings.Split(page, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				lineCount[line]++
			}
		}
	}
	threshold := max(3, len(pages)/10)
	var out []string
	for _, page := range pages {
		for _, raw := range strings.Split(page, "\n") {
			line := strings.TrimRight(raw, " \t")
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				out = append(out, "")
				continue
			}
			if pdfPageNumber.MatchString(trimmed) {
				continue
			}
			if lineCount[trimmed] >= threshold && len(trimmed) < 120 {
				continue // repeated header/footer
			}
			if strings.Contains(trimmed, "  ") || strings.Contains(trimmed, "\t\t") {
				continue // internal layout columns, lists, or table rows
			}
			out = append(out, line)
		}
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

func isBodyStart(text string) bool {
	normalized := normalizeSection(text)
	for _, prefix := range []string{"abstract", "chapter one", "chapter 1", "introduction"} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func normalizeSection(text string) string {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if line, _, ok := strings.Cut(normalized, "\n"); ok {
		normalized = line
	}
	return strings.Trim(normalized, " .:-")
}

func trimReferences(paras []ParagraphInput) []ParagraphInput {
	for i, para := range paras {
		if isReferenceHeading(para.Text) {
			return paras[:i]
		}
	}
	return paras
}

func trimFrontMatter(paras []ParagraphInput) []ParagraphInput {
	for i, para := range paras {
		if isBodyStart(para.Text) {
			return paras[i+1:]
		}
	}
	return paras
}

func filterFrontSections(paras []ParagraphInput) []ParagraphInput {
	var out []ParagraphInput
	skip := false
	for _, para := range paras {
		text := strings.ToLower(strings.TrimSpace(para.Text))
		text = strings.Trim(text, ".:")
		if isSkippableFrontHeading(text) {
			skip = true
			continue
		}
		if isBodyStart(para.Text) {
			skip = false
			continue
		}
		if skip {
			continue
		}
		out = append(out, para)
	}
	return out
}

func isSkippableFrontHeading(text string) bool {
	normalized := normalizeSection(text)
	for _, prefix := range []string{
		"table of contents", "list of figures", "list of tables", "list of abbreviations",
		"list of symbols", "acknowledgements", "acknowledgment", "dedication",
		"declaration", "certification", "approval",
	} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func paragraphsFromPlain(text string, txt bool) []ParagraphInput {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\u00a0", " ")

	blocks := splitBlankBlocks(text)
	if len(blocks) <= 1 && txt {
		// A plain text file may use one paragraph per line.
		lines := strings.Split(text, "\n")
		if len(lines) > 3 {
			blocks = lines
		}
	}
	var out []ParagraphInput
	for _, block := range blocks {
		block = strings.TrimSpace(collapseSpace(block))
		if block == "" || !hasLetter(block) {
			continue
		}
		if docxCaptionPattern.MatchString(block) {
			continue
		}
		out = append(out, ParagraphInput{Text: block})
	}
	return out
}

func splitBlankBlocks(text string) []string {
	re := regexp.MustCompile(`\n[ \t]*\n+`)
	return re.Split(strings.TrimSpace(text), -1)
}

func cleanMarkdown(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimPrefix(text, "\ufeff")

	// YAML front matter.
	if strings.HasPrefix(strings.TrimSpace(text), "---") {
		trimmed := strings.TrimSpace(text)
		if end := strings.Index(trimmed[3:], "\n---"); end >= 0 {
			text = trimmed[3+end+4:]
		}
	}

	var (
		out       []string
		fence     string
		inRefs    bool
		blankLine bool
	)
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			continue
		}
		if isReferenceHeading(trimmed) {
			inRefs = true
			continue
		}
		if inRefs {
			continue
		}
		if trimmed == "" {
			if !blankLine {
				out = append(out, "")
				blankLine = true
			}
			continue
		}
		blankLine = false
		if strings.HasPrefix(trimmed, "#") {
			continue // headings are not prose
		}
		if strings.HasPrefix(trimmed, ">") {
			continue // blockquotes are quoted material, not the author's prose
		}
		if strings.HasPrefix(trimmed, "|") || strings.Count(trimmed, "|") >= 2 {
			continue // table row
		}
		if regexp.MustCompile(`^[-*_]{3,}$`).MatchString(trimmed) {
			continue // horizontal rule
		}
		line = stripMarkdownInline(line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func stripMarkdownInline(line string) string {
	line = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`).ReplaceAllString(line, " ")
	line = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`).ReplaceAllString(line, "$1")
	line = regexp.MustCompile("`[^`]*`").ReplaceAllString(line, " ")
	line = regexp.MustCompile(`\$\$[^$]*\$\$`).ReplaceAllString(line, " ")
	line = regexp.MustCompile(`\$[^$\n]+\$`).ReplaceAllString(line, " ")
	line = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(line, " ")
	line = regexp.MustCompile(`https?://\S+`).ReplaceAllString(line, " ")
	line = regexp.MustCompile(`\[\^[^\]]+\]`).ReplaceAllString(line, " ")
	line = regexp.MustCompile(`^\s*[-*+]\s+`).ReplaceAllString(line, "")
	line = regexp.MustCompile(`^\s*\d+[.)]\s+`).ReplaceAllString(line, "")
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "__", "")
	line = strings.ReplaceAll(line, "*", "")
	line = strings.ReplaceAll(line, "_", "")
	return line
}

func docxText(element *etree.Element) string {
	var b strings.Builder
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		for _, child := range el.ChildElements() {
			switch child.Tag {
			case "t":
				if child.Space == "w" {
					b.WriteString(child.Text())
				}
			case "tab":
				b.WriteByte(' ')
			case "br", "cr":
				b.WriteByte('\n')
			case "drawing", "pict", "object", "oMath", "fldSimple", "instrText", "delText":
				// figures, equations, fields, and deleted revisions are not prose
			default:
				walk(child)
			}
		}
	}
	walk(element)
	return b.String()
}

func docxStyle(p *etree.Element) string {
	pPr := docxDirectChild(p, "pPr")
	if pPr == nil {
		return ""
	}
	style := docxDirectChild(pPr, "pStyle")
	if style == nil {
		return ""
	}
	for _, attr := range style.Attr {
		if attr.Key == "val" {
			return attr.Value
		}
	}
	return ""
}

func docxDirectChild(parent *etree.Element, tag string) *etree.Element {
	if parent == nil {
		return nil
	}
	for _, child := range parent.ChildElements() {
		if child.Tag == tag {
			return child
		}
	}
	return nil
}

func docxHasDescendant(element *etree.Element, tags ...string) bool {
	for _, child := range element.ChildElements() {
		for _, tag := range tags {
			if child.Tag == tag {
				return true
			}
		}
		if docxHasDescendant(child, tags...) {
			return true
		}
	}
	return false
}

func looksLikeHeading(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if docxMarkdownHead.MatchString(text) {
		return true
	}
	words := len(strings.Fields(text))
	if docxSectionPattern.MatchString(text) && words <= 14 && !endsSentence(text) {
		return true
	}
	if docxNumbered.MatchString(text) && words <= 14 && !endsSentence(text) {
		return true
	}
	return words <= 10 && !endsSentence(text)
}

func endsSentence(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	last := text[len(text)-1]
	return last == '.' || last == '!' || last == '?'
}

func isReferenceHeading(text string) bool {
	text = strings.ToLower(strings.TrimSpace(strings.TrimLeft(text, "# ")))
	return text == "references" || text == "bibliography" || text == "works cited"
}

func hasLetter(text string) bool {
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func collapseSpace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
