package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/core"
)

func sample() core.Pipeline {
	return core.Pipeline{
		Name:        "shots-clean",
		Version:     1,
		Description: "Screenshots → JPEG → compressed",
		Steps: []core.Step{
			{ID: "step-1", Op: "image.convert", Params: core.Values{"format": "jpeg", "quality": 85}},
			{ID: "step-2", Op: "image.compress", Input: core.StepInput{From: "previous"}, Params: core.Values{"quality": 75}},
		},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	p := sample()
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load("shots-clean")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != p.Name || loaded.Description != p.Description || len(loaded.Steps) != 2 {
		t.Errorf("loaded = %+v", loaded)
	}
	if got := loaded.Steps[0].Params.String("format"); got != "jpeg" {
		t.Errorf("format = %q, want jpeg", got)
	}
	if got := loaded.Steps[1].Params.Int("quality"); got != 75 {
		t.Errorf("quality = %d, want 75", got)
	}
	if loaded.Steps[1].Input.From != "previous" {
		t.Errorf("input.from = %q, want previous", loaded.Steps[1].Input.From)
	}
}

func TestSavedFileIsReadableTOML(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Save(sample()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.Path("shots-clean"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name = 'shots-clean'", "[[step]]", "op = 'image.convert'", "quality = 85"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("file missing %q:\n%s", want, data)
		}
	}
}

func TestListAndSoftDelete(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Save(sample()); err != nil {
		t.Fatal(err)
	}
	other := sample()
	other.Name = "copy-files"
	other.Steps = other.Steps[:1]
	other.Steps[0] = core.Step{Op: "file.copy"}
	if err := store.Save(other); err != nil {
		t.Fatal(err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "copy-files" || list[1].Name != "shots-clean" {
		t.Fatalf("list = %+v", list)
	}

	if err := store.Delete("copy-files"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("copy-files"); err == nil {
		t.Error("deleted pipeline still loads")
	}
	trash, err := os.ReadDir(filepath.Join(store.Dir, ".trash"))
	if err != nil || len(trash) != 1 {
		t.Errorf("trash entries = %v, %v; want 1", trash, err)
	}
}

func TestValidation(t *testing.T) {
	store := NewStore(t.TempDir())

	bad := sample()
	bad.Name = "Not A Slug"
	if err := store.Save(bad); err == nil {
		t.Error("invalid name was saved")
	}

	empty := sample()
	empty.Steps = nil
	if err := store.Save(empty); err == nil {
		t.Error("empty pipeline was saved")
	}

	missing := sample()
	missing.Steps[0].Op = ""
	if err := store.Save(missing); err == nil {
		t.Error("step without an operation was saved")
	}
}

func TestMalformedFileAppearsInList(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path("broken"), []byte("[step\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Err == nil {
		t.Errorf("list = %+v, want one entry with an error", list)
	}
}

func TestVersionGuard(t *testing.T) {
	store := NewStore(t.TempDir())
	p := sample()
	p.Version = core.PipelineVersion + 1
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("name = 'future'\nversion = 99\n[[step]]\nop = 'file.copy'\n")
	if err := os.WriteFile(store.Path("future"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("future"); err == nil {
		t.Error("newer pipeline version loaded without an error")
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Convert image + Compress": "convert-image-compress",
		"  PDF → DOCX  ":           "pdf-docx",
		"!!!":                      "pipeline",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
