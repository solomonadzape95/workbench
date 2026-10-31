package doc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/errgroup"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/engines/opencode"
	"github.com/solomonadzape95/workbench/internal/ops/humanize"
)

const transformCallTimeout = 90 * time.Second

// textGenerator is the part of the OpenCode client these operations need.
type textGenerator interface {
	FreeModels(context.Context) ([]opencode.ModelRef, error)
	Generate(context.Context, opencode.ModelRef, string) (string, error)
}

func transformParams(extra ...core.ParamSpec) []core.ParamSpec {
	out := append([]core.ParamSpec(nil), extra...)
	out = append(out,
		core.ParamSpec{
			Key: "model", Label: "Model", Type: core.ParamEnum, Default: "auto",
			Options: humanize.Models(),
			Help:    "Auto tries free models in order and falls back when one is limited or unavailable.",
		},
		core.ParamSpec{
			Key: "chunk_words", Label: "Chunk size", Type: core.ParamInt, Default: 900,
			Min: floatPtr(200), Max: floatPtr(4000), Advanced: true,
			Help: "Words per model call. Smaller chunks are safer for long documents.",
		},
		core.ParamSpec{
			Key: "workers", Label: "Workers", Type: core.ParamInt, Default: 3,
			Min: floatPtr(1), Max: floatPtr(8), Advanced: true,
			Help: "Chunks processed in parallel. Higher is faster but can hit free-tier limits.",
		},
	)
	return out
}

func textAccept() []core.Accept {
	return []core.Accept{{Kind: core.Document, Formats: []string{"txt", "md", "markdown"}}}
}

var translateSpec = core.Spec{
	ID:      "doc.translate",
	Title:   "Translate document",
	Summary: "Translate Markdown or plain text with a free OpenCode model",
	Domain:  core.Document,
	Accepts: textAccept(),
	Params: transformParams(core.ParamSpec{
		Key: "language", Label: "Target language", Type: core.ParamString,
		Default: "English", Required: true,
		Help: "Language name or tag, for example English, French, or pt-BR.",
	}),
	Output:         core.OutputSpec{Kind: core.Document},
	Engines:        []core.EngineID{engines.OpenCode},
	RequireEngines: true,
	Batchable:      true,
	Cost:           core.CostSlow,
}

var summarizeSpec = core.Spec{
	ID:      "doc.summarize",
	Title:   "Summarize document",
	Summary: "Condense Markdown or plain text into a short, medium, or long summary",
	Domain:  core.Document,
	Accepts: textAccept(),
	Params: transformParams(
		core.ParamSpec{
			Key: "length", Label: "Length", Type: core.ParamEnum, Default: "medium",
			Options: []core.Option{
				{Value: "short", Label: "Short"},
				{Value: "medium", Label: "Medium"},
				{Value: "long", Label: "Long"},
			},
		},
		core.ParamSpec{
			Key: "style", Label: "Style", Type: core.ParamEnum, Default: "paragraph",
			Options: []core.Option{
				{Value: "paragraph", Label: "Paragraphs"},
				{Value: "bullets", Label: "Bullet points"},
			},
		},
	),
	Output:         core.OutputSpec{Kind: core.Document},
	Engines:        []core.EngineID{engines.OpenCode},
	RequireEngines: true,
	Batchable:      true,
	Cost:           core.CostSlow,
}

var rewriteSpec = core.Spec{
	ID:      "doc.rewrite",
	Title:   "Rewrite document",
	Summary: "Rewrite Markdown or plain text according to a custom instruction",
	Domain:  core.Document,
	Accepts: textAccept(),
	Params: transformParams(core.ParamSpec{
		Key: "instruction", Label: "Instruction", Type: core.ParamString,
		Default:  "Make the prose clearer and more direct without changing the meaning.",
		Required: true,
		Help:     "Tell the model how to rewrite the document.",
	}),
	Output:         core.OutputSpec{Kind: core.Document},
	Engines:        []core.EngineID{engines.OpenCode},
	RequireEngines: true,
	Batchable:      true,
	Cost:           core.CostSlow,
}

type Translate struct {
	Client  textGenerator
	Timeout time.Duration
}

func (Translate) Spec() core.Spec { return translateSpec }

func (o Translate) Validate(ctx context.Context, req core.Request) error {
	return validateTransform(ctx, translateSpec, o.Client, req)
}

