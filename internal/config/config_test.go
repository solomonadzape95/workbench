package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, warnings := Load(filepath.Join(t.TempDir(), "config.toml"))
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if got, want := cfg.General.Theme, ThemeAuto; got != want {
		t.Errorf("theme = %q, want %q", got, want)
	}
	if got, want := cfg.General.MaxJobs, 4; got != want {
		t.Errorf("max jobs = %d, want %d", got, want)
	}
	if got, want := cfg.Output.Collision, CollisionSuffix; got != want {
		t.Errorf("collision = %q, want %q", got, want)
	}
}

func TestLoadOverridesKeepDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `
[general]
theme = "light"
max_jobs = 2

[output]
collision = "skip"

[engines.paths]
ffmpeg = "/opt/bin/ffmpeg"

[[alias]]
name = "@shots"
path = "~/Pictures/Screenshots"
glob = "*.png"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, warnings := Load(path)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if cfg.General.Theme != ThemeLight || cfg.General.MaxJobs != 2 {
		t.Errorf("general = %+v, want theme light and max jobs 2", cfg.General)
	}
	if !cfg.General.Notify {
		t.Error("notify lost its default; partial config must not reset unrelated keys")
	}
	if cfg.Output.Template != Defaults().Output.Template {
		t.Errorf("template = %q, want default %q", cfg.Output.Template, Defaults().Output.Template)
	}
	if cfg.Output.Collision != CollisionSkip {
		t.Errorf("collision = %q, want skip", cfg.Output.Collision)
	}
	if got := cfg.Engines.Paths["ffmpeg"]; got != "/opt/bin/ffmpeg" {
		t.Errorf("ffmpeg path = %q", got)
	}
	if got := cfg.Engines.Limits["rembg"]; got != 2 {
		t.Errorf("rembg limit = %d, want default 2", got)
	}
	if len(cfg.Aliases) != 1 || cfg.Aliases[0].Name != "shots" {
		t.Fatalf("aliases = %+v, want one alias named shots", cfg.Aliases)
	}
}

func TestLoadInvalidTOMLFallsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[general\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, warnings := Load(path)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one", warnings)
	}
	if !strings.Contains(warnings[0], "invalid config") {
		t.Errorf("warning = %q, want it to mention invalid config", warnings[0])
	}
	if cfg.General.Theme != ThemeAuto || cfg.General.MaxJobs != 4 {
		t.Errorf("config did not fall back to defaults: %+v", cfg.General)
	}
}

func TestNormalizeRepairsValues(t *testing.T) {
	cfg := Defaults()
	cfg.General.Theme = "blue"
	cfg.General.MaxJobs = -3
	cfg.Output.Collision = "clobber"
	cfg.Output.Dir = "  "
	cfg.Engines.Limits["ffmpeg"] = -1
	cfg.Aliases = []Alias{
		{Name: "", Path: "~/x"},
		{Name: "bad name", Path: "~/x"},
		{Name: "ok", Path: "~/x"},
		{Name: "ok", Path: "~/y"},
		{Name: "empty"},
	}

	warnings := cfg.normalize()
	if len(warnings) != 9 {
		t.Fatalf("warnings = %d (%v), want 9", len(warnings), warnings)
	}
	if cfg.General.Theme != ThemeAuto || cfg.General.MaxJobs != 4 {
		t.Errorf("general not repaired: %+v", cfg.General)
	}
	if cfg.Output.Collision != CollisionSuffix || cfg.Output.Dir != Defaults().Output.Dir {
		t.Errorf("output not repaired: %+v", cfg.Output)
	}
	if got, want := cfg.Engines.Limits["ffmpeg"], Defaults().Engines.Limits["ffmpeg"]; got != want {
		t.Errorf("ffmpeg limit = %d, want the built-in %d", got, want)
	}
	if len(cfg.Aliases) != 1 || cfg.Aliases[0].Name != "ok" {
		t.Errorf("aliases = %+v, want only ok", cfg.Aliases)
	}
}

func TestExpandHome(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	cases := map[string]string{
		"~":                      "/home/tester",
		"~/Pictures":             "/home/tester/Pictures",
		"~/":                     "/home/tester",
		"/absolute/path":         "/absolute/path",
		"relative/path":          "relative/path",
		"~notauser/file":         "~notauser/file",
		"/tmp/$HOME-should-stay": "/tmp/$HOME-should-stay",
	}
	for in, want := range cases {
		if got := ExpandHome(in); got != want {
			t.Errorf("ExpandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	cfg := Defaults()
	cfg.General.Theme = ThemeDark
	cfg.Engines.Enabled = []string{"internal", "ffmpeg"}
	cfg.Aliases = []Alias{{Name: "shots", Path: "~/Pictures", Glob: "*.png"}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, warnings := Load(path)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if loaded.General.Theme != ThemeDark {
		t.Errorf("theme = %q", loaded.General.Theme)
	}
	if !contains(loaded.Engines.Enabled, "internal") || !contains(loaded.Engines.Enabled, "ffmpeg") {
		t.Errorf("enabled = %v", loaded.Engines.Enabled)
	}
	if len(loaded.Aliases) != 1 || loaded.Aliases[0].Name != "shots" {
		t.Errorf("aliases = %+v", loaded.Aliases)
	}
}

func TestSaveIncludesEditingGuide(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, Defaults()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# Workbench configuration") || !strings.Contains(string(data), "[[alias]]") {
		t.Fatalf("config guide missing:\n%s", data)
	}
}

func TestSaveIsAtomicAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := Save(path, Defaults()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.toml" {
		t.Errorf("dir entries = %v", names(entries))
	}
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
