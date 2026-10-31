package humanize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/errgroup"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/opencode"
)

const defaultCallTimeout = 90 * time.Second

var styleOptions = []core.Option{
	{Value: "professional", Label: "Professional"},
	{Value: "casual", Label: "Casual"},
	{Value: "academic", Label: "Academic"},
	{Value: "blog", Label: "Blog"},
	{Value: "social", Label: "Social"},
	{Value: "narrative", Label: "Narrative"},
}

var modelOptions = []core.Option{
	{Value: "auto", Label: "Auto (free fallback)"},
	{Value: "nvidia/nvidia/nemotron-3-ultra-550b-a55b", Label: "Nemotron 3 Ultra (free)"},
	{Value: "nvidia/nvidia/nemotron-3.5-lightning-30b-a3b", Label: "Nemotron Lightning (free)"},
	{Value: "nvidia/nvidia/nemotron-3.5-super-vl-120b-a12b", Label: "Nemotron Super VL (free)"},
	{Value: "nvidia/minimaxai/minimax-m3.1-flash", Label: "MiniMax M3.1 Flash (free)"},
	{Value: "nvidia/poolside/laguna-xs-2.1", Label: "Laguna XS (free)"},
	{Value: "nvidia/openai/gpt-oss-20b", Label: "GPT OSS 20B (free)"},
	{Value: "nvidia/stepfun-ai/step-5-preview", Label: "Step 5 Preview (free)"},
	{Value: "opencode/deepseek-v4.1-flash-free", Label: "DeepSeek V4.1 Flash (free)"},
	{Value: "opencode/glm-5.3-flash-free", Label: "GLM-5.3 Flash (free)"},
	{Value: "opencode/mimo-v2.6-flash-free", Label: "MiMo V2.6 Flash (free)"},
	{Value: "opencode/big-pickle", Label: "Big Pickle (free)"},
	{Value: "opencode-go/iceman-contributor-free", Label: "Iceman Contributor (free)"},
	{Value: "opencode-go/beam-free", Label: "Beam (free)"},
}

var humanizeSpec = core.Spec{
	ID:      "doc.humanize",
	Title:   "Humanize text",
	Summary: "Rewrite text, Markdown, or a DOCX while keeping its figures and tables",
	Domain:  core.Document,
	Accepts: []core.Accept{{Kind: core.Document, Formats: []string{"txt", "md", "markdown", "docx"}}},
	Params: []core.ParamSpec{
		{
			Key: "style", Label: "Style", Type: core.ParamEnum, Default: "professional",
			Options: styleOptions,
			Help:    "The rules file supplies the voice for each style.",
		},
		{
			Key: "model", Label: "Model", Type: core.ParamEnum, Default: "auto",
			Options: modelOptions,
			Help:    "Auto tries free models in order and falls back when one is limited or unavailable.",
		},
		{
			Key: "voice", Label: "Voice sample", Type: core.ParamPath, Default: "", Advanced: true,
			Help: "Optional text file with a sample of your writing.",
		},
		{
			Key: "chunk_words", Label: "Chunk size", Type: core.ParamInt, Default: 900,
			Min: floatPtr(200), Max: floatPtr(4000), Advanced: true,
			Help: "Words per model call. Smaller chunks are safer for long documents.",
		},
		{
			Key: "workers", Label: "Workers", Type: core.ParamInt, Default: 3,
			Min: floatPtr(1), Max: floatPtr(8), Advanced: true,
			Help: "Chunks rewritten in parallel. Higher is faster but can hit free-tier limits.",
		},
		{
			Key: "rules", Label: "Rules file", Type: core.ParamPath, Default: "", Advanced: true,
			Help: "Optional rules file; defaults to the embedded humanizer rules.",
		},
	},
	Output:         core.OutputSpec{Kind: core.Document},
	Engines:        []core.EngineID{engines.OpenCode},
	RequireEngines: true,
	Batchable:      true,
	Cost:           core.CostSlow,
}

// Generator is the part of the OpenCode client the operation needs. Keeping it
// small makes the operation testable without a running service.
type Generator interface {
	FreeModels(context.Context) ([]opencode.ModelRef, error)
	Generate(context.Context, opencode.ModelRef, string) (string, error)
}

// Humanize rewrites a document chunk by chunk through OpenCode.
type Humanize struct {
	Client  Generator
	Timeout time.Duration

	mu        sync.Mutex
	models    []opencode.ModelRef
	modelsAt  time.Time
	down      map[string]string
	preferred opencode.ModelRef
}

