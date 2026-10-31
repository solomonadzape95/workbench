package humanize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines/opencode"
)

type nopReporter struct{}

func (nopReporter) Stage(string)                   {}
func (nopReporter) Progress(int, int)              {}
func (nopReporter) Detail(core.Artifact, string)   {}
func (nopReporter) Log(core.Level, string, ...any) {}

type fakeGenerator struct {
	models []opencode.ModelRef
	fn     func(opencode.ModelRef, string) (string, error)

	mu    sync.Mutex
	calls []string
}

func (f *fakeGenerator) FreeModels(context.Context) ([]opencode.ModelRef, error) {
	return f.models, nil
}

func (f *fakeGenerator) Generate(_ context.Context, model opencode.ModelRef, prompt string) (string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, model.String())
	f.mu.Unlock()
	if f.fn != nil {
		return f.fn(model, prompt)
	}
	return rewriteFromPrompt(prompt), nil
}

func (f *fakeGenerator) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func rewriteFromPrompt(prompt string) string {
	const marker = "TEXT TO REWRITE:\n"
	idx := strings.Index(prompt, marker)
	if idx < 0 {
		return "rewritten"
	}
	text := prompt[idx+len(marker):]
	text = strings.ReplaceAll(text, "In conclusion, ", "So, ")
	text = strings.ReplaceAll(text, "crucial", "important")
	text = strings.ReplaceAll(text, "utilize", "use")
	return text
}

func TestEmbeddedRulesMatchSource(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "humanizer-rules.md"))
	if err != nil {
		t.Skip("repository rules file is not available")
	}
	if string(data) != embeddedRules {
		t.Fatal("internal/ops/humanize/rules.md is out of sync with humanizer-rules.md; copy the source file into the package")
	}
}

func TestEveryStyleHasAModule(t *testing.T) {
	rules := embeddedRules
	if !strings.Contains(CoreRules(rules), "Rhythm") {
		t.Fatal("core rules do not contain the rhythm section")
	}
	for _, style := range styleOptions {
		module := StyleModule(rules, style.Value)
		if strings.TrimSpace(module) == "" {
			t.Errorf("style %q has no module", style.Value)
		}
		if !strings.HasPrefix(strings.ToUpper(module), "STYLE:") {
			t.Errorf("style %q does not start with STYLE:", style.Value)
		}
	}
}

func TestProtectDocumentHidesCodeLinksAndStructure(t *testing.T) {
	input := "---\ntitle: Notes\n---\n\n# Heading\n\nUse `code` and [a link](https://example.com/x).\n\n```go\nfmt.Println(\"hello\")\n```\n\n> a quotation\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n- list item\n"
	protected, p := ProtectDocument(input)
	for _, secret := range []string{"fmt.Println", "https://example.com/x", "title: Notes", "| a | b |", "a quotation"} {
		if strings.Contains(protected, secret) {
			t.Errorf("protected text still contains %q:\n%s", secret, protected)
		}
	}
	if !strings.Contains(protected, "[[WB") {
		t.Fatal("protected text has no placeholders")
	}
	restored, missing := p.restore(protected)
	if len(missing) != 0 {
		t.Fatalf("missing = %v", missing)
	}
	if !strings.Contains(restored, "fmt.Println(\"hello\")") {
		t.Errorf("code block was not restored:\n%s", restored)
	}
	if !strings.Contains(restored, "https://example.com/x") {
		t.Errorf("link was not restored:\n%s", restored)
	}
	if !strings.Contains(restored, "# Heading") {
		t.Errorf("heading marker was not restored:\n%s", restored)
	}
	if !strings.Contains(restored, "- list item") {
		t.Errorf("list marker was not restored:\n%s", restored)
	}
}

func TestProtectorReportsDroppedPlaceholder(t *testing.T) {
	_, p := ProtectDocument("Before.\n\n```go\nx := 1\n```\n\nAfter.")
	_, missing := p.restore("Before.\n\nAfter.")
	if len(missing) != 1 {
		t.Fatalf("missing = %v, want one token", missing)
	}
}

func TestChunkTextKeepsOrderAndHeadings(t *testing.T) {
	heading := "Heading " + strings.Repeat("word ", 4)
	bodyA := strings.Repeat("alpha ", 70)
	bodyB := strings.Repeat("beta ", 70)
	input := "# " + heading + "\n\n" + bodyA + "\n\n" + bodyB
	protected, p := ProtectDocument(input)
	chunks := ChunkText(protected, 100, p)
	if len(chunks) < 2 {
		t.Fatalf("chunks = %d, want at least two", len(chunks))
	}
	joined := strings.Join(chunks, "\n\n")
	if strings.Index(joined, "alpha") > strings.Index(joined, "beta") {
		t.Error("chunk order changed")
	}
	if !strings.Contains(chunks[0], "Heading") {
		t.Errorf("first chunk did not keep the heading: %q", chunks[0])
	}
}

