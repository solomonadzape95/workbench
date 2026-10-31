// Package tui is the terminal front end. It renders state and forwards
// intent; it does not execute file operations itself.
package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/history"
	"github.com/solomonadzape95/workbench/internal/notify"
	"github.com/solomonadzape95/workbench/internal/pipeline"
	"github.com/solomonadzape95/workbench/internal/preview"
	"github.com/solomonadzape95/workbench/internal/runner"
	"github.com/solomonadzape95/workbench/internal/state"
)

const (
	minWidth  = 30
	minHeight = 10
)

type screen int

const (
	screenHome screen = iota
	screenOperations
	screenCompose
	screenJobs
	screenReview
	screenPreview
	screenAliases
	screenOnboarding
	screenEngines
	screenThemes
	screenEditor
	screenSettings
	screenStats
	screenDetector
	screenDetectorReport
)

// Options is everything the TUI needs from the composition root.
type Options struct {
	Config     config.Config
	Paths      config.Paths
	Registry   *engines.Registry
	Operations *core.Registry
	Pipelines  *pipeline.Store
	History    *history.Store
	State      *state.Store
	Saved      state.State
	Runner     *runner.Runner
	FirstRun   bool
	Warnings   []string
}

// App is the root Bubble Tea model.
type App struct {
	opts  Options
	theme theme

	width  int
	height int
	ready  bool

	showHelp    bool
	helpScroll  int
	confirmQuit bool
	probing     bool
	engines     []engines.Info
	notice      string
	toast       string
	toastLevel  core.Level

	jobFiltering    bool
	jobQuery        string
	jobSort         int
	aliasFiltering  bool
	aliasQuery      string
	engineFiltering bool
	engineQuery     string

	screen        screen
	ops           []core.Operation
	pipelineMetas []pipeline.Meta
	opQuery       string
	opIndex       int
	compose       *compose
	jobs          []runner.JobView
	historical    []runner.JobView
	selectedJob   string
	lastInput     string
	lastOutput    string
	restoreDraft  bool

	palette        *palette
	prompt         *prompt
	preview        *previewState
	review         *reviewState
	logView        *logState
	onboarding     *onboarding
	themePicker    *themePicker
	editor         *editorState
	editorReturn   screen
	settingsCursor int
	settingsReturn screen
	detector       *detectorState
	detectorReport *detectorReportState
	confirm        *confirmState
}

type previewState struct {
	path     string
	loading  bool
	returnTo screen
	data     preview.Preview
	err      error
}

type confirmKind int

const (
	confirmDeleteStep confirmKind = iota
	confirmDeletePipeline
	confirmCancelJob
	confirmUndoJob
	confirmDiscardCompose
	confirmQuitApp
)

type confirmState struct {
	kind    confirmKind
	title   string
	body    string
	subject string
}

func New(opts Options) App {
	th, themeErr := themeForConfig(opts.Config, true)
	app := App{
		opts:       opts,
		theme:      th,
		probing:    opts.Registry != nil && opts.Registry.Len() > 0,
		screen:     screenHome,
		ops:        opts.Operations.All(),
		lastInput:  opts.Saved.LastInput,
		lastOutput: opts.Saved.LastOutput,
	}
	if len(opts.Warnings) > 0 {
		app.notice = opts.Warnings[0]
	}
	if themeErr != nil {
		app.notice = themeErr.Error()
	}
	if opts.FirstRun {
		app.screen = screenOnboarding
		app.onboarding = newOnboarding(opts.Config)
	}
	if opts.Pipelines != nil {
		if metas, err := opts.Pipelines.List(); err == nil {
			app.pipelineMetas = metas
		}
	}
	if opts.Registry != nil && opts.Paths.CacheDir != "" {
		if infos := engines.LoadCache(filepath.Join(opts.Paths.CacheDir, "engines.json"), 24*time.Hour); len(infos) > 0 {
			app.engines = infos
			app.probing = false
		}
	}
	if opts.History != nil {
		if records, err := opts.History.Recent(200); err == nil {
			for _, record := range records {
				view := record.View
				view.Historical = true
				app.historical = append(app.historical, view)
			}
		}
	}
	if opts.Runner != nil {
		app.jobs = mergeJobViews(opts.Runner.Jobs(), app.historical)
		if len(app.jobs) > 0 {
			app.selectedJob = app.jobs[0].ID
		}
	} else {
		app.jobs = app.historical
		if len(app.jobs) > 0 {
			app.selectedJob = app.jobs[0].ID
		}
	}
	if opts.Saved.SelectedJob != "" {
		for _, job := range app.jobs {
			if job.ID == opts.Saved.SelectedJob {
				app.selectedJob = job.ID
				break
			}
		}
	}
	if !opts.FirstRun && opts.Operations != nil && opts.Saved.Draft != nil &&
		len(opts.Saved.Draft.Pipeline.Steps) > 0 {
		c := app.newComposeFromPipeline(opts.Saved.Draft.Pipeline)
		c.input = opts.Saved.Draft.Input
		if opts.Saved.Draft.Output != "" {
			c.output = opts.Saved.Draft.Output
		}
		c.savedName = opts.Saved.Draft.SavedName
		c.selected = clamp(opts.Saved.Draft.Selected, 0, max(len(c.steps)-1, 0))
		c.dirty = true
		app.compose = c
		app.screen = screenCompose
		app.restoreDraft = true
		app.toast = "restored unsaved pipeline"
		app.toastLevel = core.LevelInfo
	}
	return app
}

