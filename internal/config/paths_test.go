package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPathsUsesXDG(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")
	t.Setenv("XDG_DATA_HOME", "/xdg/data")
	t.Setenv("XDG_CACHE_HOME", "/xdg/cache")
	t.Setenv("WORKBENCH_CONFIG_DIR", "")
	t.Setenv("WORKBENCH_DATA_DIR", "")
	t.Setenv("WORKBENCH_CACHE_DIR", "")

	paths := DefaultPaths()
	if got, want := paths.ConfigDir, "/xdg/config/workbench"; got != want {
		t.Errorf("config dir = %q, want %q", got, want)
	}
	if got, want := paths.ConfigFile, filepath.Join("/xdg/config/workbench", "config.toml"); got != want {
		t.Errorf("config file = %q, want %q", got, want)
	}
	if got, want := paths.DataDir, "/xdg/data/workbench"; got != want {
		t.Errorf("data dir = %q, want %q", got, want)
	}
	if got, want := paths.CacheDir, "/xdg/cache/workbench"; got != want {
		t.Errorf("cache dir = %q, want %q", got, want)
	}
}

func TestDefaultPathsFallsBackToHome(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("WORKBENCH_CONFIG_DIR", "")

	paths := DefaultPaths()
	if got, want := paths.ConfigDir, "/home/tester/.config/workbench"; got != want {
		t.Errorf("config dir = %q, want %q", got, want)
	}
	if got, want := paths.DataDir, "/home/tester/.local/share/workbench"; got != want {
		t.Errorf("data dir = %q, want %q", got, want)
	}
	if got, want := paths.CacheDir, "/home/tester/.cache/workbench"; got != want {
		t.Errorf("cache dir = %q, want %q", got, want)
	}
}

func TestWorkbenchDirsOverrideXDG(t *testing.T) {
	t.Setenv("WORKBENCH_CONFIG_DIR", "/tmp/wb-config")
	t.Setenv("WORKBENCH_DATA_DIR", "/tmp/wb-data")
	t.Setenv("WORKBENCH_CACHE_DIR", "/tmp/wb-cache")

	paths := DefaultPaths()
	if got, want := paths.ConfigDir, "/tmp/wb-config"; got != want {
		t.Errorf("config dir = %q, want %q", got, want)
	}
	if got, want := paths.DataDir, "/tmp/wb-data"; got != want {
		t.Errorf("data dir = %q, want %q", got, want)
	}
	if got, want := paths.CacheDir, "/tmp/wb-cache"; got != want {
		t.Errorf("cache dir = %q, want %q", got, want)
	}
}

func TestWindowsPathsUseAppData(t *testing.T) {
	env := map[string]string{
		"APPDATA":      `C:\Users\me\AppData\Roaming`,
		"LOCALAPPDATA": `C:\Users\me\AppData\Local`,
	}
	getenv := func(key string) string { return env[key] }
	paths := pathsFor("windows", `C:\Users\me`, getenv)
	if !strings.Contains(paths.ConfigDir, "Roaming") || !strings.Contains(paths.ConfigDir, "workbench") {
		t.Errorf("config dir = %q", paths.ConfigDir)
	}
	if !strings.Contains(paths.DataDir, "Local") || !strings.Contains(paths.DataDir, "workbench") {
		t.Errorf("data dir = %q", paths.DataDir)
	}
}

func TestXDGOverridesWindows(t *testing.T) {
	env := map[string]string{
		"XDG_CONFIG_HOME": "/tmp/cfg",
		"APPDATA":         `C:\Users\me\AppData\Roaming`,
	}
	paths := pathsFor("windows", `C:\Users\me`, func(key string) string { return env[key] })
	if paths.ConfigDir != filepath.Join("/tmp/cfg", "workbench") {
		t.Errorf("config dir = %q", paths.ConfigDir)
	}
}
