package tui

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"

	"github.com/solomonadzape95/workbench/internal/alias"
	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
)

const inputDebounce = 120 * time.Millisecond

type composeFocus int

const (
	focusInput composeFocus = iota
	focusOutput
	focusSteps
)

type compose struct {
	input     string
	output    string
	collision core.Collision
	steps     []*composeStep
	selected  int
	focus     composeFocus
	savedName string
	mode      core.SourceKind

	editing    int // -1 when the chain is shown; otherwise the step being edited
	fields     []*composeField
	fieldFocus int

	dirty bool

	inputVersion int
	resolving    bool
	artifacts    []core.Artifact
	suggestions  []string
	suggestIndex int
	inputErr     error
	formErr      error
	broken       bool
}

type composeStep struct {
	op      core.Operation
	spec    core.Spec
	params  core.Values
	onError core.OnError
}

type composeField struct {
	spec    core.ParamSpec
	text    string
	cursor  int
	boolean bool
	option  int
}

func newCompose(op core.Operation, cfg config.Config) *compose {
	c := newComposeBase(cfg)
	c.steps = append(c.steps, newComposeStep(op))
	c.refreshInputMode()
	return c
}

func newComposeFromPipeline(p core.Pipeline, reg *core.Registry, cfg config.Config) *compose {
	c := newComposeBase(cfg)
	c.savedName = p.Name

	for i, step := range p.Steps {
		op, ok := reg.Get(step.Op)
		if !ok {
			c.formErr = fmt.Errorf("step %d uses an operation that is not available: %s", i+1, step.Op)
			c.broken = true
			continue
		}
		params := op.Spec().DefaultValues()
		for key, value := range step.Params {
			params[key] = value
		}
		onError := step.OnError
		if onError == "" {
			onError = core.OnErrorContinue
		}
		c.steps = append(c.steps, &composeStep{op: op, spec: op.Spec(), params: params, onError: onError})
	}
	if len(c.steps) == 0 {
		c.formErr = fmt.Errorf("this pipeline has no runnable steps")
		c.broken = true
	}
	c.refreshInputMode()
	return c
}

func newComposeBase(cfg config.Config) *compose {
	c := &compose{
		output:    defaultOutputDir(cfg),
		collision: core.Collision(cfg.Output.Collision),
		editing:   -1,
		focus:     focusInput,
	}
	if c.collision == "" {
		c.collision = core.CollisionSuffix
	}
	return c
}

func (m App) newCompose(op core.Operation) *compose {
	c := newCompose(op, m.opts.Config)
	if m.lastOutput != "" {
		c.output = m.lastOutput
	}
	if m.lastInput != "" && sourceKindMatches(op.Spec(), m.lastInput) {
		c.input = m.lastInput
		c.dirty = true
		c.refreshInputMode()
	}
	return c
}

func sourceKindMatches(spec core.Spec, input string) bool {
	url := strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://")
	return (spec.SourceKind() == core.SourceURLs) == url
}

func (m App) newComposeFromPipeline(p core.Pipeline) *compose {
	c := newComposeFromPipeline(p, m.opts.Operations, m.opts.Config)
	if m.lastOutput != "" {
		c.output = m.lastOutput
	}
	return c
}

func newComposeStep(op core.Operation) *composeStep {
	spec := op.Spec()
	return &composeStep{op: op, spec: spec, params: spec.DefaultValues(), onError: core.OnErrorContinue}
}

func (c *compose) addStep(op core.Operation) *composeStep {
	step := newComposeStep(op)
	c.steps = append(c.steps, step)
	c.selected = len(c.steps) - 1
	c.focus = focusSteps
	c.dirty = true
	c.refreshInputMode()
	return step
}

func (c *compose) deleteSelected() {
	if c.selected < 0 || c.selected >= len(c.steps) {
		return
	}
	c.steps = append(c.steps[:c.selected], c.steps[c.selected+1:]...)
	c.selected = clamp(c.selected, 0, max(len(c.steps)-1, 0))
	c.formErr = nil
	c.dirty = true
	c.refreshInputMode()
}

func (c *compose) moveSelected(delta int) {
	target := c.selected + delta
	if c.selected < 0 || c.selected >= len(c.steps) || target < 0 || target >= len(c.steps) {
		return
	}
	c.steps[c.selected], c.steps[target] = c.steps[target], c.steps[c.selected]
	c.selected = target
	c.formErr = nil
	c.dirty = true
	c.refreshInputMode()
}

func (c *compose) openStep(index int) {
	if index < 0 || index >= len(c.steps) {
		return
	}
	c.selected = index
	c.editing = index
	c.focus = focusSteps
	c.fieldFocus = 0
	c.formErr = nil
	c.fields = buildFields(c.steps[index].spec, c.steps[index].params)
}

