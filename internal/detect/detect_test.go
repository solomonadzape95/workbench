package detect

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func analyzeText(t *testing.T, text string) Report {
	t.Helper()
	doc := Document{Source: Source{Path: "test.txt", Name: "test.txt", Format: "txt"}, Paragraphs: []ParagraphInput{{Text: text}}}
	report, err := Analyze(doc, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	return report
}

const aiSample = `In today's fast-paced world, it is important to note that organizations must leverage robust strategies to facilitate seamless collaboration. Moreover, the multifaceted landscape of modern work requires a comprehensive approach to communication. Furthermore, teams that utilize these strategies can unlock significant potential and drive meaningful outcomes. Additionally, leaders should foster a culture of transparency and accountability. In conclusion, by embracing these principles, organizations can elevate their performance and embark on a journey toward sustainable success.`

const humanSample = `I started using the thing on a Tuesday, mostly because the Monday meeting had run forty minutes over and nobody had decided anything. By Friday I had deleted two spreadsheets. The surprising part wasn't the time saved. It was how quiet the channel got. People stopped asking permission for small things and just did them. We kept the Friday demo, though. That one earned its keep.`

func TestAnalyzeSeparatesAIAndHuman(t *testing.T) {
	ai := analyzeText(t, aiSample)
	human := analyzeText(t, humanSample)
	if ai.Summary.AI < 70 {
		t.Fatalf("AI sample scored %.0f, want at least 70", ai.Summary.AI)
	}
	if human.Summary.AI > 30 {
		t.Fatalf("human sample scored %.0f, want at most 30", human.Summary.AI)
	}
	if ai.Summary.AI <= human.Summary.AI+30 {
		t.Fatalf("separation too weak: AI %.0f human %.0f", ai.Summary.AI, human.Summary.AI)
	}
}

func TestAttributionRecognisesChatGPTFingerprint(t *testing.T) {
	report := analyzeText(t, aiSample)
	if len(report.Attribution) == 0 || report.Attribution[0].Family != "chatgpt" {
		t.Fatalf("attribution = %+v", report.Attribution)
	}
}

func TestSplitSentencesHandlesAbbreviationsAndDecimals(t *testing.T) {
	got := splitSentences("Dr. Smith wrote this in 2008. The system uses RSA-2048. It costs 3.5 million naira in 2026.")
	if len(got) != 3 {
		t.Fatalf("sentences = %#v", got)
	}
}

func TestMarkdownExtractionSkipsNonProse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.md")
	text := "# Heading\n\nThis is a real prose paragraph with enough words to be measured by the detector. It has two sentences.\n\n```go\nfunc main() { println(\"ignore me\") }\n```\n\n| Name | Value |\n|---|---|\n| Alpha | One |\n\n## References\n\nSmith, J. (2020). A paper that should be ignored by the detector.\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := Extract(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Paragraphs) != 1 {
		t.Fatalf("paragraphs = %#v", doc.Paragraphs)
	}
	if strings.Contains(doc.Paragraphs[0].Text, "ignore me") || strings.Contains(doc.Paragraphs[0].Text, "Smith") {
		t.Fatalf("non-prose leaked: %q", doc.Paragraphs[0].Text)
	}
}

