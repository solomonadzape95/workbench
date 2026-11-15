package engines

import (
	"runtime"
	"strings"
	"testing"
)

func TestInstallCatalogIsUniqueAndKnown(t *testing.T) {
	seen := map[ID]bool{}
	for _, spec := range InstallCatalog() {
		if spec.ID == "" {
			t.Fatal("install spec with empty id")
		}
		if seen[spec.ID] {
			t.Fatalf("duplicate install spec %q", spec.ID)
		}
		seen[spec.ID] = true
		if spec.Name == "" || spec.Summary == "" {
			t.Errorf("%s: missing display name or summary", spec.ID)
		}
		if len(spec.Binaries) == 0 {
			t.Errorf("%s: no binaries to probe", spec.ID)
		}
		if !spec.Auto() && spec.ManualURL == "" && spec.Command().Args == nil {
			t.Errorf("%s: no automatic command, URL, or manual hint", spec.ID)
		}
	}
}

func TestInstallCommandsArePlatformAware(t *testing.T) {
	for _, spec := range InstallCatalog() {
		cmd := spec.Command()
		if len(cmd.Args) == 0 {
			continue
		}
		if cmd.Args[0] == "" {
			t.Errorf("%s: empty install command", spec.ID)
		}
		if cmd.NeedsSudo && spec.Auto() {
			t.Errorf("%s: sudo command must not be auto-run", spec.ID)
		}
	}
}

func TestInstallHint(t *testing.T) {
	spec := InstallSpec{
		Commands: map[string]InstallCommand{
			runtime.GOOS: {Args: []string{"brew", "install", "thing"}},
		},
	}
	if got := spec.Hint(); got != "brew install thing" {
		t.Fatalf("Hint() = %q", got)
	}
	if !strings.Contains((InstallSpec{ManualURL: "https://example.com"}).Hint(), "example.com") {
		t.Fatal("manual URL missing from hint")
	}
}