func (o Translate) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	language := strings.TrimSpace(req.Params.String("language"))
	if language == "" {
		language = "English"
	}
	return runTransform(ctx, req, rep, transformOptions{
		spec:       translateSpec,
		client:     o.Client,
		timeout:    o.Timeout,
		task:       "translate",
		suffix:     ".translated",
		requireAll: true,
		prompt: func(chunk string) string {
			return fmt.Sprintf("Translate the following text into %s. Preserve Markdown formatting, headings, lists, links, code, and protected tokens exactly. Return only the translation, with no commentary.\n\n%s", language, chunk)
		},
		meta: map[string]any{"language": language},
	})
}

type Summarize struct {
	Client  textGenerator
	Timeout time.Duration
}

func (Summarize) Spec() core.Spec { return summarizeSpec }

func (o Summarize) Validate(ctx context.Context, req core.Request) error {
	return validateTransform(ctx, summarizeSpec, o.Client, req)
}

func (o Summarize) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	length := req.Params.String("length")
	if length == "" {
		length = "medium"
	}
	style := req.Params.String("style")
	if style == "" {
		style = "paragraph"
	}
	lengthGuide := "about 25% of the original length, and no more than 400 words"
	switch length {
	case "short":
		lengthGuide = "about 10% of the original length, and no more than 150 words"
	case "long":
		lengthGuide = "about 40% of the original length, and no more than 800 words"
	}
	styleGuide := "Use flowing paragraphs."
	if style == "bullets" {
		styleGuide = "Use concise bullet points."
	}
	return runTransform(ctx, req, rep, transformOptions{
		spec:       summarizeSpec,
		client:     o.Client,
		timeout:    o.Timeout,
		task:       "summarize",
		suffix:     ".summary",
		requireAll: false,
		prompt: func(chunk string) string {
			return fmt.Sprintf("Summarize this section in 2-4 sentences. Preserve the key facts, names, numbers, and conclusions. Return only the section summary.\n\n%s", chunk)
		},
		combine: func(partial string) string {
			return fmt.Sprintf("Combine these section summaries into one coherent summary. Target length: %s. %s Preserve the important facts and remove repetition. Return only the final summary.\n\n%s", lengthGuide, styleGuide, partial)
		},
		meta: map[string]any{"length": length, "summary_style": style},
	})
}

type Rewrite struct {
	Client  textGenerator
	Timeout time.Duration
}

func (Rewrite) Spec() core.Spec { return rewriteSpec }

func (o Rewrite) Validate(ctx context.Context, req core.Request) error {
	return validateTransform(ctx, rewriteSpec, o.Client, req)
}

func (o Rewrite) Run(ctx context.Context, req core.Request, rep core.Reporter) (core.Result, error) {
	instruction := strings.TrimSpace(req.Params.String("instruction"))
	if instruction == "" {
		instruction = "Make the prose clearer and more direct without changing the meaning."
	}
	return runTransform(ctx, req, rep, transformOptions{
		spec:       rewriteSpec,
		client:     o.Client,
		timeout:    o.Timeout,
		task:       "rewrite",
		suffix:     ".rewritten",
		requireAll: true,
		prompt: func(chunk string) string {
			return fmt.Sprintf("Rewrite the following document. Instruction: %s\n\nPreserve Markdown formatting, headings, lists, links, code, facts, and protected tokens exactly. Return only the rewritten text, with no commentary.\n\n%s", instruction, chunk)
		},
		meta: map[string]any{"instruction": instruction},
	})
}

func validateTransform(ctx context.Context, spec core.Spec, client textGenerator, req core.Request) error {
	if err := core.Validate(spec.Params, req.Params); err != nil {
		return err
	}
	if len(req.Items) == 0 {
		return core.NewError(core.CodeUsage, spec.ID, "", "no input files", "")
	}
	if client == nil {
		return core.NewError(core.CodeEngineMissing, spec.ID, "", "OpenCode is not available", opencode.InstallHint())
	}
	if spec.ID == translateSpec.ID && strings.TrimSpace(req.Params.String("language")) == "" {
		return core.NewError(core.CodeUsage, spec.ID, "", "target language is required", "")
	}
	if spec.ID == rewriteSpec.ID && strings.TrimSpace(req.Params.String("instruction")) == "" {
		return core.NewError(core.CodeUsage, spec.ID, "", "rewrite instruction is required", "")
	}
	for _, item := range req.Items {
		if !spec.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, spec.ID, item.Path,
				fmt.Sprintf("%s is not a text document", item.Format),
				"supported inputs: .txt, .md, or .markdown; convert DOCX or PDF to text first")
		}
	}
	return nil
}