func TestReportJSONRoundTrip(t *testing.T) {
	report := analyzeText(t, aiSample)
	path := filepath.Join(t.TempDir(), "report.json")
	if err := WriteJSON(report, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Summary.AI != report.Summary.AI || loaded.Summary.Verdict != report.Summary.Verdict {
		t.Fatalf("round trip changed summary: %+v vs %+v", loaded.Summary, report.Summary)
	}
	md := RenderMarkdown(report)
	for _, want := range []string{"AI detection report", "How this was scored", "Likely source"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
}

func TestFormalProfileOnLongAcademicProse(t *testing.T) {
	text := `The proposed architecture separates voter identity from ballot content through a blind signature protocol. The registration authority verifies eligibility before issuing a token. The ballot box validates the signature without learning the voter identity. The tallying contract records each accepted ballot on a public ledger. The audit process recomputes the tally from published records. The governance model requires multiple administrators to approve critical actions. The evaluation uses simulated voters and controlled network conditions. The results demonstrate correctness under the stated assumptions. The implementation follows a three-tier design with a presentation layer, an application layer, and a data layer. The cryptographic component uses RSA blind signatures and a deterministic verification procedure. The database stores eligibility records separately from submitted ballots. The audit report lists the verification steps and the final tally.`
	report := analyzeText(t, text)
	if report.Summary.Profile != "formal / academic" {
		t.Fatalf("profile = %q", report.Summary.Profile)
	}
}

func TestFrontMatterAndReferenceTrimming(t *testing.T) {
	paras := []ParagraphInput{
		{Text: "A PROJECT TITLE"},
		{Text: "ABSTRACT"},
		{Text: "This is the abstract body with enough words to be prose."},
		{Text: "TABLE OF CONTENTS"},
		{Text: "1. Introduction 1"},
		{Text: "CHAPTER ONE: INTRODUCTION"},
		{Text: "This is the body prose with enough words to be measured."},
		{Text: "REFERENCES"},
		{Text: "Smith, J. (2020). A reference that should be ignored."},
	}
	got := trimReferences(filterFrontSections(trimFrontMatter(paras)))
	if len(got) != 2 {
		t.Fatalf("trimmed = %#v", got)
	}
	if !strings.Contains(got[0].Text, "abstract body") || !strings.Contains(got[1].Text, "body prose") {
		t.Fatalf("wrong paragraphs: %#v", got)
	}
}

func TestCleanPDFTextRemovesFurniture(t *testing.T) {
	text := "Repeated Header\n1\nThis is a body line with enough words.\n   Column one   Column two\n\f" +
		"Repeated Header\n2\nAnother body line with enough words.\n\f" +
		"Repeated Header\n3\nThird body line with enough words.\n\f" +
		"Repeated Header\n4\nFourth body line with enough words.\n"
	cleaned := cleanPDFText(text)
	if strings.Contains(cleaned, "Repeated Header") {
		t.Errorf("repeated header survived: %q", cleaned)
	}
	if strings.Contains(cleaned, "Column one") {
		t.Errorf("table row survived: %q", cleaned)
	}
	for _, want := range []string{"This is a body line", "Another body line", "Third body line"} {
		if !strings.Contains(cleaned, want) {
			t.Errorf("body line %q missing: %q", want, cleaned)
		}
	}
}

func TestDOCXExtractionSkipsFrontMatterAndReferences(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:r><w:t>A PROJECT TITLE</w:t></w:r></w:p>
<w:p><w:r><w:t>Agbo Casmir Chidubem</w:t></w:r></w:p>
<w:p><w:r><w:t>ABSTRACT</w:t></w:r></w:p>
<w:p><w:r><w:t>This is the abstract body with enough words to be measured as prose.</w:t></w:r></w:p>
<w:p><w:r><w:t>CHAPTER ONE</w:t></w:r></w:p>
<w:p><w:r><w:t>1.1 Background to the Study</w:t></w:r></w:p>
<w:p><w:r><w:t>This is the body paragraph with enough words to be measured as prose.</w:t></w:r></w:p>
<w:p><w:r><w:t>REFERENCES</w:t></w:r></w:p>
<w:p><w:r><w:t>Smith, J. (2020). A reference that should be ignored.</w:t></w:r></w:p>
<w:sectPr/></w:body></w:document>`
	path := filepath.Join(t.TempDir(), "test.docx")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	part, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := Extract(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Paragraphs) != 2 {
		t.Fatalf("paragraphs = %#v", doc.Paragraphs)
	}
	if !strings.Contains(doc.Paragraphs[0].Text, "abstract body") || !strings.Contains(doc.Paragraphs[1].Text, "body paragraph") {
		t.Fatalf("wrong extraction: %#v", doc.Paragraphs)
	}
}

func TestLogProbSignalIsOptional(t *testing.T) {
	doc := Document{Source: Source{Path: "test.txt", Name: "test.txt", Format: "txt"}, Paragraphs: []ParagraphInput{{Text: humanSample}}}
	score := 0.9
	opts := DefaultOptions()
	opts.LogProbScore = &score
	opts.LogProbWeight = 0.2
	report, err := Analyze(doc, opts)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, signal := range report.Signals {
		if signal.Key == "logprob" {
			found = true
			if signal.AIScore != 90 {
				t.Errorf("logprob score = %v", signal.AIScore)
			}
		}
	}
	if !found {
		t.Fatal("logprob signal missing")
	}
}
