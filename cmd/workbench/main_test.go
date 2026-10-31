package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
)

func captureStdout(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := fn()
	_ = w.Close()
	os.Stdout = old
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return code, string(data)
}

func isolateProfile(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("WORKBENCH_CONFIG_DIR", filepath.Join(root, "config"))
	t.Setenv("WORKBENCH_DATA_DIR", filepath.Join(root, "data"))
	t.Setenv("WORKBENCH_CACHE_DIR", filepath.Join(root, "cache"))
	t.Setenv("HOME", filepath.Join(root, "home"))
	return root
}

func TestParseParamValue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	enum := core.ParamSpec{
		Key: "format", Type: core.ParamEnum, Default: "zip",
		Options: []core.Option{{Value: "zip"}, {Value: "tgz"}, {Value: "zst"}},
	}
	tests := []struct {
		name string
		spec core.ParamSpec
		raw  string
		want any
		ok   bool
	}{
		{"int", core.ParamSpec{Type: core.ParamInt}, " 9 ", 9, true},
		{"float", core.ParamSpec{Type: core.ParamFloat}, "1.5", 1.5, true},
		{"bool", core.ParamSpec{Type: core.ParamBool}, "true", true, true},
		{"duration", core.ParamSpec{Type: core.ParamDuration}, "2s", 2 * time.Second, true},
		{"enum", enum, "tgz", "tgz", true},
		{"string", core.ParamSpec{Type: core.ParamString}, "hello", "hello", true},
		{"bad int", core.ParamSpec{Type: core.ParamInt}, "nope", nil, false},
		{"bad enum", enum, "rar", nil, false},
		{"bad duration", core.ParamSpec{Type: core.ParamDuration}, "soon", nil, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseParamValue(test.spec, test.raw)
			if (err == nil) != test.ok {
				t.Fatalf("err = %v, want ok=%v", err, test.ok)
			}
			if test.ok && got != test.want {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
		})
	}

	home := os.Getenv("HOME")
	got, err := parseParamValue(core.ParamSpec{Type: core.ParamPath}, "~/shots")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "shots"); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		raw  string
		want int64
		ok   bool
	}{
		{"", 0, true},
		{"10", 10, true},
		{"10k", 10 << 10, true},
		{"1.5kb", 1536, true},
		{"2mb", 2 << 20, true},
		{"1g", 1 << 30, true},
		{"nope", 0, false},
	}
	for _, test := range tests {
		got, err := parseSize(test.raw)
		if (err == nil) != test.ok {
			t.Fatalf("parseSize(%q) err = %v, want ok=%v", test.raw, err, test.ok)
		}
		if test.ok && got != test.want {
			t.Errorf("parseSize(%q) = %d, want %d", test.raw, got, test.want)
		}
	}
}

func TestCompletionScripts(t *testing.T) {
	for shell, want := range map[string]string{
		"bash": "complete -F _wb wb",
		"zsh":  "#compdef wb",
		"fish": "complete -c wb",
	} {
		code, out := captureStdout(t, func() int { return runCompletion([]string{shell}) })
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("%s completion = code %d, missing %q\n%s", shell, code, want, out)
		}
	}
}

func TestResolveCLIOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := config.Defaults()
	if got, want := resolveCLIOutput(cfg, "~/picked"), filepath.Join(home, "picked"); got != want {
		t.Fatalf("override = %q, want %q", got, want)
	}
	cfg.Output.Dir = "~/Workbench/{date}"
	if got, want := resolveCLIOutput(cfg, ""), filepath.Join(home, "Workbench", time.Now().Format("2006-01-02")); got != want {
		t.Fatalf("dated default = %q, want %q", got, want)
	}
	cfg.Output.Dir = ""
	if got := resolveCLIOutput(cfg, ""); !strings.Contains(got, filepath.Join("Workbench", time.Now().Format("2006-01-02"))) {
		t.Fatalf("built-in default = %q", got)
	}
}