type transformOptions struct {
	spec       core.Spec
	client     textGenerator
	timeout    time.Duration
	task       string
	suffix     string
	requireAll bool
	prompt     func(string) string
	combine    func(string) string
	meta       map[string]any
}

type transformStats struct {
	model string
}

func runTransform(ctx context.Context, req core.Request, rep core.Reporter, opts transformOptions) (core.Result, error) {
	started := time.Now()
	transformer := &textTransformer{client: opts.client, timeout: opts.timeout}
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
	requested := req.Params.String("model")

	var out []core.Artifact
	var bytesIn, bytesOut int64
	usedModels := map[string]bool{}

	for _, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return core.Result{}, err
		}
		data, err := os.ReadFile(item.Path)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeInput, opts.spec.ID, item.Path, err,
				"check that the file exists and is readable")
		}
		if len(data) > 64<<20 {
			return core.Result{}, core.NewError(core.CodeInput, opts.spec.ID, item.Path,
				"file is larger than 64 MB", "split the document or convert it to plain text first")
		}
		if !utf8.Valid(data) {
			return core.Result{}, core.NewError(core.CodeInput, opts.spec.ID, item.Path,
				"file is not valid UTF-8 text", "convert it to UTF-8 Markdown or plain text first")
		}
		input := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
		if strings.TrimSpace(input) == "" {
			return core.Result{}, core.NewError(core.CodeInput, opts.spec.ID, item.Path,
				"file is empty", "choose a document with text")
		}

		protected, guard := protectTransformText(input)
		chunks := chunkTransform(protected, chunkWords)
		if len(chunks) == 0 {
			return core.Result{}, core.NewError(core.CodeInput, opts.spec.ID, item.Path,
				"no text to process", "the document contains only formatting or code")
		}

		rep.Stage(fmt.Sprintf("%s %d chunk(s)", opts.task, len(chunks)))
		rewritten, stats, err := transformer.mapChunks(ctx, item, chunks, opts, workers, requested, rep)
		if err != nil {
			if core.CodeOf(err) == core.CodeEngineMissing {
				return core.Result{}, err
			}
			return core.Result{}, core.Wrap(core.CodeEngineFailed, opts.spec.ID, item.Path, err,
				"check that OpenCode is running and that a free model is enabled; run `opencode models`")
		}
		if stats.model != "" {
			usedModels[stats.model] = true
		}

		joined := strings.Join(rewritten, "\n\n")
		if opts.combine != nil && len(rewritten) > 1 {
			rep.Stage("combining " + fmt.Sprint(len(rewritten)) + " summaries")
			combined, ref, combineErr := transformer.generateText(ctx, opts.combine(joined), requested, rep)
			if combineErr != nil {
				return core.Result{}, core.Wrap(core.CodeEngineFailed, opts.spec.ID, item.Path, combineErr,
					"check that OpenCode is running and that a free model is enabled")
			}
			if ref.ID != "" {
				usedModels[ref.String()] = true
			}
			joined = combined
		}

		restored, missing := guard.restore(joined, opts.requireAll)
		if len(missing) > 0 {
			return core.Result{}, core.NewError(core.CodeEngineFailed, opts.spec.ID, item.Path,
				fmt.Sprintf("the model dropped %d protected block(s)", len(missing)),
				"try a different model or lower the chunk size")
		}
		restored = strings.TrimSpace(restored)
		if restored == "" {
			return core.Result{}, core.NewError(core.CodeEngineFailed, opts.spec.ID, item.Path,
				"the model returned no text", "try a different model")
		}
		if strings.HasSuffix(input, "\n") && !strings.HasSuffix(restored, "\n") {
			restored += "\n"
		}

		format := transformOutputExt(item.Ext())
		output := filepath.Join(req.Target.Dir, core.SanitizeStem(item.Stem())+opts.suffix+"."+format)
		if err := os.MkdirAll(req.Target.Dir, 0o755); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, opts.spec.ID, req.Target.Dir, err, "")
		}
		if err := os.WriteFile(output, []byte(restored), 0o644); err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, opts.spec.ID, output, err,
				"check permissions on the output directory")
		}
		info, err := os.Stat(output)
		if err != nil {
			return core.Result{}, core.Wrap(core.CodeOutput, opts.spec.ID, output, err, "")
		}

		meta := map[string]any{
			"source": item.Path,
			"task":   opts.task,
			"model":  stats.model,
			"chunks": len(chunks),
			"words":  len(strings.Fields(restored)),
		}
		for key, value := range opts.meta {
			meta[key] = value
		}
		out = append(out, core.Artifact{
			Path: output, Kind: core.Document, Format: format, Size: info.Size(),
			SuggestedName: filepath.Base(output), Meta: meta,
		})
		bytesIn += item.Size
		bytesOut += info.Size()
		rep.Progress(len(chunks), len(chunks))
	}

	modelName := ""
	if len(usedModels) == 1 {
		for name := range usedModels {
			modelName = name
		}
	} else if len(usedModels) > 1 {
		modelName = "mixed"
	}
	return core.Result{
		Artifacts: out,
		Metrics: core.Metrics{
			BytesIn: bytesIn, BytesOut: bytesOut, Duration: time.Since(started),
			Engine: engines.OpenCode, Model: modelName,
		},
	}, nil
}

