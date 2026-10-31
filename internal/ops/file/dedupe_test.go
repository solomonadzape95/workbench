package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestDedupeFindsGroups(t *testing.T) {
	dir := t.TempDir()
	var items []core.Artifact
	for name, body := range map[string]string{"a.txt": "same", "b.txt": "same", "c.txt": "other"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		art, err := core.Probe(path)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, art)
	}
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	req := core.Request{Items: items, Params: core.Values{"format": "json"}, Target: core.Target{Dir: out}}
	if err := (Dedupe{}).Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := (Dedupe{}).Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	var report duplicateReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 1 || len(report.Groups[0].Files) != 2 || report.WastedBytes != 4 {
		t.Fatalf("report = %+v", report)
	}
}
