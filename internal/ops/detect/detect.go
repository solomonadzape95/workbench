// Package detect exposes the explainable AI-writing detector as a Workbench
// operation. It is intentionally separate from the detector engine so the TUI
// and agents can reuse the report types without depending on the operation.
package detectops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/detect"
	"github.com/solomonadzape95/workbench/internal/engines"
)

var spec = core.Spec{
	ID:      "doc.detect_ai",
	Title:   "Detect AI writing",
	Summary: "Estimate how much prose looks AI-written and which model family likely left fingerprints",
	Domain:  core.Document,
	Accepts: []core.Accept{{Kind: core.Document, Formats: []string{
		"txt", "md", "markdown", "docx", "pdf",
	}}},
	Params: []core.ParamSpec{
		{
			Key: "threshold", Label: "Sentence threshold", Type: core.ParamFloat, Default: 0.55,
			Min: floatPtr(0.35), Max: floatPtr(0.85), Advanced: true,
			Help: "A sentence is flagged when its AI-likeness crosses this value. Lower values flag more text.",
		},
		{
			Key: "min_words", Label: "Minimum sentence words", Type: core.ParamInt, Default: 5,
			Min: floatPtr(3), Max: floatPtr(20), Advanced: true,
			Help: "Very short fragments are ignored. Five words is a good default for prose.",
		},
		{
			Key: "json", Label: "Write JSON sidecar", Type: core.ParamBool, Default: true, Advanced: true,
			Help: "Writes a machine-readable report beside the Markdown report.",
		},
	},
	Output:    core.OutputSpec{Kind: core.Document},
	Batchable: true,
	Cost:      core.CostFast,
}

// Detect is the operation. LogProbCommand is an optional external scorer that
// receives document text on stdin and prints an AI-likelihood score.
type Detect struct {
	LogProbCommand string
	LogProbWeight  float64
}

func (Detect) Spec() core.Spec { return spec }

func (Detect) Validate(_ context.Context, req core.Request) error {
	if err := core.Validate(spec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, spec.ID, "", "no input files", "")
	}
	for _, item := range req.Items {
		if !spec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, spec.ID, item.Path,
				fmt.Sprintf("%s is not a supported text document", item.Format),
				"supported inputs: .txt, .md, .markdown, .docx, or .pdf")
		}
		if strings.EqualFold(item.Ext(), "pdf") {
			if _, err := os.Stat(item.Path); err != nil {
				return core.Wrap(core.CodeInput, spec.ID, item.Path, err, "check that the file exists")
			}
		}
	}
	return nil
}