func (t *textTransformer) mapChunks(
	ctx context.Context,
	item core.Artifact,
	chunks []string,
	opts transformOptions,
	workers int,
	requested string,
	rep core.Reporter,
) ([]string, transformStats, error) {
	results := make([]string, len(chunks))
	var (
		group    errgroup.Group
		mu       sync.Mutex
		firstErr error
		used     = map[string]int{}
		done     atomic.Int64
	)
	group.SetLimit(workers)
	for i := range chunks {
		i := i
		group.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !hasTransformWords(chunks[i]) {
				if opts.requireAll {
					results[i] = chunks[i]
				}
				rep.Progress(int(done.Add(1)), len(chunks))
				return nil
			}
			text, ref, err := t.generateText(ctx, opts.prompt(chunks[i]), requested, rep)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				rep.Log(core.LevelWarn, "%s: chunk %d/%d failed: %v", item.Name(), i+1, len(chunks), err)
			} else {
				mu.Lock()
				results[i] = cleanTransformOutput(text)
				used[ref.String()]++
				mu.Unlock()
			}
			rep.Progress(int(done.Add(1)), len(chunks))
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, transformStats{}, err
	}
	if firstErr != nil {
		return nil, transformStats{}, firstErr
	}
	stats := transformStats{}
	if len(used) == 1 {
		for name := range used {
			stats.model = name
		}
	} else if len(used) > 1 {
		stats.model = "mixed"
	}
	return results, stats, nil
}

func (t *textTransformer) generateText(ctx context.Context, prompt, requested string, rep core.Reporter) (string, opencode.ModelRef, error) {
	models, err := t.freeModels(ctx)
	if err != nil {
		return "", opencode.ModelRef{}, err
	}
	chain := t.chain(requested, models)
	candidates := t.available(chain)
	if len(candidates) == 0 {
		t.clearDown()
		candidates = t.available(chain)
		rep.Log(core.LevelWarn, "all free models were marked unavailable; retrying the fallback chain")
	}
	var lastErr error
	for _, model := range candidates {
		if err := ctx.Err(); err != nil {
			return "", model, err
		}
		out, err := t.call(ctx, model, prompt)
		if err != nil {
			lastErr = err
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
				t.markDown(model, err)
				rep.Log(core.LevelWarn, "model %s timed out; trying the next free model", model)
				continue
			}
			if opencode.IsModelUnavailable(err) {
				t.markDown(model, err)
				rep.Log(core.LevelWarn, "model %s unavailable; trying the next free model: %v", model, err)
				continue
			}
			rep.Log(core.LevelWarn, "model %s failed; trying the next free model: %v", model, err)
			continue
		}
		t.setPreferred(model)
		return out, model, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no free model was available")
	}
	return "", opencode.ModelRef{}, lastErr
}