func (h *Humanize) Spec() core.Spec { return humanizeSpec }

func (h *Humanize) Validate(_ context.Context, req core.Request) error {
	if err := core.Validate(humanizeSpec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, humanizeSpec.ID, "", "no input files", "")
	}
	if h.Client == nil {
		return core.NewError(core.CodeEngineMissing, humanizeSpec.ID, "",
			"OpenCode is not available", opencode.InstallHint())
	}

	rules, err := LoadRules(req.Params.String("rules"))
	if err != nil {
		return core.NewError(core.CodeUsage, humanizeSpec.ID, "", err.Error(),
			"point the Rules file field at a readable Markdown rules file, or clear it")
	}
	if strings.TrimSpace(CoreRules(rules)) == "" {
		return core.NewError(core.CodeUsage, humanizeSpec.ID, "",
			"the humanizer rules file has no core rules", "use the bundled rules or a file with a STYLE: section")
	}
	style := req.Params.String("style")
	if StyleModule(rules, style) == "" {
		return core.NewError(core.CodeUsage, humanizeSpec.ID, "",
			fmt.Sprintf("the rules file has no STYLE: %s module", strings.ToUpper(style)),
			"add the style to the rules file or choose another style")
	}
	if err := checkOptionalTextFile(req.Params.String("voice"), "voice sample"); err != nil {
		return err
	}
	if err := checkOptionalTextFile(req.Params.String("rules"), "rules file"); err != nil {
		return err
	}

	for _, item := range req.Items {
		if !humanizeSpec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, humanizeSpec.ID, item.Path,
				fmt.Sprintf("%s is not a text document", item.Format),
				"supported inputs: .txt, .md, .markdown, or .docx; DOCX keeps its figures, tables, and styles")
		}
	}
	return nil
}

func (h *Humanize) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	started := time.Now()
	rules, err := LoadRules(req.Params.String("rules"))
	if err != nil {
		return core.Result{}, core.NewError(core.CodeUsage, humanizeSpec.ID, "", err.Error(), "")
	}
	coreRules := CoreRules(rules)
	styleName := req.Params.String("style")
	styleText := StyleModule(rules, styleName)
	voice, err := readOptionalTextFile(req.Params.String("voice"))
	if err != nil {
		return core.Result{}, core.NewError(core.CodeUsage, humanizeSpec.ID, "", err.Error(), "")
	}
	modelParam := req.Params.String("model")
	if modelParam == "" {
		modelParam = "auto"
	}
	chunkWords := req.Params.Int("chunk_words")
	if chunkWords <= 0 {
		chunkWords = 900
	}
	workers := req.Params.Int("workers")
	if workers <= 0 {
		workers = 3
	}
	if workers > 8 {
		workers = 8
	}

	var out []core.Artifact
	var bytesIn, bytesOut int64
	modelNames := map[string]bool{}

	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		if strings.EqualFold(item.Ext(), "docx") {
			artifact, modelName, _, docxErr := h.runDOCX(ctx, item, req.Target.Dir,
				coreRules, styleText, voice, modelParam, styleName, workers, rep)
			if docxErr != nil {
				return core.Result{}, docxErr
			}
			out = append(out, artifact)
			bytesIn += item.Size
			bytesOut += artifact.Size
			if modelName != "" {
				modelNames[modelName] = true
			}
			continue
		}
		data, err := os.ReadFile(item.Path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, humanizeSpec.ID, item.Path, err,
				"check that the file exists and is readable")
		}
		if len(data) > 64<<20 {
			return core.Result{}, core.NewError(core.CodeInput, humanizeSpec.ID, item.Path,
				"file is larger than 64 MB", "split the document or convert it to plain text first")
		}
		if !utf8.Valid(data) {
			return core.Result{}, core.NewError(core.CodeInput, humanizeSpec.ID, item.Path,
				"file is not valid UTF-8 text", "convert it to UTF-8 Markdown or plain text first")
		}
		input := string(data)
		if strings.TrimSpace(input) == "" {
			return core.Result{}, core.NewError(core.CodeInput, humanizeSpec.ID, item.Path,
				"file is empty", "choose a document with text")
		}

		protected, p := ProtectDocument(input)
		chunks := ChunkText(protected, chunkWords, p)
		protectedNote := p.PromptNote()
		if len(chunks) == 0 {
			return core.Result{}, core.NewError(core.CodeInput, humanizeSpec.ID, item.Path,
				"no text to rewrite", "the document contains only formatting or code")
		}

		rep.Stage(fmt.Sprintf("humanizing %d chunk(s)", len(chunks)))
		rewritten, stats, err := h.runChunks(ctx, item, chunks, coreRules, styleText, voice, protectedNote, modelParam, workers, rep)
		if err != nil {
			if core.CodeOf(err) == core.CodeEngineMissing {
				return core.Result{}, err
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, humanizeSpec.ID, item.Path, err,
				"check that OpenCode is running and that a free model is enabled; run `opencode models`")
		}
		if stats.model != "" {
			rep.Log(core.LevelInfo, "%s: humanized with %s (%d chunk(s), %d kept original)",
				item.Name(), stats.model, len(chunks), stats.fallback)
		}

		joined := strings.Join(rewritten, "\n\n")
		restored, missing := p.restore(joined)
		if len(missing) > 0 {
			return core.Result{}, core.NewError(core.CodeEngineFailed, humanizeSpec.ID, item.Path,
				fmt.Sprintf("the model dropped %d protected block(s)", len(missing)),
				"try a different model or lower the chunk size")
		}
		restored = strings.TrimSpace(restored)
		if strings.HasSuffix(input, "\n") && !strings.HasSuffix(restored, "\n") {
			restored += "\n"
		}

		format := outputFormat(item.Ext())
		output := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+"."+format)
		if err := os.WriteFile(output, []byte(restored), 0o644); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, humanizeSpec.ID, output, err,
				"check permissions on the output directory")
		}
		info, err := os.Stat(output)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, humanizeSpec.ID, output, err, "")
		}

		modelName := stats.model
		if modelName != "" {
			modelNames[modelName] = true
		}
		out = append(out, core.Artifact{
			Path:   output,
			Kind:   core.Document,
			Format: format,
			Size:   info.Size(),
			Meta: map[string]any{
				"source":          item.Path,
				"style":           styleName,
				"model":           modelName,
				"chunks":          len(chunks),
				"fallback_chunks": stats.fallback,
				"words":           len(strings.Fields(restored)),
			},
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(len(chunks), len(chunks))
	}

	modelName := ""
	if len(modelNames) == 1 {
		for name := range modelNames {
			modelName = name
		}
	} else if len(modelNames) > 1 {
		modelName = "mixed"
	}
	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn:  bytesIn,
			BytesOut: bytesOut,
			Duration: time.Since(started),
			Engine:   engines.OpenCode,
			Model:    modelName,
		},
	}, nil
}