func (c *compose) commitStep() bool {
	if c.editing < 0 || c.editing >= len(c.steps) {
		return false
	}
	step := c.steps[c.editing]
	values, err := valuesFromFields(step.spec, c.fields)
	if err != nil {
		c.formErr = err
		return false
	}
	step.params = values
	c.editing = -1
	c.fields = nil
	c.formErr = nil
	c.dirty = true
	return true
}

func (c *compose) cancelStep() {
	c.editing = -1
	c.fields = nil
	c.formErr = nil
}

func (c *compose) handleChainKey(msg tea.KeyPressMsg, cfg config.Config) tea.Cmd {
	key := msg.String()
	if c.focus == focusInput && c.mode != core.SourceURLs && len(c.suggestions) > 0 {
		switch key {
		case "down":
			c.suggestIndex = clamp(c.suggestIndex+1, 0, len(c.suggestions)-1)
			return nil
		case "up":
			c.suggestIndex = clamp(c.suggestIndex-1, 0, len(c.suggestions)-1)
			return nil
		case "enter", "tab":
			c.setText(replaceLastToken(c.input, c.suggestions[c.suggestIndex]))
			return c.afterTextChange(cfg)
		}
	}
	switch key {
	case "tab":
		c.focus = composeFocus((int(c.focus) + 1) % 3)
		return nil
	case "shift+tab":
		c.focus = composeFocus((int(c.focus) + 2) % 3)
		return nil
	case "ctrl+right", "alt+right":
		c.focus = composeFocus((int(c.focus) + 1) % 3)
		return nil
	case "ctrl+left", "alt+left":
		c.focus = composeFocus((int(c.focus) + 2) % 3)
		return nil
	}

	if c.focus == focusSteps {
		switch key {
		case "j", "down":
			c.selected = clamp(c.selected+1, 0, max(len(c.steps)-1, 0))
		case "k", "up":
			c.selected = clamp(c.selected-1, 0, max(len(c.steps)-1, 0))
		case "alt+down":
			c.moveSelected(1)
		case "alt+up":
			c.moveSelected(-1)
		}
		return nil
	}

	return c.handleTextKey(msg, cfg)
}

func (c *compose) handleTextKey(msg tea.KeyPressMsg, cfg config.Config) tea.Cmd {
	switch msg.String() {
	case "backspace":
		c.setText(strings.TrimSuffix(c.text(), lastRune(c.text())))
		return c.afterTextChange(cfg)
	case "ctrl+u":
		c.setText("")
		return c.afterTextChange(cfg)
	}
	if msg.Text == "" {
		return nil
	}
	c.setText(c.text() + msg.Text)
	return c.afterTextChange(cfg)
}

func (c *compose) text() string {
	if c.focus == focusInput {
		return c.input
	}
	return c.output
}

func (c *compose) setText(value string) {
	if c.focus == focusInput {
		c.input = value
	} else {
		c.output = value
	}
}

func (c *compose) afterTextChange(cfg config.Config) tea.Cmd {
	c.formErr = nil
	c.dirty = true
	if c.focus != focusInput {
		return nil
	}
	c.inputErr = nil
	c.artifacts = nil
	c.inputVersion++
	if c.focus == focusInput && c.mode != core.SourceURLs {
		c.suggestions = completeInput(c.input, cfg.Aliases)
		c.suggestIndex = clamp(c.suggestIndex, 0, max(len(c.suggestions)-1, 0))
	} else {
		c.suggestions = nil
	}

	if c.mode == core.SourceURLs {
		c.resolving = false
		arts, err := parseURLArtifacts(c.input)
		if err != nil {
			c.inputErr = err
			return nil
		}
		c.artifacts = arts
		return nil
	}

	if strings.TrimSpace(c.input) == "" {
		c.resolving = false
		return nil
	}
	c.resolving = true
	version := c.inputVersion
	return tea.Tick(inputDebounce, func(time.Time) tea.Msg {
		return resolveTickMsg{version: version}
	})
}

func (c *compose) refreshInputMode() {
	mode := core.SourceFiles
	if len(c.steps) > 0 && c.steps[0].spec.SourceKind() == core.SourceURLs {
		mode = core.SourceURLs
	}
	if c.mode == "" {
		c.mode = mode
		return
	}
	if c.mode != mode {
		c.mode = mode
		c.input = ""
		c.artifacts = nil
		c.inputErr = nil
		c.resolving = false
		c.formErr = nil
	}
}

func (c *compose) isURLFlow() bool {
	return c.mode == core.SourceURLs
}

func parseURLArtifacts(raw string) ([]core.Artifact, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' })
	var out []core.Artifact
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parsed, err := url.Parse(part)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("not an http(s) URL: %s", part)
		}
		out = append(out, core.Artifact{
			Path:   part,
			Kind:   core.Video,
			Format: "url",
			Label:  part,
			Meta:   map[string]any{"url": part},
		})
	}
	return out, nil
}

