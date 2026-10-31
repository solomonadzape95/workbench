package humanize

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/opencode"
)

const testDOCXXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Original body paragraph.</w:t></w:r></w:p>
    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Heading stays</w:t></w:r></w:p>
    <w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Label:</w:t></w:r><w:r><w:t> Original definition text.</w:t></w:r></w:p>
    <w:p><w:r><w:drawing/></w:r></w:p>
    <w:tbl><w:tr><w:tc><w:p><w:r><w:t>Table cell stays</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
    <w:p><w:r><w:t>Second body paragraph.</w:t></w:r></w:p>
    <w:sectPr/>
  </w:body>
</w:document>`

const structuredDOCXXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Original body paragraph.</w:t></w:r></w:p>
    <w:p><w:r><w:t>1.1 Background to the Study</w:t></w:r></w:p>
    <w:p><w:r><w:t>Agbo Casmir Chidubem</w:t></w:r></w:p>
    <w:p><w:r><w:t>DECLARATION</w:t></w:r></w:p>
    <w:p><w:r><w:t>i.Analyze the structural weaknesses of centralized voting.</w:t></w:r></w:p>
    <w:p><w:r><w:t>1. Academic information is distributed through informal channels.</w:t></w:r></w:p>
    <w:p><w:r><w:t>Second body paragraph.</w:t></w:r></w:p>
    <w:sectPr/>
  </w:body>
</w:document>`

func writeTestDOCX(t *testing.T) string {
	t.Helper()
	return writeDOCXWithXML(t, testDOCXXML)
}

