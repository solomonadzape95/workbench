// Package config loads and validates Workbench's TOML configuration.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Theme and collision values are strings rather than enums so a hand-edited
// config stays readable.
const (
	ThemeAuto  = "auto"
	ThemeDark  = "dark"
	ThemeLight = "light"

	CollisionSuffix    = "suffix"
	CollisionOverwrite = "overwrite"
	CollisionSkip      = "skip"
	CollisionFail      = "fail"
)

// Header is prepended to every saved config. The TOML marshaller cannot retain
// comments on its own, so the important editing guidance lives here.
const Header = `# Workbench configuration.
# Edit values, save, and restart Workbench to apply engine/alias changes.
#
# Add your own aliases as tables:
# [[alias]]
# name = "shots"
# path = "~/Pictures/Screenshots"
# glob = "*.png"
#
# [[alias]]
# name = "projects"
# path = "~/Documents/Development"
#
# Built-ins such as @home, @downloads, @documents, @development,
# @pictures, @videos, @music, @desktop, and @screenshots work without config.
#
# The humanizer talks to your local OpenCode service. Auto-discovery usually
# works. To point it somewhere else:
#
# [opencode]
# url = "http://127.0.0.1:4096"
# password = "..."  # prefer OPENCODE_SERVICE_PASSWORD in your environment
`

// Config is the on-disk configuration. Every field has a default, so a
// partial file only overrides what it mentions.
type Config struct {
	General  General      `toml:"general"`
	Output   Output       `toml:"output"`
	Engines  Engines      `toml:"engines"`
	Logs     Logs         `toml:"logs"`
	Notify   NotifyConfig `toml:"notify"`
	OpenCode OpenCode     `toml:"opencode"`
	Detect   DetectConfig `toml:"detect"`
	Aliases  []Alias      `toml:"alias,omitempty"`
}

type NotifyConfig struct {
	Webhook string `toml:"webhook,omitempty"`
}

type Logs struct {
	MaxAgeDays int `toml:"max_age_days"`
	MaxFiles   int `toml:"max_files"`
}

type General struct {
	Theme     string `toml:"theme"`
	ThemeName string `toml:"theme_name,omitempty"`
	Notify    bool   `toml:"notify"`
	MaxJobs   int    `toml:"max_jobs"`
}

type Output struct {
	Dir       string `toml:"dir"`
	Collision string `toml:"collision"`
	Template  string `toml:"template"`
}

type Engines struct {
	Enabled []string          `toml:"enabled"`
	Paths   map[string]string `toml:"paths"`
	Limits  map[string]int    `toml:"limits"`
}

// OpenCode points at the local OpenCode background service. Empty values use
// auto-discovery: `opencode service status` for the URL and OpenCode's own
// service.json for the password.
type OpenCode struct {
	URL      string `toml:"url,omitempty"`
	Password string `toml:"password,omitempty"`
}

// DetectConfig controls the optional external log-probability scorer. The
// command receives document text on stdin and must print a score (0-1 or
// 0-100) or JSON containing "score", "ai", or "logprob".
type DetectConfig struct {
	LogProbCommand string  `toml:"logprob_command,omitempty"`
	LogProbWeight  float64 `toml:"logprob_weight,omitempty"`
}

// Alias is a named shortcut for a directory, glob, or fixed file list.
type Alias struct {
	Name  string   `toml:"name"`
	Path  string   `toml:"path"`
	Glob  string   `toml:"glob"`
	Files []string `toml:"files"`
}

// Defaults returns the configuration used when the file is missing or a value
// cannot be understood.
func Defaults() Config {
	return Config{
		General: General{
			Theme:     ThemeAuto,
			ThemeName: "opencode",
			Notify:    true,
			MaxJobs:   4,
		},
		Output: Output{
			Dir:       "~/Workbench/{date}",
			Collision: CollisionSuffix,
			Template:  "{stem}.{ext}",
		},
		Logs: Logs{MaxAgeDays: 30, MaxFiles: 500},
		Engines: Engines{
			Enabled: []string{"internal", "ffmpeg", "ytdlp", "imagemagick", "libreoffice", "pandoc", "ghostscript", "tesseract", "pdf2docx", "poppler", "rembg", "demucs", "realesrgan", "whisper", "opencode"},
			Paths:   map[string]string{},
			Limits: map[string]int{
				"ffmpeg":      2,
				"rembg":       2,
				"demucs":      1,
				"realesrgan":  1,
				"whisper":     1,
				"libreoffice": 1,
				"ghostscript": 1,
				"tesseract":   1,
				"opencode":    2,
			},
		},
		Detect:  DetectConfig{LogProbWeight: 0.15},
		Aliases: []Alias{},
	}
}

