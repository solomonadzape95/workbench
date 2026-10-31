package tui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/history"
	audioops "github.com/solomonadzape95/workbench/internal/ops/audio"
	"github.com/solomonadzape95/workbench/internal/ops/file"
	humanizeops "github.com/solomonadzape95/workbench/internal/ops/humanize"
	imageops "github.com/solomonadzape95/workbench/internal/ops/image"
	videoops "github.com/solomonadzape95/workbench/internal/ops/video"
	"github.com/solomonadzape95/workbench/internal/pipeline"
	"github.com/solomonadzape95/workbench/internal/runner"
)

func sized(t *testing.T, opts Options) App {
	t.Helper()
	m := New(opts)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(App)
}

func press(m App, key tea.KeyPressMsg) App {
	next, _ := m.Update(key)
	return next.(App)
}

func TestViewAtMinimumSize(t *testing.T) {
	m := sized(t, Options{})
	content := m.View().Content
	if !strings.Contains(content, "Workbench") {
		t.Errorf("view is missing the title:\n%s", content)
	}
	if !strings.Contains(content, "No jobs yet") {
		t.Errorf("view is missing the empty state:\n%s", content)
	}
}

func TestViewTooSmall(t *testing.T) {
	m := New(Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 6})
	content := next.(App).View().Content
	if !strings.Contains(content, "Need") {
		t.Errorf("view does not explain the size requirement:\n%s", content)
	}
	if !strings.Contains(content, "30×10") {
		t.Errorf("view does not state the required size:\n%s", content)
	}
}

func TestViewRendersAtCompactSizes(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {40, 12}, {60, 18}} {
		m := New(Options{})
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		content := next.(App).View().Content
		if strings.Contains(content, "Need 30×10") {
			t.Errorf("%dx%d unexpectedly shows the size fallback:\n%s", size[0], size[1], content)
		}
		if !strings.Contains(content, "Workbench") {
			t.Errorf("%dx%d has no header:\n%s", size[0], size[1], content)
		}
	}
}

func TestHelpToggle(t *testing.T) {
	m := sized(t, Options{})
	m = press(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if !strings.Contains(m.View().Content, "Keys") {
		t.Errorf("help did not open:\n%s", m.View().Content)
	}

	m = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if strings.Contains(m.View().Content, "Keys") {
		t.Errorf("help did not close:\n%s", m.View().Content)
	}
}

func TestWarningBannerCanBeDismissed(t *testing.T) {
	m := sized(t, Options{Warnings: []string{"theme is nonsense"}})
	if !strings.Contains(m.View().Content, "theme is nonsense") {
		t.Errorf("warning is not visible:\n%s", m.View().Content)
	}

	m = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if strings.Contains(m.View().Content, "theme is nonsense") {
		t.Errorf("warning was not dismissed:\n%s", m.View().Content)
	}
}

func TestEngineSummary(t *testing.T) {
	m := sized(t, Options{Registry: engines.NewRegistry(engines.Builtin{})})
	next, _ := m.Update(engineProbeMsg(nil))
	m = next.(App)
	if !strings.Contains(m.homeEngineLine(70), "no external engines") {
		t.Errorf("summary missing empty state: %q", m.homeEngineLine(70))
	}

	next, _ = m.Update(engineProbeMsg{engines.Info{ID: engines.FFmpeg, Available: false}})
	m = next.(App)
	line := m.homeEngineLine(70)
	if !strings.Contains(line, "ffmpeg not found") {
		t.Errorf("summary does not show a missing ffmpeg: %q", line)
	}
	if !strings.Contains(line, "wb doctor") {
		t.Errorf("empty state does not suggest wb doctor: %q", line)
	}
}

func key(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

var (
	enterKey     = tea.KeyPressMsg{Code: tea.KeyEnter}
	escKey       = tea.KeyPressMsg{Code: tea.KeyEscape}
	ctrlRKey     = tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	ctrlRightKey = tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModCtrl}
	ctrlLeftKey  = tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl}
	altRightKey  = tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt}
	altLeftKey   = tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt}
	ctrlLKey     = tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}
	ctrlHKey     = tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl}
)

func TestComposeFormIsGeneratedFromSpec(t *testing.T) {
	ops := core.NewRegistry(imageops.Convert{})
	m := sized(t, Options{Config: config.Defaults(), Operations: ops})

	m = press(m, key('n'))
	if !strings.Contains(m.View().Content, "New job") {
		t.Fatalf("operation picker did not open:\n%s", m.View().Content)
	}
	m = press(m, enterKey)
	content := m.View().Content
	for _, want := range []string{"Convert image", "Input", "Output", "Steps"} {
		if !strings.Contains(content, want) {
			t.Errorf("compose chain is missing %q:\n%s", want, content)
		}
	}
	// ctrl+right moves focus input → output → steps, then enter opens the
	// selected step. Tab is reserved for path completion.
	m = press(m, ctrlRightKey)
	m = press(m, ctrlRightKey)
	m = press(m, enterKey)
	content = m.View().Content
	for _, want := range []string{"Step 1", "Format", "JPEG quality"} {
		if !strings.Contains(content, want) {
			t.Errorf("step editor is missing %q:\n%s", want, content)
		}
	}
}

func TestHumanizeComposeShowsStyleAndModel(t *testing.T) {
	op := &humanizeops.Humanize{}
	ops := core.NewRegistry(op)
	m := sized(t, Options{Config: config.Defaults(), Operations: ops})
	m.compose = newCompose(op, m.opts.Config)
	m.screen = screenCompose
	content := m.View().Content
	for _, want := range []string{"Humanize text", "Style", "Model", "Voice sample"} {
		if !strings.Contains(content, want) {
			t.Errorf("humanize compose is missing %q:\n%s", want, content)
		}
	}
}