type engineProbeMsg []engines.Info

type jobEventMsg struct {
	Event core.Event
}

type runnerClosedMsg struct{}

type previewMsg struct {
	path string
	data preview.Preview
	err  error
}

type revealMsg struct {
	err error
}

type clipboardMsg struct {
	text string
	err  error
}

func readClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		text, err := clipboard.ReadAll()
		return clipboardMsg{text: text, err: err}
	}
}

func (m App) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.RequestBackgroundColor}
	if m.opts.Registry != nil && m.opts.Registry.Len() > 0 {
		cmds = append(cmds, probeEngines(m.opts.Registry))
	}
	if m.opts.Runner != nil {
		cmds = append(cmds, waitForJobEvent(m.opts.Runner))
	}
	// A restored draft has text but no resolved artifacts yet. Kick the same
	// debounce path the input field uses so ctrl+r works immediately.
	if m.restoreDraft && m.compose != nil && strings.TrimSpace(m.compose.input) != "" {
		cmds = append(cmds, m.compose.afterTextChange(m.opts.Config))
	}
	return tea.Batch(cmds...)
}

func probeEngines(registry *engines.Registry) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return engineProbeMsg(registry.ProbeAll(ctx))
	}
}

func previewFile(path string, width, height int) tea.Cmd {
	return func() tea.Msg {
		data, err := preview.Generate(path, width, height)
		return previewMsg{path: path, data: data, err: err}
	}
}

func (m App) openPreview(path string, returnTo screen) (App, tea.Cmd) {
	if path == "" {
		return m, nil
	}
	m.preview = &previewState{path: path, loading: true, returnTo: returnTo}
	m.screen = screenPreview
	return m, previewFile(path, max(m.width-4, 10), max(m.height-9, 3))
}

func waitForJobEvent(r *runner.Runner) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-r.Events()
		if !ok {
			return runnerClosedMsg{}
		}
		return jobEventMsg{Event: event}
	}
}

func (m App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

	case tea.BackgroundColorMsg:
		th, err := themeForConfig(m.opts.Config, msg.IsDark())
		m.theme = th
		if err != nil {
			m.notice = err.Error()
		}

	case engineProbeMsg:
		m.engines = []engines.Info(msg)
		m.probing = false
		if m.opts.Paths.CacheDir != "" {
			_ = engines.SaveCache(filepath.Join(m.opts.Paths.CacheDir, "engines.json"), m.engines)
		}

	case resolveTickMsg:
		if m.screen == screenCompose && m.compose != nil && msg.version == m.compose.inputVersion {
			return m, resolveInputs(msg.version, m.compose.input, m.opts.Config.Aliases, m.lastInputs())
		}

	case resolvedMsg:
		if m.screen == screenCompose && m.compose != nil && msg.version == m.compose.inputVersion {
			m.compose.artifacts = msg.artifacts
			m.compose.inputErr = msg.err
			m.compose.resolving = false
		}

	case previewMsg:
		if m.preview != nil && m.preview.path == msg.path {
			m.preview.loading = false
			m.preview.data = msg.data
			m.preview.err = msg.err
		}

	case revealMsg:
		if msg.err != nil {
			m.toast = "open output: " + msg.err.Error()
			m.toastLevel = core.LevelError
		}

	case jobEventMsg:
		m.refreshJobs()
		var notifyCmd tea.Cmd
		if done, ok := msg.Event.(core.JobDone); ok {
			notifyCmd = m.onJobDone(done)
			m.saveState()
		}
		if m.opts.Runner != nil {
			return m, tea.Batch(waitForJobEvent(m.opts.Runner), notifyCmd)
		}
		return m, notifyCmd

	case runnerClosedMsg:
		// The runner shut down; stop listening.

	case tea.PasteMsg:
		return m.insertText(msg.Content)

	case clipboardMsg:
		if msg.err != nil {
			m.toast = "clipboard: " + msg.err.Error()
			m.toastLevel = core.LevelError
			return m, nil
		}
		return m.insertText(msg.text)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m App) pasteTarget() bool {
	return m.prompt != nil || m.palette != nil || m.editor != nil || m.themePicker != nil ||
		m.screen == screenOperations || m.jobFiltering || m.aliasFiltering || m.engineFiltering ||
		m.textEntryFocused()
}

