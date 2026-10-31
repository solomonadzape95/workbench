package detect

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
)

// LoadReport reads a detector JSON sidecar.
func LoadReport(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Report{}, err
	}
	if report.Version == "" {
		return Report{}, fmt.Errorf("not a detector report")
	}
	return report, nil
}

// WriteJSON writes the machine-readable report.
func WriteJSON(report Report, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// RenderMarkdown builds the human-readable report. The UI has its own richer
// screen; this file is for the output folder, preview, and agents.
func RenderMarkdown(report Report) string {
	var b strings.Builder
	summary := report.Summary
	aiDisplay := math.Round(summary.AI)
	humanDisplay := 100 - aiDisplay
	fmt.Fprintf(&b, "# AI detection report: %s\n\n", report.Source.Name)
	fmt.Fprintf(&b, "## AI %.0f%%  ·  Human %.0f%%\n\n", aiDisplay, humanDisplay)
	fmt.Fprintf(&b, "**Verdict:** %s  \n", summary.Verdict)
	fmt.Fprintf(&b, "**Confidence:** %s (%s)  \n", titleCase(summary.Confidence), reliabilityText(summary.Reliability))
	fmt.Fprintf(&b, "**Likely source:** %s  \n", summary.AttributionTop)
	if summary.AttributionNote != "" {
		fmt.Fprintf(&b, "**Attribution note:** %s  \n", summary.AttributionNote)
	}
	fmt.Fprintf(&b, "**Text:** %s · %d words · %d prose sentences · %d paragraphs  \n", report.Source.Format, summary.Words, summary.Sentences, summary.Paragraphs)
	if summary.Profile != "" {
		fmt.Fprintf(&b, "**Profile:** %s  \n", summary.Profile)
	}
	fmt.Fprintf(&b, "**Flagged sentences:** %d (%.0f%% of prose)\n\n", summary.Flagged, summary.SentenceAI)

	if len(report.Warnings) > 0 {
		b.WriteString("> **Read this first**\n>\n")
		for _, warning := range report.Warnings {
			fmt.Fprintf(&b, "> - %s\n", warning)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Likely source\n\n")
	b.WriteString("| Family | Fingerprint strength | Confidence | Evidence |\n")
	b.WriteString("|---|---:|---|---|\n")
	for _, attribution := range report.Attribution {
		fmt.Fprintf(&b, "| %s | %.0f/100 | %s | %s |\n",
			escapeCell(attribution.Label), attribution.Score, titleCase(attribution.Confidence),
			escapeCell(strings.Join(attribution.Evidence, ", ")))
	}
	b.WriteString("\nModel attribution is a best-effort phrase/style fingerprint, not forensic proof. Models imitate each other, and humanizers erase many signals.\n\n")

	b.WriteString("## Signals\n\n")
	b.WriteString("Signals are sorted by how much they pull the score toward AI. Raw values are shown so you can judge them yourself.\n\n")
	b.WriteString("| Signal | Raw value | AI-likeness | Weight | What it means |\n")
	b.WriteString("|---|---:|---:|---:|---|\n")
	for _, signal := range report.Signals {
		fmt.Fprintf(&b, "| %s | %s | %.0f/100 | %.0f%% | %s |\n",
			escapeCell(signal.Label), escapeCell(signal.Display), signal.AIScore, signal.Weight*100,
			escapeCell(signal.Detail))
	}
	b.WriteString("\n")

	b.WriteString("## Flagged sentences\n\n")
	if len(report.Flagged) == 0 {
		b.WriteString("No individual sentence crossed the AI threshold.\n\n")
	} else {
		limit := len(report.Flagged)
		if limit > 10 {
			limit = 10
		}
		for i := 0; i < limit; i++ {
			sentence := report.Flagged[i]
			fmt.Fprintf(&b, "**%.0f%% · paragraph %d**  \n%s  \n_%s_\n\n",
				sentence.AI, sentence.Paragraph+1, escapeMarkdown(sentence.Text),
				escapeMarkdown(strings.Join(sentence.Reasons, "; ")))
		}
	}

	if len(report.Paragraphs) > 0 {
		b.WriteString("## Paragraphs\n\n")
		b.WriteString("| Paragraph | AI-like | Flagged sentences | Words | Preview |\n")
		b.WriteString("|---:|---:|---:|---:|---|\n")
		// Show the strongest paragraphs, not the first ten in the document.
		paras := append([]Paragraph(nil), report.Paragraphs...)
		sortParagraphs(paras)
		limit := len(paras)
		if limit > 10 {
			limit = 10
		}
		for i := 0; i < limit; i++ {
			para := paras[i]
			fmt.Fprintf(&b, "| %d | %.0f%% | %d/%d | %d | %s |\n",
				para.Index+1, para.AI, para.Flagged, para.Sentences, para.Words,
				escapeCell(para.Preview))
		}
		b.WriteString("\n")
	}

	b.WriteString("## How this was scored\n\n")
	for _, line := range report.Methodology {
		fmt.Fprintf(&b, "- %s\n", line)
	}
	b.WriteString("\n")

	b.WriteString("## Limitations\n\n")
	b.WriteString("- No detector can prove authorship. This is a diagnostic, not a gate.\n")
	b.WriteString("- False positives are more likely on formulaic human writing, non-native English, legal or academic boilerplate, and heavily edited text.\n")
	b.WriteString("- Paraphrasing, translation, and humanizing tools can reduce or erase the signals.\n")
	b.WriteString("- Short texts are much less reliable. Treat a score below 300 words as a weak signal.\n")
	b.WriteString("- Model attribution is a phrase/style fingerprint. It cannot identify an exact model or version.\n\n")

	b.WriteString("## References\n\n")
	for _, reference := range report.References {
		fmt.Fprintf(&b, "- %s. %s (%s). %s\n", reference.Authors, reference.Title, reference.Year, reference.URL)
	}
	b.WriteString("\n---\n")
	fmt.Fprintf(&b, "Generated by Workbench AI detector v%s. Score is diagnostic, not proof.\n", report.Version)
	return b.String()
}

// SummaryText is a compact summary for clipboard and job metadata.
func SummaryText(report Report) string {
	aiDisplay := math.Round(report.Summary.AI)
	humanDisplay := 100 - aiDisplay
	return fmt.Sprintf("AI %.0f%% · Human %.0f%%\nVerdict: %s\nConfidence: %s\nLikely source: %s\nFile: %s",
		aiDisplay, humanDisplay, report.Summary.Verdict,
		titleCase(report.Summary.Confidence), report.Summary.AttributionTop, report.Source.Path)
}

func reliabilityText(reliability float64) string {
	switch {
	case reliability >= 70:
		return "high reliability"
	case reliability >= 40:
		return "medium reliability"
	default:
		return "low reliability"
	}
}

func titleCase(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}

func escapeCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func escapeMarkdown(value string) string {
	return strings.ReplaceAll(value, "\n", " ")
}

func sortParagraphs(paras []Paragraph) {
	for i := 1; i < len(paras); i++ {
		for j := i; j > 0 && paras[j].AI > paras[j-1].AI; j-- {
			paras[j], paras[j-1] = paras[j-1], paras[j]
		}
	}
}
