package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/pelletier/go-toml/v2"

	"github.com/solomonadzape95/workbench/internal/alias"
	"github.com/solomonadzape95/workbench/internal/api"
	"github.com/solomonadzape95/workbench/internal/app"
	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/mcp"
	"github.com/solomonadzape95/workbench/internal/notify"
	"github.com/solomonadzape95/workbench/internal/runner"
	"github.com/solomonadzape95/workbench/internal/tui"
	"github.com/solomonadzape95/workbench/internal/version"
	"github.com/solomonadzape95/workbench/internal/watch"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		return runTUI()
	}

	switch args[0] {
	case "doctor":
		return runDoctor(args[1:])
	case "serve":
		return runServe(args[1:])
	case "mcp":
		return runMCP(args[1:])
	case "run":
		return runJob(args[1:])
	case "watch":
		return runWatch(args[1:])
	case "ops":
		return runOps(args[1:])
	case "engines":
		return runEngines(args[1:])
	case "pipeline":
		return runPipeline(args[1:])
	case "history":
		return runHistory(args[1:])
	case "completion":
		return runCompletion(args[1:])
	case "version", "--version", "-v":
		printVersion()
		return 0
	case "help", "--help", "-h":
		usage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "wb: unknown command %q\n\n", args[0])
		usage(os.Stderr)
		return 2
	}
}

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	addr := fs.String("addr", "127.0.0.1:8765", "listen address (use 127.0.0.1 for local agents)")
	token := fs.String("token", os.Getenv("WORKBENCH_API_TOKEN"), "optional bearer token (env WORKBENCH_API_TOKEN)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "wb serve: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	a := app.Boot()
	for _, warning := range a.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	jobRunner := a.NewRunner()
	defer jobRunner.Close()

	srv := api.New(api.Options{
		Config:     a.Config,
		Operations: a.Operations,
		Engines:    a.Registry,
		Runner:     jobRunner,
		Pipelines:  a.Pipelines,
		History:    a.History,
		Address:    *addr,
		Token:      strings.TrimSpace(*token),
		Version:    version.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	auth := "no token (local-only recommended)"
	if srv.TokenRequired() {
		auth = "bearer token required"
	}
	fmt.Fprintf(os.Stderr, "Workbench API listening on http://%s · %s\n", srv.Address(), auth)
	if err := srv.ListenAndServe(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "wb serve: %v\n", err)
		return 1
	}
	return 0
}

func runHistory(args []string) int {
	asJSON := false
	rest := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			asJSON = true
			continue
		}
		rest = append(rest, arg)
	}
	action := "list"
	if len(rest) > 0 {
		action = rest[0]
	}
	if len(rest) > 1 {
		fmt.Fprintf(os.Stderr, "wb history: unexpected argument %q\n", rest[1])
		return 2
	}
	a := app.Boot()
	switch action {
	case "list":
		records, err := a.History.Recent(50)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wb history: %v\n", err)
			return 1
		}
		if asJSON {
			out := make([]historyJSON, 0, len(records))
			for _, record := range records {
				out = append(out, historyRecordJSON(record.View))
			}
			if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
				fmt.Fprintf(os.Stderr, "wb history: %v\n", err)
				return 1
			}
			return 0
		}
		if len(records) == 0 {
			fmt.Println("no history yet")
			return 0
		}
		for _, record := range records {
			view := record.View
			finished := view.FinishedAt
			if finished.IsZero() {
				finished = view.QueuedAt
			}
			fmt.Printf("%s  %-9s  %-32s  %s\n",
				finished.Format("2006-01-02 15:04"), view.Status,
				truncateString(view.Name, 32), view.ID)
		}
		return 0
	case "clear":
		if err := a.History.Clear(); err != nil {
			fmt.Fprintf(os.Stderr, "wb history: %v\n", err)
			return 1
		}
		fmt.Println("history cleared")
		return 0
	case "help", "--help", "-h":
		fmt.Println("usage: wb history [list|clear] [--json]")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "wb history: unknown action %q (use list or clear)\n", action)
		return 2
	}
}

