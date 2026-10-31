package doc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/opencode"
)

type fakeTextGenerator struct {
	models []opencode.ModelRef
	fn     func(string) (string, error)

	mu    sync.Mutex
	calls []string
}

func (f *fakeTextGenerator) FreeModels(context.Context) ([]opencode.ModelRef, error) {
	return f.models, nil
}

func (f *fakeTextGenerator) Generate(_ context.Context, _ opencode.ModelRef, prompt string) (string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, prompt)
	f.mu.Unlock()
	return f.fn(prompt)
}

func transformInput(t *testing.T, dir, name, body string) core.Artifact {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	return art
}

func TestTranslatePreservesCodeAndLinks(t *testing.T) {
	dir := t.TempDir()
	input := transformInput(t, dir, "note.md", "Hello world\n\n```go\nfmt.Println(\"hi\")\n```\n\nSee https://example.com/docs.\n")
	gen := &fakeTextGenerator{
		models: []opencode.ModelRef{{ProviderID: "test", ID: "model"}},
		fn: func(prompt string) (string, error) {
			const marker = "Return only the translation, with no commentary.\n\n"
			payload := prompt
			if index := strings.Index(prompt, marker); index >= 0 {
				payload = prompt[index+len(marker):]
			}
			return "Hola.\n\n" + payload, nil
		},
	}
	out := t.TempDir()
	result, err := (Translate{Client: gen}).Run(context.Background(), core.Request{
		Items: []core.Artifact{input},
		Params: core.Values{
			"language": "Spanish", "chunk_words": 200, "workers": 1, "model": "auto",
		},
		Target: core.Target{Dir: out},
	}, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	if got, want := filepath.Base(result.Artifacts[0].Path), "note.translated.md"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"Hola.", "```go", "fmt.Println", "https://example.com/docs"} {
		if !strings.Contains(text, want) {
			t.Errorf("translation missing %q:\n%s", want, text)
		}
	}
	if result.Artifacts[0].Meta["language"] != "Spanish" {
		t.Errorf("meta = %+v", result.Artifacts[0].Meta)
	}
	if len(gen.calls) != 1 || !strings.Contains(gen.calls[0], "Spanish") {
		t.Errorf("prompt = %q", gen.calls)
	}
}

func TestSummarizeCombinesSections(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("Alpha beta gamma delta epsilon zeta eta theta. ", 18) + "\n\n" +
		strings.Repeat("Iota kappa lambda mu nu xi omicron pi. ", 18)
	input := transformInput(t, dir, "long.md", body)
	gen := &fakeTextGenerator{
		models: []opencode.ModelRef{{ProviderID: "test", ID: "model"}},
		fn: func(prompt string) (string, error) {
			if strings.Contains(prompt, "Combine these section summaries") {
				return "FINAL SUMMARY", nil
			}
			return "Section summary.", nil
		},
	}
	out := t.TempDir()
	result, err := (Summarize{Client: gen}).Run(context.Background(), core.Request{
		Items: []core.Artifact{input},
		Params: core.Values{
			"length": "short", "style": "bullets", "chunk_words": 20, "workers": 2, "model": "auto",
		},
		Target: core.Target{Dir: out},
	}, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "FINAL SUMMARY") {
		t.Fatalf("summary = %q", data)
	}
	if !strings.HasSuffix(result.Artifacts[0].Path, ".summary.md") {
		t.Errorf("output = %q", result.Artifacts[0].Path)
	}
	if len(gen.calls) < 3 {
		t.Errorf("calls = %d, want at least one map call per chunk plus combine", len(gen.calls))
	}
}

func TestRewriteFailsWhenProtectedTokenIsDropped(t *testing.T) {
	dir := t.TempDir()
	input := transformInput(t, dir, "code.md", "Before\n\n```go\nfmt.Println(\"hi\")\n```\n\nAfter\n")
	gen := &fakeTextGenerator{
		models: []opencode.ModelRef{{ProviderID: "test", ID: "model"}},
		fn: func(string) (string, error) {
			return "rewritten without the code block", nil
		},
	}
	_, err := (Rewrite{Client: gen}).Run(context.Background(), core.Request{
		Items: []core.Artifact{input},
		Params: core.Values{
			"instruction": "Tighten the prose.", "chunk_words": 200, "workers": 1, "model": "auto",
		},
		Target: core.Target{Dir: t.TempDir()},
	}, nopReporter{})
	if err == nil {
		t.Fatal("dropping a protected block succeeded")
	}
}

func TestTranslateRejectsNonTextInput(t *testing.T) {
	err := (Translate{Client: &fakeTextGenerator{}}).Validate(context.Background(), core.Request{
		Items:  []core.Artifact{{Path: "photo.png", Kind: core.Image, Format: "png"}},
		Params: core.Values{"language": "French"},
		Target: core.Target{Dir: t.TempDir()},
	})
	if err == nil {
		t.Fatal("image passed validation")
	}
}
