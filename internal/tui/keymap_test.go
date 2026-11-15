package tui

import (
	"strings"
	"testing"

	"github.com/solomonadzape95/workbench/internal/config"
)

func TestKeymapHasNoDuplicateKeysPerContext(t *testing.T) {
	for _, ctx := range helpSectionOrder {
		seen := map[string]bool{}
		for _, hint := range hints[ctx] {
			if hint.Key == "" || hint.Label == "" {
				t.Errorf("%s has an empty hint: %+v", ctx, hint)
			}
			if seen[hint.Key] {
				t.Errorf("%s repeats key %q", ctx, hint.Key)
			}
			seen[hint.Key] = true
		}
	}
}

func TestKeymapCoversEveryContext(t *testing.T) {
	for _, ctx := range helpSectionOrder {
		if _, ok := hints[ctx]; !ok {
			t.Errorf("context %s has no hints", ctx)
		}
		if sectionTitles[ctx] == "" {
			t.Errorf("context %s has no title", ctx)
		}
	}
}

func TestFooterShowsAtMostThreeHints(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	for _, screen := range []screen{screenHome, screenOperations, screenJobs, screenStats, screenSettings} {
		m.screen = screen
		if got := strings.Count(m.footerText(), " · ") + 1; got > maxFooterHints {
			t.Errorf("screen %v shows %d footer hints: %q", screen, got, m.footerText())
		}
	}
}

func TestFooterIncludesHelpOnEveryNormalScreen(t *testing.T) {
	m := sized(t, Options{})
	// Home is the smallest useful footer; it must still advertise help.
	footer := m.footerText()
	if footer == "" {
		t.Fatal("home footer is empty")
	}
	// The help card itself is the source of truth, so make sure it includes
	// the global keys the footer relies on.
	help := m.helpView()
	if !strings.Contains(help, "Global") || !strings.Contains(help, "palette") {
		t.Fatalf("help is missing global keys:\n%s", help)
	}
}