func (d Detect) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	started := time.Now()
	threshold := req.Params.Float("threshold")
	if threshold <= 0 {
		threshold = 0.55
	}
	minWords := req.Params.Int("min_words")
	if minWords <= 0 {
		minWords = 5
	}
	writeJSON := req.Params.Bool("json")

	opts := detect.Options{SentenceThreshold: threshold, MinWords: minWords}
	var artifacts []core.Artifact
	var bytesIn, bytesOut int64
	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		rep.Stage("reading " + item.Name())
		doc, err := detect.Extract(ctx, item.Path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, spec.ID, item.Path, err,
				"use TXT, Markdown, DOCX, or a PDF with selectable text; scanned PDFs need OCR")
		}
		if d.LogProbCommand != "" {
			text := documentText(doc)
			score, detail, logErr := runLogProb(ctx, d.LogProbCommand, text)
			if logErr != nil {
				rep.Log(core.LevelWarn, "%s: log-probability scorer skipped: %v", item.Name(), logErr)
			} else {
				opts.LogProbScore = &score
				opts.LogProbWeight = d.LogProbWeight
				opts.LogProbDetail = detail
			}
		}
		rep.Stage("scoring " + item.Name())
		report, err := detect.Analyze(doc, opts)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, spec.ID, item.Path, err,
				"the document has no prose sentences to score")
		}
		if report.Source.Name == "" {
			report.Source = doc.Source
		}

		rep.Stage("writing report")
		stem := core.SanitizeStem(item.Stem())
		mdPath := filepath.Join(req.Target.Dir, stem+".ai-detection.md")
		jsonPath := filepath.Join(req.Target.Dir, stem+".ai-detection.json")
		if err := os.WriteFile(mdPath, []byte(detect.RenderMarkdown(report)), 0o644); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, spec.ID, mdPath, err,
				"check permissions on the output directory")
		}
		mdInfo, err := os.Stat(mdPath)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, spec.ID, mdPath, err, "")
		}
		bytesIn += item.Size
		bytesOut += mdInfo.Size()

		meta := map[string]any{
			"kind":                "ai-detection",
			"source":              item.Path,
			"ai_percent":          report.Summary.AI,
			"human_percent":       report.Summary.Human,
			"verdict":             report.Summary.Verdict,
			"confidence":          report.Summary.Confidence,
			"reliability":         report.Summary.Reliability,
			"words":               report.Summary.Words,
			"sentences":           report.Summary.Sentences,
			"flagged_sentences":   report.Summary.Flagged,
			"ai_sentence_percent": report.Summary.SentenceAI,
			"profile":             report.Summary.Profile,
			"top_model":           report.Summary.AttributionTop,
		}
		artifacts = append(artifacts, core.Artifact{
			Path:          mdPath,
			Kind:          core.Document,
			Format:        "md",
			Size:          mdInfo.Size(),
			SuggestedName: filepath.Base(mdPath),
			Meta:          meta,
		})

		if writeJSON {
			if err := detect.WriteJSON(report, jsonPath); err != nil {
				return core.Result{}, core.Wrap(core.CodeOutput, spec.ID, jsonPath, err,
					"check permissions on the output directory")
			}
			jsonInfo, err := os.Stat(jsonPath)
			if err != nil {
				return core.Result{}, core.Wrap(core.CodeOutput, spec.ID, jsonPath, err, "")
			}
			bytesOut += jsonInfo.Size()
			artifacts = append(artifacts, core.Artifact{
				Path:          jsonPath,
				Kind:          core.Document,
				Format:        "json",
				Size:          jsonInfo.Size(),
				SuggestedName: filepath.Base(jsonPath),
				Meta:          map[string]any{"kind": "ai-detection-json"},
			})
		}
		rep.Log(core.LevelInfo, "%s: AI %.0f%% · human %.0f%% · %s · confidence %s · likely %s",
			item.Name(), report.Summary.AI, report.Summary.Human, report.Summary.Verdict,
			report.Summary.Confidence, report.Summary.AttributionTop)
		rep.Progress(1, 1)
	}

	return core.Result{
		Artifacts: artifacts,
		Metrics: core.Metrics{
			BytesIn:  bytesIn,
			BytesOut: bytesOut,
			Duration: time.Since(started),
			Engine:   engines.Internal,
		},
	}, nil
}

func floatPtr(value float64) *float64 { return &value }

func documentText(doc detect.Document) string {
	var b strings.Builder
	for _, paragraph := range doc.Paragraphs {
		if text := strings.TrimSpace(paragraph.Text); text != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(text)
		}
	}
	return b.String()
}

func runLogProb(ctx context.Context, command, text string) (float64, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Stdin = strings.NewReader(text)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return 0, "", ctx.Err()
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if len(detail) > 300 {
			detail = detail[:300] + "…"
		}
		return 0, "", fmt.Errorf("scorer failed: %v (%s)", err, detail)
	}
	raw := strings.TrimSpace(stdout.String())
	score, err := parseLogProbScore(raw)
	if err != nil {
		return 0, "", err
	}
	return score, "external local log-probability scorer", nil
}

func parseLogProbScore(text string) (float64, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err == nil {
		for _, key := range []string{"score", "ai", "logprob", "ai_probability"} {
			if value, ok := payload[key]; ok {
				if f, ok := numeric(value); ok {
					return normalizeLogProb(f), nil
				}
			}
		}
		return 0, fmt.Errorf("scorer JSON has no score/ai/logprob field")
	}
	var f float64
	if _, err := fmt.Sscan(text, &f); err != nil {
		return 0, fmt.Errorf("scorer output is not a number or JSON score")
	}
	return normalizeLogProb(f), nil
}

func numeric(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case string:
		var f float64
		if _, err := fmt.Sscan(v, &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

func normalizeLogProb(value float64) float64 {
	if value > 1 {
		value /= 100
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
