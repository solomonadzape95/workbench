package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Paths holds every directory Workbench owns.
//
// The layout follows the XDG base directory specification on every platform,
// including macOS. Native macOS Library paths are deliberately not used: this
// is a developer tool, and ~/.config is where its users look for config.
type Paths struct {
	ConfigDir  string
	DataDir    string
	CacheDir   string
	ConfigFile string
}

// DefaultPaths resolves the XDG paths, then applies Workbench-specific
// overrides. The overrides are primarily for tests and throwaway profiles.
func DefaultPaths() Paths {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return pathsFor(runtime.GOOS, home, os.Getenv)
}

// pathsFor is separated from DefaultPaths so Windows layout can be tested on
// any host. XDG variables still win everywhere, which keeps throwaway profiles
// and existing Linux/macOS users working.
func pathsFor(goos, home string, getenv func(string) string) Paths {
	configBase := getenv("XDG_CONFIG_HOME")
	dataBase := getenv("XDG_DATA_HOME")
	cacheBase := getenv("XDG_CACHE_HOME")
	if goos == "windows" {
		if configBase == "" {
			configBase = getenv("APPDATA")
		}
		if dataBase == "" {
			dataBase = getenv("LOCALAPPDATA")
		}
		if cacheBase == "" {
			cacheBase = getenv("LOCALAPPDATA")
		}
	}
	if configBase == "" {
		configBase = filepath.Join(home, ".config")
	}
	if dataBase == "" {
		dataBase = filepath.Join(home, ".local", "share")
	}
	if cacheBase == "" {
		cacheBase = filepath.Join(home, ".cache")
	}

	configDir := envOrFunc(getenv, "WORKBENCH_CONFIG_DIR", filepath.Join(configBase, "workbench"))
	return Paths{
		ConfigDir:  configDir,
		DataDir:    envOrFunc(getenv, "WORKBENCH_DATA_DIR", filepath.Join(dataBase, "workbench")),
		CacheDir:   envOrFunc(getenv, "WORKBENCH_CACHE_DIR", filepath.Join(cacheBase, "workbench")),
		ConfigFile: filepath.Join(configDir, "config.toml"),
	}
}

// ExpandHome expands a leading ~ or ~/ in a user-supplied path. Other
// environment expansion is intentionally not performed: a literal $ in a
// filename should stay literal.
func ExpandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

func envOrFunc(getenv func(string) string, key, fallback string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return fallback
}