func (c *compose) handleFieldKey(msg tea.KeyPressMsg) {
	if len(c.fields) == 0 {
		return
	}
	switch msg.String() {
	case "tab":
		c.fieldFocus = (c.fieldFocus + 1) % len(c.fields)
		c.resetFieldCursor()
		return
	case "shift+tab":
		c.fieldFocus = (c.fieldFocus - 1 + len(c.fields)) % len(c.fields)
		c.resetFieldCursor()
		return
	case "up":
		c.fieldFocus = (c.fieldFocus - 1 + len(c.fields)) % len(c.fields)
		c.resetFieldCursor()
		return
	case "down":
		c.fieldFocus = (c.fieldFocus + 1) % len(c.fields)
		c.resetFieldCursor()
		return
	case "left":
		if c.moveFieldCursor(-1) {
			return
		}
		if !c.changeNumeric(-1) {
			c.changeChoice(-1)
		}
		return
	case "right":
		if c.moveFieldCursor(1) {
			return
		}
		if !c.changeNumeric(1) {
			c.changeChoice(1)
		}
		return
	case "ctrl+left", "alt+left":
		c.moveFieldWord(-1)
		return
	case "ctrl+right", "alt+right":
		c.moveFieldWord(1)
		return
	case "home", "ctrl+a":
		if c.fieldAcceptsText() {
			c.fields[c.fieldFocus].cursor = 0
		}
		return
	case "end", "ctrl+e":
		if c.fieldAcceptsText() {
			c.fields[c.fieldFocus].cursor = len([]rune(c.fieldText()))
		}
		return
	case " ", "space":
		if c.activateChoice() {
			return
		}
	case "backspace":
		if c.fieldAcceptsText() {
			c.backspaceField()
		}
		return
	case "delete":
		if c.fieldAcceptsText() {
			c.deleteField()
		}
		return
	case "ctrl+w":
		if c.fieldAcceptsText() {
			c.deleteFieldWord()
		}
		return
	case "ctrl+u":
		if c.fieldAcceptsText() {
			c.deleteFieldToStart()
		}
		return
	}
	if msg.Text != "" && c.fieldAcceptsText() {
		c.insertFieldText(msg.Text)
	}
}

func (c *compose) resetFieldCursor() {
	if c.fieldFocus < 0 || c.fieldFocus >= len(c.fields) {
		return
	}
	f := c.fields[c.fieldFocus]
	f.cursor = clampi(f.cursor, 0, len([]rune(f.text)))
}

func (c *compose) moveFieldCursor(delta int) bool {
	if !c.fieldAcceptsText() {
		return false
	}
	f := c.fields[c.fieldFocus]
	// Numeric fields keep the familiar left/right = decrement/increment
	// behavior; home/end and word keys still edit the text.
	switch f.spec.Type {
	case core.ParamInt, core.ParamFloat:
		return false
	}
	f.cursor = clampi(f.cursor+delta, 0, len([]rune(f.text)))
	return true
}

func (c *compose) moveFieldWord(delta int) {
	if !c.fieldAcceptsText() {
		return
	}
	f := c.fields[c.fieldFocus]
	runes := []rune(f.text)
	pos := clampi(f.cursor, 0, len(runes))
	if delta < 0 {
		for pos > 0 && !isWordRune(runes[pos-1]) {
			pos--
		}
		for pos > 0 && isWordRune(runes[pos-1]) {
			pos--
		}
	} else {
		for pos < len(runes) && isWordRune(runes[pos]) {
			pos++
		}
		for pos < len(runes) && !isWordRune(runes[pos]) {
			pos++
		}
	}
	f.cursor = pos
}

func (c *compose) insertFieldText(text string) {
	f := c.fields[c.fieldFocus]
	runes := []rune(f.text)
	pos := clampi(f.cursor, 0, len(runes))
	insert := []rune(singleLine(text))
	out := make([]rune, 0, len(runes)+len(insert))
	out = append(out, runes[:pos]...)
	out = append(out, insert...)
	out = append(out, runes[pos:]...)
	f.text = string(out)
	f.cursor = pos + len(insert)
}

func (c *compose) backspaceField() {
	f := c.fields[c.fieldFocus]
	runes := []rune(f.text)
	pos := clampi(f.cursor, 0, len(runes))
	if pos == 0 {
		return
	}
	f.text = string(append(runes[:pos-1], runes[pos:]...))
	f.cursor = pos - 1
}

func (c *compose) deleteField() {
	f := c.fields[c.fieldFocus]
	runes := []rune(f.text)
	pos := clampi(f.cursor, 0, len(runes))
	if pos >= len(runes) {
		return
	}
	f.text = string(append(runes[:pos], runes[pos+1:]...))
}