func TestComposeRunsCopyJob(t *testing.T) {
	inDir := t.TempDir()
	src := filepath.Join(inDir, "a.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()

	cfg := config.Defaults()
	cfg.Output.Dir = outDir
	ops := core.NewRegistry(file.Copy{})
	jobRunner := runner.New(runner.Options{
		Operations:   ops,
		CacheDir:     t.TempDir(),
		MaxJobs:      2,
		EngineLimits: map[string]int{"internal": 2},
	})
	defer jobRunner.Close()

	opts := Options{
		Config:     cfg,
		Operations: ops,
		Registry:   engines.NewRegistry(engines.Builtin{}),
		Runner:     jobRunner,
	}
	m := sized(t, opts)
	m = press(m, key('n'))
	for i, op := range m.ops {
		if op.Spec().ID == "file.copy" {
			m.opIndex = i
		}
	}
	m = press(m, enterKey)
	if m.screen != screenCompose || m.compose == nil {
		t.Fatalf("not on compose screen: %v", m.screen)
	}

	m.compose.input = inDir
	m.compose.artifacts = []core.Artifact{art}
	m.compose.output = outDir

	next, _ := m.Update(ctrlRKey)
	m = next.(App)
	if m.screen != screenJobs {
		t.Fatalf("screen = %v, want jobs after starting", m.screen)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-jobRunner.Events():
			if !ok {
				t.Fatal("runner closed before the job finished")
			}
			next, _ := m.Update(jobEventMsg{Event: event})
			m = next.(App)
			if _, done := event.(core.JobDone); done {
				goto finished
			}
		case <-deadline:
			t.Fatal("timed out waiting for the job")
		}
	}

finished:
	if data, err := os.ReadFile(filepath.Join(outDir, "a.txt")); err != nil || string(data) != "hello" {
		t.Errorf("copied output = %q, %v; want hello", data, err)
	}
	content := m.View().Content
	if !strings.Contains(content, "Copy file") || !strings.Contains(content, "done") {
		t.Errorf("jobs screen does not show the finished job:\n%s", content)
	}
}

func TestQuitConfirmsWhenJobsAreActive(t *testing.T) {
	m := sized(t, Options{})
	m.jobs = []runner.JobView{{ID: "j1", Name: "Copy file", Status: core.StatusRunning}}
	m = press(m, key('q'))
	if !m.confirmQuit {
		t.Fatal("q did not ask for confirmation with an active job")
	}
	if !strings.Contains(m.View().Content, "Jobs are still running") {
		t.Errorf("confirmation overlay is missing:\n%s", m.View().Content)
	}

	m = press(m, escKey)
	if m.confirmQuit {
		t.Fatal("esc did not cancel the quit confirmation")
	}
}

func TestPreviewScreen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.png")
	img := image.NewRGBA(image.Rect(0, 0, 16, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 12), G: uint8(y * 24), B: 200, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	m := sized(t, Options{})
	next, cmd := m.openPreview(path, screenHome)
	m = next
	if cmd == nil || !strings.Contains(m.View().Content, "Preview") {
		t.Fatalf("preview screen did not open:\n%s", m.View().Content)
	}

	nextModel, _ := m.Update(cmd())
	m = nextModel.(App)
	if !strings.Contains(m.View().Content, "▀") {
		t.Errorf("image preview has no half blocks:\n%s", m.View().Content)
	}

	m = press(m, escKey)
	if m.screen != screenHome || m.preview != nil {
		t.Errorf("esc did not return to the previous screen: screen=%v preview=%v", m.screen, m.preview)
	}
}

func TestFuzzyScore(t *testing.T) {
	if _, ok := fuzzyScore("cv", "Convert image"); !ok {
		t.Error("cv did not match Convert image")
	}
	if _, ok := fuzzyScore("zzz", "Convert image"); ok {
		t.Error("zzz matched Convert image")
	}
	if _, ok := fuzzyScore("", "anything"); !ok {
		t.Error("empty query did not match")
	}
}

func TestPaletteFiltersAndDisables(t *testing.T) {
	ops := core.NewRegistry(imageops.Convert{}, audioops.Compress{})
	m := sized(t, Options{Config: config.Defaults(), Operations: ops})
	p := m.buildPalette(paletteNewCompose)
	p.query = "convert image"
	p.refilter()
	if len(p.items) == 0 || p.items[0].title != "Convert image" {
		t.Fatalf("items = %+v, want Convert image first", p.items)
	}
	if p.items[0].disabled {
		t.Error("internal image operation is disabled")
	}

	p.query = "compress audio"
	p.refilter()
	if len(p.items) == 0 || !p.items[0].disabled {
		t.Errorf("audio op = %+v, want disabled without ffmpeg", p.items)
	}
}

func TestComposeChainBuild(t *testing.T) {
	ops := core.NewRegistry(imageops.Convert{}, imageops.Compress{}, audioops.Compress{})
	cfg := config.Defaults()
	c := newCompose(imageops.Convert{}, cfg)
	c.addStep(imageops.Compress{})
	if ok, why := c.canAppend(audioops.Compress{}); ok {
		t.Errorf("audio op can append to an image chain: %s", why)
	}
	if err := c.validateStatic(ops); err != nil {
		t.Fatal(err)
	}
	p := c.buildPipeline("shots-clean")
	if len(p.Steps) != 2 || p.Steps[1].Input.From != "previous" {
		t.Errorf("pipeline = %+v", p)
	}
	if err := p.Validate(ops); err != nil {
		t.Fatal(err)
	}
}

