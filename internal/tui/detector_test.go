package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	aidetect "github.com/solomonadzape95/workbench/internal/detect"
	detectops "github.com/solomonadzape95/workbench/internal/ops/detect"
	docops "github.com/solomonadzape95/workbench/internal/ops/doc"
	humanizeops "github.com/solomonadzape95/workbench/internal/ops/humanize"
	"github.com/solomonadzape95/workbench/internal/runner"
)

func sampleDetectorReport(path string) aidetect.Report {
	return aidetect.Report{
		Version: "1",
		Source:  aidetect.Source{Path: path, Name: "report.docx", Format: "docx", Bytes: 1234},
		Summary: aidetect.Summary{
			AI: 63, Human: 37, Verdict: "Likely AI-assisted", Confidence: "medium",
			Reliability: 82, SentenceAI: 41, Flagged: 3, Words: 1200, Sentences: 80,
			Paragraphs: 12, Profile: "formal / academic", AttributionTop: "ChatGPT / GPT family",
		},
		Signals: []aidetect.Signal{
			{Key: "sentence_cv", Label: "Sentence rhythm", Display: "CV 0.34", AIScore: 78, Weight: 0.2, Contribution: 15, Direction: "AI-like", Detail: "uniform rhythm"},
			{Key: "ai_phrases", Label: "AI phrases", Display: "3.2/1k", AIScore: 64, Weight: 0.16, Contribution: 10, Direction: "AI-like", Detail: "assistant phrases"},
		},
		Flagged: []aidetect.Sentence{
			{Index: 0, Paragraph: 2, Text: "It is important to note that the system leverages a robust framework.", AI: 91, Reasons: []string{"assistant phrase: it is important to note"}},
		},
		Attribution: []aidetect.Attribution{
			{Family: "chatgpt", Label: "ChatGPT / GPT family", Score: 71, Confidence: "medium", Evidence: []string{"it is important to note", "robust"}},
		},
		Warnings: []string{"Formal academic prose can look AI-like."},
		Stats:    aidetect.Stats{SentenceBuckets: [10]int{1, 2, 3, 4, 5, 4, 3, 2, 1, 1}},
		Paragraphs: []aidetect.Paragraph{
			{Index: 0, Words: 120, Sentences: 8, AI: 71, Flagged: 5, Preview: "The proposed platform separates identity from ballot content."},
			{Index: 1, Words: 80, Sentences: 5, AI: 22, Flagged: 0, Preview: "The evaluation uses simulated voters."},
		},
		Methodology: []string{"Prose only.", "Sentence-level stylometry."},
		References:  []aidetect.Reference{{Title: "DetectGPT", Authors: "Mitchell et al.", Year: "2023", URL: "https://arxiv.org/abs/2301.11305"}},
	}
}

func TestDetectorViewShowsBigPercentagesAndAction(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{})})
	m.width = 110
	m.height = 40
	m.detector = &detectorState{report: sampleDetectorReport("/tmp/report.docx"), returnTo: screenJobs}
	m.screen = screenDetector
	view := m.detectorView(34)
	for _, want := range []string{"HUMAN", "LIKELY SOURCE", "Humanize this text", "Full report", "63%"} {
		if !strings.Contains(view, want) {
			t.Errorf("detector view missing %q:\n%s", want, view)
		}
	}
	body := strings.Join(m.detectorBody(72, false), "\n")
	for _, want := range []string{"WHY THIS SCORE", "SENTENCE SCORE DISTRIBUTION", "FLAGGED SENTENCES"} {
		if !strings.Contains(body, want) {
			t.Errorf("detector body missing %q:\n%s", want, body)
		}
	}
}

func TestDetectorHumanizePrefillsCompose(t *testing.T) {
	src := filepath.Join(t.TempDir(), "sample.docx")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{})})
	m.detector = &detectorState{report: sampleDetectorReport(src), returnTo: screenJobs}
	m.screen = screenDetector
	next, _ := m.handleDetectorKey(tea.KeyPressMsg{Code: 'h'})
	got := next.(App)
	if got.screen != screenCompose || got.compose == nil {
		t.Fatalf("screen = %v, compose = %v", got.screen, got.compose)
	}
	if len(got.compose.steps) != 1 || got.compose.steps[0].spec.ID != "doc.humanize" {
		t.Fatalf("steps = %+v", got.compose.steps)
	}
	if got.compose.input != src {
		t.Fatalf("input = %q, want %q", got.compose.input, src)
	}
}