func TestRunOpsJSON(t *testing.T) {
	isolateProfile(t)
	code, out := captureStdout(t, func() int { return runOps([]string{"--json"}) })
	if code != 0 {
		t.Fatalf("runOps exit = %d", code)
	}
	var ops []opJSON
	if err := json.Unmarshal([]byte(out), &ops); err != nil {
		t.Fatalf("decode ops: %v\n%s", err, out)
	}
	found := false
	for _, op := range ops {
		if op.ID == "file.copy" {
			found = true
			if op.Input != core.SourceFiles || op.Domain != core.File {
				t.Errorf("file.copy metadata = %+v", op)
			}
		}
	}
	if !found {
		t.Fatalf("file.copy missing from ops output:\n%s", out)
	}
}

func TestRunJobCopy(t *testing.T) {
	root := isolateProfile(t)
	src := filepath.Join(root, "input.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(root, "out")
	code, out := captureStdout(t, func() int {
		return runJob([]string{"--op", "file.copy", "--input", src, "--output", outDir})
	})
	if code != 0 {
		t.Fatalf("runJob exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "done") || !strings.Contains(out, "input.txt") {
		t.Fatalf("run output missing result:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(outDir, "input.txt")); err != nil {
		t.Fatalf("copied file: %v", err)
	}

	code, historyOut := captureStdout(t, func() int { return runHistory([]string{"--json"}) })
	if code != 0 {
		t.Fatalf("history exit = %d", code)
	}
	var records []historyJSON
	if err := json.Unmarshal([]byte(historyOut), &records); err != nil {
		t.Fatalf("decode history: %v\n%s", err, historyOut)
	}
	if len(records) != 1 || records[0].Status != core.StatusDone {
		t.Fatalf("history = %+v", records)
	}
	if len(records[0].Items) != 1 || len(records[0].Items[0].Outputs) != 1 {
		t.Fatalf("history items = %+v", records[0].Items)
	}
}

func TestPipelineImportListExport(t *testing.T) {
	root := isolateProfile(t)
	source := filepath.Join(root, "copy.toml")
	if err := os.WriteFile(source, []byte(`name = "test-copy"
version = 1
description = "copy files"
[[step]]
op = "file.copy"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if code, out := captureStdout(t, func() int {
		return runPipeline([]string{"import", "--name", "test-copy", source})
	}); code != 0 {
		t.Fatalf("import exit = %d\n%s", code, out)
	}

	code, out := captureStdout(t, func() int { return runPipeline([]string{"list", "--json"}) })
	if code != 0 {
		t.Fatalf("list exit = %d", code)
	}
	var pipelines []pipelineJSON
	if err := json.Unmarshal([]byte(out), &pipelines); err != nil {
		t.Fatalf("decode pipelines: %v\n%s", err, out)
	}
	if len(pipelines) != 1 || pipelines[0].Name != "test-copy" || pipelines[0].Steps != 1 {
		t.Fatalf("pipelines = %+v", pipelines)
	}

	code, exported := captureStdout(t, func() int { return runPipeline([]string{"export", "test-copy"}) })
	if code != 0 {
		t.Fatalf("export exit = %d", code)
	}
	if !strings.Contains(exported, "file.copy") || !strings.Contains(exported, "test-copy") {
		t.Fatalf("export = %q", exported)
	}
}

func TestRunDryRun(t *testing.T) {
	root := isolateProfile(t)
	src := filepath.Join(root, "dry.txt")
	if err := os.WriteFile(src, []byte("dry"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	code, text := captureStdout(t, func() int {
		return runJob([]string{"--op", "file.copy", "--input", src, "--output", out, "--dry-run"})
	})
	if code != 0 {
		t.Fatalf("dry-run exit = %d\n%s", code, text)
	}
	if !strings.Contains(text, "dry run") || !strings.Contains(text, src) || !strings.Contains(text, out) {
		t.Fatalf("dry-run output:\n%s", text)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("dry-run created output dir: %v", err)
	}
}