func (c *compose) deleteFieldWord() {
	f := c.fields[c.fieldFocus]
	runes := []rune(f.text)
	pos := clampi(f.cursor, 0, len(runes))
	start := pos
	for start > 0 && !isWordRune(runes[start-1]) {
		start--
	}
	for start > 0 && isWordRune(runes[start-1]) {
		start--
	}
	f.text = string(append(runes[:start], runes[pos:]...))
	f.cursor = start
}

func (c *compose) deleteFieldToStart() {
	f := c.fields[c.fieldFocus]
	runes := []rune(f.text)
	pos := clampi(f.cursor, 0, len(runes))
	f.text = string(runes[pos:])
	f.cursor = 0
}

func isWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '_'
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (c *compose) fieldAcceptsText() bool {
	if c.fieldFocus < 0 || c.fieldFocus >= len(c.fields) {
		return false
	}
	switch c.fields[c.fieldFocus].spec.Type {
	case core.ParamEnum, core.ParamBool:
		return false
	default:
		return true
	}
}

func (c *compose) fieldText() string { return c.fields[c.fieldFocus].text }

func (c *compose) changeNumeric(delta int) bool {
	if c.fieldFocus < 0 || c.fieldFocus >= len(c.fields) {
		return false
	}
	f := c.fields[c.fieldFocus]
	switch f.spec.Type {
	case core.ParamInt:
		n, _ := strconv.Atoi(strings.TrimSpace(f.text))
		n += delta
		if f.spec.Min != nil && float64(n) < *f.spec.Min {
			n = int(*f.spec.Min)
		}
		if f.spec.Max != nil && float64(n) > *f.spec.Max {
			n = int(*f.spec.Max)
		}
		f.text = strconv.Itoa(n)
		return true
	case core.ParamFloat:
		n, _ := strconv.ParseFloat(strings.TrimSpace(f.text), 64)
		n += float64(delta)
		if f.spec.Min != nil && n < *f.spec.Min {
			n = *f.spec.Min
		}
		if f.spec.Max != nil && n > *f.spec.Max {
			n = *f.spec.Max
		}
		f.text = strconv.FormatFloat(n, 'f', -1, 64)
		return true
	}
	return false
}

func (c *compose) changeChoice(delta int) {
	if c.fieldFocus < 0 || c.fieldFocus >= len(c.fields) {
		return
	}
	f := c.fields[c.fieldFocus]
	switch f.spec.Type {
	case core.ParamEnum:
		if len(f.spec.Options) == 0 {
			return
		}
		f.option = (f.option + delta + len(f.spec.Options)) % len(f.spec.Options)
	case core.ParamBool:
		f.boolean = !f.boolean
	}
}

func (c *compose) activateChoice() bool {
	if c.fieldFocus < 0 || c.fieldFocus >= len(c.fields) {
		return false
	}
	f := c.fields[c.fieldFocus]
	switch f.spec.Type {
	case core.ParamEnum:
		c.changeChoice(1)
		return true
	case core.ParamBool:
		f.boolean = !f.boolean
		return true
	}
	return false
}

func (c *compose) tailArtifact() core.Artifact {
	return c.chainArtifact(len(c.steps) - 1)
}

func (c *compose) chainArtifact(index int) core.Artifact {
	if index < 0 || index >= len(c.steps) {
		return core.Artifact{Kind: core.Unknown}
	}
	step := c.steps[index]
	kind := step.spec.OutputKind()
	format := stepOutputFormat(step)
	if format == "" && index > 0 {
		format = c.chainArtifact(index - 1).Format
	}
	if kind == core.Unknown && index > 0 {
		kind = c.chainArtifact(index - 1).Kind
	}
	return core.Artifact{Kind: kind, Format: format}
}

func (c *compose) canAppend(op core.Operation) (bool, string) {
	if len(c.steps) > 0 {
		last := c.steps[len(c.steps)-1].spec
		if last.Aggregate() || (last.Direct() && !last.Chainable) {
			return false, "this operation must be the last step"
		}
	}
	if op.Spec().SourceKind() == core.SourceURLs && len(c.steps) > 0 {
		return false, "URL operations must be the first step"
	}
	tail := c.tailArtifact()
	if tail.Kind == core.Unknown {
		return true, ""
	}
	if op.Spec().AcceptsArtifact(tail) {
		return true, ""
	}
	return false, fmt.Sprintf("does not accept %s/%s", tail.Kind, tail.Format)
}

