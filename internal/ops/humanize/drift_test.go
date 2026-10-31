package humanize

import (
	"strings"
	"testing"
)

func TestFatalRewriteRejectsMetaReply(t *testing.T) {
	original := "1.5 Significance of the Study"
	rewritten := "The text you provided is a heading, not a full passage to rewrite. Please provide the body text under this heading so I can apply the style rules."
	if issue := FatalRewriteIssue(original, rewritten); issue == "" {
		t.Fatal("meta reply was accepted")
	}
}

func TestFatalRewriteRejectsReplacedNames(t *testing.T) {
	original := "Agbo Casmir Chidubem"
	rewritten := "I hear Adzape Solomon Chiater. The name carries weight across fields, connecting past achievement with future promise."
	if issue := FatalRewriteIssue(original, rewritten); issue == "" {
		t.Fatal("replaced names were accepted")
	}
}

func TestFatalRewriteRejectsUnrelatedParagraph(t *testing.T) {
	original := "The proposed platform is intended for use within institutional settings, particularly universities and similar academic environments where the voter population is defined and verifiable."
	rewritten := "The study encompasses all departments and subunits within the organization. It includes financial units, research divisions, and administrative offices. Operational units are excluded from the analysis. The focus remains on headquarters and primary facilities only. Regional branches are not covered by this investigation."
	if issue := FatalRewriteIssue(original, rewritten); issue == "" {
		t.Fatal("unrelated paragraph was accepted")
	}
}

func TestFatalRewriteAllowsNormalRewrite(t *testing.T) {
	original := "Most digital voting systems store votes in centralized databases managed by administrators. This creates a link between a voter and a ballot."
	rewritten := "Most online voting systems keep votes in one central database run by administrators. That links each voter to a ballot."
	if issue := FatalRewriteIssue(original, rewritten); issue != "" {
		t.Fatalf("normal rewrite rejected: %s", issue)
	}
}

func TestDriftIssuesKeepNamesAndNumbers(t *testing.T) {
	issues := driftIssues(
		"In 2008, Adida described Helios and RSA blind signatures.",
		"The system uses blind signatures.",
	)
	joined := strings.Join(issues, "\n")
	for _, want := range []string{"2008", "Adida", "Helios", "RSA"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues missing %q:\n%s", want, joined)
		}
	}
}

func TestDriftIssuesRejectNewNumbersAndNames(t *testing.T) {
	issues := driftIssues(
		"The platform uses blind signatures.",
		"In 2024, Dr. Elena Rostova at Cambridge added three new servers.",
	)
	joined := strings.Join(issues, "\n")
	for _, want := range []string{"2024", "Elena", "Rostova", "Cambridge"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues missing %q:\n%s", want, joined)
		}
	}
}

func TestProperNounsIgnoreMissingSpaceAfterPeriod(t *testing.T) {
	got := strings.Join(properNouns("The system votes.Wood is a typo."), ",")
	if strings.Contains(got, "Wood") {
		t.Fatalf("split token was treated as a name: %s", got)
	}
	if got := strings.Join(properNouns("The platform uses Bitcoin and Ethereum."), ","); got != "Bitcoin,Ethereum" {
		t.Fatalf("proper nouns = %q", got)
	}
}

func TestAddedSingleDigitNumberIsAllowed(t *testing.T) {
	if issue := FatalRewriteIssue("The platform has one server.", "The platform has 1 server."); issue != "" {
		t.Fatalf("single-digit spelling change rejected: %s", issue)
	}
	if issue := FatalRewriteIssue("The platform has one server.", "The platform has 12 servers."); issue == "" {
		t.Fatal("new multi-digit count was accepted")
	}
}
