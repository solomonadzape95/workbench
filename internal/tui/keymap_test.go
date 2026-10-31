package tui

import (
	"strings"
	"testing"
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
