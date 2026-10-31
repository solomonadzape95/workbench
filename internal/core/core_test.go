package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultsFillsValues(t *testing.T) {
	specs := []ParamSpec{
		{Key: "format", Default: "png"},
		{Key: "quality", Default: 85},
	}
	got := Defaults(specs)
	if got.String("format") != "png" || got.Int("quality") != 85 {
		t.Errorf("Defaults() = %#v", got)
	}
}

func TestValidate(t *testing.T) {
	specs := []ParamSpec{
		{Key: "format", Label: "Format", Type: ParamEnum, Required: true, Options: []Option{{Value: "png"}, {Value: "jpeg"}}},
		{Key: "quality", Label: "Quality", Type: ParamInt, Min: floatPtr(1), Max: floatPtr(100)},
		{Key: "wait", Label: "Wait", Type: ParamDuration},
		{Key: "name", Label: "Name", Type: ParamString, Required: true},
	}
	tests := []struct {
		name    string
		values  Values
		wantErr string
	}{
		{"valid", Values{"format": "png", "quality": 80, "wait": "2s", "name": "x"}, ""},
		{"missing required", Values{"quality": 80, "name": "x"}, "Format is required"},
		{"bad enum", Values{"format": "webp", "name": "x"}, "Format must be one of"},
		{"too low", Values{"format": "png", "quality": 0, "name": "x"}, "at least 1"},
		{"too high", Values{"format": "png", "quality": 101, "name": "x"}, "at most 100"},
		{"negative duration", Values{"format": "png", "wait": "-1s", "name": "x"}, "cannot be negative"},
		{"empty required string", Values{"format": "png", "name": "  "}, "Name is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(specs, tt.values)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestTargetOutputPath(t *testing.T) {
	in := Artifact{Path: "/photos/My Shot.JPG", Kind: Image, Format: "jpg"}
	target := Target{Dir: "/out", Template: "{stem}.{ext}", Collision: CollisionSuffix}
	now := time.Date(2026, 10, 20, 15, 4, 5, 0, time.UTC)

	got := target.OutputPath(in, "png", "image.convert", 0, now)
	if want := "/out/My Shot.png"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}

	target.Template = "{op}-{index}-{date}-{stem}.{ext}"
	got = target.OutputPath(in, "jpeg", "image.convert", 4, now)
	if want := "/out/image.convert-5-2026-10-20-My Shot.jpg"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}

	in.Meta = map[string]any{"width": 1920, "height": 1080}
	target.Template = "{stem}-{width}x{height}-{counter}.{ext}"
	got = target.OutputPath(in, "png", "file.rename", 2, now)
	if want := "/out/My Shot-1920x1080-3.png"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestTargetOutputPathCannotEscape(t *testing.T) {
	in := Artifact{Path: "/photos/a.png", Kind: Image, Format: "png"}
	target := Target{Dir: "/out", Template: "../../{stem}.{ext}"}
	got := target.OutputPath(in, "png", "x", 0, time.Now())
	if filepath.Dir(got) != "/out" {
		t.Errorf("path %q escaped the target directory", got)
	}
}

func TestUniquePathPolicies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := UniquePath(path, CollisionSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "a (1).txt"); got != want {
		t.Errorf("suffix path = %q, want %q", got, want)
	}

	if got, err := UniquePath(path, CollisionOverwrite); err != nil || got != path {
		t.Errorf("overwrite = %q, %v; want %q, nil", got, err, path)
	}
	if _, err := UniquePath(path, CollisionSkip); !errors.Is(err, ErrSkip) {
		t.Errorf("skip error = %v, want ErrSkip", err)
	}
	if _, err := UniquePath(path, CollisionFail); err == nil {
		t.Error("fail policy returned no error")
	}
}

func TestExtFor(t *testing.T) {
	in := Artifact{Path: "a.png", Format: "png"}
	cases := map[string]string{"jpeg": "jpg", "JPEG": "jpg", "tiff": "tif", "": "png", "weird/format": "weirdformat"}
	for format, want := range cases {
		if got := ExtFor(format, in); got != want {
			t.Errorf("ExtFor(%q) = %q, want %q", format, got, want)
		}
	}
}

func TestRegistryRejectsDuplicateIDs(t *testing.T) {
	r := NewRegistry(stubOp{id: "a"}, stubOp{id: "b"})
	if r.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", r.Len())
	}
	if err := r.Add(stubOp{id: "a"}); err == nil {
		t.Error("duplicate registration succeeded")
	}
	all := r.All()
	if all[0].Spec().ID != "a" || all[1].Spec().ID != "b" {
		t.Error("registration order was not preserved")
	}
}

func TestProbeClassifies(t *testing.T) {
	dir := t.TempDir()
	for name, want := range map[string]Kind{
		"a.png": Image, "a.mp3": Audio, "a.mp4": Video, "a.pdf": Document, "a.zip": Archive, "a.xyz": File,
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		art, err := Probe(path)
		if err != nil {
			t.Fatalf("Probe(%q): %v", name, err)
		}
		if art.Kind != want {
			t.Errorf("Probe(%q).Kind = %q, want %q", name, art.Kind, want)
		}
	}
	if _, err := Probe(dir); err == nil {
		t.Error("Probe accepted a directory")
	}
}

type stubOp struct{ id string }

func (s stubOp) Spec() Spec                                      { return Spec{ID: s.id, Title: s.id} }
func (s stubOp) Validate(ctx context.Context, req Request) error { return nil }
func (s stubOp) Run(ctx context.Context, req Request, rep Reporter) (Result, error) {
	return Result{}, nil
}

func floatPtr(v float64) *float64 { return &v }