type chunkStats struct {
	model    string
	fallback int
}

func (h *Humanize) runChunks(
	ctx context.Context,
	item core.Artifact,
	chunks []string,
	coreRules, styleText, voice, protectedNote, requested string,
	workers int,
	rep core.Reporter,
) ([]string, chunkStats, error) {
	results := make([]string, len(chunks))
	var (
		group    errgroup.Group
		mu       sync.Mutex
		firstErr error
		used     = map[string]int{}
		fallback int
		done     atomic.Int64
	)
	group.SetLimit(workers)

	for i := range chunks {
		i := i
		group.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if wordCount(chunks[i]) == 0 {
				mu.Lock()
				results[i] = chunks[i]
				mu.Unlock()
				rep.Progress(int(done.Add(1)), len(chunks))
				return nil
			}
			previous := ""
			if i > 0 {
				previous = PreviousContext(chunks[i-1])
			}
			rewritten, ref, err := h.rewrite(ctx, chunks[i], coreRules, styleText, voice, protectedNote, previous, requested, rep)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				mu.Lock()
				fallback++
				results[i] = chunks[i]
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				rep.Log(core.LevelWarn, "%s: chunk %d/%d kept original text: %v",
					item.Name(), i+1, len(chunks), err)
			} else {
				mu.Lock()
				results[i] = rewritten
				used[ref.String()]++
				mu.Unlock()
			}
			rep.Progress(int(done.Add(1)), len(chunks))
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, chunkStats{}, err
	}

	stats := chunkStats{fallback: fallback}
	if len(used) == 1 {
		for name := range used {
			stats.model = name
		}
	} else if len(used) > 1 {
		stats.model = "mixed"
	}
	if fallback == len(chunks) {
		if firstErr == nil {
			firstErr = errors.New("no free model could rewrite the text")
		}
		return nil, stats, firstErr
	}
	if fallback > 0 {
		rep.Log(core.LevelWarn, "%s: %d of %d chunks were left unchanged",
			item.Name(), fallback, len(chunks))
	}
	return results, stats, nil
}