func (m App) insertText(text string) (tea.Model, tea.Cmd) {
	if text == "" {
		return m, nil
	}

	if m.prompt != nil {
		m.prompt.value += singleLine(text)
		m.prompt.err = ""
		return m, nil
	}
	if m.palette != nil {
		m.palette.query += singleLine(text)
		m.palette.err = ""
		m.palette.refilter()
		m.palette.cursor = 0
		return m, nil
	}
	if m.screen == screenThemes && m.themePicker != nil {
		m.themePicker.query += singleLine(text)
		m.themePicker.cursor = 0
		m.themePicker.filter()
		return m, nil
	}
	if m.screen == screenOperations {
		m.opQuery += singleLine(text)
		m.opIndex = 0
		return m, nil
	}
	if m.screen == screenJobs && m.jobFiltering {
		m.jobQuery += singleLine(text)
		m.ensureJobSelection()
		return m, nil
	}
	if m.screen == screenAliases && m.aliasFiltering {
		m.aliasQuery += singleLine(text)
		return m, nil
	}
	if m.screen == screenEngines && m.engineFiltering {
		m.engineQuery += singleLine(text)
		return m, nil
	}

	if m.showHelp || m.confirmQuit || m.confirm != nil || m.logView != nil {
		return m, nil
	}
	if m.screen == screenEditor && m.editor != nil {
		if m.editor.searching {
			m.editor.query += singleLine(text)
		} else {
			m.editor.buf.Insert(text)
			m.editor.pendingClose = false
		}
		m.scrollEditor()
		return m, nil
	}
	if m.screen != screenCompose || m.compose == nil {
		m.toast = "select a text field to paste"
		m.toastLevel = core.LevelWarn
		return m, nil
	}
	c := m.compose
	if c.editing >= 0 {
		if len(c.fields) == 0 || !fieldAcceptsText(c.fields[c.fieldFocus]) {
			return m, nil
		}
		c.insertFieldText(singleLine(text))
		return m, nil
	}
	if c.focus != focusInput && c.focus != focusOutput {
		return m, nil
	}
	if c.focus == focusInput {
		c.setText(c.text() + multiValue(text))
		return m, c.afterTextChange(m.opts.Config)
	}
	c.setText(c.text() + singleLine(text))
	return m, c.afterTextChange(m.opts.Config)
}

func singleLine(text string) string {
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	return text
}

func multiValue(text string) string {
	text = strings.ReplaceAll(text, "\r\n", ", ")
	text = strings.ReplaceAll(text, "\r", ", ")
	text = strings.ReplaceAll(text, "\n", ", ")
	return text
}

