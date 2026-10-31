package tui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/solomonadzape95/workbench/internal/config"
)

// theme holds every style the UI uses. Screens never construct ad-hoc colors;
// they pull semantic styles from here.
type theme struct {
	name      string
	isDark    bool
	bg        string
	fg        string
	bgColor   color.Color
	fgColor   color.Color
	cursor    color.Color
	accentHex string

	title   lipgloss.Style
	muted   lipgloss.Style
	accent  lipgloss.Style
	success lipgloss.Style
	warning lipgloss.Style
	danger  lipgloss.Style
	border  lipgloss.Style
	gutter  lipgloss.Style

	currentLine lipgloss.Style
	selected    lipgloss.Style
	card        lipgloss.Style
	palette     [16]string
}

func newThemeFromPalette(t paletteTheme) theme {
	pal := func(index int, fallback string) string {
		if index >= 0 && index < len(t.Palette) && t.Palette[index] != "" {
			return t.Palette[index]
		}
		return fallback
	}
	accent := t.Accent
	if accent == "" {
		accent = pal(6, pal(4, "#5FD7D7"))
	}
	success := pal(2, "#7EE787")
	warning := pal(3, "#E3B341")
	danger := pal(1, "#FF7B72")
	muted := pal(8, mixHex(t.Background, t.Foreground, 0.55))
	border := pal(8, muted)
	fg := t.Foreground
	if fg == "" {
		fg = "#E6E6E6"
	}
	surface := pal(0, mixHex(t.Background, fg, 0.08))
	if surface == t.Background {
		surface = mixHex(t.Background, fg, 0.08)
	}
	cursor := t.Cursor
	if cursor == "" {
		cursor = accent
	}

	return theme{
		name:        t.Name,
		isDark:      t.IsDark(),
		bg:          t.Background,
		fg:          fg,
		bgColor:     lipgloss.Color(t.Background),
		fgColor:     lipgloss.Color(fg),
		cursor:      lipgloss.Color(cursor),
		accentHex:   accent,
		title:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent)),
		muted:       lipgloss.NewStyle().Foreground(lipgloss.Color(muted)),
		accent:      lipgloss.NewStyle().Foreground(lipgloss.Color(accent)),
		success:     lipgloss.NewStyle().Foreground(lipgloss.Color(success)),
		warning:     lipgloss.NewStyle().Foreground(lipgloss.Color(warning)),
		danger:      lipgloss.NewStyle().Foreground(lipgloss.Color(danger)),
		border:      lipgloss.NewStyle().Foreground(lipgloss.Color(border)),
		gutter:      lipgloss.NewStyle().Foreground(lipgloss.Color(muted)),
		currentLine: lipgloss.NewStyle().Background(lipgloss.Color(surface)),
		selected:    lipgloss.NewStyle().Background(lipgloss.Color(surface)).Foreground(lipgloss.Color(fg)).Bold(true),
		palette:     t.Palette,
		card: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(border)).
			Padding(1, 3),
	}
}

func themeForConfig(cfg config.Config, _ bool) (theme, error) {
	name := strings.TrimSpace(cfg.General.ThemeName)
	if name == "" {
		name = builtinThemes[0].Name
	}
	t, ok := builtinTheme(name)
	if !ok {
		return newThemeFromPalette(builtinThemes[0]), fmt.Errorf("theme %q not found; using %s", name, builtinThemes[0].Name)
	}
	return newThemeFromPalette(t), nil
}

// mixHex blends two #rrggbb colors. It keeps the palette readable when a
// theme does not define a distinct surface color.
func mixHex(a, b string, amount float64) string {
	ar, ag, ab, okA := parseHex(a)
	br, bg, bb, okB := parseHex(b)
	if !okA || !okB {
		return a
	}
	blend := func(x, y uint8) uint8 {
		return uint8(float64(x)*(1-amount) + float64(y)*amount)
	}
	return fmt.Sprintf("#%02x%02x%02x", blend(ar, br), blend(ag, bg), blend(ab, bb))
}

func parseHex(s string) (uint8, uint8, uint8, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v), true
}