// Load reads path into a Config. A missing file is not an error: it means
// "use defaults". Any other problem is returned as a warning and the affected
// values fall back to their defaults, because a bad config must never prevent
// the app from starting.
func Load(path string) (Config, []string) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return cfg, []string{fmt.Sprintf("cannot read %s: %v (using defaults)", path, err)}
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Defaults(), []string{fmt.Sprintf("invalid config %s: %v (using defaults)", path, err)}
	}
	ensureNewEngines(&cfg)
	return cfg, cfg.normalize()
}

// ensureNewEngines makes engines added after the user's config was written
// discoverable. Their probes still report a missing binary with an install
// hint, and the user can remove them from engines.enabled to opt out.
func ensureNewEngines(c *Config) {
	if len(c.Engines.Enabled) == 0 {
		return
	}
	have := make(map[string]bool, len(c.Engines.Enabled))
	for _, id := range c.Engines.Enabled {
		have[id] = true
	}
	for _, id := range []string{"demucs", "ghostscript", "tesseract", "realesrgan"} {
		if !have[id] {
			c.Engines.Enabled = append(c.Engines.Enabled, id)
		}
	}
}

// Save writes cfg atomically, creating the parent directory if needed.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	data = append([]byte(Header), data...)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// normalize repairs out-of-range or malformed values in place and reports one
// warning per repair. Silent coercion is worse than a visible warning.
func (c *Config) normalize() []string {
	var warnings []string
	warn := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	switch c.General.Theme {
	case ThemeAuto, ThemeDark, ThemeLight:
	default:
		warn("general.theme %q is not one of auto, dark, light; using %q", c.General.Theme, ThemeAuto)
		c.General.Theme = ThemeAuto
	}
	c.General.ThemeName = strings.TrimSpace(c.General.ThemeName)
	if c.General.ThemeName == "" {
		c.General.ThemeName = "opencode"
	}

	if c.General.MaxJobs < 0 {
		warn("general.max_jobs cannot be negative; using %d", Defaults().General.MaxJobs)
		c.General.MaxJobs = Defaults().General.MaxJobs
	}

	switch c.Output.Collision {
	case CollisionSuffix, CollisionOverwrite, CollisionSkip, CollisionFail:
	default:
		warn("output.collision %q is not one of suffix, overwrite, skip, fail; using %q", c.Output.Collision, CollisionSuffix)
		c.Output.Collision = CollisionSuffix
	}

	if strings.TrimSpace(c.Output.Dir) == "" {
		warn("output.dir is empty; using %q", Defaults().Output.Dir)
		c.Output.Dir = Defaults().Output.Dir
	}
	if strings.TrimSpace(c.Output.Template) == "" {
		warn("output.template is empty; using %q", Defaults().Output.Template)
		c.Output.Template = Defaults().Output.Template
	}

	if c.Engines.Paths == nil {
		c.Engines.Paths = map[string]string{}
	}
	if c.Engines.Limits == nil {
		c.Engines.Limits = map[string]int{}
	}
	for name, limit := range c.Engines.Limits {
		if limit < 0 {
			warn("engines.limits.%s cannot be negative; using the built-in limit", name)
			delete(c.Engines.Limits, name)
		}
	}
	for name, limit := range Defaults().Engines.Limits {
		if _, ok := c.Engines.Limits[name]; !ok {
			c.Engines.Limits[name] = limit
		}
	}

	c.Detect.LogProbCommand = strings.TrimSpace(c.Detect.LogProbCommand)
	if c.Detect.LogProbWeight <= 0 || c.Detect.LogProbWeight > 1 {
		c.Detect.LogProbWeight = Defaults().Detect.LogProbWeight
	}
	c.OpenCode.URL = strings.TrimSpace(c.OpenCode.URL)
	c.OpenCode.Password = strings.TrimSpace(c.OpenCode.Password)

	c.Aliases = normalizeAliases(c.Aliases, warn)
	return warnings
}

func normalizeAliases(in []Alias, warn func(string, ...any)) []Alias {
	out := make([]Alias, 0, len(in))
	seen := map[string]bool{}
	for i, a := range in {
		a.Name = strings.TrimPrefix(strings.TrimSpace(a.Name), "@")
		if !validAliasName(a.Name) {
			warn("alias #%d: name %q is invalid (use letters, digits, - and _); entry ignored", i+1, a.Name)
			continue
		}
		if seen[a.Name] {
			warn("alias %q is defined more than once; keeping the first entry", a.Name)
			continue
		}
		if strings.TrimSpace(a.Path) == "" && len(a.Files) == 0 {
			warn("alias %q has neither path nor files; entry ignored", a.Name)
			continue
		}
		a.Path = strings.TrimSpace(a.Path)
		a.Glob = strings.TrimSpace(a.Glob)
		seen[a.Name] = true
		out = append(out, a)
	}
	return out
}

func validAliasName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