func (m App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.toast = ""
	key := msg.String()

	if key == "ctrl+c" {
		if m.screen == screenEditor && m.editor != nil {
			if m.editor.buf.Dirty() {
				m.editor.pendingClose = true
				return m, nil
			}
			return m.closeEditor(), nil
		}
		if m.screen == screenJobs {
			if job, ok := m.selectedJobView(); ok && isActive(job.Status) && !job.Historical && m.opts.Runner != nil {
				m.opts.Runner.Cancel(job.ID)
				m.toast = "canceling " + job.Name + " — ctrl+c again to quit"
				m.toastLevel = core.LevelWarn
				return m, nil
			}
		}
		return m.requestQuit()
	}
	if key == "ctrl+v" {
		if m.pasteTarget() {
			return m, readClipboardCmd()
		}
		m.toast = "select a text field to paste"
		m.toastLevel = core.LevelWarn
		return m, nil
	}

	if m.confirmQuit {
		switch key {
		case "q", "y":
			m.saveState()
			return m, tea.Quit
		case "esc", "n":
			m.confirmQuit = false
		}
		return m, nil
	}

	if m.confirm != nil {
		return m.handleConfirmKey(msg)
	}

	if m.prompt != nil {
		return m.handlePromptKey(msg)
	}
	if m.logView != nil {
		return m.handleLogKey(msg)
	}
	if m.palette != nil {
		return m.handlePaletteKey(msg)
	}

	if m.showHelp {
		switch key {
		case "?", "esc", "q", "f1":
			m.showHelp = false
			m.helpScroll = 0
		case "j", "down":
			m.helpScroll++
		case "k", "up":
			m.helpScroll = max(m.helpScroll-1, 0)
		case "pgdown", "ctrl+d":
			m.helpScroll += 8
		case "pgup", "ctrl+u":
			m.helpScroll = max(m.helpScroll-8, 0)
		case "g", "home":
			m.helpScroll = 0
		case "G", "end":
			m.helpScroll = 1 << 20
		}
		return m, nil
	}

	if key == "f1" {
		m.showHelp = true
		m.helpScroll = 0
		return m, nil
	}

	if m.screen == screenEditor && m.editor != nil {
		return m.handleEditorKey(msg)
	}
	if m.screen == screenThemes && m.themePicker != nil {
		return m.handleThemeKey(msg)
	}
	if m.screen == screenSettings {
		return m.handleSettingsKey(msg)
	}

	if m.screen == screenOnboarding {
		return m.handleOnboardingKey(msg)
	}

	if key == "ctrl+p" {
		return m.togglePalette()
	}
	if key == ":" && !m.capturesText() {
		return m.openCommandPalette()
	}
	if (key == "?" || key == "f1") && !m.capturesText() {
		m.showHelp = true
		m.helpScroll = 0
		return m, nil
	}
	if key == "q" && !m.capturesText() {
		return m.requestQuit()
	}

	if key == "esc" {
		if m.screen == screenOperations && m.opQuery != "" {
			m.opQuery = ""
			m.opIndex = 0
			return m, nil
		}
		if m.screen == screenJobs && (m.jobFiltering || m.jobQuery != "") {
			m.jobFiltering = false
			m.jobQuery = ""
			m.ensureJobSelection()
			return m, nil
		}
		if m.screen == screenAliases && (m.aliasFiltering || m.aliasQuery != "") {
			m.aliasFiltering = false
			m.aliasQuery = ""
			return m, nil
		}
		if m.screen == screenEngines && (m.engineFiltering || m.engineQuery != "") {
			m.engineFiltering = false
			m.engineQuery = ""
			return m, nil
		}
		if m.notice != "" {
			m.notice = ""
			return m, nil
		}
		switch m.screen {
		case screenCompose:
			if m.compose != nil && m.compose.editing >= 0 {
				m.compose.cancelStep()
				return m, nil
			}
			if m.composeHasWork() {
				m.confirm = &confirmState{
					kind:  confirmDiscardCompose,
					title: "Discard this pipeline?",
					body:  "The steps and input will not be saved.",
				}
				return m, nil
			}
			m.compose = nil
			m.screen = screenOperations
		case screenOperations, screenJobs, screenAliases, screenEngines:
			m.screen = screenHome
		case screenPreview:
			if m.preview != nil {
				m.screen = m.preview.returnTo
			}
			m.preview = nil
		case screenDetector:
			if m.detector != nil {
				m.screen = m.detector.returnTo
			}
			m.detector = nil
		case screenDetectorReport:
			m.detectorReport = nil
			m.screen = screenDetector
		case screenReview:
			m.review = nil
			m.screen = screenJobs
		case screenStats:
			m.screen = screenHome
		}
		return m, nil
	}

	switch m.screen {
	case screenHome:
		switch key {
		case "n":
			m.screen = screenOperations
			m.opQuery = ""
			m.opIndex = 0
		case "e":
			m.prompt = m.newFilePrompt()
		case "s":
			return m.openSettings(screenHome)
		case "t":
			return m.openThemePicker(screenHome)
		case "j":
			m.ensureJobSelection()
			m.screen = screenJobs
		case "v":
			m.screen = screenStats
		case "a":
			if op, ok := m.opts.Operations.Get("doc.detect_ai"); ok {
				m.compose = m.newCompose(op)
				m.screen = screenCompose
			}
		}

	case screenOperations:
		filtered := m.filteredOps()
		switch key {
		case "down", "ctrl+j":
			m.opIndex = clamp(m.opIndex+1, 0, max(len(filtered)-1, 0))
		case "up", "ctrl+k":
			m.opIndex = clamp(m.opIndex-1, 0, max(len(filtered)-1, 0))
		case "home", "pgup":
			m.opIndex = 0
		case "end":
			m.opIndex = max(len(filtered)-1, 0)
		case "pgdown":
			m.opIndex = clamp(m.opIndex+8, 0, max(len(filtered)-1, 0))
		case "backspace":
			runes := []rune(m.opQuery)
			if len(runes) > 0 {
				m.opQuery = string(runes[:len(runes)-1])
			}
			m.opIndex = 0
		case "ctrl+u":
			m.opQuery = ""
			m.opIndex = 0
		case "enter":
			if len(filtered) > 0 {
				m.opIndex = clamp(m.opIndex, 0, len(filtered)-1)
				m.compose = m.newCompose(filtered[m.opIndex])
				m.screen = screenCompose
			}
		default:
			if msg.Text != "" {
				m.opQuery += msg.Text
				m.opIndex = 0
			}
		}

	case screenCompose:
		c := m.compose
		if c == nil {
			m.screen = screenOperations
			return m, nil
		}

		if c.editing >= 0 {
			switch key {
			case "enter":
				c.commitStep()
			case "ctrl+s":
				if c.commitStep() {
					m.prompt = newSavePrompt(c)
				}
			case "ctrl+r":
				if c.commitStep() {
					return m.runCompose()
				}
			case "ctrl+o", "v":
				if len(c.artifacts) > 0 {
					return m.openPreview(c.artifacts[0].Path, screenCompose)
				}
			default:
				c.handleFieldKey(msg)
			}
			return m, nil
		}

		switch key {
		case "ctrl+r":
			return m.runCompose()
		case "ctrl+s":
			m.prompt = newSavePrompt(c)
			return m, nil
		case "ctrl+o", "v":
			if len(c.artifacts) > 0 {
				return m.openPreview(c.artifacts[0].Path, screenCompose)
			}
			return m, nil
		case "n":
			if c.focus == focusSteps {
				m.palette = m.buildPalette(paletteAppendStep)
				return m, nil
			}
		case "d":
			if c.focus == focusSteps && len(c.steps) > 0 {
				m.confirm = &confirmState{
					kind:  confirmDeleteStep,
					title: "Remove this step?",
					body:  c.steps[c.selected].spec.Title + " will be removed from the chain.",
				}
				return m, nil
			}
		case "J":
			if c.focus == focusSteps {
				c.moveSelected(1)
				return m, nil
			}
		case "K":
			if c.focus == focusSteps {
				c.moveSelected(-1)
				return m, nil
			}
		case "enter":
			if c.focus == focusSteps {
				c.openStep(c.selected)
				return m, nil
			}
		}
		return m, c.handleChainKey(msg, m.opts.Config)

	case screenJobs:
		jobs := m.filteredJobs()
		if m.jobFiltering {
			switch key {
			case "enter":
				m.jobFiltering = false
				if job, ok := m.selectedJobView(); ok {
					next, opened := m.openDetectorForJob(job, screenJobs)
					if opened {
						return next, nil
					}
					return m.openLogView()
				}
			case "backspace":
				runes := []rune(m.jobQuery)
				if len(runes) > 0 {
					m.jobQuery = string(runes[:len(runes)-1])
				}
				m.ensureJobSelection()
			case "ctrl+u":
				m.jobQuery = ""
				m.ensureJobSelection()
			default:
				if msg.Text != "" {
					m.jobQuery += singleLine(msg.Text)
					m.ensureJobSelection()
				}
			}
			return m, nil
		}
		switch key {
		case "/":
			m.jobFiltering = true
			m.jobQuery = ""
			m.ensureJobSelection()
		case "s":
			m.jobSort = (m.jobSort + 1) % 3
			m.toast = "jobs sorted by " + jobSortLabel(m.jobSort)
			m.toastLevel = core.LevelInfo
		case "enter":
			if job, ok := m.selectedJobView(); ok {
				next, opened := m.openDetectorForJob(job, screenJobs)
				if opened {
					return next, nil
				}
				return m.openLogView()
			}
		case "j", "down":
			m.moveJobSelection(1)
		case "k", "up":
			m.moveJobSelection(-1)
		case "g", "home":
			if len(jobs) > 0 {
				m.selectedJob = jobs[0].ID
			}
		case "G", "end":
			if len(jobs) > 0 {
				m.selectedJob = jobs[len(jobs)-1].ID
			}
		case "pgdown", "ctrl+d":
			m.moveJobSelection(8)
		case "pgup", "ctrl+u":
			m.moveJobSelection(-8)
		case "ctrl+o", "v":
			if path := m.jobPreviewPath(); path != "" {
				return m.openPreview(path, screenJobs)
			}
		case "l":
			return m.openLogView()
		case "R":
			return m.retryJob(false)
		case "r":
			return m.retryJob(true)
		case "o":
			if job, ok := m.selectedJobView(); ok {
				return m, revealDirCmd(job.Target.Dir)
			}
		case "c", "y":
			if job, ok := m.selectedJobView(); ok && job.Err != nil {
				text := job.Err.Error()
				if hint := core.HintOf(job.Err); hint != "" {
					text += "\nHint: " + hint
				}
				if err := clipboard.WriteAll(text); err != nil {
					m.toast = "clipboard: " + err.Error()
					m.toastLevel = core.LevelError
				} else {
					m.toast = "job error copied"
					m.toastLevel = core.LevelInfo
				}
			}
		case "u":
			if job, ok := m.selectedJobView(); ok {
				if isActive(job.Status) {
					m.toast = "this job is still running"
					m.toastLevel = core.LevelWarn
					return m, nil
				}
				m.confirm = &confirmState{
					kind:    confirmUndoJob,
					title:   "Undo this job?",
					body:    job.Name + " outputs move to the undo trash; overwritten files are restored.",
					subject: job.ID,
				}
				return m, nil
			}
		case "a":
			return m.openReview(), nil
		case "x":
			if job, ok := m.selectedJobView(); ok && isActive(job.Status) && m.opts.Runner != nil {
				if job.Historical {
					m.toast = "this job is from history and cannot be canceled"
					m.toastLevel = core.LevelWarn
					return m, nil
				}
				m.confirm = &confirmState{
					kind:    confirmCancelJob,
					title:   "Cancel this job?",
					body:    job.Name + " will stop and leave no partial output.",
					subject: job.ID,
				}
				return m, nil
			}
		}

	case screenReview:
		return m.handleReviewKey(msg)
	case screenDetector:
		return m.handleDetectorKey(msg)
	case screenDetectorReport:
		return m.handleDetectorReportKey(msg)

	case screenAliases:
		if m.aliasFiltering {
			switch key {
			case "enter":
				m.aliasFiltering = false
			case "backspace":
				runes := []rune(m.aliasQuery)
				if len(runes) > 0 {
					m.aliasQuery = string(runes[:len(runes)-1])
				}
			case "ctrl+u":
				m.aliasQuery = ""
			default:
				if msg.Text != "" {
					m.aliasQuery += singleLine(msg.Text)
				}
			}
			return m, nil
		}
		switch key {
		case "/":
			m.aliasFiltering = true
			m.aliasQuery = ""
		case "e":
			return m.openConfigEditor()
		}

	case screenPreview:
		switch key {
		case "e":
			if m.preview != nil {
				path := m.preview.path
				returnTo := m.preview.returnTo
				m.preview = nil
				return m.openEditor(path, returnTo)
			}
		}

	case screenEngines:
		if m.engineFiltering {
			switch key {
			case "enter":
				m.engineFiltering = false
			case "backspace":
				runes := []rune(m.engineQuery)
				if len(runes) > 0 {
					m.engineQuery = string(runes[:len(runes)-1])
				}
			case "ctrl+u":
				m.engineQuery = ""
			default:
				if msg.Text != "" {
					m.engineQuery += singleLine(msg.Text)
				}
			}
			return m, nil
		}
		switch key {
		case "/":
			m.engineFiltering = true
			m.engineQuery = ""
		case "r":
			m.probing = true
			if m.opts.Registry != nil {
				return m, probeEngines(m.opts.Registry)
			}
		}
	}

	return m, nil
}