func writeDOCXWithXML(t *testing.T, documentXML string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.docx")
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entries := map[string][]byte{
		"word/document.xml":     []byte(documentXML),
		"word/media/image1.png": []byte("not really a png"),
		"word/styles.xml":       []byte("<w:styles xmlns:w=\"http://schemas.openxmlformats.org/wordprocessingml/2006/main\"/>"),
		"[Content_Types].xml":   []byte(`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`),
	}
	for name, data := range entries {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadDOCXCollectsBodyAndPreservesStructure(t *testing.T) {
	path := writeTestDOCX(t)
	doc, err := readDOCX(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.paragraphs) != 3 {
		t.Fatalf("paragraphs = %d, want 3", len(doc.paragraphs))
	}
	if doc.paragraphs[0].text != "Original body paragraph." {
		t.Errorf("first = %q", doc.paragraphs[0].text)
	}
	if doc.paragraphs[1].labelEnd != 1 {
		t.Errorf("labelEnd = %d, want 1", doc.paragraphs[1].labelEnd)
	}
	if doc.paragraphs[2].text != "Second body paragraph." {
		t.Errorf("last = %q", doc.paragraphs[2].text)
	}
	if doc.skipped < 1 {
		t.Errorf("skipped = %d, want at least the heading", doc.skipped)
	}
}

func TestDOCXApplyKeepsTableFigureAndBoldLabel(t *testing.T) {
	path := writeTestDOCX(t)
	doc, err := readDOCX(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.apply(doc.paragraphs[0], "A person wrote this body."); err != nil {
		t.Fatal(err)
	}
	if err := doc.apply(doc.paragraphs[1], "New label: a person wrote this definition."); err != nil {
		t.Fatal(err)
	}
	if err := doc.apply(doc.paragraphs[2], "Another human paragraph."); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.docx")
	if err := doc.write(out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	xml, err := docxPart(data, "word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(xml)
	labelText := paragraphText(doc.paragraphs[1].element)
	if !strings.Contains(labelText, "New label: a person wrote this definition.") {
		t.Errorf("label spacing = %q", labelText)
	}
	for _, want := range []string{"A person wrote this body.", "New label:", "a person wrote this definition.", "Another human paragraph.", "Table cell stays", "Heading stays"} {
		if !strings.Contains(text, want) {
			t.Errorf("output document.xml missing %q", want)
		}
	}
	if strings.Contains(text, "Original body paragraph.") {
		t.Error("old body text remains")
	}
	if !strings.Contains(text, "<w:drawing") && !strings.Contains(text, ":drawing") {
		t.Error("drawing was removed")
	}
	if !strings.Contains(text, "Table cell stays") {
		t.Error("table text was removed")
	}
	parsed := etree.NewDocument()
	if err := parsed.ReadFromBytes(xml); err != nil {
		t.Fatalf("output document.xml is not valid XML: %v", err)
	}
	media, err := docxPart(data, "word/media/image1.png")
	if err != nil || string(media) != "not really a png" {
		t.Errorf("media part = %q, %v", media, err)
	}
}

func TestDOCXLabelFallbackWithoutColon(t *testing.T) {
	path := writeTestDOCX(t)
	doc, err := readDOCX(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.apply(doc.paragraphs[1], "A person wrote this definition."); err != nil {
		t.Fatal(err)
	}
	xml, err := doc.xml.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xml, "Label:") || !strings.Contains(xml, "A person wrote this definition.") {
		t.Errorf("label fallback output = %s", xml)
	}
}

func TestRunDOCXHumanizesTextAndPreservesTablesAndFigures(t *testing.T) {
	src := writeTestDOCX(t)
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	gen := &fakeGenerator{
		models: []opencode.ModelRef{{ProviderID: "nvidia", ID: "free"}},
		fn: func(_ opencode.ModelRef, prompt string) (string, error) {
			text := prompt[strings.Index(prompt, "TEXT TO REWRITE:\n")+len("TEXT TO REWRITE:\n"):]
			if strings.Contains(text, "Label:") {
				return "New label: a person wrote this definition.", nil
			}
			return "A person wrote this body.", nil
		},
	}
	op := &Humanize{Client: gen, Timeout: 2 * time.Second}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"style": "professional", "model": "auto", "workers": 2, "chunk_words": 400},
		Target: core.Target{Dir: outDir},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Format != "docx" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	xml, err := docxPart(data, "word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(xml)
	for _, want := range []string{"A person wrote this body.", "New label:", "a person wrote this definition.", "Table cell stays", "Heading stays"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if strings.Contains(text, "Original body paragraph.") || strings.Contains(text, "Original definition text.") {
		t.Error("old body text remains")
	}
}

func TestDOCXIsAnAcceptedFormat(t *testing.T) {
	if !humanizeSpec.AcceptsArtifact(core.Artifact{Kind: core.Document, Format: "docx"}) {
		t.Fatal("docx is not accepted")
	}
}

func docxPart(data []byte, name string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("missing %s", name)
}

func TestDOCXSkipsUnstyledHeadingsNamesAndSignatures(t *testing.T) {
	doc, err := readDOCX(writeDOCXWithXML(t, structuredDOCXXML))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.paragraphs) != 4 {
		t.Fatalf("paragraphs = %d, want 4", len(doc.paragraphs))
	}
	for _, paragraph := range doc.paragraphs {
		for _, bad := range []string{"Background to the Study", "Agbo Casmir", "DECLARATION"} {
			if strings.Contains(paragraph.text, bad) {
				t.Errorf("non-body text %q was collected", paragraph.text)
			}
		}
	}
}

func TestDOCXPreservesListMarkers(t *testing.T) {
	doc, err := readDOCX(writeDOCXWithXML(t, structuredDOCXXML))
	if err != nil {
		t.Fatal(err)
	}
	if doc.paragraphs[1].marker != "i." || doc.paragraphs[1].markerSpace != "" {
		t.Fatalf("roman marker = %q/%q", doc.paragraphs[1].marker, doc.paragraphs[1].markerSpace)
	}
	if doc.paragraphs[2].marker != "1." || doc.paragraphs[2].markerSpace != " " {
		t.Fatalf("numbered marker = %q/%q", doc.paragraphs[2].marker, doc.paragraphs[2].markerSpace)
	}
	if err := doc.apply(doc.paragraphs[1], "I. Analyze the structural weaknesses in centralized voting."); err != nil {
		t.Fatal(err)
	}
	if err := doc.apply(doc.paragraphs[2], "1) Academic information is shared through informal channels."); err != nil {
		t.Fatal(err)
	}
	if got := paragraphText(doc.paragraphs[1].element); !strings.HasPrefix(got, "i.Analyze") {
		t.Errorf("roman rewrite = %q", got)
	}
	if got := paragraphText(doc.paragraphs[2].element); !strings.HasPrefix(got, "1. Academic") {
		t.Errorf("numbered rewrite = %q", got)
	}
}

func TestLooksLikeHeading(t *testing.T) {
	for _, text := range []string{
		"1.1 Background to the Study",
		"1.5 Significance of the Study",
		"CHAPTER ONE",
		"DECLARATION",
		"Agbo Casmir Chidubem",
		"Prof. C. N. Udanor Date",
		"## Significance of the Study",
		"The specific objectives are to:",
	} {
		if !looksLikeHeading(text) {
			t.Errorf("looksLikeHeading(%q) = false", text)
		}
	}
	for _, text := range []string{
		"Voting is one of the most important actions a person can take.",
		"Aim: The primary aim of this study is to design a secure platform.",
		"i.Analyze the structural weaknesses of centralized voting.",
	} {
		if looksLikeHeading(text) {
			t.Errorf("looksLikeHeading(%q) = true", text)
		}
	}
}

func TestDocxListMarker(t *testing.T) {
	cases := []struct {
		text   string
		marker string
		space  string
	}{
		{"i.Analyze the structural weaknesses.", "i.", ""},
		{"iv.To integrate Ethereum smart contracts.", "iv.", ""},
		{"1. Academic information is distributed.", "1.", " "},
		{"A. First item.", "A.", " "},
		{"e.g. this is an example.", "", ""},
		{"i.e. this is a clarification.", "", ""},
		{"Voting is important.", "", ""},
	}
	for _, tc := range cases {
		marker, space := docxListMarker(tc.text)
		if marker != tc.marker || space != tc.space {
			t.Errorf("docxListMarker(%q) = %q/%q, want %q/%q", tc.text, marker, space, tc.marker, tc.space)
		}
	}
}

const mixedDOCXXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Output design specifies the information returned to each actor. It describes the outputs.</w:t></w:r></w:p>
    <w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Aim</w:t></w:r><w:r><w:t>: The aim of this study is to build a secure platform.</w:t></w:r></w:p>
    <w:p><w:r><w:t>1. </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>Absence:</w:t></w:r><w:r><w:t> In centralized systems the database links each vote.</w:t></w:r></w:p>
    <w:p><w:r><w:rPr><w:b/></w:rPr><w:t>4.1 Choice</w:t><w:br/></w:r><w:r><w:t>The environment was selected for security.</w:t></w:r></w:p>
    <w:p><w:r><w:t>Software follows the </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>Agile</w:t></w:r><w:r><w:t> methodology.</w:t></w:r></w:p>
    <w:sectPr/>
  </w:body>
</w:document>`

func TestDOCXHandlesMixedShapes(t *testing.T) {
	doc, err := readDOCX(writeDOCXWithXML(t, mixedDOCXXML))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.paragraphs) != 4 {
		t.Fatalf("paragraphs = %d, want 4", len(doc.paragraphs))
	}
	if doc.paragraphs[0].labelStart != -1 || doc.paragraphs[0].bodyStart != -1 {
		t.Errorf("heading-style prose was not treated as plain text")
	}
	if doc.paragraphs[1].labelStart != 0 || doc.paragraphs[1].labelEnd != 1 || !doc.paragraphs[1].colonInBody {
		t.Errorf("colon-in-body label = %+v", doc.paragraphs[1])
	}
	if doc.paragraphs[2].labelStart != 1 || doc.paragraphs[2].labelEnd != 2 || doc.paragraphs[2].marker != "1." {
		t.Errorf("numbered label = %+v", doc.paragraphs[2])
	}
	if doc.paragraphs[3].bodyStart != 1 || doc.paragraphs[3].text != "The environment was selected for security." {
		t.Errorf("break body = %+v", doc.paragraphs[3])
	}

	if err := doc.apply(doc.paragraphs[1], "Purpose: A new aim for the platform."); err != nil {
		t.Fatal(err)
	}
	if got := paragraphText(doc.paragraphs[1].element); got != "Purpose: A new aim for the platform." {
		t.Errorf("colon-in-body rewrite = %q", got)
	}
	if err := doc.apply(doc.paragraphs[2], "1. Missing: In centralized systems, access rules can hide the link."); err != nil {
		t.Fatal(err)
	}
	if got := paragraphText(doc.paragraphs[2].element); got != "1. Missing: In centralized systems, access rules can hide the link." {
		t.Errorf("numbered label rewrite = %q", got)
	}
	if err := doc.apply(doc.paragraphs[3], "The environment was chosen for security."); err != nil {
		t.Fatal(err)
	}
	if got := runText(doc.paragraphs[3].runs[0].element); got != "4.1 Choice" {
		t.Errorf("heading changed to %q", got)
	}
	if got := runText(doc.paragraphs[3].runs[1].element); got != "The environment was chosen for security." {
		t.Errorf("break body rewrite = %q", got)
	}
}

func TestCaptionPatternDoesNotSwallowProse(t *testing.T) {
	for _, text := range []string{
		"Table 2.1: Summary of strengths and weaknesses",
		"Figure 1. Test figure",
		"Table 2.1 - Results",
	} {
		if !captionPattern.MatchString(text) {
			t.Errorf("captionPattern(%q) = false", text)
		}
	}
	for _, text := range []string{
		"Table 2.1 summarises the principal strengths and weaknesses of the reviewed literature.",
		"Figure 3.4 presents the activity diagram for the complete voting process.",
	} {
		if captionPattern.MatchString(text) {
			t.Errorf("captionPattern(%q) = true", text)
		}
	}
}