func TestComposeLoadsPipeline(t *testing.T) {
	ops := core.NewRegistry(imageops.Convert{}, imageops.Compress{})
	p := core.Pipeline{
		Name: "shots-clean",
		Steps: []core.Step{
			{Op: "image.convert", Params: core.Values{"format": "jpeg", "quality": 85}},
			{Op: "image.compress", Input: core.StepInput{From: "previous"}, Params: core.Values{"quality": 70}},
		},
	}
	c := newComposeFromPipeline(p, ops, config.Defaults())
	if len(c.steps) != 2 || c.savedName != "shots-clean" {
		t.Fatalf("compose = %+v", c)
	}
	if c.steps[0].params.String("format") != "jpeg" {
		t.Errorf("format = %q", c.steps[0].params.String("format"))
	}
	if err := c.validateStatic(ops); err != nil {
		t.Fatal(err)
	}
}

func TestSavePipelinePrompt(t *testing.T) {
	store := pipeline.NewStore(t.TempDir())
	ops := core.NewRegistry(imageops.Convert{}, imageops.Compress{})
	m := sized(t, Options{Config: config.Defaults(), Operations: ops, Pipelines: store})
	m.compose = newCompose(imageops.Convert{}, m.opts.Config)
	m.compose.addStep(imageops.Compress{})
	m.prompt = newSavePrompt(m.compose)
	m.prompt.value = "shots-clean"

	next, _ := m.submitPrompt()
	m = next.(App)
	if m.prompt != nil {
		t.Fatalf("prompt did not close: %+v", m.prompt)
	}
	loaded, err := store.Load("shots-clean")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Steps) != 2 {
		t.Errorf("saved steps = %d, want 2", len(loaded.Steps))
	}
	if m.compose.savedName != "shots-clean" {
		t.Errorf("savedName = %q", m.compose.savedName)
	}
}

func TestRenameAndDuplicateSavedPipeline(t *testing.T) {
	store := pipeline.NewStore(t.TempDir())
	ops := core.NewRegistry(imageops.Convert{}, imageops.Compress{})
	m := sized(t, Options{Config: config.Defaults(), Operations: ops, Pipelines: store})
	m.compose = newCompose(imageops.Convert{}, m.opts.Config)
	m.compose.addStep(imageops.Compress{})
	m.compose.savedName = "shots-clean"
	if err := store.Save(m.compose.buildPipeline("shots-clean")); err != nil {
		t.Fatal(err)
	}

	m.prompt = newRenamePrompt(m.compose)
	m.prompt.value = "shots-renamed"
	next, _ := m.submitPrompt()
	m = next.(App)
	if _, err := store.Load("shots-renamed"); err != nil {
		t.Fatalf("renamed pipeline: %v", err)
	}
	if _, err := store.Load("shots-clean"); err == nil {
		t.Fatal("old pipeline still loads after rename")
	}
	if m.compose.savedName != "shots-renamed" {
		t.Fatalf("savedName = %q", m.compose.savedName)
	}

	m.prompt = newDuplicatePrompt(m.compose)
	m.prompt.value = "shots-copy"
	next, _ = m.submitPrompt()
	m = next.(App)
	if _, err := store.Load("shots-copy"); err != nil {
		t.Fatalf("duplicated pipeline: %v", err)
	}
	if _, err := store.Load("shots-renamed"); err != nil {
		t.Fatalf("source pipeline disappeared after duplicate: %v", err)
	}
	if m.compose.savedName != "shots-copy" {
		t.Fatalf("savedName = %q", m.compose.savedName)
	}
}

func TestComposeChainRuns(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.png")
	writeTestPNG(t, src)
	art, err := core.Probe(src)
	if err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()

	ops := core.NewRegistry(imageops.Convert{}, imageops.Compress{})
	jobRunner := runner.New(runner.Options{
		Operations:   ops,
		CacheDir:     t.TempDir(),
		MaxJobs:      2,
		EngineLimits: map[string]int{"internal": 2},
	})
	defer jobRunner.Close()

	cfg := config.Defaults()
	cfg.Output.Dir = outDir
	m := sized(t, Options{Config: cfg, Operations: ops, Runner: jobRunner})
	m.compose = newCompose(imageops.Convert{}, cfg)
	m.compose.addStep(imageops.Compress{})
	m.compose.input = src
	m.compose.artifacts = []core.Artifact{art}
	m.compose.output = outDir
	m.screen = screenCompose

	next, _ := m.runCompose()
	m = next.(App)
	if m.screen != screenJobs {
		t.Fatalf("screen = %v, want jobs (err: %+v)", m.screen, m.compose)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-jobRunner.Events():
			if !ok {
				t.Fatal("runner closed early")
			}
			next, _ := m.Update(jobEventMsg{Event: event})
			m = next.(App)
			if _, done := event.(core.JobDone); done {
				goto finished
			}
		case <-deadline:
			t.Fatal("timed out waiting for the chain")
		}
	}

finished:
	if _, err := os.Stat(filepath.Join(outDir, "a.png")); err != nil {
		t.Errorf("chain output missing: %v", err)
	}
	if job, ok := m.selectedJobView(); !ok || job.Status != core.StatusDone {
		t.Errorf("job = %+v, want done", job)
	}
}