func (h *Humanize) rewrite(
	ctx context.Context,
	chunk, coreRules, styleText, voice, protectedNote, previous, requested string,
	rep core.Reporter,
) (string, opencode.ModelRef, error) {
	models, err := h.freeModels(ctx)
	if err != nil {
		return "", opencode.ModelRef{}, err
	}
	chain := h.chain(requested, models)
	candidates := h.available(chain)
	if len(candidates) == 0 {
		h.clearDown()
		candidates = h.available(chain)
		rep.Log(core.LevelWarn, "all free models were marked unavailable; retrying the fallback chain")
	}

	var lastErr error
	for _, model := range candidates {
		if err := ctx.Err(); err != nil {
			return "", model, err
		}
		out, err := h.attempt(ctx, model, coreRules, styleText, voice, protectedNote, previous, chunk)
		if err != nil {
			if ctx.Err() != nil {
				return "", model, ctx.Err()
			}
			lastErr = err
			var timeout *modelTimeoutError
			if errors.As(err, &timeout) {
				h.markDown(model, err)
				rep.Log(core.LevelWarn, "model %s timed out after %s; skipping it for the rest of the run",
					model, timeout.Duration)
				continue
			}
			if opencode.IsModelUnavailable(err) {
				h.markDown(model, err)
				rep.Log(core.LevelWarn, "model %s unavailable; trying the next free model: %v", model, err)
				continue
			}
			rep.Log(core.LevelWarn, "model %s failed; trying the next free model: %v", model, err)
			continue
		}
		h.setPreferred(model)
		if issues := issuesFor(chunk, out, allowContractionsForStyle(styleText)); len(issues) > 0 {
			total := len(issues)
			if len(issues) > 3 {
				issues = issues[:3]
			}
			rep.Log(core.LevelWarn, "model %s left %d style issue(s) in a chunk: %s",
				model, total, strings.Join(issues, "; "))
		}
		return out, model, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no free model was available")
	}
	return "", opencode.ModelRef{}, lastErr
}

func (h *Humanize) attempt(
	ctx context.Context,
	model opencode.ModelRef,
	coreRules, styleText, voice, protectedNote, previous, chunk string,
) (string, error) {
	prompt := BuildPrompt(coreRules, styleText, voice, protectedNote, previous, chunk)
	raw, err := h.generate(ctx, model, prompt)
	if err != nil {
		return "", err
	}
	allow := allowContractionsForStyle(styleText)
	out := PolishStyle(cleanOutput(raw), allow)
	if out == "" {
		return "", errors.New("model returned no text")
	}
	if strings.TrimSpace(out) == strings.TrimSpace(chunk) {
		return "", errors.New("model returned the text unchanged")
	}

	best := out
	bestIssues := issuesFor(chunk, out, allow)
	bestScore := issueScore(bestIssues, chunk, out)
	for round := 0; round < 2 && bestScore > 0; round++ {
		if len(bestIssues) > 7 {
			bestIssues = bestIssues[:7]
		}
		revision, revErr := h.generate(ctx, model, RevisionPrompt(coreRules, styleText, voice, protectedNote, chunk, bestIssues))
		if revErr != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			break
		}
		revised := PolishStyle(cleanOutput(revision), allow)
		if revised == "" || len(missingPlaceholders(chunk, revised)) > 0 {
			continue
		}
		revisedIssues := issuesFor(chunk, revised, allow)
		score := issueScore(revisedIssues, chunk, revised)
		if len(missingPlaceholders(chunk, best)) > 0 || score < bestScore {
			best = revised
			bestIssues = revisedIssues
			bestScore = score
		}
	}
	if len(missingPlaceholders(chunk, best)) > 0 {
		return "", errors.New("model dropped protected placeholders")
	}
	if issue := FatalRewriteIssue(chunk, best); issue != "" {
		return "", fmt.Errorf("unsafe rewrite: %s", issue)
	}
	return best, nil
}

func issuesFor(chunk, out string, allowContractions bool) []string {
	issues := LintIssuesStyle(out, allowContractions)
	if issue := lengthIssue(chunk, out); issue != "" {
		issues = append(issues, issue)
	}
	for _, token := range missingPlaceholders(chunk, out) {
		issues = append(issues, "keep this protected placeholder exactly where it was: "+token)
	}
	issues = append(issues, driftIssues(chunk, out)...)
	return issues
}

