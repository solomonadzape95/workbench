package detectops

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/detect"
)

type testReporter struct{}

func (testReporter) Stage(string)                   {}
func (testReporter) Progress(int, int)              {}
func (testReporter) Detail(core.Artifact, string)   {}
func (testReporter) Log(core.Level, string, ...any) {}

func TestDetectRunWritesReportAndJSON(t *testing.T) {
	src := filepath.Join(t.TempDir(), "sample.txt")
	text := `In today's fast-paced world, it is important to note that organizations must leverage robust strategies to facilitate seamless collaboration. Moreover, the multifaceted landscape of modern work requires a comprehensive approach to communication. Furthermore, teams that utilize these strategies can unlock significant potential and drive meaningful outcomes. In conclusion, by embracing these principles, organizations can elevate their performance and embark on a journey toward sustainable success.`
	if err := os.WriteFile(src, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	op := Detect{}
	params := spec.DefaultValues()
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: params,
		Target: core.Target{Dir: out},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, testReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("artifacts = %d, want markdown + json", len(result.Artifacts))
	}
	var jsonPath string
	for _, output := range result.Artifacts {
		if output.Format == "json" {
			jsonPath = output.Path
		}
		if output.Meta["kind"] == "ai-detection" {
			if got := output.Meta["ai_percent"]; got == nil {
				t.Fatal("markdown artifact has no ai_percent metadata")
			}
		}
	}
	if jsonPath == "" {
		t.Fatal("no JSON sidecar")
	}
	report, err := detect.LoadReport(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.AI < 70 {
		t.Fatalf("AI sample scored %.0f", report.Summary.AI)
	}
}

func TestDetectRejectsUnsupportedInput(t *testing.T) {
	op := Detect{}
	req := core.Request{
		Items:  []core.Artifact{{Path: "x.png", Kind: core.Image, Format: "png"}},
		Params: spec.DefaultValues(),
	}
	if err := op.Validate(context.Background(), req); err == nil {
		t.Fatal("unsupported input was accepted")
	}
}

func TestParseLogProbScore(t *testing.T) {
	cases := map[string]float64{
		"0.82":              0.82,
		"82":                0.82,
		`{"score":0.7}`:     0.7,
		`{"ai": 91}`:        0.91,
		`{"logprob":"0.4"}`: 0.4,
	}
	for input, want := range cases {
		got, err := parseLogProbScore(input)
		if err != nil {
			t.Fatalf("%s: %v", input, err)
		}
		if got != want {
			t.Errorf("%s = %v, want %v", input, got, want)
		}
	}
}