func TestChunkTextSplitsAHugeParagraph(t *testing.T) {
	sentence := "This is a sentence with a few words in it. "
	protected, p := ProtectDocument(strings.Repeat(sentence, 80))
	chunks := ChunkText(protected, 100, p)
	if len(chunks) < 2 {
		t.Fatalf("chunks = %d, want a split", len(chunks))
	}
	for _, chunk := range chunks {
		if len(strings.Fields(chunk)) > 220 {
			t.Errorf("chunk has %d words, want under 220", len(strings.Fields(chunk)))
		}
	}
}

func TestLintIssuesFindsBannedLanguage(t *testing.T) {
	issues := LintIssues("In today's fast-paced world, we leverage robust tools; ultimately it is crucial.")
	joined := strings.Join(issues, "\n")
	for _, want := range []string{"fast-paced world", "leverage", "robust", "crucial", "semicolon"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues missing %q:\n%s", want, joined)
		}
	}
	issues = LintIssues("This is not just a tool, but a platform.")
	if !strings.Contains(strings.Join(issues, "\n"), "not just") {
		t.Errorf("issues = %v, want not-just construction", issues)
	}
	issues = LintIssues("Communication isn't just about talking, it's about listening.")
	if !strings.Contains(strings.Join(issues, "\n"), "isn't just") {
		t.Errorf("issues = %v, want isn't-just construction", issues)
	}
	if issues := LintIssues("A clean sentence with plain words."); len(issues) != 0 {
		t.Errorf("clean sentence produced issues: %v", issues)
	}
}