func issueScore(issues []string, chunk, out string) int {
	score := len(issues)
	ratio := lengthRatio(chunk, out)
	if ratio < 0.65 {
		score += int((0.65-ratio)*20) + 1
	}
	if ratio > 1.35 {
		score += int((ratio-1.35)*20) + 1
	}
	score += 10 * len(missingPlaceholders(chunk, out))
	return score
}

func allowContractionsForStyle(styleText string) bool {
	lower := strings.ToLower(styleText)
	if strings.Contains(lower, "style: academic") {
		return false
	}
	if strings.Contains(lower, "no contractions") {
		return false
	}
	return true
}

func (h *Humanize) generate(ctx context.Context, model opencode.ModelRef, prompt string) (string, error) {
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = defaultCallTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	text, err := h.Client.Generate(callCtx, model, prompt)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return "", &modelTimeoutError{Duration: timeout}
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	return text, nil
}

type modelTimeoutError struct {
	Duration time.Duration
}

func (e *modelTimeoutError) Error() string {
	return fmt.Sprintf("model did not answer within %s", e.Duration)
}

func (h *Humanize) freeModels(ctx context.Context) ([]opencode.ModelRef, error) {
	h.mu.Lock()
	if len(h.models) > 0 && time.Since(h.modelsAt) < 10*time.Minute {
		models := append([]opencode.ModelRef(nil), h.models...)
		h.mu.Unlock()
		return models, nil
	}
	h.mu.Unlock()

	models, err := h.Client.FreeModels(ctx)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.models = append([]opencode.ModelRef(nil), models...)
	h.modelsAt = time.Now()
	h.mu.Unlock()
	return models, nil
}

func (h *Humanize) chain(requested string, models []opencode.ModelRef) []opencode.ModelRef {
	seen := map[string]bool{}
	var out []opencode.ModelRef
	add := func(ref opencode.ModelRef) {
		if ref.ID == "" {
			return
		}
		key := ref.String()
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, ref)
	}

	if strings.TrimSpace(requested) != "" && requested != "auto" {
		if ref, err := opencode.ParseRef(requested); err == nil {
			add(withVariant(ref, models))
		}
	}
	h.mu.Lock()
	preferred := h.preferred
	h.mu.Unlock()
	if preferred.ID != "" {
		add(withVariant(preferred, models))
	}
	for _, model := range models {
		add(model)
	}
	return out
}

func withVariant(ref opencode.ModelRef, models []opencode.ModelRef) opencode.ModelRef {
	for _, model := range models {
		if model.ProviderID == ref.ProviderID && model.ID == ref.ID {
			if ref.Variant == "" {
				ref.Variant = model.Variant
			}
			return ref
		}
	}
	return ref
}

func (h *Humanize) available(chain []opencode.ModelRef) []opencode.ModelRef {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]opencode.ModelRef, 0, len(chain))
	for _, ref := range chain {
		if _, down := h.down[ref.String()]; down {
			continue
		}
		out = append(out, ref)
	}
	return out
}

func (h *Humanize) markDown(ref opencode.ModelRef, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.down == nil {
		h.down = map[string]string{}
	}
	h.down[ref.String()] = err.Error()
}

func (h *Humanize) clearDown() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.down = nil
}

func (h *Humanize) setPreferred(ref opencode.ModelRef) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.preferred = ref
}

func checkOptionalTextFile(path, label string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	info, err := os.Stat(config.ExpandHome(path))
	if err != nil {
		return core.NewError(core.CodeUsage, humanizeSpec.ID, "",
			fmt.Sprintf("%s cannot be read: %v", label, err), "choose a readable text file or clear the field")
	}
	if info.IsDir() {
		return core.NewError(core.CodeUsage, humanizeSpec.ID, "",
			fmt.Sprintf("%s is a directory", label), "choose a text file or clear the field")
	}
	if info.Size() > 4<<20 {
		return core.NewError(core.CodeUsage, humanizeSpec.ID, "",
			fmt.Sprintf("%s is larger than 4 MB", label), "use a short sample instead")
	}
	return nil
}

func readOptionalTextFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(config.ExpandHome(path))
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%s is not valid UTF-8 text", path)
	}
	return strings.TrimSpace(string(data)), nil
}

func outputFormat(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	switch ext {
	case "markdown":
		return "md"
	case "":
		return "txt"
	default:
		return ext
	}
}

func floatPtr(v float64) *float64 { return &v }

// Models returns the static model choices for tests and the API catalog.
func Models() []core.Option {
	out := append([]core.Option(nil), modelOptions...)
	return out
}

// Styles returns the static style choices.
func Styles() []core.Option {
	out := append([]core.Option(nil), styleOptions...)
	return out
}
