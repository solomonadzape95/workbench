package detect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The calibration corpus is deliberately small and plain: one file is one
// document, paragraphs are separated by blank lines, and manifest.json records
// the label, genre, and first language. It is a starter set, not a claim of
// statistical representativeness. Add cases rather than tuning this file to a
// single model family.
type calibrationEntry struct {
	File          string `json:"file"`
	Label         string `json:"label"` // human, ai, humanized
	Genre         string `json:"genre"`
	FirstLanguage string `json:"first_language"`
	PairedWith    string `json:"paired_with,omitempty"`
}

type calibrationManifest struct {
	Version int                `json:"version"`
	Notes   string             `json:"notes"`
	Entries []calibrationEntry `json:"entries"`
}

type calibrationResult struct {
	calibrationEntry
	AI         float64
	Verdict    string
	Confidence string
}

type calibrationMetrics struct {
	human, ai, humanized []calibrationResult
	falsePositive        int
	unsure               int
	detected             int
	byGenre              map[string][2]int
	byLanguage           map[string][2]int
}

func loadCalibration(t *testing.T) []calibrationResult {
	t.Helper()
	dir := filepath.Join("testdata", "calibration")
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	var manifest calibrationManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if len(manifest.Entries) == 0 {
		t.Fatal("calibration manifest has no entries")
	}

	results := make([]calibrationResult, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		text, err := os.ReadFile(filepath.Join(dir, entry.File))
		if err != nil {
			t.Fatalf("%s: %v", entry.File, err)
		}
		report, err := Analyze(calibrationDocument(entry.File, string(text)), DefaultOptions())
		if err != nil {
			t.Fatalf("%s: %v", entry.File, err)
		}
		results = append(results, calibrationResult{
			calibrationEntry: entry,
			AI:               report.Summary.AI,
			Verdict:          report.Summary.Verdict,
			Confidence:       report.Summary.Confidence,
		})
	}
	return results
}

func calibrationDocument(name, text string) Document {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var paragraphs []ParagraphInput
	for _, block := range strings.Split(text, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		paragraphs = append(paragraphs, ParagraphInput{Index: len(paragraphs), Text: block})
	}
	return Document{
		Source:     Source{Path: name, Name: name, Format: "txt"},
		Paragraphs: paragraphs,
	}
}

func measureCalibration(results []calibrationResult) calibrationMetrics {
	m := calibrationMetrics{
		byGenre:    map[string][2]int{},
		byLanguage: map[string][2]int{},
	}
	for _, result := range results {
		switch result.Label {
		case "human":
			m.human = append(m.human, result)
		case "ai":
			m.ai = append(m.ai, result)
			if result.AI >= 60 {
				m.detected++
			}
		case "humanized":
			m.humanized = append(m.humanized, result)
		}
	}
	for _, result := range m.human {
		genre := m.byGenre[result.Genre]
		genre[1]++
		language := m.byLanguage[result.FirstLanguage]
		language[1]++
		switch {
		case result.AI >= 60:
			m.falsePositive++
			genre[0]++
			language[0]++
		case result.AI >= 30:
			m.unsure++
		}
		m.byGenre[result.Genre] = genre
		m.byLanguage[result.FirstLanguage] = language
	}
	return m
}

