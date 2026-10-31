package tui

import (
	"strconv"
	"strings"
)

type paletteTheme struct {
	Name       string
	Background string
	Foreground string
	Cursor     string
	Accent     string // optional semantic accent; falls back to palette[6]
	Palette    [16]string
}

func (t paletteTheme) IsDark() bool {
	c := strings.TrimPrefix(t.Background, "#")
	if len(c) != 6 {
		return true
	}
	v, err := strconv.ParseUint(c, 16, 32)
	if err != nil {
		return true
	}
	r := float64((v>>16)&0xff) / 255
	g := float64((v>>8)&0xff) / 255
	b := float64(v&0xff) / 255
	return 0.2126*r+0.7152*g+0.0722*b < 0.5
}

// Built-in palettes. Eight is deliberate: enough range to be useful, few
// enough that a picker can show every option and a preview at once.
var builtinThemes = []paletteTheme{
	{
		Name:       "opencode",
		Background: "#0b0d10",
		Foreground: "#e6e6e6",
		Cursor:     "#f5a742",
		Palette: [16]string{
			"#181c22", "#e06c75", "#98c379", "#e5c07b",
			"#61afef", "#c678dd", "#f5a742", "#abb2bf",
			"#5c6370", "#e06c75", "#98c379", "#e5c07b",
			"#61afef", "#c678dd", "#56b6c2", "#ffffff",
		},
	},
	{
		Name:       "tokyonight",
		Background: "#1a1b26",
		Foreground: "#c0caf5",
		Cursor:     "#7aa2f7",
		Palette: [16]string{
			"#15161e", "#f7768e", "#9ece6a", "#e0af68",
			"#7aa2f7", "#bb9af7", "#7dcfff", "#a9b1d6",
			"#414868", "#f7768e", "#9ece6a", "#e0af68",
			"#7aa2f7", "#bb9af7", "#7dcfff", "#c0caf5",
		},
	},
	{
		Name:       "catppuccin",
		Background: "#1e1e2e",
		Foreground: "#cdd6f4",
		Cursor:     "#f5e0dc",
		Palette: [16]string{
			"#45475a", "#f38ba8", "#a6e3a1", "#f9e2af",
			"#89b4fa", "#f5c2e7", "#94e2d5", "#a6adc8",
			"#585b70", "#f37799", "#89d88b", "#ebd391",
			"#74a8fc", "#f2aede", "#6bd7ca", "#bac2de",
		},
	},
	{
		Name:       "gruvbox",
		Background: "#282828",
		Foreground: "#ebdbb2",
		Cursor:     "#fabd2f",
		Palette: [16]string{
			"#3c3836", "#fb4934", "#b8bb26", "#fabd2f",
			"#83a598", "#d3869b", "#8ec07c", "#bdae93",
			"#665c54", "#fb4934", "#b8bb26", "#fabd2f",
			"#83a598", "#d3869b", "#8ec07c", "#ebdbb2",
		},
	},
	{
		Name:       "nord",
		Background: "#2e3440",
		Foreground: "#d8dee9",
		Cursor:     "#88c0d0",
		Palette: [16]string{
			"#3b4252", "#bf616a", "#a3be8c", "#ebcb8b",
			"#81a1c1", "#b48ead", "#88c0d0", "#e5e9f0",
			"#4c566a", "#bf616a", "#a3be8c", "#ebcb8b",
			"#81a1c1", "#b48ead", "#8fbcbb", "#eceff4",
		},
	},
	{
		Name:       "monokai",
		Background: "#0c0c0c",
		Foreground: "#d9d9d9",
		Cursor:     "#fc971f",
		Palette: [16]string{
			"#1a1a1a", "#f4005f", "#98e024", "#fd971f",
			"#9d65ff", "#f4005f", "#58d1eb", "#c4c5b5",
			"#625e4c", "#f4005f", "#98e024", "#e0d561",
			"#9d65ff", "#f4005f", "#58d1eb", "#f6f6ef",
		},
	},
	{
		Name:       "vscode",
		Background: "#1e1e1e",
		Foreground: "#d4d4d4",
		Cursor:     "#aeafad",
		Accent:     "#007acc",
		Palette: [16]string{
			"#000000", "#cd3131", "#0dbc79", "#e5e510",
			"#2472c8", "#bc3fbc", "#11a8cd", "#e5e5e5",
			"#666666", "#f14c4c", "#23d18b", "#f5f543",
			"#3b8eea", "#d670d6", "#29b8db", "#e5e5e5",
		},
	},
	{
		Name:       "rosepine",
		Background: "#191724",
		Foreground: "#e0def4",
		Cursor:     "#c4a7e7",
		Palette: [16]string{
			"#26233a", "#eb6f92", "#9ccfd8", "#f6c177",
			"#31748f", "#c4a7e7", "#ebbcba", "#e0def4",
			"#6e6a86", "#eb6f92", "#9ccfd8", "#f6c177",
			"#31748f", "#c4a7e7", "#ebbcba", "#e0def4",
		},
	},
}

func builtinTheme(name string) (paletteTheme, bool) {
	for _, theme := range builtinThemes {
		if theme.Name == name {
			return theme, true
		}
	}
	return paletteTheme{}, false
}
