package file

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func TestManifestJSONAndCSV(t *testing.T) {
	dir := t.TempDir()
	var items []core.Artifact
	for _, name := range []string{"a.txt", "b.txt"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		art, err := core.Probe(path)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, art)
	}
	run := func(format string, hash bool) []byte {
		out := t.TempDir()
		op := Manifest{}
		req := core.Request{Items: items, Params: core.Values{"format": format, "hash": hash}, Target: core.Target{Dir: out}}
		if err := op.Validate(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		result, err := op.Run(context.Background(), req, nopReporter{})
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(result.Artifacts[0].Path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	var entries []manifestEntry
	if err := json.Unmarshal(run("json", true), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].SHA256 == "" {
		t.Fatalf("entries = %+v", entries)
	}

	csvData := run("csv", false)
	rows, err := csv.NewReader(bytes.NewReader(csvData)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0][0] != "path" {
		t.Fatalf("rows = %+v", rows)
	}
}
