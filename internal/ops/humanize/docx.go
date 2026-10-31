package humanize

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beevik/etree"
	"golang.org/x/sync/errgroup"

	"github.com/solomonadzape95/workbench/internal/core"
)

const (
	docxDocumentPath = "word/document.xml"
	docxMaxBytes     = 128 << 20
)

var captionPattern = regexp.MustCompile(`(?i)^(figure|table)\s+\d+(?:\.\d+)*\s*(?::|\.\s+|[-–—]\s*)`)

type docxEntry struct {
	name     string
	method   uint16
	modified time.Time
	data     []byte
}

type docxRun struct {
	element   *etree.Element
	text      string
	bold      bool
	italic    bool
	underline bool
	strike    bool
	hasBreak  bool
}

type docxParagraph struct {
	element     *etree.Element
	text        string
	runs        []docxRun
	labelStart  int // first bold label run; -1 when there is no label
	labelEnd    int // first body run after a label
	colonInBody bool
	bodyStart   int // first body run after an inline heading + line break; -1 otherwise
	marker      string
	markerSpace string
}

type docxDoc struct {
	entries    []docxEntry
	xml        *etree.Document
	paragraphs []*docxParagraph
	skipped    int
	formatted  int
}

// readDOCX loads the whole package into memory. DOCX files are usually small
// enough for this, and keeping every part means media, styles, headers and
// relationships are copied back byte-for-byte.
func readDOCX(path string) (*docxDoc, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > docxMaxBytes {
		return nil, fmt.Errorf("DOCX is larger than %d MB", docxMaxBytes>>20)
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open DOCX: %w", err)
	}
	defer reader.Close()

	doc := &docxDoc{}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			doc.entries = append(doc.entries, docxEntry{name: file.Name, method: file.Method, modified: file.Modified})
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file.Name, err)
		}
		data, readErr := io.ReadAll(rc)
		rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", file.Name, readErr)
		}
		doc.entries = append(doc.entries, docxEntry{
			name: file.Name, method: file.Method, modified: file.Modified, data: data,
		})
	}

	var documentXML []byte
	for _, entry := range doc.entries {
		if entry.name == docxDocumentPath {
			documentXML = entry.data
			break
		}
	}
	if documentXML == nil {
		return nil, fmt.Errorf("DOCX has no %s", docxDocumentPath)
	}
	doc.xml = etree.NewDocument()
	if err := doc.xml.ReadFromBytes(documentXML); err != nil {
		return nil, fmt.Errorf("parse %s: %w", docxDocumentPath, err)
	}
	body := directChild(doc.xml.Root(), "body")
	if body == nil {
		return nil, fmt.Errorf("DOCX has no document body")
	}
	doc.collect(body)
	return doc, nil
}

func (d *docxDoc) collect(body *etree.Element) {
	inReferences := false
	for _, element := range body.ChildElements() {
		if element.Tag != "p" {
			continue
		}
		text := strings.TrimSpace(paragraphText(element))
		if text == "" {
			continue
		}
		style := paragraphStyle(element)
		headingStyle := strings.HasPrefix(strings.ToLower(style), "heading") || strings.EqualFold(style, "Title")
		if headingStyle {
			if strings.Contains(strings.ToUpper(text), "REFERENCES") {
				inReferences = true
				d.skipped++
				continue
			}
			// Converted reports sometimes put real body prose in a Heading
			// style. Keep it unless it actually looks like a heading.
			if looksLikeHeading(text) || !endsSentence(text) || len(strings.Fields(text)) < 10 {
				d.skipped++
				continue
			}
		}
		if inReferences || strings.EqualFold(text, "REFERENCES") {
			inReferences = true
			d.skipped++
			continue
		}
		if captionPattern.MatchString(text) {
			d.skipped++
			continue
		}
		if !hasLetter(text) || len(text) < 4 {
			d.skipped++
			continue
		}
		// Converted reports often leave every heading in the body style. Treat
		// short, unpunctuated lines as headings anyway. This is what stops a
		// free model from turning "1.5 Significance of the Study" into a new
		// paragraph on a completely different subject.
		if looksLikeHeading(text) {
			d.skipped++
			continue
		}
		words := len(strings.Fields(text))
		if text == strings.ToUpper(text) && len(text) < 100 && words <= 15 && !strings.ContainsAny(text, "!?") {
			d.skipped++
			continue
		}
		if hasDescendant(element, "drawing", "pict", "object", "oMath", "fldSimple", "instrText", "hyperlink") {
			d.skipped++
			continue
		}

		runs := textRuns(element)
		if len(runs) == 0 {
			d.skipped++
			continue
		}
		if hasDescendant(element, "tab") {
			d.skipped++
			continue
		}
		if hasDescendant(element, "br") {
			bodyStart := breakBodyStart(runs)
			if bodyStart < 0 {
				d.skipped++
				continue
			}
			bodyText := strings.TrimSpace(joinRunText(runs[bodyStart:]))
			if bodyText == "" {
				d.skipped++
				continue
			}
			d.paragraphs = append(d.paragraphs, &docxParagraph{
				element: element, text: bodyText, runs: runs,
				labelStart: -1, labelEnd: -1, bodyStart: bodyStart,
			})
			continue
		}
		labelStart, labelEnd, colonInBody, ok := classifyRuns(runs)
		if !ok {
			d.formatted++
			d.skipped++
			continue
		}
		marker, markerSpace := docxListMarker(text)
		d.paragraphs = append(d.paragraphs, &docxParagraph{
			element: element, text: text, runs: runs,
			labelStart: labelStart, labelEnd: labelEnd, colonInBody: colonInBody,
			bodyStart: -1, marker: marker, markerSpace: markerSpace,
		})
	}
}

