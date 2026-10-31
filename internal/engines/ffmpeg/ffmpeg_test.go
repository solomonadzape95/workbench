package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProbeFindsVersionAndFFprobe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	path := writeScript(t, dir, "ffmpeg", "#!/bin/sh\necho 'ffmpeg version 9.9.9-test Copyright (c) fake'\n")
	writeScript(t, dir, "ffprobe", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir)

	info := New("").Probe(context.Background())
	if !info.Available {
		t.Fatalf("not available: %v", info.Err)
	}
	if info.Version != "9.9.9-test" {
		t.Errorf("version = %q, want 9.9.9-test", info.Version)
	}
	if info.Path != path {
		t.Errorf("path = %q, want %q", info.Path, path)
	}
	if !strings.Contains(info.Detail, "ffprobe") || strings.Contains(info.Detail, "missing") {
		t.Errorf("detail = %q, want ffprobe detected", info.Detail)
	}
}

func TestProbeExplicitPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture")
	}
	dir := t.TempDir()
	path := writeScript(t, dir, "my-ffmpeg", "#!/bin/sh\necho 'ffmpeg version 8.0-custom'\n")
	t.Setenv("PATH", t.TempDir())

	info := New(path).Probe(context.Background())
	if !info.Available {
		t.Fatalf("not available: %v", info.Err)
	}
	if info.Version != "8.0-custom" {
		t.Errorf("version = %q, want 8.0-custom", info.Version)
	}
}

func TestProbeMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	info := New("").Probe(context.Background())
	if info.Available {
		t.Fatal("engine reported available with an empty PATH")
	}
	if info.Hint == "" {
		t.Error("missing engine has no install hint")
	}
	if info.ID != "ffmpeg" {
		t.Errorf("ID = %q, want ffmpeg", info.ID)
	}
}

func TestProbeBadExplicitPath(t *testing.T) {
	info := New(filepath.Join(t.TempDir(), "nope")).Probe(context.Background())
	if info.Available {
		t.Fatal("engine reported available for a nonexistent path")
	}
	if info.Hint == "" {
		t.Error("missing engine has no install hint")
	}
}

func TestParseVersion(t *testing.T) {
	cases := map[string]string{
		"ffmpeg version 7.1.1 Copyright (c) 2000-2025": "7.1.1",
		"ffmpeg version n7.0.2-static":                 "n7.0.2-static",
		"ffmpeg version 6.1.1-full_build-www.gyan.dev": "6.1.1-full_build-www.gyan.dev",
		"something else entirely":                      "unknown",
	}
	for in, want := range cases {
		if got := parseVersion([]byte(in)); got != want {
			t.Errorf("parseVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