func TestDetectorAutoOpensFromJobDone(t *testing.T) {
	report := sampleDetectorReport("/tmp/source.docx")
	path := filepath.Join(t.TempDir(), "report.json")
	if err := aidetect.WriteJSON(report, path); err != nil {
		t.Fatal(err)
	}
	m := sized(t, Options{Config: config.Defaults()})
	m.screen = screenJobs
	m.jobs = []runner.JobView{{
		ID:     "job-1",
		Name:   "Detect AI",
		Status: core.StatusDone,
		Items: []runner.ItemView{{
			Status: core.ItemStatusDone,
			Outputs: []core.Artifact{{
				Format: "json",
				Path:   path,
				Meta:   map[string]any{"kind": "ai-detection-json"},
			}},
		}},
	}}
	m.onJobDone(core.JobDone{JobID: "job-1", Status: core.StatusDone})
	if m.screen != screenDetector || m.detector == nil {
		t.Fatalf("screen = %v, detector = %v", m.screen, m.detector)
	}
	if m.detector.report.Summary.AI != 63 {
		t.Fatalf("report AI = %.0f", m.detector.report.Summary.AI)
	}
}

func TestDetectorCompactViewLeadsWithHeadline(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{})})
	m.detector = &detectorState{report: sampleDetectorReport("/tmp/report.docx"), returnTo: screenJobs}
	m.screen = screenDetector
	view := m.detectorView(20)
	for _, want := range []string{"AI", "63%", "HUMAN", "WHY THIS SCORE", "FLAGGED SENTENCES"} {
		if !strings.Contains(view, want) {
			t.Errorf("compact detector view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "SENTENCE SCORE DISTRIBUTION") {
		t.Errorf("compact view should defer the histogram to the full report:\n%s", view)
	}
}

func TestBigScoreUsesFiveRowBlockFace(t *testing.T) {
	lines := bigScore(87, lipgloss.NewStyle())
	if len(lines) != 5 {
		t.Fatalf("lines = %d, want 5", len(lines))
	}
	if !strings.Contains(lines[0], "█") {
		t.Fatalf("score has no block glyphs: %q", lines[0])
	}
	if !strings.Contains(lines[0], "%") {
		t.Fatalf("score is missing the percent unit: %q", lines[0])
	}
}

func TestDetectorWideViewShowsBothScores(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{})})
	m.width = 110
	m.height = 40
	m.detector = &detectorState{report: sampleDetectorReport("/tmp/report.docx"), returnTo: screenJobs}
	m.screen = screenDetector
	view := m.detectorView(34)
	for _, want := range []string{"HUMAN", "Likely AI-assisted", "LIKELY SOURCE"} {
		if !strings.Contains(view, want) {
			t.Errorf("wide detector view missing %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "█") {
		t.Errorf("wide detector view has no large score blocks:\n%s", view)
	}
}

func TestDetectorViewUsesBrailleMeters(t *testing.T) {
	for _, width := range []int{80, 110} {
		m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{})})
		m.width = width
		m.detector = &detectorState{report: sampleDetectorReport("/tmp/report.docx"), returnTo: screenJobs}
		m.screen = screenDetector
		view := m.detectorView(30)
		if !strings.Contains(view, "⣿") {
			t.Errorf("width %d: detector view has no braille meter:\n%s", width, view)
		}
		if strings.Contains(view, "▓") || strings.Contains(view, "░") {
			t.Errorf("width %d: detector view still uses dither glyphs:\n%s", width, view)
		}
	}
}

func TestDetectorHistogramFitsWidth(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	buckets := [10]int{1, 2, 3, 4, 5, 4, 3, 2, 1, 1}
	for _, width := range []int{22, 30, 49, 72, 102} {
		for _, line := range m.sentenceHistogram(buckets, width) {
			if got := lipgloss.Width(line); got > width {
				t.Errorf("histogram line width %d exceeds %d at width %d: %q", got, width, width, line)
			}
		}
	}
}

func TestDetectorViewFitsMinimumTerminal(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{})})
	m.detector = &detectorState{report: sampleDetectorReport("/tmp/report.docx"), returnTo: screenJobs}
	m.screen = screenDetector
	view := m.detectorView(18)
	for _, want := range []string{"HUMAN", "Humanize this text"} {
		if !strings.Contains(view, want) {
			t.Errorf("minimum-size detector view missing %q:\n%s", want, view)
		}
	}
}