type historyJSON struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Status     core.Status    `json:"status"`
	QueuedAt   time.Time      `json:"queued_at"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
	DurationMS int64          `json:"duration_ms"`
	Metrics    historyMetrics `json:"metrics"`
	Items      []historyItem  `json:"items"`
	Error      string         `json:"error,omitempty"`
}

type historyMetrics struct {
	BytesIn  int64 `json:"bytes_in"`
	BytesOut int64 `json:"bytes_out"`
}

type historyItem struct {
	Input   string          `json:"input"`
	Status  core.ItemStatus `json:"status"`
	Outputs []string        `json:"outputs,omitempty"`
	Error   string          `json:"error,omitempty"`
}

func historyRecordJSON(view runner.JobView) historyJSON {
	out := historyJSON{
		ID:         view.ID,
		Name:       view.Name,
		Status:     view.Status,
		QueuedAt:   view.QueuedAt,
		StartedAt:  view.StartedAt,
		FinishedAt: view.FinishedAt,
		DurationMS: view.Duration().Milliseconds(),
		Metrics: historyMetrics{
			BytesIn:  view.Metrics.BytesIn,
			BytesOut: view.Metrics.BytesOut,
		},
	}
	if view.Err != nil {
		out.Error = view.Err.Error()
	}
	for _, item := range view.Items {
		entry := historyItem{
			Input:  item.Input.Path,
			Status: item.Status,
		}
		if entry.Input == "" {
			entry.Input = item.Input.Name()
		}
		if item.Err != nil {
			entry.Error = item.Err.Error()
		}
		for _, artifact := range item.Outputs {
			entry.Outputs = append(entry.Outputs, artifact.Path)
		}
		out.Items = append(out.Items, entry)
	}
	return out
}

type paramFlag []string

func (p *paramFlag) String() string { return strings.Join(*p, ",") }

func (p *paramFlag) Set(value string) error {
	*p = append(*p, value)
	return nil
}

func runJob(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opID := fs.String("op", "", "operation id (for example image.convert)")
	pipelineName := fs.String("pipeline", "", "saved pipeline name")
	input := fs.String("input", "", "input path, glob, or @alias")
	output := fs.String("output", "", "output directory (defaults to the configured output dir)")
	recursive := fs.Bool("recursive", false, "walk directories and simple globs recursively")
	var includes, excludes paramFlag
	fs.Var(&includes, "include", "include glob (repeatable)")
	fs.Var(&excludes, "exclude", "exclude glob (repeatable)")
	minSize := fs.String("min-size", "", "minimum file size, e.g. 10k or 2mb")
	maxSize := fs.String("max-size", "", "maximum file size, e.g. 10k or 2mb")
	newerThan := fs.Duration("newer-than", 0, "only files modified within this age, e.g. 24h")
	skipHidden := fs.Bool("skip-hidden", false, "skip dot-files and dot-directories")
	dryRun := fs.Bool("dry-run", false, "print the resolved plan without running")
	var params paramFlag
	fs.Var(&params, "param", "operation parameter key=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "wb run: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if (*opID == "") == (*pipelineName == "") {
		fmt.Fprintln(os.Stderr, "wb run: use exactly one of --op or --pipeline")
		return 2
	}
	if strings.TrimSpace(*input) == "" {
		fmt.Fprintln(os.Stderr, "wb run: --input is required")
		return 2
	}
	if *opID == "" && len(params) > 0 {
		fmt.Fprintln(os.Stderr, "wb run: --param requires --op; a saved pipeline carries its own parameters")
		return 2
	}

	a := app.Boot()
	for _, warning := range a.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	var p core.Pipeline
	if *opID != "" {
		op, ok := a.Operations.Get(*opID)
		if !ok {
			fmt.Fprintf(os.Stderr, "wb run: unknown operation %q\n", *opID)
			return 2
		}
		spec := op.Spec()
		values := spec.DefaultValues()
		for _, raw := range params {
			key, value, ok := strings.Cut(raw, "=")
			key = strings.TrimSpace(key)
			if !ok || key == "" {
				fmt.Fprintf(os.Stderr, "wb run: --param %q must be key=value\n", raw)
				return 2
			}
			specParam, ok := spec.Param(key)
			if !ok {
				fmt.Fprintf(os.Stderr, "wb run: %s has no parameter %q\n", spec.ID, key)
				return 2
			}
			parsed, err := parseParamValue(specParam, value)
			if err != nil {
				fmt.Fprintf(os.Stderr, "wb run: --param %s: %v\n", key, err)
				return 2
			}
			values[key] = parsed
		}
		p = core.Pipeline{
			Name:    spec.ID,
			Version: core.PipelineVersion,
			Steps:   []core.Step{{Op: spec.ID, Params: values}},
		}
	} else {
		var err error
		p, err = a.Pipelines.Load(*pipelineName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wb run: pipeline %q: %v\n", *pipelineName, err)
			return 1
		}
	}

	minBytes, err := parseSize(*minSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wb run: --min-size: %v\n", err)
		return 2
	}
	maxBytes, err := parseSize(*maxSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wb run: --max-size: %v\n", err)
		return 2
	}
	options := alias.Options{
		Recursive:  *recursive,
		Include:    includes,
		Exclude:    excludes,
		MinSize:    minBytes,
		MaxSize:    maxBytes,
		SkipHidden: *skipHidden,
	}
	if *newerThan > 0 {
		options.NewerThan = time.Now().Add(-*newerThan)
	}
	inputs, err := alias.ResolveOptions(*input, a.Config.Aliases, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wb run: %v\n", err)
		return 1
	}
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "wb run: no input files matched")
		return 1
	}

	target := core.Target{
		Dir:       resolveCLIOutput(a.Config, *output),
		Template:  a.Config.Output.Template,
		Collision: core.Collision(a.Config.Output.Collision),
	}
	if *dryRun {
		return printDryRun(a, p, inputs, target)
	}
	jobRunner := a.NewRunner()
	defer jobRunner.Close()

	job, err := jobRunner.Start(p, inputs, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wb run: %v\n", err)
		return 1
	}

	done := waitForJob(context.Background(), jobRunner, job.ID())
	view := job.Snapshot()
	printJobResult(view, done)
	notifyJob(a, view, done)
	switch done.Status {
	case core.StatusDone, core.StatusPartial:
		return 0
	default:
		return 1
	}
}

func parseParamValue(spec core.ParamSpec, raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	switch spec.Type {
	case core.ParamInt:
		value, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("want an integer, got %q", raw)
		}
		return value, nil
	case core.ParamFloat:
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("want a number, got %q", raw)
		}
		return value, nil
	case core.ParamBool:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("want true or false, got %q", raw)
		}
		return value, nil
	case core.ParamDuration:
		value, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("want a duration such as 2s, got %q", raw)
		}
		return value, nil
	case core.ParamEnum:
		for _, option := range spec.Options {
			if option.Value == raw {
				return raw, nil
			}
		}
		values := make([]string, 0, len(spec.Options))
		for _, option := range spec.Options {
			values = append(values, option.Value)
		}
		return nil, fmt.Errorf("want one of %s, got %q", strings.Join(values, ", "), raw)
	case core.ParamPath:
		return config.ExpandHome(raw), nil
	default:
		return raw, nil
	}
}

func waitForJob(ctx context.Context, r *runner.Runner, id string) core.JobDone {
	for {
		select {
		case <-ctx.Done():
			r.Cancel(id)
			return core.JobDone{JobID: id, Status: core.StatusCanceled}
		case event, ok := <-r.Events():
			if !ok {
				return core.JobDone{JobID: id}
			}
			if done, ok := event.(core.JobDone); ok && done.JobID == id {
				return done
			}
		}
	}
}

func runWatch(args []string) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pipelineName := fs.String("pipeline", "", "saved pipeline name")
	dir := fs.String("dir", "", "directory to watch")
	recursive := fs.Bool("recursive", false, "watch subdirectories")
	settle := fs.Duration("settle", 2*time.Second, "wait this long after the last change")
	output := fs.String("output", "", "output directory")
	once := fs.Bool("once", false, "exit after the first completed batch")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "wb watch: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if *pipelineName == "" || *dir == "" {
		fmt.Fprintln(os.Stderr, "wb watch: --pipeline and --dir are required")
		return 2
	}
	if *settle <= 0 {
		fmt.Fprintln(os.Stderr, "wb watch: --settle must be positive")
		return 2
	}

	a := app.Boot()
	for _, warning := range a.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	p, err := a.Pipelines.Load(*pipelineName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wb watch: pipeline %q: %v\n", *pipelineName, err)
		return 1
	}
	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wb watch: %v\n", err)
		return 1
	}
	outDir := resolveCLIOutput(a.Config, *output)
	watcher, err := watch.New(absDir, watch.Options{
		Recursive:  *recursive,
		SkipHidden: true,
		Settle:     *settle,
		Ignore:     []string{outDir},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "wb watch: %v\n", err)
		return 1
	}
	if err := watcher.Baseline(); err != nil {
		fmt.Fprintf(os.Stderr, "wb watch: %v\n", err)
		return 1
	}

	target := core.Target{
		Dir:       outDir,
		Template:  a.Config.Output.Template,
		Collision: core.Collision(a.Config.Output.Collision),
	}
	jobRunner := a.NewRunner()
	defer jobRunner.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mode := "settle " + settle.String()
	if *recursive {
		mode += " · recursive"
	}
	fmt.Fprintf(os.Stderr, "watching %s · pipeline %s · %s\n", absDir, p.Name, mode)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0
		case <-ticker.C:
		}
		paths, err := watcher.Scan(time.Now())
		if err != nil {
			fmt.Fprintf(os.Stderr, "wb watch: scan: %v\n", err)
			continue
		}
		if len(paths) == 0 {
			continue
		}
		inputs := make([]core.Artifact, 0, len(paths))
		for _, path := range paths {
			artifact, err := core.Probe(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "wb watch: %s: %v\n", path, err)
				continue
			}
			inputs = append(inputs, artifact)
		}
		if len(inputs) == 0 {
			continue
		}
		fmt.Fprintf(os.Stderr, "batch: %d changed file(s)\n", len(inputs))
		job, err := jobRunner.Start(p, inputs, target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wb watch: %v\n", err)
			if *once {
				return 1
			}
			continue
		}
		done := waitForJob(ctx, jobRunner, job.ID())
		view := job.Snapshot()
		printJobResult(view, done)
		notifyJob(a, view, done)
		if *once {
			if done.Status == core.StatusDone || done.Status == core.StatusPartial {
				return 0
			}
			return 1
		}
	}
}

func parseSize(raw string) (int64, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return 0, nil
	}
	multiplier := float64(1)
	for _, suffix := range []struct {
		unit string
		mult float64
	}{
		{"gb", 1 << 30}, {"g", 1 << 30},
		{"mb", 1 << 20}, {"m", 1 << 20},
		{"kb", 1 << 10}, {"k", 1 << 10},
		{"b", 1},
	} {
		if strings.HasSuffix(raw, suffix.unit) {
			multiplier = suffix.mult
			raw = strings.TrimSpace(strings.TrimSuffix(raw, suffix.unit))
			break
		}
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("want a size such as 10k or 2mb, got %q", raw)
	}
	return int64(value * multiplier), nil
}

func resolveCLIOutput(cfg config.Config, override string) string {
	dir := strings.TrimSpace(override)
	if dir == "" {
		dir = strings.TrimSpace(cfg.Output.Dir)
	}
	if dir == "" {
		dir = "~/Workbench/{date}"
	}
	dir = strings.ReplaceAll(dir, "{date}", time.Now().Format("2006-01-02"))
	return config.ExpandHome(dir)
}

func printJobResult(view runner.JobView, done core.JobDone) {
	fmt.Printf("%s  %s  %d item(s)  %s\n", view.ID, view.Status, len(view.Items), view.Duration().Round(time.Millisecond))
	for _, item := range view.Items {
		name := item.Input.Name()
		if item.Err != nil {
			fmt.Printf("  ✗ %s: %v\n", name, item.Err)
			continue
		}
		if len(item.Outputs) == 0 {
			fmt.Printf("  · %s\n", name)
			continue
		}
		for _, output := range item.Outputs {
			fmt.Printf("  ✓ %s -> %s\n", name, output.Path)
		}
	}
	for _, output := range view.Aggregate {
		fmt.Printf("  ✓ aggregate -> %s\n", output.Path)
	}
	if done.Err != nil {
		fmt.Fprintf(os.Stderr, "wb run: %v\n", done.Err)
		if hint := core.HintOf(done.Err); hint != "" {
			fmt.Fprintf(os.Stderr, "hint: %s\n", hint)
		}
	}
}

type opJSON struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Summary   string          `json:"summary"`
	Domain    core.Kind       `json:"domain"`
	Input     core.SourceKind `json:"input"`
	Output    core.Kind       `json:"output_kind"`
	Engines   []core.EngineID `json:"engines,omitempty"`
	Batchable bool            `json:"batchable"`
	Cost      core.Cost       `json:"cost,omitempty"`
	Params    []paramJSON     `json:"params,omitempty"`
}

type paramJSON struct {
	Key      string         `json:"key"`
	Label    string         `json:"label"`
	Type     core.ParamType `json:"type"`
	Default  any            `json:"default,omitempty"`
	Required bool           `json:"required,omitempty"`
	Options  []optionJSON   `json:"options,omitempty"`
	Help     string         `json:"help,omitempty"`
}

type optionJSON struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func notifyJob(a *app.App, view runner.JobView, done core.JobDone) {
	if a == nil || (!a.Config.General.Notify && a.Config.Notify.Webhook == "") {
		return
	}
	job := notify.Job{
		ID:         done.JobID,
		Name:       view.Name,
		Status:     string(done.Status),
		DurationMS: done.Duration.Milliseconds(),
		BytesIn:    done.Metrics.BytesIn,
		BytesOut:   done.Metrics.BytesOut,
	}
	if view.Err != nil {
		job.Error = view.Err.Error()
	}
	for _, item := range view.Items {
		for _, output := range item.Outputs {
			job.Outputs = append(job.Outputs, output.Path)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if a.Config.General.Notify {
		_ = notify.Desktop(ctx, "Workbench · "+job.Status, job.Name+" · "+done.Duration.Round(time.Millisecond).String())
	}
	if a.Config.Notify.Webhook != "" {
		_ = notify.Webhook(ctx, a.Config.Notify.Webhook, job)
	}
}

func runOps(args []string) int {
	fs := flag.NewFlagSet("ops", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "write machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "wb ops: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	a := app.Boot()
	ops := a.Operations.All()
	sort.Slice(ops, func(i, j int) bool { return ops[i].Spec().ID < ops[j].Spec().ID })
	if *asJSON {
		out := make([]opJSON, 0, len(ops))
		for _, op := range ops {
			spec := op.Spec()
			item := opJSON{
				ID:        spec.ID,
				Title:     spec.Title,
				Summary:   spec.Summary,
				Domain:    spec.Domain,
				Input:     spec.SourceKind(),
				Output:    spec.OutputKind(),
				Engines:   spec.Engines,
				Batchable: spec.Batchable,
				Cost:      spec.Cost,
			}
			for _, param := range spec.Params {
				p := paramJSON{
					Key:      param.Key,
					Label:    param.Label,
					Type:     param.Type,
					Default:  param.Default,
					Required: param.Required,
					Help:     param.Help,
				}
				for _, option := range param.Options {
					p.Options = append(p.Options, optionJSON{Value: option.Value, Label: option.Label})
				}
				item.Params = append(item.Params, p)
			}
			out = append(out, item)
		}
		if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "wb ops: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Printf("%-24s %-10s %s\n", "ID", "DOMAIN", "TITLE")
	for _, op := range ops {
		spec := op.Spec()
		fmt.Printf("%-24s %-10s %s\n", spec.ID, spec.Domain, spec.Title)
	}
	return 0
}

type doctorJSON struct {
	Version    string       `json:"version"`
	ConfigFile string       `json:"config_file"`
	DataDir    string       `json:"data_dir"`
	CacheDir   string       `json:"cache_dir"`
	Engines    []engineJSON `json:"engines"`
	Warnings   []string     `json:"warnings,omitempty"`
}

func runDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "write machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "wb doctor: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	a := app.Boot()
	if !*asJSON {
		a.PrintDoctor(os.Stdout)
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	infos := a.Registry.ProbeAll(ctx)
	out := doctorJSON{
		Version:    version.Version,
		ConfigFile: a.Paths.ConfigFile,
		DataDir:    a.Paths.DataDir,
		CacheDir:   a.Paths.CacheDir,
		Warnings:   a.Warnings,
	}
	for _, info := range infos {
		item := engineJSON{
			ID:        string(info.ID),
			Name:      info.Name,
			Path:      info.Path,
			Version:   info.Version,
			Available: info.Available,
			Detail:    info.Detail,
			Hint:      info.Hint,
		}
		if info.Err != nil {
			item.Error = info.Err.Error()
		}
		out.Engines = append(out.Engines, item)
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "wb doctor: %v\n", err)
		return 1
	}
	return 0
}

type engineJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
	Hint      string `json:"hint,omitempty"`
	Error     string `json:"error,omitempty"`
}

func runEngines(args []string) int {
	fs := flag.NewFlagSet("engines", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "write machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "wb engines: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	a := app.Boot()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	infos := a.Registry.ProbeAll(ctx)

	if *asJSON {
		out := make([]engineJSON, 0, len(infos))
		for _, info := range infos {
			item := engineJSON{
				ID:        string(info.ID),
				Name:      info.Name,
				Path:      info.Path,
				Version:   info.Version,
				Available: info.Available,
				Detail:    info.Detail,
				Hint:      info.Hint,
			}
			if info.Err != nil {
				item.Error = info.Err.Error()
			}
			out = append(out, item)
		}
		if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "wb engines: %v\n", err)
			return 1
		}
		return 0
	}

	for _, info := range infos {
		mark := "✓"
		detail := info.Version
		if !info.Available {
			mark = "○"
			detail = info.Hint
			if detail == "" {
				detail = info.Detail
			}
		}
		if detail == "" {
			detail = info.Path
		}
		fmt.Printf("%s %-12s %s\n", mark, info.ID, detail)
	}
	return 0
}

type pipelineJSON struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Steps       int    `json:"steps"`
	Error       string `json:"error,omitempty"`
}

func runPipeline(args []string) int {
	if len(args) == 0 {
		return pipelineList(nil)
	}
	switch args[0] {
	case "list":
		return pipelineList(args[1:])
	case "export":
		return pipelineExport(args[1:])
	case "import":
		return pipelineImport(args[1:])
	case "help", "--help", "-h":
		fmt.Println("usage: wb pipeline [list [--json]|export [--out file] <name>|import [--name name] <file>]")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "wb pipeline: unknown action %q (use list, export, or import)\n", args[0])
		return 2
	}
}

func pipelineList(args []string) int {
	fs := flag.NewFlagSet("pipeline list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "write machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "wb pipeline list: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	a := app.Boot()
	metas, err := a.Pipelines.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wb pipeline list: %v\n", err)
		return 1
	}
	if *asJSON {
		out := make([]pipelineJSON, 0, len(metas))
		for _, meta := range metas {
			item := pipelineJSON{Name: meta.Name, Description: meta.Description, Steps: meta.Steps}
			if meta.Err != nil {
				item.Error = meta.Err.Error()
			}
			out = append(out, item)
		}
		if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "wb pipeline list: %v\n", err)
			return 1
		}
		return 0
	}
	if len(metas) == 0 {
		fmt.Println("no saved pipelines")
		return 0
	}
	fmt.Printf("%-28s %5s  %s\n", "NAME", "STEPS", "DESCRIPTION")
	for _, meta := range metas {
		description := meta.Description
		if meta.Err != nil {
			description = "invalid: " + meta.Err.Error()
		}
		fmt.Printf("%-28s %5d  %s\n", meta.Name, meta.Steps, description)
	}
	return 0
}

func pipelineExport(args []string) int {
	fs := flag.NewFlagSet("pipeline export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	outPath := fs.String("out", "", "write to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "wb pipeline export: provide exactly one pipeline name")
		return 2
	}
	name := fs.Arg(0)
	a := app.Boot()
	if _, err := a.Pipelines.Load(name); err != nil {
		fmt.Fprintf(os.Stderr, "wb pipeline export: %v\n", err)
		return 1
	}
	data, err := os.ReadFile(a.Pipelines.Path(name))
	if err != nil {
		fmt.Fprintf(os.Stderr, "wb pipeline export: %v\n", err)
		return 1
	}
	if *outPath == "" {
		if _, err := os.Stdout.Write(data); err != nil {
			fmt.Fprintf(os.Stderr, "wb pipeline export: %v\n", err)
			return 1
		}
		return 0
	}
	path := config.ExpandHome(*outPath)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "wb pipeline export: %v\n", err)
		return 1
	}
	fmt.Printf("exported %s -> %s\n", name, path)
	return 0
}

func pipelineImport(args []string) int {
	fs := flag.NewFlagSet("pipeline import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "save under this name instead of the file's name")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "wb pipeline import: provide exactly one TOML file")
		return 2
	}
	path := config.ExpandHome(fs.Arg(0))
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wb pipeline import: %v\n", err)
		return 1
	}
	var p core.Pipeline
	if err := toml.Unmarshal(data, &p); err != nil {
		fmt.Fprintf(os.Stderr, "wb pipeline import: %v\n", err)
		return 1
	}
	if *name != "" {
		p.Name = *name
	} else if p.Name == "" {
		p.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	a := app.Boot()
	if err := a.Pipelines.Save(p); err != nil {
		fmt.Fprintf(os.Stderr, "wb pipeline import: %v\n", err)
		return 1
	}
	fmt.Printf("imported %s (%d steps)\n", p.Name, len(p.Steps))
	return 0
}

func runCompletion(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "wb completion: choose bash, zsh, or fish")
		return 2
	}
	switch args[0] {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	case "help", "--help", "-h":
		fmt.Println("usage: wb completion <bash|zsh|fish>")
	default:
		fmt.Fprintf(os.Stderr, "wb completion: unsupported shell %q\n", args[0])
		return 2
	}
	return 0
}

const bashCompletion = `# bash completion for Workbench
_wb() {
  local cur
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  if [ "$COMP_CWORD" -eq 1 ]; then
    COMPREPLY=( $(compgen -W "run watch ops engines pipeline serve doctor history completion version help" -- "$cur") )
    return
  fi
  case "${COMP_WORDS[1]}" in
    run)
      COMPREPLY=( $(compgen -W "--op --pipeline --input --output --param --recursive --include --exclude --min-size --max-size --newer-than --skip-hidden" -- "$cur") )
      ;;
    watch)
      COMPREPLY=( $(compgen -W "--pipeline --dir --recursive --settle --output --once" -- "$cur") )
      ;;
    pipeline)
      COMPREPLY=( $(compgen -W "list export import --json --out --name" -- "$cur") )
      ;;
    serve)
      COMPREPLY=( $(compgen -W "--addr --token" -- "$cur") )
      ;;
    history)
      COMPREPLY=( $(compgen -W "list clear --json" -- "$cur") )
      ;;
    ops|engines|doctor)
      COMPREPLY=( $(compgen -W "--json" -- "$cur") )
      ;;
    completion)
      COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
      ;;
  esac
}
complete -F _wb wb
`

const zshCompletion = `#compdef wb