func (t *textTransformer) call(ctx context.Context, model opencode.ModelRef, prompt string) (string, error) {
	timeout := t.timeout
	if timeout <= 0 {
		timeout = transformCallTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return t.client.Generate(callCtx, model, prompt)
}

func (t *textTransformer) freeModels(ctx context.Context) ([]opencode.ModelRef, error) {
	t.mu.Lock()
	if len(t.models) > 0 && time.Since(t.modelsAt) < 10*time.Minute {
		models := append([]opencode.ModelRef(nil), t.models...)
		t.mu.Unlock()
		return models, nil
	}
	t.mu.Unlock()
	models, err := t.client.FreeModels(ctx)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.models = append([]opencode.ModelRef(nil), models...)
	t.modelsAt = time.Now()
	t.mu.Unlock()
	return models, nil
}

func (t *textTransformer) chain(requested string, models []opencode.ModelRef) []opencode.ModelRef {
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
			add(ref)
		}
	}
	t.mu.Lock()
	preferred := t.preferred
	t.mu.Unlock()
	if preferred.ID != "" {
		add(preferred)
	}
	for _, model := range models {
		add(model)
	}
	return out
}

func (t *textTransformer) available(chain []opencode.ModelRef) []opencode.ModelRef {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]opencode.ModelRef, 0, len(chain))
	for _, ref := range chain {
		if _, down := t.down[ref.String()]; down {
			continue
		}
		out = append(out, ref)
	}
	return out
}

func (t *textTransformer) markDown(ref opencode.ModelRef, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.down == nil {
		t.down = map[string]string{}
	}
	t.down[ref.String()] = err.Error()
}

func (t *textTransformer) clearDown() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.down = nil
}

func (t *textTransformer) setPreferred(ref opencode.ModelRef) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.preferred = ref
}

type textTransformer struct {
	client    textGenerator
	timeout   time.Duration
	mu        sync.Mutex
	models    []opencode.ModelRef
	modelsAt  time.Time
	down      map[string]string
	preferred opencode.ModelRef
}

var (
	transformFencePattern = regexp.MustCompile("(?ms)^[ \\t]*```[^\\n]*\\n.*?^[ \\t]*```[ \\t]*$")
	transformInlineCode   = regexp.MustCompile("`[^`\\n]+`")
	transformURL          = regexp.MustCompile(`https?://[^\s<>()\[\]]+`)
	transformTokenPattern = regexp.MustCompile(`\[\[WBT\d+\]\]`)
)

type textGuard struct {
	values []string
}

func protectTransformText(text string) (string, *textGuard) {
	guard := &textGuard{}
	text = protectTransformPattern(text, transformFencePattern, guard)
	text = protectTransformPattern(text, transformInlineCode, guard)
	text = protectTransformPattern(text, transformURL, guard)
	return text, guard
}

func protectTransformPattern(text string, pattern *regexp.Regexp, guard *textGuard) string {
	return pattern.ReplaceAllStringFunc(text, func(match string) string {
		guard.values = append(guard.values, match)
		return fmt.Sprintf("[[WBT%d]]", len(guard.values)-1)
	})
}

func (g *textGuard) restore(text string, requireAll bool) (string, []string) {
	var missing []string
	pairs := make([]string, 0, len(g.values)*2)
	for i, value := range g.values {
		token := fmt.Sprintf("[[WBT%d]]", i)
		if !strings.Contains(text, token) {
			if requireAll {
				missing = append(missing, token)
			}
			continue
		}
		pairs = append(pairs, token, value)
	}
	return strings.NewReplacer(pairs...).Replace(text), missing
}

func chunkTransform(text string, maxWords int) []string {
	if maxWords <= 0 {
		maxWords = 900
	}
	paragraphs := strings.Split(text, "\n\n")
	var chunks []string
	var current []string
	count := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, strings.TrimSpace(strings.Join(current, "\n\n")))
		current = nil
		count = 0
	}
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		words := len(strings.Fields(paragraph))
		if words > maxWords {
			flush()
			fields := strings.Fields(paragraph)
			for len(fields) > 0 {
				n := maxWords
				if len(fields) < n {
					n = len(fields)
				}
				chunks = append(chunks, strings.Join(fields[:n], " "))
				fields = fields[n:]
			}
			continue
		}
		if count > 0 && count+words > maxWords {
			flush()
		}
		current = append(current, paragraph)
		count += words
	}
	flush()
	return chunks
}

func hasTransformWords(chunk string) bool {
	withoutTokens := transformTokenPattern.ReplaceAllString(chunk, " ")
	return strings.TrimSpace(withoutTokens) != ""
}

func cleanTransformOutput(text string) string {
	text = strings.TrimSpace(text)
	lines := strings.Split(text, "\n")
	if len(lines) >= 2 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") &&
		strings.TrimSpace(lines[len(lines)-1]) == "```" {
		text = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return text
}

func transformOutputExt(ext string) string {
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
