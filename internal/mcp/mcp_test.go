package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/app"
	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	imageops "github.com/solomonadzape95/workbench/internal/ops/image"
	"github.com/solomonadzape95/workbench/internal/runner"
)

func TestMCPListsAndCallsTools(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.png")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	outDir := filepath.Join(dir, "out")
	cfg := config.Defaults()
	cfg.Output.Dir = outDir
	ops := core.NewRegistry(imageops.Convert{})
	a := &app.App{
		Config: cfg, Operations: ops,
		Paths: config.Paths{DataDir: filepath.Join(dir, "data"), CacheDir: filepath.Join(dir, "cache")},
	}
	r := runner.New(runner.Options{
		Operations: ops, CacheDir: filepath.Join(dir, "cache"), MaxJobs: 1,
		EngineLimits: map[string]int{"internal": 1},
	})
	defer r.Close()

	call := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"wb_image_convert","arguments":{"inputs":[%q],"output_dir":%q,"format":"png"}}}`, src, outDir)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		call,
	}, "\n") + "\n"
	var out bytes.Buffer
	server := New(a, r, strings.NewReader(input), &out)
	if err := server.Serve(context.Background()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("responses = %d:\n%s", len(lines), out.String())
	}
	var listed struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &listed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range listed.Result.Tools {
		if tool.Name == "wb_image_convert" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tools = %+v", listed.Result.Tools)
	}
	var called struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
			IsError bool                    `json:"isError"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &called); err != nil {
		t.Fatal(err)
	}
	if called.Error != nil {
		t.Fatalf("error = %+v", called.Error)
	}
	if called.Result.IsError || len(called.Result.Content) != 1 {
		t.Fatalf("call = %+v", called.Result)
	}
	if !strings.Contains(called.Result.Content[0].Text, `"status": "done"`) {
		t.Fatalf("result text = %s", called.Result.Content[0].Text)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("outputs = %v, %v", entries, err)
	}
}