_wb() {
  local -a commands
  commands=(run watch ops engines pipeline serve doctor history completion version help)
  if (( CURRENT == 2 )); then
    compadd -a commands
    return
  fi
  case ${words[2]} in
    run) compadd -- --op --pipeline --input --output --param --recursive --include --exclude --min-size --max-size --newer-than --skip-hidden ;;
    watch) compadd -- --pipeline --dir --recursive --settle --output --once ;;
    pipeline) compadd -- list export import --json --out --name ;;
    serve) compadd -- --addr --token ;;
    history) compadd -- list clear --json ;;
    ops|engines|doctor) compadd -- --json ;;
    completion) compadd -- bash zsh fish ;;
  esac
}

compdef _wb wb
`

const fishCompletion = `complete -c wb -f -n '__fish_use_subcommand' -a run -d 'Run an operation or saved pipeline'
complete -c wb -f -n '__fish_use_subcommand' -a watch -d 'Watch a folder and run a pipeline'
complete -c wb -f -n '__fish_use_subcommand' -a ops -d 'List operations'
complete -c wb -f -n '__fish_use_subcommand' -a engines -d 'Probe engines'
complete -c wb -f -n '__fish_use_subcommand' -a pipeline -d 'Manage saved pipelines'
complete -c wb -f -n '__fish_use_subcommand' -a serve -d 'Start the local API'
complete -c wb -f -n '__fish_use_subcommand' -a doctor -d 'Check engines and configuration'
complete -c wb -f -n '__fish_use_subcommand' -a history -d 'List or clear job history'
complete -c wb -f -n '__fish_use_subcommand' -a completion -d 'Generate shell completions'
complete -c wb -n '__fish_seen_subcommand_from run' -l op -r
complete -c wb -n '__fish_seen_subcommand_from run' -l pipeline -r
complete -c wb -n '__fish_seen_subcommand_from run' -l input -r
complete -c wb -n '__fish_seen_subcommand_from run' -l output -r
complete -c wb -n '__fish_seen_subcommand_from run' -l param -r
complete -c wb -n '__fish_seen_subcommand_from run' -l recursive
complete -c wb -n '__fish_seen_subcommand_from run' -l include -r
complete -c wb -n '__fish_seen_subcommand_from run' -l exclude -r
complete -c wb -n '__fish_seen_subcommand_from run' -l min-size -r
complete -c wb -n '__fish_seen_subcommand_from run' -l max-size -r
complete -c wb -n '__fish_seen_subcommand_from run' -l newer-than -r
complete -c wb -n '__fish_seen_subcommand_from run' -l skip-hidden
complete -c wb -n '__fish_seen_subcommand_from watch' -l pipeline -r
complete -c wb -n '__fish_seen_subcommand_from watch' -l dir -r
complete -c wb -n '__fish_seen_subcommand_from watch' -l recursive
complete -c wb -n '__fish_seen_subcommand_from watch' -l settle -r
complete -c wb -n '__fish_seen_subcommand_from watch' -l output -r
complete -c wb -n '__fish_seen_subcommand_from watch' -l once
complete -c wb -n '__fish_seen_subcommand_from ops engines doctor' -l json
complete -c wb -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
`

func truncateString(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func runTUI() int {
	a := app.Boot()
	jobRunner := a.NewRunner()
	defer jobRunner.Close()

	model := tui.New(tui.Options{
		Config:     a.Config,
		Paths:      a.Paths,
		Registry:   a.Registry,
		Operations: a.Operations,
		Pipelines:  a.Pipelines,
		History:    a.History,
		State:      a.State,
		Saved:      a.Saved,
		Runner:     jobRunner,
		FirstRun:   a.FirstRun,
		Warnings:   a.Warnings,
	})
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "wb: %v\n", err)
		return 1
	}
	return 0
}

func printVersion() {
	fmt.Printf("workbench %s\n", version.Version)
	fmt.Printf("commit %s · built %s · %s %s/%s\n",
		version.Commit, version.Date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func usage(w *os.File) {
	fmt.Fprint(w, `Workbench — a terminal workbench for file operations.