func TestDetectorScreenRendersInShell(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{})})
	m.detector = &detectorState{report: sampleDetectorReport("/tmp/report.docx"), returnTo: screenJobs}
	m.screen = screenDetector
	view := m.View().Content
	for _, want := range []string{"AI detection", "Humanize this text"} {
		if !strings.Contains(view, want) {
			t.Errorf("shell view missing %q:\n%s", want, view)
		}
	}
}

func TestDetectorHumanizePDFBuildsConvertChain(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{}, docops.Convert{})})
	report := sampleDetectorReport("/tmp/report.pdf")
	report.Source.Format = "pdf"
	m.detector = &detectorState{report: report, returnTo: screenJobs}
	m.screen = screenDetector
	next, _ := m.handleDetectorKey(tea.KeyPressMsg{Code: 'h'})
	got := next.(App)
	if got.screen != screenCompose || got.compose == nil {
		t.Fatal("compose did not open")
	}
	if len(got.compose.steps) != 2 || got.compose.steps[0].spec.ID != "doc.convert" || got.compose.steps[1].spec.ID != "doc.humanize" {
		t.Fatalf("steps = %+v", got.compose.steps)
	}
}

func TestDetectorFullReportIsStyled(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{})})
	m.width = 110
	m.height = 40
	m.detector = &detectorState{report: sampleDetectorReport("/tmp/report.docx"), returnTo: screenJobs}
	m.screen = screenDetector
	next, _ := m.handleDetectorKey(tea.KeyPressMsg{Code: 'o'})
	got := next.(App)
	if got.screen != screenDetectorReport || got.detectorReport == nil {
		t.Fatalf("screen = %v, report = %v", got.screen, got.detectorReport)
	}
	body := strings.Join(got.detectorReportBody(100), "\n")
	for _, want := range []string{"LIKELY SOURCE", "SENTENCE SCORE DISTRIBUTION", "PARAGRAPHS", "HOW THIS WAS SCORED", "REFERENCES"} {
		if !strings.Contains(body, want) {
			t.Errorf("full report body missing %q:\n%s", want, body)
		}
	}
	view := got.detectorReportView(34)
	for _, want := range []string{"AI report", "Humanize this text"} {
		if !strings.Contains(view, want) {
			t.Errorf("full report view missing %q:\n%s", want, view)
		}
	}
}

func TestDetectorWorksAtSmallTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {40, 12}, {60, 18}} {
		m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{})})
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(App)
		m.detector = &detectorState{report: sampleDetectorReport("/tmp/report.docx"), returnTo: screenJobs}
		m.screen = screenDetector
		view := m.View().Content
		if strings.Contains(view, "Terminal too small") {
			t.Errorf("%dx%d still shows the too-small error:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "AI detection") {
			t.Errorf("%dx%d detector view missing title:\n%s", size[0], size[1], view)
		}
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(App)
		if m.screen != screenDetectorReport {
			t.Errorf("%dx%d: enter did not open the report (screen %v)", size[0], size[1], m.screen)
		}
		if strings.Contains(m.View().Content, "Terminal too small") {
			t.Errorf("%dx%d report view shows the too-small error", size[0], size[1])
		}
	}
}

func TestDetectorEnterOpensFullReport(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{})})
	m.detector = &detectorState{report: sampleDetectorReport("/tmp/report.docx"), returnTo: screenJobs}
	m.screen = screenDetector
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := next.(App)
	if got.screen != screenDetectorReport || got.detectorReport == nil {
		t.Fatalf("screen = %v, report = %v", got.screen, got.detectorReport)
	}
}

func TestDetectorFullReportReturnsToDashboard(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	m.detector = &detectorState{report: sampleDetectorReport("/tmp/report.docx"), returnTo: screenJobs}
	m.detectorReport = &detectorReportState{report: m.detector.report}
	m.screen = screenDetectorReport
	next, _ := m.handleDetectorReportKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := next.(App)
	if got.screen != screenDetector || got.detectorReport != nil {
		t.Fatalf("screen = %v, report = %v", got.screen, got.detectorReport)
	}
}

func TestDetectorEnterOpensReportAndHHumanizes(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(detectops.Detect{}, &humanizeops.Humanize{})})
	m.detector = &detectorState{report: sampleDetectorReport("/tmp/report.docx"), returnTo: screenJobs}
	m.screen = screenDetector
	next, _ := m.handleDetectorKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := next.(App)
	if got.screen != screenDetectorReport || got.detectorReport == nil {
		t.Fatalf("enter opened screen %v, want the report", got.screen)
	}
	next, _ = got.handleDetectorReportKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if next.(App).screen != screenCompose {
		t.Fatal("h did not humanize from the report")
	}
}