var (
	sectionHeadingPattern  = regexp.MustCompile(`(?i)^(chapter|section|part|appendix|appendices|table of contents|list of (tables|figures)|abstract|declaration|certification|dedication|acknowledg(e)?ments?|references|bibliography|glossary|abbreviations|definitions?)\b`)
	numberedHeadingPattern = regexp.MustCompile(`^\d+(?:\.\d+)*[.)]?\s+\S`)
	markdownHeadingPattern = regexp.MustCompile(`^#{1,6}\s`)
	docxListMarkerPattern  = regexp.MustCompile(`^((?:\d+|[ivxlcdmIVXLCDM]+|[A-Za-z])[.)])(\s*)`)
)

// looksLikeHeading catches the many converted Word documents that do not tag
// headings with a Heading style. Short lines without terminal punctuation are
// headings, labels, names, or signature lines, not prose. Sending them to a
// model invites it to invent a paragraph around the fragment.
func looksLikeHeading(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if markdownHeadingPattern.MatchString(text) {
		return true
	}
	words := len(strings.Fields(text))
	if sectionHeadingPattern.MatchString(text) && words <= 14 && !endsSentence(text) {
		return true
	}
	if numberedHeadingPattern.MatchString(text) && words <= 14 && !endsSentence(text) {
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

// docxListMarker returns a leading i./ii./1./A. marker and the whitespace after it,
// so a rewrite cannot renumber a list item.
func docxListMarker(text string) (string, string) {
	match := docxListMarkerPattern.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return "", ""
	}
	lower := strings.ToLower(strings.TrimSpace(text))
	if strings.HasPrefix(lower, "i.e.") || strings.HasPrefix(lower, "e.g.") {
		return "", ""
	}
	return match[1], match[2]
}

// classifyRuns accepts plain paragraphs and two common label shapes:
// "Bold label: body" and "number + bold label + body". Anything with genuinely
// mixed formatting is left alone so links, italics and other inline formatting
// cannot be damaged.
func classifyRuns(runs []docxRun) (labelStart, labelEnd int, colonInBody, ok bool) {
	if len(runs) == 0 {
		return -1, -1, false, false
	}
	if allSameVisual(runs) {
		return -1, -1, false, true
	}
	start := 0
	if len(runs) >= 2 && !runs[0].bold && isMarkerOnly(runs[0].text) {
		start = 1
	}
	end := start
	for end < len(runs) && runs[end].bold {
		end++
	}
	if end == start || end == len(runs) {
		return -1, -1, false, false
	}
	labelKey := visualKey(runs[start])
	for _, run := range runs[start:end] {
		if visualKey(run) != labelKey {
			return -1, -1, false, false
		}
	}
	bodyKey := visualKey(runs[end])
	for _, run := range runs[end:] {
		if visualKey(run) != bodyKey {
			return -1, -1, false, false
		}
	}
	label := strings.TrimSpace(joinRunText(runs[start:end]))
	if strings.HasSuffix(label, ":") {
		return start, end, false, true
	}
	if strings.HasPrefix(strings.TrimSpace(runs[end].text), ":") {
		return start, end, true, true
	}
	return -1, -1, false, false
}

func visualKey(run docxRun) string {
	return fmt.Sprintf("%t/%t/%t/%t", run.bold, run.italic, run.underline, run.strike)
}

func allSameVisual(runs []docxRun) bool {
	if len(runs) == 0 {
		return true
	}
	key := visualKey(runs[0])
	for _, run := range runs[1:] {
		if visualKey(run) != key {
			return false
		}
	}
	return true
}

func isMarkerOnly(text string) bool {
	text = strings.TrimSpace(text)
	marker, space := docxListMarker(text)
	if marker == "" {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(text, marker+space)) == ""
}

// breakBodyStart recognises a paragraph that embeds a heading, a line break, and
// a body sentence in one Word paragraph. Converted reports do this in chapter
// sections. The heading and break stay untouched; only the body is rewritten.
func breakBodyStart(runs []docxRun) int {
	for i, run := range runs {
		if !run.hasBreak {
			continue
		}
		if i+1 >= len(runs) || !run.bold {
			return -1
		}
		heading := strings.TrimSpace(run.text)
		if !looksLikeHeading(heading) && !(len(strings.Fields(heading)) <= 12 && !endsSentence(heading)) {
			return -1
		}
		bodyKey := visualKey(runs[i+1])
		for _, bodyRun := range runs[i+1:] {
			if visualKey(bodyRun) != bodyKey {
				return -1
			}
		}
		return i + 1
	}
	return -1
}

func (d *docxDoc) apply(paragraph *docxParagraph, text string) error {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if text == "" {
		return fmt.Errorf("empty rewrite")
	}
	if paragraph.bodyStart >= 0 {
		if paragraph.bodyStart >= len(paragraph.runs) {
			return fmt.Errorf("missing body run")
		}
		setRunText(paragraph.runs[paragraph.bodyStart].element, text)
		for i := paragraph.bodyStart + 1; i < len(paragraph.runs); i++ {
			setRunText(paragraph.runs[i].element, "")
		}
		return nil
	}
	if paragraph.labelStart >= 0 && paragraph.labelEnd > paragraph.labelStart {
		if paragraph.labelStart > 0 {
			// The list marker is its own run. Keep it and strip any marker the
			// model put at the front of the rewrite.
			text = stripMarker(text)
		} else {
			text = restoreMarker(paragraph, text)
		}
		originalLabel := strings.TrimSpace(joinRunText(paragraph.runs[paragraph.labelStart:paragraph.labelEnd]))
		label := originalLabel
		body := text
		if paragraph.colonInBody {
			if colon := strings.Index(text, ":"); colon >= 0 && colon <= 100 {
				label = strings.TrimSpace(text[:colon])
				body = ": " + strings.TrimSpace(text[colon+1:])
			} else {
				body = ": " + strings.TrimSpace(text)
			}
		} else if colon := strings.Index(text, ":"); colon >= 0 && colon <= 100 {
			label = strings.TrimSpace(text[:colon+1])
			body = strings.TrimSpace(text[colon+1:])
		} else if bare := strings.TrimSuffix(originalLabel, ":"); bare != "" &&
			strings.HasPrefix(strings.ToLower(text), strings.ToLower(bare)) {
			body = strings.TrimSpace(text[len(bare):])
		}
		if strings.TrimSpace(body) == "" || strings.TrimSpace(body) == ":" {
			return fmt.Errorf("empty rewritten body")
		}
		if !paragraph.colonInBody {
			body = " " + strings.TrimSpace(body)
		}
		setRunText(paragraph.runs[paragraph.labelStart].element, label)
		for i := paragraph.labelStart + 1; i < paragraph.labelEnd; i++ {
			setRunText(paragraph.runs[i].element, "")
		}
		if paragraph.labelEnd < len(paragraph.runs) {
			setRunText(paragraph.runs[paragraph.labelEnd].element, body)
		}
		for i := paragraph.labelEnd + 1; i < len(paragraph.runs); i++ {
			setRunText(paragraph.runs[i].element, "")
		}
		return nil
	}

	text = restoreMarker(paragraph, text)
	setRunText(paragraph.runs[0].element, text)
	for i := 1; i < len(paragraph.runs); i++ {
		setRunText(paragraph.runs[i].element, "")
	}
	return nil
}

func stripMarker(text string) string {
	text = strings.TrimSpace(text)
	if marker, space := docxListMarker(text); marker != "" {
		return strings.TrimSpace(strings.TrimPrefix(text, marker+space))
	}
	return text
}

func restoreMarker(paragraph *docxParagraph, text string) string {
	if paragraph.marker == "" {
		return text
	}
	if marker, space := docxListMarker(text); marker != "" {
		text = strings.TrimSpace(strings.TrimPrefix(text, marker+space))
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return paragraph.marker
	}
	return paragraph.marker + paragraph.markerSpace + text
}

func (d *docxDoc) write(path string) error {
	xmlBytes, err := d.xml.WriteToBytes()
	if err != nil {
		return fmt.Errorf("serialize %s: %w", docxDocumentPath, err)
	}
	tmp := path + ".part"
	file, err := os.Create(tmp)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	for _, entry := range d.entries {
		header := &zip.FileHeader{Name: entry.name, Method: entry.method, Modified: entry.modified}
		part, err := writer.CreateHeader(header)
		if err != nil {
			writer.Close()
			file.Close()
			os.Remove(tmp)
			return err
		}
		data := entry.data
		if entry.name == docxDocumentPath {
			data = xmlBytes
		}
		if _, err := part.Write(data); err != nil {
			writer.Close()
			file.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := writer.Close(); err != nil {
		file.Close()
		os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func (h *Humanize) runDOCX(
	ctx context.Context,
	item core.Artifact,
	outputDir string,
	coreRules, styleText, voice, requested, styleName string,
	workers int,
	rep core.Reporter,
) (core.Artifact, string, int, error) {
	doc, err := readDOCX(item.Path)
	if err != nil {
		return core.Artifact{}, "", 0, core.Wrap(core.CodeInput, humanizeSpec.ID, item.Path, err,
			"check that the file is a valid DOCX")
	}
	if len(doc.paragraphs) == 0 {
		return core.Artifact{}, "", 0, core.NewError(core.CodeInput, humanizeSpec.ID, item.Path,
			"no body text found in the DOCX", "the document may contain only tables, figures, or headings")
	}

	results := make([]string, len(doc.paragraphs))
	var (
		group    errgroup.Group
		mu       sync.Mutex
		used     = map[string]int{}
		fallback int
		done     atomic.Int64
	)
	group.SetLimit(workers)
	for i, paragraph := range doc.paragraphs {
		i, paragraph := i, paragraph
		group.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			previous := ""
			if i > 0 {
				previous = lastWords(doc.paragraphs[i-1].text, 80)
			}
			out, ref, err := h.rewrite(ctx, paragraph.text, coreRules, styleText, voice, "", previous, requested, rep)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				mu.Lock()
				fallback++
				results[i] = paragraph.text
				mu.Unlock()
				rep.Log(core.LevelWarn, "%s: paragraph %d/%d kept original text: %v",
					item.Name(), i+1, len(doc.paragraphs), err)
			} else {
				mu.Lock()
				results[i] = out
				used[ref.String()]++
				mu.Unlock()
			}
			rep.Progress(int(done.Add(1)), len(doc.paragraphs))
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return core.Artifact{}, "", 0, err
	}
	if fallback == len(doc.paragraphs) {
		return core.Artifact{}, "", 0, core.NewError(core.CodeEngineFailed, humanizeSpec.ID, item.Path,
			"no free model could rewrite the DOCX text", "check that OpenCode is running and a free model is enabled")
	}

	applied := 0
	for i, paragraph := range doc.paragraphs {
		if results[i] == paragraph.text {
			continue
		}
		if err := doc.apply(paragraph, results[i]); err != nil {
			rep.Log(core.LevelWarn, "%s: paragraph %d kept original formatting: %v", item.Name(), i+1, err)
			continue
		}
		applied++
	}

	output := filepath.Join(outputDir, core.SanitizeStem(item.Stem())+".docx")
	if err := doc.write(output); err != nil {
		return core.Artifact{}, "", 0, core.Wrap(core.CodeOutput, humanizeSpec.ID, output, err,
			"check permissions on the output directory")
	}
	info, err := os.Stat(output)
	if err != nil {
		return core.Artifact{}, "", 0, core.Wrap(core.CodeOutput, humanizeSpec.ID, output, err, "")
	}

	modelName := ""
	if len(used) == 1 {
		for name := range used {
			modelName = name
		}
	} else if len(used) > 1 {
		modelName = "mixed"
	}
	artifact := core.Artifact{
		Path:   output,
		Kind:   core.Document,
		Format: "docx",
		Size:   info.Size(),
		Meta: map[string]any{
			"source":           item.Path,
			"style":            styleName,
			"model":            modelName,
			"paragraphs":       len(doc.paragraphs),
			"rewritten":        applied,
			"skipped":          doc.skipped,
			"mixed_formatting": doc.formatted,
			"fallback":         fallback,
		},
	}
	rep.Log(core.LevelInfo, "%s: humanized %d/%d paragraphs with %s (%d skipped, %d mixed-format left original, %d kept original)",
		item.Name(), applied, len(doc.paragraphs), modelName, doc.skipped, doc.formatted, fallback)
	return artifact, modelName, fallback, nil
}

func paragraphText(paragraph *etree.Element) string {
	var b strings.Builder
	var walk func(*etree.Element)
	walk = func(element *etree.Element) {
		for _, child := range element.ChildElements() {
			switch child.Tag {
			case "t":
				if child.Space == "w" {
					b.WriteString(child.Text())
				}
			case "tab", "br", "cr":
				b.WriteByte(' ')
			default:
				walk(child)
			}
		}
	}
	walk(paragraph)
	return b.String()
}

func paragraphStyle(paragraph *etree.Element) string {
	pPr := directChild(paragraph, "pPr")
	if pPr == nil {
		return ""
	}
	style := directChild(pPr, "pStyle")
	if style == nil {
		return ""
	}
	return attrValue(style, "val")
}

func textRuns(paragraph *etree.Element) []docxRun {
	var runs []docxRun
	for _, element := range paragraph.ChildElements() {
		if element.Tag != "r" {
			continue
		}
		text := runText(element)
		if strings.TrimSpace(text) == "" {
			continue
		}
		runs = append(runs, docxRun{
			element:   element,
			text:      text,
			bold:      runFlag(element, "b"),
			italic:    runFlag(element, "i"),
			underline: runFlag(element, "u"),
			strike:    runFlag(element, "strike"),
			hasBreak:  directChild(element, "br") != nil,
		})
	}
	return runs
}

func runText(run *etree.Element) string {
	var b strings.Builder
	for _, child := range run.ChildElements() {
		if child.Tag == "t" && child.Space == "w" {
			b.WriteString(child.Text())
		}
	}
	return b.String()
}

func joinRunText(runs []docxRun) string {
	var b strings.Builder
	for _, run := range runs {
		b.WriteString(run.text)
	}
	return b.String()
}

func runFlag(run *etree.Element, tag string) bool {
	rPr := directChild(run, "rPr")
	if rPr == nil {
		return false
	}
	flag := directChild(rPr, tag)
	if flag == nil {
		return false
	}
	value := strings.ToLower(attrValue(flag, "val"))
	return value == "" || value == "1" || value == "true" || value == "on"
}

func setRunText(run *etree.Element, text string) {
	for _, child := range run.ChildElements() {
		if child.Tag == "t" {
			run.RemoveChild(child)
		}
	}
	if text == "" {
		return
	}
	t := run.CreateElement("w:t")
	t.CreateAttr("xml:space", "preserve")
	t.SetText(text)
}

func directChild(parent *etree.Element, tag string) *etree.Element {
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

func hasDescendant(element *etree.Element, tags ...string) bool {
	for _, child := range element.ChildElements() {
		for _, tag := range tags {
			if child.Tag == tag {
				return true
			}
		}
		if hasDescendant(child, tags...) {
			return true
		}
	}
	return false
}

func attrValue(element *etree.Element, key string) string {
	if element == nil {
		return ""
	}
	for _, attr := range element.Attr {
		if attr.Key == key {
			return attr.Value
		}
	}
	return ""
}

func hasLetter(text string) bool {
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func lastWords(text string, limit int) string {
	words := strings.Fields(text)
	if len(words) <= limit {
		return strings.Join(words, " ")
	}
	return strings.Join(words[len(words)-limit:], " ")
}