Usage:
  wb              start the TUI
  wb run          run one operation or saved pipeline
  wb watch        watch a folder and run a saved pipeline on changes
  wb ops          list operations (--json)
  wb engines      probe engines (--json)
  wb pipeline     list, export, or import saved pipelines
  wb serve        start the local agent API (default 127.0.0.1:8765)
  wb mcp          run the Model Context Protocol server on stdio
  wb doctor       check engines and configuration
  wb history      list or clear saved job history
  wb completion   generate bash, zsh, or fish completions
  wb version      print build information
  wb help         show this help

Run options:
  --op            operation id, for example image.convert
  --pipeline      saved pipeline name (use instead of --op)
  --input         input path, glob, or @alias
  --recursive     walk directories and simple globs recursively
  --include       include glob (repeatable)
  --exclude       exclude glob (repeatable)
  --min-size      minimum file size, e.g. 10k or 2mb
  --max-size      maximum file size, e.g. 10k or 2mb
  --newer-than    only files modified within this age, e.g. 24h
  --skip-hidden   skip dot-files and dot-directories
  --output        output directory; defaults to the configured output dir
  --param         operation parameter key=value (repeatable)
  --dry-run       print the resolved plan without running

Watch options:
  --pipeline      saved pipeline name (required)
  --dir           directory to watch (required)
  --recursive     watch subdirectories
  --settle        wait after the last change (default 2s)
  --output        output directory
  --once          exit after the first completed batch