func TestResolveLast(t *testing.T) {
	art := core.Artifact{Path: "/tmp/a.png", Kind: core.Image, Format: "png"}
	msg := resolveInputs(7, "@last", nil, []core.Artifact{art})()
	resolved, ok := msg.(resolvedMsg)
	if !ok || len(resolved.artifacts) != 1 || resolved.version != 7 {
		t.Fatalf("resolved = %+v", msg)
	}
	msg = resolveInputs(8, "@last", nil, nil)()
	if resolved, ok := msg.(resolvedMsg); !ok || resolved.err == nil {
		t.Errorf("empty @last = %+v, want an error", msg)
	}
}

func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 10), B: 120, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOnboardingSavesConfig(t *testing.T) {
	dir := t.TempDir()
	paths := config.Paths{ConfigFile: filepath.Join(dir, "config.toml")}
	cfg := config.Defaults()
	m := sized(t, Options{Config: cfg, Paths: paths, FirstRun: true})
	if m.screen != screenOnboarding || m.onboarding == nil {
		t.Fatalf("screen = %v, want onboarding", m.screen)
	}
	if !strings.Contains(m.View().Content, "Welcome to Workbench") {
		t.Errorf("onboarding view missing:\n%s", m.View().Content)
	}

	// Cursor starts on Media; disable it, leave Documents enabled, then finish.
	m = press(m, tea.KeyPressMsg{Code: ' ', Text: " "})
	m = press(m, enterKey)
	if m.screen != screenHome || m.onboarding != nil {
		t.Fatalf("screen = %v after finish", m.screen)
	}
	loaded, warnings := config.Load(paths.ConfigFile)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	foundFFmpeg, foundPandoc := false, false
	for _, id := range loaded.Engines.Enabled {
		if id == "ffmpeg" {
			foundFFmpeg = true
		}
		if id == "pandoc" {
			foundPandoc = true
		}
	}
	if foundFFmpeg {
		t.Errorf("media pack was disabled but ffmpeg is enabled: %v", loaded.Engines.Enabled)
	}
	if !foundPandoc {
		t.Errorf("documents pack was enabled but pandoc is missing: %v", loaded.Engines.Enabled)
	}
}

func TestEnginesView(t *testing.T) {
	m := sized(t, Options{})
	next, _ := m.Update(engineProbeMsg([]engines.Info{
		{ID: engines.Internal, Name: "internal", Available: true, Version: "built-in"},
		{ID: engines.FFmpeg, Name: "ffmpeg", Available: false, Hint: "brew install ffmpeg"},
	}))
	m = next.(App)
	m.screen = screenEngines
	content := m.View().Content
	if !strings.Contains(content, "Engines") || !strings.Contains(content, "missing") {
		t.Errorf("engines view =\\n%s", content)
	}
}

func TestURLComposeFlow(t *testing.T) {
	ops := core.NewRegistry(videoops.Download{}, videoops.Compress{})
	m := sized(t, Options{Config: config.Defaults(), Operations: ops})
	m.compose = newCompose(videoops.Download{}, m.opts.Config)
	m.screen = screenCompose
	if !m.compose.isURLFlow() {
		t.Fatal("download compose is not in URL mode")
	}
	m.compose.input = "https://example.com/v, https://example.com/w"
	m.compose.afterTextChange(m.opts.Config)
	if len(m.compose.artifacts) != 2 {
		t.Fatalf("artifacts = %+v, want two URLs", m.compose.artifacts)
	}
	if m.compose.artifacts[0].Name() != "https://example.com/v" {
		t.Errorf("label = %q", m.compose.artifacts[0].Name())
	}
	if !strings.Contains(m.View().Content, "URL") {
		t.Errorf("compose view missing URL field:\n%s", m.View().Content)
	}
	if ok, why := m.compose.canAppend(videoops.Compress{}); !ok {
		t.Errorf("cannot append compress after download: %s", why)
	}
	if ok, _ := m.compose.canAppend(videoops.Download{}); ok {
		t.Error("a second download can be appended")
	}
}

func TestURLComposeRejectsInvalidURL(t *testing.T) {
	ops := core.NewRegistry(videoops.Download{})
	m := sized(t, Options{Config: config.Defaults(), Operations: ops})
	m.compose = newCompose(videoops.Download{}, m.opts.Config)
	m.compose.input = "ftp://example.com/v"
	m.compose.afterTextChange(m.opts.Config)
	if m.compose.inputErr == nil || len(m.compose.artifacts) != 0 {
		t.Errorf("inputErr = %v, artifacts = %+v", m.compose.inputErr, m.compose.artifacts)
	}
}

func TestHomeAndJobsLayout(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	content := m.View().Content
	if !strings.Contains(content, "Workbench") {
		t.Errorf("home wordmark missing:\n%s", content)
	}
	if !strings.Contains(content, "New job") || !strings.Contains(content, "Help") {
		t.Errorf("home actions missing:\n%s", content)
	}

	m.jobs = []runner.JobView{{
		ID:      "j1",
		Name:    "Compress file",
		Status:  core.StatusDone,
		Items:   []runner.ItemView{{ID: "i1", Input: core.Artifact{Path: "/tmp/a.txt"}, Status: core.ItemStatusDone}},
		Metrics: core.Metrics{BytesIn: 100, BytesOut: 25},
		Samples: []int{0, 40, 100},
		Target:  core.Target{Dir: t.TempDir()},
	}}
	m.selectedJob = "j1"
	m.screen = screenJobs
	content = m.View().Content
	if !strings.Contains(content, "╭─") || !strings.Contains(content, "Jobs") || !strings.Contains(content, "Job detail") {
		t.Errorf("jobs panels missing:\n%s", content)
	}
	if !strings.Contains(content, "space saved 75%") {
		t.Errorf("saved badge missing:\n%s", content)
	}
}