func (m App) textEntryFocused() bool {
	if m.screen != screenCompose || m.compose == nil {
		return false
	}
	c := m.compose
	if c.editing >= 0 {
		if len(c.fields) == 0 {
			return false
		}
		return fieldAcceptsText(c.fields[c.fieldFocus])
	}
	return c.focus == focusInput || c.focus == focusOutput
}

func (m App) togglePalette() (tea.Model, tea.Cmd) {
	if m.palette != nil {
		m.palette = nil
		return m, nil
	}
	target := paletteNewCompose
	if m.screen == screenCompose && m.compose != nil {
		if m.compose.editing >= 0 && !m.compose.commitStep() {
			return m, nil
		}
		target = paletteAppendStep
	}
	m.palette = m.buildPalette(target)
	return m, nil
}

// openCommandPalette opens the same palette as ctrl+p, pre-filled with the
// command prefix. It is the TUI-native ":" path; ctrl+p remains the muscle
// memory shortcut.
func (m App) openCommandPalette() (tea.Model, tea.Cmd) {
	next, cmd := m.togglePalette()
	app := next.(App)
	if app.palette == nil {
		return app, cmd
	}
	app.palette.query = ">"
	app.palette.refilter()
	app.palette.cursor = 0
	return app, cmd
}

func (m App) retryJob(all bool) (tea.Model, tea.Cmd) {
	job, ok := m.selectedJobView()
	if !ok || m.opts.Runner == nil {
		return m, nil
	}
	var (
		next *runner.Job
		err  error
	)
	if job.Historical {
		pipeline := job.Pipeline
		if len(pipeline.Steps) == 0 {
			m.toast = "this history entry has no pipeline to rerun"
			m.toastLevel = core.LevelWarn
			return m, nil
		}
		var inputs []core.Artifact
		for _, item := range job.Items {
			if all || item.Status == core.ItemStatusFailed || item.Status == core.ItemStatusCanceled {
				inputs = append(inputs, item.Input)
			}
		}
		if len(inputs) == 0 {
			m.toast = "this job has no failed items"
			m.toastLevel = core.LevelWarn
			return m, nil
		}
		pipeline.Name = job.Name
		if !all {
			pipeline.Name = job.Name + " retry"
		}
		next, err = m.opts.Runner.Start(pipeline, inputs, job.Target)
	} else if all {
		next, err = m.opts.Runner.Rerun(job.ID)
	} else {
		next, err = m.opts.Runner.RetryFailed(job.ID)
	}
	if err != nil {
		m.toast = err.Error()
		m.toastLevel = core.LevelWarn
		return m, nil
	}
	m.refreshJobs()
	m.selectedJob = next.ID()
	verb := "retry"
	if all {
		verb = "re-run"
	}
	m.toast = verb + " started: " + next.Name()
	m.toastLevel = core.LevelInfo
	return m, nil
}

func revealDirCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		if dir == "" {
			return revealMsg{err: fmt.Errorf("no output directory")}
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return revealMsg{err: err}
		}
		var name string
		switch runtime.GOOS {
		case "darwin":
			name = "open"
		case "windows":
			name = "explorer"
		default:
			name = "xdg-open"
		}
		return revealMsg{err: exec.Command(name, dir).Run()}
	}
}

func (m App) requestQuit() (tea.Model, tea.Cmd) {
	if m.confirmQuit {
		m.saveState()
		return m, tea.Quit
	}
	if m.activeJobCount() > 0 {
		m.confirmQuit = true
		return m, nil
	}
	if m.composeHasWork() {
		m.confirm = &confirmState{
			kind:  confirmQuitApp,
			title: "Quit and discard this pipeline?",
			body:  "The current steps and input are not saved.",
		}
		return m, nil
	}
	m.saveState()
	return m, tea.Quit
}

func (m App) handleConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	c := m.confirm
	if c == nil {
		return m, nil
	}
	switch msg.String() {
	case "esc", "n", "q":
		m.confirm = nil
		return m, nil
	case "y", "enter":
		// Enter confirms the generic danger dialogs; quit is the one place
		// where Enter deliberately does nothing.
	}
	if msg.String() != "y" && msg.String() != "enter" {
		return m, nil
	}
	if msg.String() == "enter" && c.kind == confirmQuitApp {
		return m, nil
	}

	switch c.kind {
	case confirmDeleteStep:
		m.confirm = nil
		if m.compose != nil {
			m.compose.deleteSelected()
		}
		m.toast = "step removed"
		m.toastLevel = core.LevelInfo
	case confirmDeletePipeline:
		m.confirm = nil
		m.deletePipelineNow(c.subject)
	case confirmCancelJob:
		m.confirm = nil
		if m.opts.Runner != nil && c.subject != "" {
			m.opts.Runner.Cancel(c.subject)
			m.toast = "canceling " + c.title
			m.toastLevel = core.LevelWarn
		}
	case confirmUndoJob:
		m.confirm = nil
		job, ok := m.jobByID(c.subject)
		if !ok {
			m.toast = "job not found"
			m.toastLevel = core.LevelWarn
			return m, nil
		}
		result, err := runner.UndoJob(job, filepath.Join(m.opts.Paths.DataDir, "undo"))
		if err != nil {
			m.toast = "undo: " + err.Error()
			m.toastLevel = core.LevelError
			return m, nil
		}
		m.toast = fmt.Sprintf("undid %d output(s), restored %d overwritten file(s)", result.Files, result.Restored)
		m.toastLevel = core.LevelInfo
	case confirmDiscardCompose:
		m.confirm = nil
		m.compose = nil
		m.screen = screenOperations
		m.saveState()
	case confirmQuitApp:
		m.confirm = nil
		m.saveState()
		return m, tea.Quit
	}
	return m, nil
}