Serve options:
  --addr          listen address (default 127.0.0.1:8765)
  --token         bearer token; defaults to WORKBENCH_API_TOKEN
`)
}

func printDryRun(a *app.App, p core.Pipeline, inputs []core.Artifact, target core.Target) int {
	fmt.Printf("dry run · %d input(s) · %s\n", len(inputs), target.Dir)
	names := make([]string, 0, len(p.Steps))
	for _, step := range p.Steps {
		op, ok := a.Operations.Get(step.Op)
		if !ok {
			names = append(names, step.Op)
			continue
		}
		names = append(names, op.Spec().Title)
	}
	fmt.Printf("pipeline: %s\n", strings.Join(names, " → "))
	collision := string(target.Collision)
	if collision == "" {
		collision = "suffix"
	}
	template := target.Template
	if template == "" {
		template = "{stem}.{ext}"
	}
	fmt.Printf("target: %s · template %s · collision %s\n", target.Dir, template, collision)

	last := p.Steps[len(p.Steps)-1]
	if op, ok := a.Operations.Get(last.Op); ok {
		spec := op.Spec()
		format := spec.Output.StaticFormat
		if spec.Output.FormatParam != "" {
			format = last.Params.String(spec.Output.FormatParam)
			if format == "" {
				if param, ok := spec.Param(spec.Output.FormatParam); ok && param.Default != nil {
					format = fmt.Sprint(param.Default)
				}
			}
		}
		for i, input := range inputs {
			fmt.Printf("  %s -> %s\n", input.Path, target.OutputPath(input, format, last.Op, i, time.Now()))
		}
	}

	needed := map[engines.ID]bool{}
	for _, step := range p.Steps {
		if op, ok := a.Operations.Get(step.Op); ok {
			for _, id := range op.Spec().Engines {
				if id != engines.Internal {
					needed[id] = true
				}
			}
		}
	}
	if len(needed) > 0 && a.Registry != nil && a.Registry.Len() > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		infos := a.Registry.ProbeAll(ctx)
		cancel()
		available := make(map[engines.ID]bool, len(infos))
		for _, info := range infos {
			available[info.ID] = info.Available
		}
		missing := false
		for _, step := range p.Steps {
			op, ok := a.Operations.Get(step.Op)
			if !ok {
				continue
			}
			for _, id := range op.Spec().Engines {
				if id == engines.Internal || !needed[id] {
					continue
				}
				if !available[id] {
					fmt.Fprintf(os.Stderr, "  missing engine: %s\n", id)
					if op.Spec().RequireEngines {
						missing = true
					}
				}
			}
		}
		if missing {
			return 1
		}
	}

	return 0
}

func runMCP(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "help", "--help", "-h":
			fmt.Println("usage: wb mcp")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "wb mcp: unexpected argument %q\n", args[0])
			return 2
		}
	}
	a := app.Boot()
	for _, warning := range a.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	r := a.NewRunner()
	defer r.Close()
	server := mcp.New(a, r, os.Stdin, os.Stdout)
	if err := server.Serve(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "wb mcp: %v\n", err)
		return 1
	}
	return 0
}