// TestCalibrationReport is the held-out measurement pass. It prints a table on
// every run (`go test ./internal/detect -run TestCalibrationReport -v`) and
// fails only on broad separation regressions, so weight changes are visible
// without making a tiny starter corpus a brittle gate.
func TestCalibrationReport(t *testing.T) {
	results := loadCalibration(t)
	m := measureCalibration(results)
	if len(m.human) == 0 || len(m.ai) == 0 {
		t.Fatalf("need at least one human and one ai sample, got %d/%d", len(m.human), len(m.ai))
	}

	t.Logf("calibration: %d entries (%d human, %d ai, %d humanized)",
		len(results), len(m.human), len(m.ai), len(m.humanized))
	t.Logf("%-26s %-9s %-9s %-4s %6s  %s", "file", "label", "genre", "L1", "AI%", "verdict")
	for _, result := range results {
		t.Logf("%-26s %-9s %-9s %-4s %6.0f  %s (%s)",
			result.File, result.Label, result.Genre, result.FirstLanguage,
			result.AI, result.Verdict, result.Confidence)
	}

	t.Logf("human false positives (AI >= 60): %d/%d (%.0f%%); unsure (30-59): %d",
		m.falsePositive, len(m.human), percent(m.falsePositive, len(m.human)), m.unsure)
	for _, genre := range sortedKeys(m.byGenre) {
		cell := m.byGenre[genre]
		t.Logf("  genre %-10s %d/%d (%.0f%%)", genre, cell[0], cell[1], percent(cell[0], cell[1]))
	}
	for _, language := range sortedKeys(m.byLanguage) {
		cell := m.byLanguage[language]
		t.Logf("  first language %-4s %d/%d (%.0f%%)", language, cell[0], cell[1], percent(cell[0], cell[1]))
	}
	t.Logf("ai detection rate (AI >= 60): %d/%d (%.0f%%)",
		m.detected, len(m.ai), percent(m.detected, len(m.ai)))

	// The broad check: the human mean must stay well below the AI mean. This
	// catches an inverted or collapsed weighting change without overfitting a
	// seven-document starter set.
	humanMean := meanAI(m.human)
	aiMean := meanAI(m.ai)
	t.Logf("mean AI: human %.1f, ai %.1f, separation %.1f", humanMean, aiMean, aiMean-humanMean)
	if humanMean >= aiMean {
		t.Fatalf("calibration collapsed: human mean %.1f is not below ai mean %.1f", humanMean, aiMean)
	}

	// Humanized output must move toward the human side. The pair is explicit in
	// the manifest so the check does not guess which file was rewritten.
	byFile := make(map[string]calibrationResult, len(results))
	for _, result := range results {
		byFile[result.File] = result
	}
	for _, result := range m.humanized {
		source, ok := byFile[result.PairedWith]
		if !ok {
			t.Fatalf("humanized %s points at missing source %q", result.File, result.PairedWith)
		}
		t.Logf("humanized %s: %.0f -> %.0f", result.File, source.AI, result.AI)
		if result.AI >= source.AI {
			t.Errorf("humanized %s scored %.0f, not lower than source %s at %.0f",
				result.File, result.AI, source.File, source.AI)
		}
	}

	if os.Getenv("WORKBENCH_WRITE_CALIBRATION") == "1" {
		writeCalibrationReport(t, results, m)
	}
}

func writeCalibrationReport(t *testing.T, results []calibrationResult, m calibrationMetrics) {
	t.Helper()
	var b strings.Builder
	b.WriteString("# AI detector calibration report\n\n")
	b.WriteString("Generated by `WORKBENCH_WRITE_CALIBRATION=1 go test ./internal/detect -run TestCalibrationReport -v`.\n\n")
	b.WriteString("| File | Label | Genre | L1 | AI % | Verdict |\n")
	b.WriteString("|---|---|---|---|---:|---|\n")
	for _, result := range results {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %.0f | %s |\n",
			result.File, result.Label, result.Genre, result.FirstLanguage,
			result.AI, result.Verdict)
	}

	fmt.Fprintf(&b, "\n## Summary\n\n")
	fmt.Fprintf(&b, "- Human mean: %.1f\n", meanAI(m.human))
	fmt.Fprintf(&b, "- AI mean: %.1f\n", meanAI(m.ai))
	fmt.Fprintf(&b, "- Separation: %.1f\n", meanAI(m.ai)-meanAI(m.human))
	fmt.Fprintf(&b, "- Human false positives (AI >= 60): %d/%d (%.0f%%)\n",
		m.falsePositive, len(m.human), percent(m.falsePositive, len(m.human)))
	fmt.Fprintf(&b, "- Human unsure (30–59): %d\n", m.unsure)
	fmt.Fprintf(&b, "- AI detection rate (AI >= 60): %d/%d (%.0f%%)\n",
		m.detected, len(m.ai), percent(m.detected, len(m.ai)))

	fmt.Fprintf(&b, "\n## False positives by genre\n\n")
	for _, genre := range sortedKeys(m.byGenre) {
		cell := m.byGenre[genre]
		fmt.Fprintf(&b, "- %s: %d/%d (%.0f%%)\n", genre, cell[0], cell[1], percent(cell[0], cell[1]))
	}
	fmt.Fprintf(&b, "\n## False positives by first language\n\n")
	for _, language := range sortedKeys(m.byLanguage) {
		cell := m.byLanguage[language]
		fmt.Fprintf(&b, "- %s: %d/%d (%.0f%%)\n", language, cell[0], cell[1], percent(cell[0], cell[1]))
	}
	fmt.Fprintf(&b, "\n## Humanized pairs\n\n")
	for _, result := range m.humanized {
		fmt.Fprintf(&b, "- %s: paired with `%s`\n", result.File, result.PairedWith)
	}

	path := filepath.Join("testdata", "calibration", "report.md")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Logf("wrote %s", path)
}

func meanAI(results []calibrationResult) float64 {
	if len(results) == 0 {
		return 0
	}
	var total float64
	for _, result := range results {
		total += result.AI
	}
	return total / float64(len(results))
}

func sortedKeys(cells map[string][2]int) []string {
	keys := make([]string, 0, len(cells))
	for key := range cells {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func percent(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}