func TestCleanOutputStripsFenceAndPreamble(t *testing.T) {
	cases := map[string]string{
		"```markdown\nHello.\n```":            "Hello.",
		"Here is the rewritten text:\nHello.": "Hello.",
		"Hello.":                              "Hello.",
	}
	for input, want := range cases {
		if got := cleanOutput(input); got != want {
			t.Errorf("cleanOutput(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRunRewritesMarkdownAndPreservesCode(t *testing.T) {
	dir := t.TempDir()
	input := "# Notes\n\nIn conclusion, this is a crucial test.\n\n```go\nfmt.Println(\"hello\")\n```\n"
	src := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(src, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gen := &fakeGenerator{models: []opencode.ModelRef{{ProviderID: "nvidia", ID: "free"}}}
	op := &Humanize{Client: gen}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"style": "professional", "model": "auto", "chunk_words": 900, "workers": 2},
		Target: core.Target{Dir: outDir},
	}
	if err := op.Validate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	output := result.Artifacts[0]
	if output.Format != "md" || filepath.Base(output.Path) != "notes.md" {
		t.Errorf("output = %+v", output)
	}
	data, err := os.ReadFile(output.Path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "important test.") {
		t.Errorf("humanized prose missing:\n%s", text)
	}
	if !strings.Contains(text, "fmt.Println(\"hello\")") {
		t.Errorf("code block was not preserved:\n%s", text)
	}
	if !strings.Contains(text, "# Notes") {
		t.Errorf("heading was not preserved:\n%s", text)
	}
}

func TestRunFallsBackWhenAModelIsUnavailable(t *testing.T) {
	gen := &fakeGenerator{
		models: []opencode.ModelRef{
			{ProviderID: "nvidia", ID: "bad"},
			{ProviderID: "nvidia", ID: "good"},
		},
		fn: func(model opencode.ModelRef, prompt string) (string, error) {
			if model.ID == "bad" {
				return "", &opencode.APIError{Status: 429, Message: "free limit reached"}
			}
			return rewriteFromPrompt(prompt), nil
		},
	}
	text, err := runOne(t, gen, "alpha beta gamma delta epsilon zeta eta theta. In conclusion, this is crucial.\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "So, this is important") {
		t.Errorf("output = %q", text)
	}
	if gen.count() < 2 {
		t.Errorf("calls = %d, want a fallback call", gen.count())
	}
}

func TestRunRevisesWhenLengthIsOff(t *testing.T) {
	const input = "This is a sentence that needs rewriting today because it has too many words in it. Another sentence follows here to make the paragraph longer and to cross the thirty word threshold for the length check.\n"
	gen := &fakeGenerator{
		models: []opencode.ModelRef{{ProviderID: "nvidia", ID: "free"}},
		fn: func(_ opencode.ModelRef, prompt string) (string, error) {
			if strings.Contains(prompt, "REVISION TASK") {
				return input, nil
			}
			return "Short.", nil
		},
	}
	text, err := runOne(t, gen, input)
	if err != nil {
		t.Fatal(err)
	}
	if text != input {
		t.Errorf("output = %q", text)
	}
	if gen.count() < 2 {
		t.Errorf("calls = %d, want a revision call", gen.count())
	}
}

func TestPolishRemovesBannedLanguage(t *testing.T) {
	input := "In today's fast-paced world, this is crucial and robust. Communication isn't about talking. It's about listening. Teams utilize these tools to unlock potential and collaborate seamlessly."
	got := Polish(input)
	lower := strings.ToLower(got)
	for _, banned := range []string{"fast-paced world", "crucial", "robust", "utilize", "unlock", "isn't about", "seamlessly"} {
		if strings.Contains(lower, banned) {
			t.Errorf("polished text still contains %q: %q", banned, got)
		}
	}
	if !strings.Contains(got, "Communication's about talking. It's about listening.") {
		t.Errorf("structural rewrite failed: %q", got)
	}
	if got := Polish("Communication isn't just talking. It's listening."); got != "Communication is talking and listening." {
		t.Errorf("isn't-just rewrite = %q", got)
	}
	if got := Polish("Communication is about listening, not just talking."); got != "Communication's about listening. It's about talking." {
		t.Errorf("trailing not-just rewrite = %q", got)
	}
	if got := Polish("The future of work is about technology and people."); got != "The future of work's about technology. And people." {
		t.Errorf("is-about split = %q", got)
	}
	if got := Polish("Paragraph 1.1 opens with a claim. The next one follows."); got != "Paragraph 1.1 opens with a claim. The next one follows." {
		t.Errorf("decimal capitalization changed the text: %q", got)
	}
}

func TestRunKeepsOriginalForAChunkThatFails(t *testing.T) {
	alpha := strings.Repeat("alpha ", 220)
	beta := strings.Repeat("beta ", 220)
	gen := &fakeGenerator{
		models: []opencode.ModelRef{{ProviderID: "nvidia", ID: "free"}},
		fn: func(_ opencode.ModelRef, prompt string) (string, error) {
			text := prompt[strings.Index(prompt, "TEXT TO REWRITE:\n")+len("TEXT TO REWRITE:\n"):]
			if strings.Contains(text, "alpha") {
				return "", errors.New("model exploded")
			}
			return rewriteFromPrompt(prompt), nil
		},
	}
	text, err := runOne(t, gen, alpha+"\n\n"+beta+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "alpha") || !strings.Contains(text, "beta") {
		t.Errorf("output lost a chunk: %q", text)
	}
}

func TestRunFailsWhenEveryChunkFails(t *testing.T) {
	gen := &fakeGenerator{
		models: []opencode.ModelRef{{ProviderID: "nvidia", ID: "free"}},
		fn: func(opencode.ModelRef, string) (string, error) {
			return "", errors.New("model exploded")
		},
	}
	_, err := runOne(t, gen, "Some text that should be rewritten.\n")
	if err == nil {
		t.Fatal("Run succeeded when every model failed")
	}
}

func TestRunPreservesChunkOrder(t *testing.T) {
	alpha := strings.Repeat("alpha ", 220)
	beta := strings.Repeat("beta ", 220)
	gamma := strings.Repeat("gamma ", 220)
	gen := &fakeGenerator{
		models: []opencode.ModelRef{{ProviderID: "nvidia", ID: "free"}},
		fn: func(_ opencode.ModelRef, prompt string) (string, error) {
			switch {
			case strings.Contains(prompt, "alpha"):
				time.Sleep(20 * time.Millisecond)
			case strings.Contains(prompt, "beta"):
				time.Sleep(10 * time.Millisecond)
			}
			return rewriteFromPrompt(prompt), nil
		},
	}
	text, err := runOne(t, gen, alpha+"\n\n"+beta+"\n\n"+gamma+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if !(strings.Index(text, "alpha") < strings.Index(text, "beta") &&
		strings.Index(text, "beta") < strings.Index(text, "gamma")) {
		t.Errorf("chunk order changed:\n%.200s", text)
	}
}

func TestValidateRejectsUnsupportedDocument(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "paper.pdf")
	if err := os.WriteFile(src, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	op := &Humanize{Client: &fakeGenerator{}}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"style": "professional", "model": "auto"},
		Target: core.Target{Dir: dir},
	}
	err = op.Validate(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "not a text document") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunSendsNoModelCallForCodeOnlyDocument(t *testing.T) {
	input := "```go\nfmt.Println(\"unchanged\")\n```\n"
	gen := &fakeGenerator{
		models: []opencode.ModelRef{{ProviderID: "nvidia", ID: "free"}},
		fn: func(opencode.ModelRef, string) (string, error) {
			t.Fatal("the model was called for a code-only document")
			return "", nil
		},
	}
	text, err := runOne(t, gen, input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "fmt.Println(\"unchanged\")") {
		t.Errorf("code was not preserved: %q", text)
	}
	if gen.count() != 0 {
		t.Errorf("calls = %d, want 0", gen.count())
	}
}

func TestOutputFormat(t *testing.T) {
	cases := map[string]string{"md": "md", "markdown": "md", "txt": "txt", "": "txt"}
	for input, want := range cases {
		if got := outputFormat(input); got != want {
			t.Errorf("outputFormat(%q) = %q, want %q", input, got, want)
		}
	}
}

func runOne(t *testing.T, gen *fakeGenerator, content string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	op := &Humanize{Client: gen, Timeout: 2 * time.Second}
	req := core.Request{
		Items:  []core.Artifact{art},
		Params: core.Values{"style": "professional", "model": "auto", "chunk_words": 200, "workers": 3},
		Target: core.Target{Dir: outDir},
	}
	result, err := op.Run(context.Background(), req, nopReporter{})
	if err != nil {
		return "", err
	}
	if len(result.Artifacts) != 1 {
		return "", fmt.Errorf("artifacts = %+v", result.Artifacts)
	}
	data, err := os.ReadFile(result.Artifacts[0].Path)
	return string(data), err
}

func TestLintIssuesFlagsDetectorPatterns(t *testing.T) {
	text := "Remote work has fundamentally changed how professionals operate. Organizations need strong communication strategies to help distributed teams collaborate well. Asynchronous tools play a key role here. When teams use these platforms effectively, they can tap into significant potential and encourage innovation while managing hybrid work complexities. Leaders should also prioritize transparency and set clear expectations. The future of work is about technology — and about people. Organizations that embrace this shift will be better positioned to compete. Meetings remain important for team alignment. But too many meetings can hurt productivity and block deep work. Teams should use asynchronous updates to cut down on unnecessary gatherings and build a culture of focused execution. The most effective teams handle this balance with intention."
	stats := analyzeRhythm(text)
	if stats.short != 0 || stats.contractions != 0 || stats.emDashes != 1 {
		t.Fatalf("unexpected baseline stats: %+v", stats)
	}
	issues := strings.Join(LintIssues(text), "\n")
	for _, want := range []string{"under 6 words", "contractions", "shorter, plainer"} {
		if !strings.Contains(issues, want) {
			t.Errorf("issues missing %q:\n%s", want, issues)
		}
	}
}

func TestPolishBreaksUpUniformRhythm(t *testing.T) {
	text := "Remote work has fundamentally changed how professionals operate. Organizations need strong communication strategies to help distributed teams collaborate well. Asynchronous tools play a key role here. When teams use these platforms effectively, they can tap into significant potential and encourage innovation while managing hybrid work complexities. Leaders should also prioritize transparency and set clear expectations. The future of work is about technology — and about people. Organizations that embrace this shift will be better positioned to compete. Meetings remain important for team alignment. But too many meetings can hurt productivity and block deep work. Teams should use asynchronous updates to cut down on unnecessary gatherings and build a culture of focused execution. The most effective teams handle this balance with intention."
	got := Polish(text)
	stats := analyzeRhythm(got)
	if stats.short == 0 {
		t.Errorf("polished text has no short sentence: %+v", stats)
	}
	if stats.maxRun >= 4 {
		t.Errorf("polished text still has a run of similar sentence lengths: %+v", stats)
	}
	if stats.contractions == 0 {
		t.Errorf("polished text has no contractions: %+v", stats)
	}
	if stats.emDashes != 0 {
		t.Errorf("polished text still has em dashes: %+v", stats)
	}
	if stats.avgWordLength > 4.9 {
		t.Errorf("polished text still uses long words: %+v", stats)
	}
	if !strings.Contains(got, "And people.") {
		t.Errorf("polished text did not split the closing claim:\n%s", got)
	}
	if issues := LintIssues(got); len(issues) > 0 {
		t.Errorf("polished text still has lint issues: %v", issues)
	}
}

func TestPolishFormalKeepsContractions(t *testing.T) {
	got := PolishStyle("It is not a problem. The results are not final.", false)
	if strings.Contains(strings.ToLower(got), "isn't") || strings.Contains(strings.ToLower(got), "aren't") {
		t.Errorf("formal polish added contractions: %q", got)
	}
	got = Polish("It is not a problem. The results are not final.")
	if !strings.Contains(got, "isn't") || !strings.Contains(got, "aren't") {
		t.Errorf("casual polish did not add contractions: %q", got)
	}
}