func (m App) confirmStateView() string {
	c := m.confirm
	if c == nil {
		return ""
	}
	lines := []string{
		m.theme.warning.Render(c.title),
		"",
		m.theme.muted.Render(c.body),
		"",
		m.theme.muted.Render("y  confirm"),
		m.theme.muted.Render("esc  cancel"),
	}
	return m.theme.card.Render(strings.Join(lines, "\n"))
}

func (m App) runCompose() (tea.Model, tea.Cmd) {
	c := m.compose
	if c == nil {
		return m, nil
	}
	if c.editing >= 0 && !c.commitStep() {
		return m, nil
	}
	if m.opts.Runner == nil {
		c.formErr = fmt.Errorf("job runner is unavailable")
		return m, nil
	}
	if c.resolving {
		c.formErr = fmt.Errorf("still resolving inputs — press ctrl+r again")
		return m, nil
	}
	if len(c.artifacts) == 0 {
		if c.isURLFlow() {
			c.formErr = fmt.Errorf("no URLs — enter at least one http(s) URL")
		} else {
			c.formErr = fmt.Errorf("no input files — enter a path, glob, or alias")
		}
		return m, nil
	}
	if err := c.validateChain(m.opts.Operations); err != nil {
		c.formErr = err
		return m, nil
	}

	job, err := m.opts.Runner.Start(c.buildPipeline(""), c.artifacts, c.target(m.opts.Config))
	if err != nil {
		c.formErr = err
		return m, nil
	}

	m.refreshJobs()
	m.selectedJob = job.ID()
	m.lastInput = c.input
	m.lastOutput = c.output
	m.screen = screenJobs
	m.compose = nil
	m.palette = nil
	m.prompt = nil
	m.saveState()
	return m, nil
}

func (m App) openConfigEditor() (tea.Model, tea.Cmd) {
	path := m.opts.Paths.ConfigFile
	if path == "" {
		m.toast = "config path is unavailable"
		m.toastLevel = core.LevelError
		return m, nil
	}
	return m.openEditor(path, m.screen)
}

func (m App) lastInputs() []core.Artifact {
	if len(m.jobs) == 0 {
		return nil
	}
	var out []core.Artifact
	for _, item := range m.jobs[0].Items {
		if item.Input.Path != "" {
			out = append(out, item.Input)
		}
	}
	return out
}

func (m *App) refreshJobs() {
	if m.opts.Runner == nil {
		return
	}
	m.jobs = mergeJobViews(m.opts.Runner.Jobs(), m.historical)
	m.refreshLogView()
	if len(m.jobs) == 0 {
		m.selectedJob = ""
		return
	}
	for _, job := range m.jobs {
		if job.ID == m.selectedJob {
			return
		}
	}
	m.selectedJob = m.jobs[0].ID
}

func mergeJobViews(live, old []runner.JobView) []runner.JobView {
	out := make([]runner.JobView, 0, len(live)+len(old))
	out = append(out, live...)
	out = append(out, old...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].QueuedAt.After(out[j].QueuedAt)
	})
	return out
}

func (m *App) onJobDone(done core.JobDone) tea.Cmd {
	name := "job"
	for _, job := range m.jobs {
		if job.ID == done.JobID {
			name = job.Name
			break
		}
	}
	m.toast = fmt.Sprintf("%s · %s · %s", name, done.Status, humanDuration(done.Duration))
	if m.screen == screenJobs {
		for _, job := range m.jobs {
			if job.ID != done.JobID {
				continue
			}
			if next, opened := m.openDetectorForJob(job, screenJobs); opened {
				*m = next
				return m.notifyJobDone(done)
			}
		}
	}
	switch done.Status {
	case core.StatusFailed:
		m.toastLevel = core.LevelError
	case core.StatusPartial, core.StatusCanceled:
		m.toastLevel = core.LevelWarn
	default:
		m.toastLevel = core.LevelInfo
	}
	return m.notifyJobDone(done)
}