func (c *compose) buildPipeline(name string) core.Pipeline {
	if name == "" {
		if c.savedName != "" {
			name = c.savedName
		} else {
			name = c.defaultName()
		}
	}
	steps := make([]core.Step, 0, len(c.steps))
	titles := make([]string, 0, len(c.steps))
	for i, step := range c.steps {
		input := core.StepInput{}
		if i > 0 {
			input.From = "previous"
		}
		steps = append(steps, core.Step{
			ID:      fmt.Sprintf("step-%d", i+1),
			Op:      step.spec.ID,
			Params:  step.params,
			Input:   input,
			OnError: step.onError,
		})
		titles = append(titles, step.spec.Title)
	}
	return core.Pipeline{
		Name:        name,
		Version:     core.PipelineVersion,
		Description: strings.Join(titles, " → "),
		Steps:       steps,
	}
}

func (c *compose) defaultName() string {
	if len(c.steps) == 0 {
		return "pipeline"
	}
	title := c.steps[0].spec.Title
	if len(c.steps) > 1 {
		title += " chain"
	}
	return title
}

func (c *compose) validateStatic(reg *core.Registry) error {
	if c.broken {
		return core.NewError(core.CodeUsage, "", "", "this pipeline references operations that are not available", "discard it and rebuild the chain")
	}
	if len(c.steps) == 0 {
		return core.NewError(core.CodeUsage, "", "", "add at least one step", "")
	}
	for i, step := range c.steps {
		if step.op == nil {
			return core.NewError(core.CodeUsage, "", "", fmt.Sprintf("step %d is unavailable", i+1), "")
		}
		if step.spec.SourceKind() == core.SourceURLs && i != 0 {
			return core.NewError(core.CodeUsage, step.spec.ID, "",
				"a URL operation must be the first step", "move it to the top of the chain")
		}
		if err := core.Validate(step.spec.Params, step.params); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	if err := c.buildPipeline("").Validate(reg); err != nil {
		return err
	}
	for i := 1; i < len(c.steps); i++ {
		prev := c.chainArtifact(i - 1)
		if prev.Kind == core.Unknown {
			continue
		}
		if !c.steps[i].spec.AcceptsArtifact(prev) {
			return core.NewError(core.CodeUsage, c.steps[i].spec.ID, "",
				fmt.Sprintf("step %d (%s) cannot consume %s/%s from step %d",
					i+1, c.steps[i].spec.Title, prev.Kind, prev.Format, i),
				"change the earlier output format or remove the incompatible step")
		}
	}
	return nil
}

func (c *compose) validateChain(reg *core.Registry) error {
	if err := c.validateStatic(reg); err != nil {
		return err
	}
	if len(c.artifacts) == 0 {
		if c.mode == core.SourceURLs {
			return core.NewError(core.CodeUsage, "", "", "no URLs", "enter at least one http(s) URL")
		}
		return core.NewError(core.CodeUsage, "", "", "no input files", "enter a path, glob, or alias")
	}
	first := c.steps[0].spec
	for _, item := range c.artifacts {
		if !first.AcceptsArtifact(item) {
			return core.NewError(core.CodeInput, first.ID, item.Path,
				fmt.Sprintf("%s does not accept this file type", first.Title),
				"choose a different first step")
		}
	}
	return nil
}

func (c *compose) target(cfg config.Config) core.Target {
	return core.Target{
		Dir:       expandOutputDir(c.output),
		Template:  cfg.Output.Template,
		Collision: c.collision,
	}
}

func stepOutputFormat(step *composeStep) string {
	if step == nil {
		return ""
	}
	if step.spec.Output.FormatParam != "" {
		return step.params.String(step.spec.Output.FormatParam)
	}
	return step.spec.Output.StaticFormat
}

func buildFields(spec core.Spec, params core.Values) []*composeField {
	out := make([]*composeField, 0, len(spec.Params))
	for _, ps := range spec.Params {
		value := params[ps.Key]
		if value == nil {
			value = ps.Default
		}
		f := &composeField{spec: ps}
		switch ps.Type {
		case core.ParamEnum:
			want := fmt.Sprint(value)
			for i, opt := range ps.Options {
				if opt.Value == want {
					f.option = i
					break
				}
			}
		case core.ParamBool:
			f.boolean, _ = value.(bool)
		default:
			if value != nil {
				f.text = fmt.Sprint(value)
			}
			f.cursor = len([]rune(f.text))
		}
		out = append(out, f)
	}
	return out
}

func valuesFromFields(spec core.Spec, fields []*composeField) (core.Values, error) {
	values := make(core.Values, len(fields))
	for _, f := range fields {
		switch f.spec.Type {
		case core.ParamEnum:
			if len(f.spec.Options) == 0 {
				return nil, fmt.Errorf("%s has no choices", labelOf(f.spec))
			}
			values[f.spec.Key] = f.spec.Options[f.option].Value
		case core.ParamBool:
			values[f.spec.Key] = f.boolean
		case core.ParamInt:
			n, err := strconv.Atoi(strings.TrimSpace(f.text))
			if err != nil {
				return nil, fmt.Errorf("%s must be a whole number", labelOf(f.spec))
			}
			values[f.spec.Key] = n
		case core.ParamFloat:
			n, err := strconv.ParseFloat(strings.TrimSpace(f.text), 64)
			if err != nil {
				return nil, fmt.Errorf("%s must be a number", labelOf(f.spec))
			}
			values[f.spec.Key] = n
		default:
			values[f.spec.Key] = f.text
		}
	}
	if err := core.Validate(spec.Params, values); err != nil {
		return nil, err
	}
	return values, nil
}

type resolveTickMsg struct{ version int }

type resolvedMsg struct {
	version   int
	artifacts []core.Artifact
	err       error
}

func resolveInputs(version int, raw string, aliases []config.Alias, last []core.Artifact) tea.Cmd {
	return func() tea.Msg {
		raw = strings.TrimSpace(raw)
		if raw == "@last" {
			if len(last) == 0 {
				return resolvedMsg{version: version, err: fmt.Errorf("no previous job inputs")}
			}
			return resolvedMsg{version: version, artifacts: last}
		}
		if raw == "@clipboard" {
			text, err := clipboard.ReadAll()
			if err != nil {
				return resolvedMsg{version: version, err: fmt.Errorf("clipboard is unavailable: %w", err)}
			}
			arts, err := alias.Resolve(text, aliases)
			return resolvedMsg{version: version, artifacts: arts, err: err}
		}
		arts, err := alias.Resolve(raw, aliases)
		return resolvedMsg{version: version, artifacts: arts, err: err}
	}
}

func (m App) composeView(height int) string {
	c := m.compose
	if c == nil {
		return ""
	}
	if c.editing >= 0 {
		return m.stepEditorView(height)
	}

	width := m.width - 4
	title := "Pipeline"
	if c.savedName != "" {
		title = "Pipeline · #" + c.savedName
	}

	if width >= 118 {
		leftW := max(width*52/100, 50)
		rightW := width - leftW - 1
		left := m.panel(title, m.composePipelineContent(leftW-4), leftW, height, c.focus != focusSteps)
		right := m.panel("Steps", m.composeStepsContent(rightW-4), rightW, height, c.focus == focusSteps)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	}

	pipeH := max(height*45/100, 7)
	if pipeH > height-7 {
		pipeH = height - 7
	}
	stepsH := height - pipeH - 1
	if stepsH < 6 {
		stepsH = 6
		pipeH = height - stepsH - 1
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.panel(title, m.composePipelineContent(width-4), width, pipeH, c.focus != focusSteps),
		m.panel("Steps", m.composeStepsContent(width-4), width, stepsH, c.focus == focusSteps))
}

func (m App) composePipelineContent(width int) string {
	c := m.compose
	valueWidth := width - 14 - 4
	if valueWidth < 16 {
		valueWidth = 16
	}
	inputLabel := "Input"
	inputPlaceholder := "path, glob, or @alias"
	if c.isURLFlow() {
		inputLabel = "URL"
		inputPlaceholder = "https://… (comma or newline separated)"
	}

	var lines []string
	if summary := m.chainSummary(c); summary != "" {
		lines = append(lines, m.theme.muted.Render(truncate(summary, width)))
	}
	lines = append(lines,
		m.chainRow(c, focusInput, inputLabel, c.input, inputPlaceholder, valueWidth),
		m.inputStatus(c),
	)

	lines = append(lines,
		m.chainRow(c, focusOutput, "Output", c.output, "", valueWidth),
		"",
	)
	if c.formErr != nil {
		lines = append(lines,
			m.theme.danger.Render(truncate(c.formErr.Error(), width)))
		if hint := core.HintOf(c.formErr); hint != "" {
			lines = append(lines, m.theme.warning.Render(truncate("  "+hint, width)))
		}
	} else if help := m.chainHelp(c); help != "" {
		lines = append(lines, m.theme.muted.Render(truncate(help, width)))
	}
	return strings.Join(lines, "\n")
}

func (m App) composeStepsContent(width int) string {
	c := m.compose
	var lines []string
	if len(c.steps) == 0 {
		lines = append(lines, m.theme.warning.Render("no steps — press n to add one"))
	} else {
		lines = append(lines, strings.Split(strings.TrimRight(m.stepList(c, width), "\n"), "\n")...)
	}
	lines = append(lines, "", m.theme.muted.Render("enter edit · n add · d remove · J/K move"))
	if len(c.steps) > 0 {
		step := c.steps[clamp(c.selected, 0, len(c.steps)-1)]
		lines = append(lines, "", m.theme.accent.Render("Selected step"))
		if len(step.spec.Params) == 0 {
			lines = append(lines, m.theme.muted.Render("  no options"))
		}
		for _, p := range step.spec.Params {
			value := fmt.Sprint(step.params[p.Key])
			if p.Type == core.ParamEnum {
				for _, opt := range p.Options {
					if opt.Value == value && opt.Label != "" {
						value = opt.Label
						break
					}
				}
			}
			if p.Type == core.ParamBool {
				if step.params.Bool(p.Key) {
					value = "on"
				} else {
					value = "off"
				}
			}
			lines = append(lines, "  "+m.theme.muted.Render(padRight(labelOf(p), 14))+truncate(value, max(width-18, 8)))
		}
	}
	return strings.Join(lines, "\n")
}

func (m App) stepEditorView(height int) string {
	c := m.compose
	if c == nil || c.editing < 0 || c.editing >= len(c.steps) {
		return ""
	}
	step := c.steps[c.editing]
	width := m.width - 4
	contentWidth := width - 4
	valueWidth := contentWidth - 14 - 4
	if valueWidth < 16 {
		valueWidth = 16
	}

	var lines []string
	if step.spec.Summary != "" {
		lines = append(lines, m.theme.muted.Render(truncate(step.spec.Summary, contentWidth)), "")
	}
	if len(c.fields) == 0 {
		lines = append(lines, m.theme.muted.Render("This step has no options."))
	}
	for i, f := range c.fields {
		focused := i == c.fieldFocus
		marker := "  "
		labelStyle := m.theme.muted
		if focused {
			marker = m.theme.accent.Render("▸ ")
			labelStyle = m.theme.accent
		}
		label, value := renderField(m.theme, f, valueWidth, focused && fieldAcceptsText(f))
		lines = append(lines, marker+labelStyle.Render(padRight(label, 14))+"  "+value)
		if focused {
			if hint := fieldHint(f); hint != "" {
				lines = append(lines, "    "+m.theme.muted.Render(hint))
			}
		}
	}
	if len(c.fields) > 0 {
		f := c.fields[c.fieldFocus]
		if f.spec.Help != "" {
			lines = append(lines, "", m.theme.muted.Render(truncate(labelOf(f.spec)+" — "+f.spec.Help, contentWidth)))
		}
	}
	if c.formErr != nil {
		lines = append(lines, "", m.theme.danger.Render(truncate(c.formErr.Error(), contentWidth)))
	}
	lines = append(lines, "", m.theme.muted.Render("tab field · ←/→ adjust or move · space toggle · enter save"))
	return m.panel(fmt.Sprintf("Step %d · %s", c.editing+1, step.spec.Title), strings.Join(lines, "\n"), width, height, true)
}

func (m App) chainRow(c *compose, focus composeFocus, label, value, placeholder string, valueWidth int) string {
	marker := "  "
	labelStyle := m.theme.muted
	if c.focus == focus {
		marker = m.theme.accent.Render("▸ ")
		labelStyle = m.theme.accent
	}
	display := truncate(value, valueWidth)
	if value == "" && placeholder != "" {
		display = m.theme.muted.Render(placeholder)
	}
	if c.focus == focus {
		display += m.theme.accent.Render("▏")
	}
	return marker + labelStyle.Render(padRight(label, 14)) + "  " + display
}

func (m App) stepList(c *compose, width int) string {
	const visible = 5
	start := 0
	if c.selected >= visible {
		start = c.selected - visible + 1
	}
	var b strings.Builder
	for i := start; i < start+visible && i < len(c.steps); i++ {
		step := c.steps[i]
		marker := "  "
		if i == c.selected {
			marker = m.theme.accent.Render("▸ ")
		}
		titleStyle := lipgloss.NewStyle()
		if c.focus == focusSteps && i == c.selected {
			titleStyle = m.theme.accent
		}
		summary := stepParamsSummary(step)
		b.WriteString(marker)
		b.WriteString(titleStyle.Render(fmt.Sprintf("%d. %s", i+1, padRight(step.spec.Title, 24))))
		b.WriteString(m.theme.muted.Render(truncate(summary, max(width-32, 10))))
		b.WriteString("\n")
	}
	if len(c.steps) > visible {
		b.WriteString(m.theme.muted.Render(fmt.Sprintf("  … %d steps total", len(c.steps))))
		b.WriteString("\n")
	}
	return b.String()
}

func renderField(th theme, f *composeField, valueWidth int, showCursor bool) (label, value string) {
	label = labelOf(f.spec)
	switch f.spec.Type {
	case core.ParamEnum:
		optLabel := f.spec.Options[f.option].Value
		if f.spec.Options[f.option].Label != "" {
			optLabel = f.spec.Options[f.option].Label
		}
		value = "‹ " + optLabel + " ›"
	case core.ParamBool:
		if f.boolean {
			value = "[x] On"
		} else {
			value = "[ ] Off"
		}
	default:
		text := f.text
		if showCursor {
			runes := []rune(text)
			pos := clampi(f.cursor, 0, len(runes))
			text = string(runes[:pos]) + "▏" + string(runes[pos:])
		}
		value = "[" + truncate(text, max(valueWidth-2, 1)) + "]"
	}
	return label, value
}

func fieldHint(f *composeField) string {
	switch f.spec.Type {
	case core.ParamEnum:
		return "←/→ choose"
	case core.ParamBool:
		return "space toggle"
	case core.ParamInt, core.ParamFloat:
		return "←/→ adjust · type a number · home/end · ctrl+u clear"
	case core.ParamPath:
		return "type a path or @alias · ←/→ cursor · ctrl+w word · ctrl+u clear"
	default:
		return "type to edit · ←/→ cursor · ctrl+w word · ctrl+u clear"
	}
}

func fieldAcceptsText(f *composeField) bool {
	switch f.spec.Type {
	case core.ParamEnum, core.ParamBool:
		return false
	default:
		return true
	}
}

func (m App) chainSummary(c *compose) string {
	if len(c.steps) == 0 {
		return ""
	}
	titles := make([]string, 0, len(c.steps))
	for _, step := range c.steps {
		titles = append(titles, step.spec.Title)
	}
	return strings.Join(titles, " → ")
}

func (m App) chainHelp(c *compose) string {
	switch c.focus {
	case focusInput:
		if c.isURLFlow() {
			return "input: one or more http(s) URLs, separated by commas"
		}
		return "type a path or @alias · tab completes · tab / shift+tab moves focus"
	case focusOutput:
		return "where finished files are written · tab / shift+tab moves focus"
	default:
		return "enter edits Format/Quality · n adds a step · ctrl+r runs · ctrl+s saves"
	}
}

func stepParamsSummary(step *composeStep) string {
	if step == nil || len(step.spec.Params) == 0 {
		return ""
	}
	parts := make([]string, 0, len(step.spec.Params))
	for _, p := range step.spec.Params {
		value := step.params[p.Key]
		if value == nil {
			continue
		}
		display := fmt.Sprint(value)
		if p.Type == core.ParamEnum {
			for _, opt := range p.Options {
				if opt.Value == display && opt.Label != "" {
					display = opt.Label
					break
				}
			}
		}
		if p.Type == core.ParamBool {
			if value == true {
				display = "on"
			} else {
				display = "off"
			}
		}
		parts = append(parts, labelOf(p)+" "+display)
	}
	return strings.Join(parts, " · ")
}

func (m App) inputStatus(c *compose) string {
	if c.isURLFlow() {
		switch {
		case c.inputErr != nil:
			return m.theme.danger.Render(truncate("  ! "+c.inputErr.Error(), m.width-6))
		case len(c.artifacts) > 0:
			return m.theme.success.Render(fmt.Sprintf("  %d URL(s)", len(c.artifacts)))
		case c.input == "":
			return m.theme.muted.Render("  enter one or more http(s) URLs")
		default:
			return m.theme.muted.Render("  no valid URLs")
		}
	}
	switch {
	case c.resolving:
		return m.theme.muted.Render("  resolving…")
	case len(c.suggestions) > 0:
		choice := c.suggestions[clamp(c.suggestIndex, 0, len(c.suggestions)-1)]
		return m.theme.muted.Render(fmt.Sprintf("  ↳ %s  (%d/%d) · ↑↓ enter",
			truncate(choice, max(m.width-24, 8)), c.suggestIndex+1, len(c.suggestions)))
	case c.inputErr != nil:
		return m.theme.danger.Render(truncate("  ! "+c.inputErr.Error(), m.width-6))
	case len(c.artifacts) > 0:
		var size int64
		for _, a := range c.artifacts {
			size += a.Size
		}
		noun := "files"
		if len(c.artifacts) == 1 {
			noun = "file"
		}
		return m.theme.success.Render(fmt.Sprintf("  %d %s · %s", len(c.artifacts), noun, humanBytes(size)))
	case c.input == "":
		return m.theme.muted.Render("  type a path, glob (~/Pictures/*.png), or @alias")
	default:
		return m.theme.muted.Render("  no matches yet")
	}
}

func defaultOutputDir(cfg config.Config) string {
	dir := cfg.Output.Dir
	if strings.TrimSpace(dir) == "" {
		dir = "~/Workbench/{date}"
	}
	return expandOutputDir(dir)
}

func expandOutputDir(dir string) string {
	dir = strings.ReplaceAll(dir, "{date}", time.Now().Format("2006-01-02"))
	return config.ExpandHome(dir)
}

func labelOf(spec core.ParamSpec) string {
	if spec.Label != "" {
		return spec.Label
	}
	return spec.Key
}

func padRight(s string, width int) string {
	s = truncate(s, width)
	if n := width - lipgloss.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

func lastRune(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[len(runes)-1])
}