func TestHomeLogoAboveWordmarkAndCentered(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	lines := strings.Split(xansi.Strip(m.View().Content), "\n")
	markLine, wordLine := -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case "▓▓▓▓▓▓▓▓▓":
			markLine = i
		case logoWide[0]:
			wordLine = i
		}
	}
	if markLine < 0 || wordLine != markLine+2 {
		t.Fatalf("logo mark %d is not above a blank line and wordmark %d:\n%s", markLine, wordLine, strings.Join(lines, "\n"))
	}
	if strings.TrimSpace(lines[markLine+1]) != "" {
		t.Fatalf("logo mark does not have breathing room: %q", lines[markLine+1])
	}
	if left := strings.Index(lines[markLine], "▓"); left < 34 || left > 37 {
		t.Fatalf("logo mark is not centered: left=%d line=%q", left, lines[markLine])
	}
}

func TestSparkline(t *testing.T) {
	got := sparkline([]int{0, 50, 100}, 3)
	if len([]rune(got)) != 3 {
		t.Errorf("sparkline = %q", got)
	}
	if sparkline(nil, 4) != "" {
		t.Error("empty sparkline should be empty")
	}
}

func TestPasteIntoURLCompose(t *testing.T) {
	ops := core.NewRegistry(videoops.Download{})
	m := sized(t, Options{Config: config.Defaults(), Operations: ops})
	m.compose = newCompose(videoops.Download{}, m.opts.Config)
	m.screen = screenCompose

	next, _ := m.insertText("https://example.com/a\nhttps://example.com/b")
	m = next.(App)
	if len(m.compose.artifacts) != 2 {
		t.Fatalf("pasted URLs = %+v", m.compose.artifacts)
	}
	if !strings.Contains(m.compose.input, ", ") {
		t.Errorf("multi-line paste was not joined: %q", m.compose.input)
	}
}

func TestPasteIntoPaletteAndPrompt(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	m.palette = m.buildPalette(paletteNewCompose)
	next, _ := m.insertText("download\nvideo")
	m = next.(App)
	if m.palette.query != "download video" {
		t.Errorf("palette query = %q", m.palette.query)
	}

	m.palette = nil
	m.prompt = &prompt{action: promptSavePipeline, title: "Save", value: ""}
	next, _ = m.insertText("my\npipeline")
	m = next.(App)
	if m.prompt.value != "my pipeline" {
		t.Errorf("prompt value = %q", m.prompt.value)
	}
}

func TestBracketedPasteMsg(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	m.palette = m.buildPalette(paletteNewCompose)
	next, _ := m.Update(tea.PasteMsg{Content: "convert image"})
	m = next.(App)
	if m.palette.query != "convert image" {
		t.Errorf("query = %q after PasteMsg", m.palette.query)
	}
}