func (m App) notifyJobDone(done core.JobDone) tea.Cmd {
	webhook := strings.TrimSpace(m.opts.Config.Notify.Webhook)
	if !m.opts.Config.General.Notify && webhook == "" {
		return nil
	}
	job := notify.Job{
		ID:         done.JobID,
		Status:     string(done.Status),
		DurationMS: done.Duration.Milliseconds(),
		BytesIn:    done.Metrics.BytesIn,
		BytesOut:   done.Metrics.BytesOut,
	}
	title := "Workbench · " + string(done.Status)
	for _, view := range m.jobs {
		if view.ID != done.JobID {
			continue
		}
		job.Name = view.Name
		if view.Err != nil {
			job.Error = view.Err.Error()
		}
		for _, item := range view.Items {
			for _, output := range item.Outputs {
				job.Outputs = append(job.Outputs, output.Path)
			}
		}
		break
	}
	body := job.Name
	if body == "" {
		body = done.JobID
	}
	body += " · " + humanDuration(done.Duration)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if m.opts.Config.General.Notify {
			_ = notify.Desktop(ctx, title, body)
		}
		if webhook != "" {
			_ = notify.Webhook(ctx, webhook, job)
		}
		return nil
	}
}

func (m *App) ensureJobSelection() {
	jobs := m.filteredJobs()
	if len(jobs) == 0 {
		m.selectedJob = ""
		return
	}
	for _, job := range jobs {
		if job.ID == m.selectedJob {
			return
		}
	}
	m.selectedJob = jobs[0].ID
}

func (m *App) moveJobSelection(delta int) {
	jobs := m.filteredJobs()
	if len(jobs) == 0 {
		return
	}
	m.ensureJobSelection()
	index := 0
	for i, job := range jobs {
		if job.ID == m.selectedJob {
			index = i
			break
		}
	}
	index = clamp(index+delta, 0, len(jobs)-1)
	m.selectedJob = jobs[index].ID
}

func (m App) selectedJobView() (runner.JobView, bool) {
	jobs := m.filteredJobs()
	for _, job := range jobs {
		if job.ID == m.selectedJob {
			return job, true
		}
	}
	if len(jobs) > 0 {
		return jobs[0], true
	}
	return runner.JobView{}, false
}

func (m App) jobByID(id string) (runner.JobView, bool) {
	for _, job := range m.jobs {
		if job.ID == id {
			return job, true
		}
	}
	return runner.JobView{}, false
}

func (m App) jobPreviewPath() string {
	job, ok := m.selectedJobView()
	if !ok {
		return ""
	}
	for _, item := range job.Items {
		if item.Status == core.ItemStatusDone && len(item.Outputs) > 0 && item.Outputs[0].Path != "" {
			return item.Outputs[0].Path
		}
	}
	for _, item := range job.Items {
		if item.Input.Path != "" {
			return item.Input.Path
		}
	}
	return ""
}

func (m App) activeJobCount() int {
	var count int
	for _, job := range m.jobs {
		if isActive(job.Status) {
			count++
		}
	}
	return count
}

func (m App) composeHasWork() bool {
	c := m.compose
	if c == nil {
		return false
	}
	return c.dirty
}

// saveState snapshots the small session surface. It is called at quit, after a
// job starts or finishes, and after a compose discard. Drafts are only written
// when the composer is dirty, so opening and leaving an untouched form does not
// resurrect it on the next launch.
func (m App) saveState() {
	if m.opts.State == nil {
		return
	}
	st := state.State{
		Version:     state.Version,
		LastInput:   m.lastInput,
		LastOutput:  m.lastOutput,
		SelectedJob: m.selectedJob,
	}
	if c := m.compose; c != nil {
		if c.input != "" {
			st.LastInput = c.input
		}
		if c.output != "" {
			st.LastOutput = c.output
		}
		if m.composeHasWork() && len(c.steps) > 0 {
			st.Draft = &state.Draft{
				Pipeline:  c.buildPipeline(""),
				Input:     c.input,
				Output:    c.output,
				SavedName: c.savedName,
				Selected:  c.selected,
			}
		}
	}
	_ = m.opts.State.Save(st)
}

func isActive(status core.Status) bool {
	return status == core.StatusQueued || status == core.StatusRunning || status == core.StatusCancelling
}

func (m App) View() tea.View {
	var content string
	switch {
	case !m.ready:
		content = "Starting Workbench…"
	case m.width < minWidth || m.height < minHeight:
		content = m.tooSmallView()
	default:
		content = m.render()
	}

	view := tea.NewView(content)
	view.AltScreen = true
	if m.theme.bgColor != nil {
		view.BackgroundColor = m.theme.bgColor
		view.ForegroundColor = m.theme.fgColor
	}
	if x, y, ok := m.editorCursor(); ok {
		view.Cursor = tea.NewCursor(x, y)
		view.Cursor.Shape = tea.CursorBar
		view.Cursor.Blink = true
		view.Cursor.Color = m.theme.cursor
	}
	return view
}