func TestEditorOpensEditsAndSaves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(path, []byte("# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := sized(t, Options{Config: config.Defaults(), Paths: config.Paths{ConfigFile: filepath.Join(dir, "config.toml")}})
	next, _ := m.openEditor(path, screenHome)
	m = next.(App)
	if m.screen != screenEditor || m.editor == nil {
		t.Fatalf("editor did not open: %v", m.screen)
	}
	view := m.View()
	if view.Cursor == nil {
		t.Fatal("editor cursor is not visible")
	}
	lines := strings.Split(xansi.Strip(view.Content), "\n")
	codeLine, codeX := -1, -1
	for i, line := range lines {
		if j := strings.Index(line, "# Notes"); j >= 0 && strings.Contains(line, "  1 │") {
			codeLine = i
			codeX = lipgloss.Width(line[:j])
			break
		}
	}
	if codeLine < 0 {
		t.Fatalf("editor code row not found:\n%s", strings.Join(lines, "\n"))
	}
	if view.Cursor.X != codeX || view.Cursor.Y != codeLine {
		t.Errorf("cursor = %d,%d; edit point is %d,%d", view.Cursor.X, view.Cursor.Y, codeX, codeLine)
	}
	m = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = press(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = press(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.editor == nil {
		t.Fatal("ctrl+s closed the editor")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\nx") {
		t.Errorf("saved file = %q", data)
	}
}

func TestEditorWarnsBeforeDiscarding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	m := sized(t, Options{Config: config.Defaults()})
	next, _ := m.openEditor(path, screenHome)
	m = next.(App)
	m = press(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.editor == nil || !m.editor.pendingClose {
		t.Fatal("dirty editor closed without confirmation")
	}
	m = press(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if m.screen != screenHome || m.editor != nil {
		t.Error("discard did not close the editor")
	}
}

func TestThemePickerAppliesTheme(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()
	m := sized(t, Options{Config: cfg, Paths: config.Paths{ConfigFile: filepath.Join(dir, "config.toml")}})
	next, _ := m.openThemePicker(screenHome)
	m = next.(App)
	if m.screen != screenThemes || m.themePicker == nil {
		t.Fatal("theme picker did not open")
	}
	found := false
	for i, choice := range m.themePicker.items {
		if choice.name == "tokyonight" {
			m.themePicker.cursor = i
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("tokyonight not listed: %+v", m.themePicker.items)
	}
	next, _ = m.handleThemeKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(App)
	if m.screen != screenHome || m.opts.Config.General.ThemeName != "tokyonight" {
		t.Errorf("theme not applied: screen=%v cfg=%+v", m.screen, m.opts.Config.General)
	}
	if m.theme.name != "tokyonight" {
		t.Errorf("theme name = %q", m.theme.name)
	}
}

func TestHomeActionsOpenEditorPromptAndThemes(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	m = press(m, tea.KeyPressMsg{Code: 'e', Text: "e"})
	if m.prompt == nil || m.prompt.action != promptOpenFile {
		t.Fatal("home e did not open the file prompt")
	}
	m = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	next, _ := m.openThemePicker(screenHome)
	m = next.(App)
	if m.screen != screenThemes {
		t.Fatalf("themes screen = %v", m.screen)
	}
}

func TestProgressBarUsesDotMatrix(t *testing.T) {
	m := sized(t, Options{})
	bar := m.progressBarStyle(50, 100, 10, m.theme.accent)
	if !strings.Contains(bar, "⣿") || !strings.Contains(bar, "⣀") {
		t.Fatalf("bar = %q, want filled and empty dot cells", bar)
	}
	if lipgloss.Width(bar) != 10 {
		t.Errorf("bar width = %d, want 10", lipgloss.Width(bar))
	}
}

func TestComposePathSuggestions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "photo.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	ops := core.NewRegistry(file.Copy{})
	m := sized(t, Options{Config: cfg, Operations: ops})
	m.compose = newCompose(file.Copy{}, cfg)
	m.screen = screenCompose
	m.compose.focus = focusInput
	m.compose.input = dir + "/ph"
	m.compose.afterTextChange(cfg)
	if len(m.compose.suggestions) != 1 || filepath.Base(m.compose.suggestions[0]) != "photo.png" {
		t.Fatalf("suggestions = %v", m.compose.suggestions)
	}
	m.compose.suggestIndex = 0
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(App)
	if m.compose.input != dir+"/photo.png" {
		t.Errorf("accepted input = %q", m.compose.input)
	}
}

func TestSettingsOutputFolder(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()
	m := sized(t, Options{Config: cfg, Paths: config.Paths{ConfigFile: filepath.Join(dir, "config.toml")}})
	next, _ := m.openSettings(screenHome)
	m = next.(App)
	next, _ = m.runSettingsItem(m.settingsItems()[0])
	m = next.(App)
	m.prompt.value = "~/Pictures"
	next, _ = m.submitPrompt()
	m = next.(App)
	if m.screen != screenSettings || m.opts.Config.Output.Dir != "~/Pictures" {
		t.Errorf("settings not saved: screen=%v cfg=%+v", m.screen, m.opts.Config.Output)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Pictures") {
		t.Errorf("config = %s", data)
	}
}

func TestNewJobSearch(t *testing.T) {
	cfg := config.Defaults()
	ops := core.NewRegistry(file.Copy{}, imageops.Convert{})
	m := sized(t, Options{Config: cfg, Operations: ops})
	m = press(m, key('n'))
	m = press(m, key('c'))
	m = press(m, key('o'))
	m = press(m, key('p'))
	m = press(m, key('y'))
	if len(m.filteredOps()) != 1 || m.filteredOps()[0].Spec().ID != "file.copy" {
		t.Fatalf("filtered = %v", m.filteredOps())
	}
	m = press(m, enterKey)
	if m.screen != screenCompose || m.compose == nil || m.compose.steps[0].spec.ID != "file.copy" {
		t.Fatal("search selection did not open compose")
	}
}

func TestCompoundPresetBuildsChain(t *testing.T) {
	cfg := config.Defaults()
	ops := core.NewRegistry(imageops.Convert{}, imageops.Compress{})
	m := sized(t, Options{Config: cfg, Operations: ops})
	var web jobPreset
	for _, preset := range jobPresets {
		if preset.name == "Web image" {
			web = preset
			break
		}
	}
	p, ok := m.presetPipeline(web)
	if !ok || len(p.Steps) != 2 || p.Steps[0].Op != "image.convert" || p.Steps[1].Op != "image.compress" {
		t.Fatalf("preset = %+v, ok=%v", p, ok)
	}
}

func TestPaletteFitsAndScrolls(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	m.palette = m.buildPalette(paletteNewCompose)
	view := xansi.Strip(m.paletteView(20))
	if h := lipgloss.Height(view); h > 20 {
		t.Fatalf("palette card height = %d, want <= 20:\n%s", h, view)
	}
	if !strings.Contains(view, "╰") {
		t.Fatalf("palette bottom border is clipped:\n%s", view)
	}

	m.palette.cursor = len(m.palette.items) - 1
	view = xansi.Strip(m.paletteView(20))
	last := m.palette.items[m.palette.cursor].title
	if !strings.Contains(view, truncate(last, 30)) {
		t.Fatalf("scrolled palette does not show the selected item %q:\n%s", last, view)
	}
}

func TestHelpFitsAtMinimumSize(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	view := xansi.Strip(m.helpView())
	if h := lipgloss.Height(view); h > 20 {
		t.Fatalf("help card height = %d, want <= 20:\n%s", h, view)
	}
}

func TestPalettePrefersLiteralMatches(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(imageops.Convert{}, imageops.Crop{})})
	p := m.buildPalette(paletteNewCompose)
	p.query = "crop"
	p.refilter()
	if len(p.items) != 1 || p.items[0].title != "Crop image" {
		t.Fatalf("crop returned %d items: %+v", len(p.items), p.items)
	}
	p.query = "cv"
	p.refilter()
	if len(p.items) == 0 || !strings.Contains(p.items[0].title, "Convert") {
		t.Fatalf("fuzzy abbreviation did not match Convert: %+v", p.items[:min(3, len(p.items))])
	}
}

func TestSelectedJobKeepsProgressColor(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	job := runner.JobView{ID: "j1", Name: "Copy", Status: core.StatusDone, Items: []runner.ItemView{{ID: "i", Status: core.ItemStatusDone}}}
	m.jobs = []runner.JobView{job}
	m.selectedJob = "j1"
	line := m.jobLine(job, 60, true)
	successPrefix := strings.SplitN(m.theme.success.Render("⣿"), "⣿", 2)[0]
	if !strings.Contains(line, successPrefix+"⣿") {
		t.Fatalf("selected done bar lost its success color:\n%q", line)
	}
}

func TestJobMetricsExplainSizeAndProgress(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	job := runner.JobView{
		Name:    "Compress file",
		Metrics: core.Metrics{BytesIn: 55 * 1024 * 1024, BytesOut: 4 * 1024 * 1024},
		Samples: []int{0, 25, 100},
	}
	line := xansi.Strip(m.jobMetricsLine(job))
	for _, want := range []string{"size", "space saved", "progress"} {
		if !strings.Contains(line, want) {
			t.Errorf("metrics missing %q: %q", want, line)
		}
	}
}

func TestGhosttyAndVSCodeThemes(t *testing.T) {
	monokai, ok := builtinTheme("monokai")
	if !ok || monokai.Background != "#0c0c0c" || monokai.Palette[2] != "#98e024" {
		t.Fatalf("monokai palette = %+v, ok=%v", monokai, ok)
	}
	vscode, ok := builtinTheme("vscode")
	if !ok || vscode.Background != "#1e1e1e" || vscode.Accent != "#007acc" {
		t.Fatalf("vscode palette = %+v, ok=%v", vscode, ok)
	}
	th := newThemeFromPalette(vscode)
	if th.accentHex != "#007acc" {
		t.Errorf("vscode accent = %q, want #007acc", th.accentHex)
	}
}

func TestComposeTabCompletesAndCtrlArrowsFocus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(path, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(imageops.Convert{})})
	m.compose = newCompose(imageops.Convert{}, m.opts.Config)
	m.screen = screenCompose
	m.compose.focus = focusInput
	m.compose.input = filepath.Join(dir, "ph")
	m.compose.afterTextChange(m.opts.Config)
	if len(m.compose.suggestions) == 0 {
		t.Fatal("expected a path suggestion")
	}
	m = press(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.compose.input != path {
		t.Errorf("tab completion = %q, want %q", m.compose.input, path)
	}
	if m.compose.focus != focusInput {
		t.Errorf("tab changed focus to %v", m.compose.focus)
	}
	m = press(m, ctrlRightKey)
	if m.compose.focus != focusOutput {
		t.Errorf("ctrl+right focus = %v, want output", m.compose.focus)
	}
	m = press(m, ctrlLeftKey)
	if m.compose.focus != focusInput {
		t.Errorf("ctrl+left focus = %v, want input", m.compose.focus)
	}
	m = press(m, altRightKey)
	if m.compose.focus != focusOutput {
		t.Errorf("alt+right focus = %v, want output", m.compose.focus)
	}
	m = press(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.compose.focus != focusSteps {
		t.Errorf("tab focus = %v, want steps", m.compose.focus)
	}
	m = press(m, altLeftKey)
	if m.compose.focus != focusOutput {
		t.Errorf("alt+left focus = %v, want output", m.compose.focus)
	}
	m = press(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.compose.focus != focusInput {
		t.Errorf("shift+tab focus = %v, want input", m.compose.focus)
	}
}

func TestStepEditorFocusAndWordKeys(t *testing.T) {
	c := newCompose(imageops.Convert{}, config.Defaults())
	c.openStep(0)
	if len(c.fields) < 2 {
		t.Fatalf("fields = %d, want at least 2", len(c.fields))
	}
	c.handleFieldKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if c.fieldFocus != 1 {
		t.Errorf("tab field focus = %d, want 1", c.fieldFocus)
	}
	c.handleFieldKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if c.fieldFocus != 0 {
		t.Errorf("shift+tab field focus = %d, want 0", c.fieldFocus)
	}
	c.handleFieldKey(altRightKey)
	if c.fieldFocus != 0 {
		t.Errorf("alt+right should move the cursor, not the field: focus = %d", c.fieldFocus)
	}
	c.handleFieldKey(altLeftKey)
	if c.fieldFocus != 0 {
		t.Errorf("alt+left should move the cursor, not the field: focus = %d", c.fieldFocus)
	}
}

func TestOperationFilterAcceptsQAndQuestionMark(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(imageops.Convert{})})
	m.screen = screenOperations
	m = press(m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = press(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if m.opQuery != "q?" {
		t.Fatalf("filter = %q, want q?", m.opQuery)
	}
	if m.showHelp {
		t.Fatal("question mark opened help instead of filtering")
	}
}

func TestJobsFilterKeepsTypingLocal(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	m.screen = screenJobs
	now := time.Now()
	m.jobs = []runner.JobView{
		{ID: "job-1", Name: "resize photos", Status: core.StatusDone, QueuedAt: now},
		{ID: "job-2", Name: "convert docs", Status: core.StatusDone, QueuedAt: now.Add(-time.Minute)},
	}
	m.selectedJob = "job-1"
	m = press(m, tea.KeyPressMsg{Code: '/', Text: "/"})
	m = press(m, tea.KeyPressMsg{Code: 'c', Text: "c"})
	if !m.jobFiltering || m.jobQuery != "c" {
		t.Fatalf("filtering = %v query = %q", m.jobFiltering, m.jobQuery)
	}
	if got := len(m.filteredJobs()); got != 1 {
		t.Fatalf("filtered jobs = %d, want 1", got)
	}
	m = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.jobFiltering || m.jobQuery != "" {
		t.Fatal("escape did not clear the job filter")
	}
}

func TestDeleteStepRequiresConfirmation(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(imageops.Convert{})})
	m.compose = newCompose(imageops.Convert{}, m.opts.Config)
	m.screen = screenCompose
	m.compose.focus = focusSteps
	m = press(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if m.confirm == nil || len(m.compose.steps) != 1 {
		t.Fatal("delete step did not ask for confirmation")
	}
	m = press(m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if m.confirm != nil || len(m.compose.steps) != 0 {
		t.Fatalf("confirm = %v steps = %d", m.confirm, len(m.compose.steps))
	}
}

func TestComposeEscapeRequiresConfirmation(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults(), Operations: core.NewRegistry(imageops.Convert{})})
	m.compose = newCompose(imageops.Convert{}, m.opts.Config)
	m.compose.input = "/tmp/a.png"
	m.compose.afterTextChange(m.opts.Config)
	m.screen = screenCompose
	m = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.confirm == nil || m.screen != screenCompose {
		t.Fatal("escape did not ask before discarding the pipeline")
	}
	m = press(m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if m.compose != nil || m.screen != screenOperations {
		t.Fatalf("compose = %v screen = %v", m.compose, m.screen)
	}
}

func TestHistoryLoadsIntoJobsScreen(t *testing.T) {
	store := history.New(t.TempDir())
	rec := runner.Record{Version: 1, View: runner.JobView{
		ID: "job-old", Name: "old resize", Status: core.StatusDone,
		QueuedAt: time.Now().Add(-time.Hour), FinishedAt: time.Now().Add(-59 * time.Minute),
		Pipeline: core.Pipeline{Name: "old resize", Steps: []core.Step{{Op: "image.convert"}}},
	}}
	if err := store.Append(rec); err != nil {
		t.Fatal(err)
	}
	m := sized(t, Options{Config: config.Defaults(), History: store})
	if len(m.jobs) != 1 || !m.jobs[0].Historical {
		t.Fatalf("jobs = %+v", m.jobs)
	}
	if m.selectedJob != "job-old" {
		t.Fatalf("selected = %q", m.selectedJob)
	}
}

func TestJobLogOpensWithLAndSearches(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	m.screen = screenJobs
	m.jobs = []runner.JobView{{
		ID: "job-1", Name: "log job", Status: core.StatusFailed, QueuedAt: time.Now(),
		Logs: []string{"alpha", "beta", "gamma beta"},
	}}
	m.selectedJob = "job-1"
	m = press(m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.logView == nil {
		t.Fatal("l did not open the log")
	}
	m = press(m, tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "beta" {
		m = press(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.logView == nil || m.logView.offset != 1 {
		t.Fatalf("search offset = %v, want 1", m.logView)
	}
	m = press(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.logView.offset != 2 {
		t.Fatalf("next match offset = %d, want 2", m.logView.offset)
	}
	m = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.logView != nil {
		t.Fatal("escape did not close the log")
	}
}

func TestJobSortModes(t *testing.T) {
	m := sized(t, Options{Config: config.Defaults()})
	now := time.Now()
	m.jobs = []runner.JobView{
		{ID: "1", Name: "beta", Status: core.StatusDone, QueuedAt: now.Add(-time.Minute)},
		{ID: "2", Name: "alpha", Status: core.StatusRunning, QueuedAt: now},
		{ID: "3", Name: "gamma", Status: core.StatusFailed, QueuedAt: now.Add(-2 * time.Minute)},
	}
	m.jobSort = 0
	if got := m.filteredJobs(); got[0].Name != "alpha" {
		t.Fatalf("newest order = %v", got)
	}
	m.jobSort = 1
	if got := m.filteredJobs(); got[0].Name != "alpha" || got[2].Name != "gamma" {
		t.Fatalf("name order = %v", got)
	}
	m.jobSort = 2
	if got := m.filteredJobs(); got[0].Status != core.StatusDone {
		t.Fatalf("status order = %v", got)
	}
}

func TestReviewRejectsSelectedOutputToTrash(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(first, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := sized(t, Options{
		Config: config.Defaults(),
		Paths:  config.Paths{DataDir: filepath.Join(dir, "data")},
	})
	m.jobs = []runner.JobView{{
		ID: "review-job", Name: "Review", Status: core.StatusDone,
		Items: []runner.ItemView{
			{Input: core.Artifact{Size: 100}, Status: core.ItemStatusDone, Outputs: []core.Artifact{{Path: first, Size: 5}}},
			{Input: core.Artifact{Size: 200}, Status: core.ItemStatusDone, Outputs: []core.Artifact{{Path: second, Size: 6}}},
		},
	}}
	m.selectedJob = "review-job"
	m = m.openReview()
	if m.screen != screenReview || m.review == nil || len(m.review.entries) != 2 {
		t.Fatalf("review = %+v screen = %v", m.review, m.screen)
	}
	m = press(m, tea.KeyPressMsg{Code: ' ', Text: " "})
	if m.review.entries[0].keep {
		t.Fatal("space did not reject the first output")
	}
	m = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.screen != screenJobs || m.review != nil {
		t.Fatalf("screen = %v review = %+v", m.screen, m.review)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("rejected output still exists: %v", err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("kept output removed: %v", err)
	}
}
