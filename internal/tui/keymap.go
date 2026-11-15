package tui

import "strings"

// This file is the single source of truth for the keys the UI advertises.
// Handlers still live with their screens, but the footer and the help card are
// generated from these tables so they cannot drift apart silently.

type keyContext string

const (
	ctxGlobal         keyContext = "global"
	ctxHome           keyContext = "home"
	ctxOperations     keyContext = "operations"
	ctxComposeInput   keyContext = "compose-input"
	ctxComposeOutput  keyContext = "compose-output"
	ctxCompose        keyContext = "compose"
	ctxComposeStep    keyContext = "compose-step"
	ctxJobs           keyContext = "jobs"
	ctxReview         keyContext = "review"
	ctxJobLog         keyContext = "job-log"
	ctxDetector       keyContext = "detector"
	ctxDetectorReport keyContext = "detector-report"
	ctxPreview        keyContext = "preview"
	ctxAliases        keyContext = "aliases"
	ctxEngines        keyContext = "engines"
	ctxEngineInstall  keyContext = "engine-install"
	ctxSettings       keyContext = "settings"
	ctxStats          keyContext = "statistics"
	ctxThemes         keyContext = "themes"
	ctxEditor         keyContext = "editor"
	ctxPalette        keyContext = "palette"
	ctxPicker         keyContext = "picker"
	ctxPrompt         keyContext = "prompt"
	ctxOnboarding     keyContext = "onboarding"
	ctxConfirm        keyContext = "confirm"
	ctxHelp           keyContext = "help"
)

type keyHint struct {
	Key   string
	Label string
}

type helpSection struct {
	Title string
	Hints []keyHint
}

var helpSectionOrder = []keyContext{
	ctxGlobal, ctxHome, ctxOperations, ctxComposeInput, ctxComposeOutput, ctxCompose,
	ctxComposeStep, ctxJobs,
	ctxReview, ctxJobLog, ctxDetector, ctxDetectorReport, ctxPreview, ctxAliases,
	ctxEngines, ctxEngineInstall, ctxSettings, ctxStats, ctxThemes, ctxEditor,
	ctxPalette, ctxPicker, ctxPrompt, ctxOnboarding, ctxConfirm, ctxHelp,
}

var sectionTitles = map[keyContext]string{
	ctxGlobal:         "Global",
	ctxHome:           "Home",
	ctxOperations:     "New job search",
	ctxComposeInput:   "Pipeline · input",
	ctxComposeOutput:  "Pipeline · output",
	ctxCompose:        "Pipeline · steps",
	ctxComposeStep:    "Step editor",
	ctxJobs:           "Jobs",
	ctxReview:         "Review outputs",
	ctxJobLog:         "Job log",
	ctxDetector:       "AI detection",
	ctxDetectorReport: "AI report",
	ctxPreview:        "Preview",
	ctxAliases:        "Aliases",
	ctxEngines:        "Engines",
	ctxEngineInstall:  "Install engines",
	ctxSettings:       "Settings",
	ctxStats:          "Statistics",
	ctxThemes:         "Themes",
	ctxEditor:         "Editor",
	ctxPalette:        "Command palette",
	ctxPicker:         "File picker",
	ctxPrompt:         "Prompt",
	ctxOnboarding:     "Setup",
	ctxConfirm:        "Confirm",
	ctxHelp:           "Help",
}

// hints are ordered by importance: the footer takes as many as fit, so the
// first entries are the ones a new user needs most.
var hints = map[keyContext][]keyHint{
	ctxGlobal: {
		{"esc", "back / close"},
		{"?", "help"},
		{"ctrl+p", "palette"},
		{":", "command mode"},
		{"q", "quit"},
		{"ctrl+c", "quit / interrupt"},
	},
	ctxHome: {
		{"ctrl+p", "palette"},
		{"t", "theme"},
		{"q", "quit"},
		{"n", "new job"},
		{"a", "AI detector"},
		{"e", "editor"},
		{"j", "jobs"},
		{"v", "statistics"},
		{"s", "settings"},
		{"?", "help"},
	},
	ctxOperations: {
		{"type", "filter"},
		{"enter", "open"},
		{"esc", "clear / back"},
		{"ctrl+j/k", "move"},
	},
	ctxComposeInput: {
		{"enter", "continue"},
		{"tab", "complete"},
		{"esc", "back"},
		{"ctrl+f", "pick files"},
	},
	ctxComposeOutput: {
		{"enter", "continue"},
		{"tab", "beside input"},
		{"esc", "back"},
	},
	ctxCompose: {
		{"enter", "edit step"},
		{"n", "add step"},
		{"ctrl+r", "run"},
		{"ctrl+s", "save"},
		{"ctrl+o", "preview"},
		{"d", "delete step"},
		{"alt+↑/↓", "reorder"},
	},
	ctxComposeStep: {
		{"tab", "field"},
		{"enter", "save step"},
		{"esc", "cancel"},
		{"←/→", "adjust / move"},
		{"space", "toggle"},
	},
	ctxJobs: {
		{"enter", "open"},
		{"/", "filter"},
		{"l", "logs"},
		{"j/k", "move"},
		{"a", "review outputs"},
		{"u", "undo output"},
		{"r", "rerun"},
		{"R", "retry failed"},
		{"o", "reveal"},
		{"c", "copy"},
		{"x", "cancel"},
		{"s", "sort"},
	},
	ctxReview: {
		{"space", "toggle"},
		{"enter", "apply"},
		{"esc", "cancel"},
		{"a", "keep all"},
		{"d", "reject"},
	},
	ctxJobLog: {
		{"j/k", "scroll"},
		{"/", "search"},
		{"n/N", "next / previous"},
		{"c", "copy"},
		{"esc", "close"},
	},
	ctxDetector: {
		{"j/k", "scroll"},
		{"enter", "report"},
		{"h", "humanize"},
		{"c", "copy"},
		{"r", "re-detect"},
		{"esc", "back"},
	},
	ctxDetectorReport: {
		{"j/k", "scroll"},
		{"h", "humanize"},
		{"c", "copy"},
		{"esc", "back"},
	},
	ctxPreview: {
		{"e", "edit"},
		{"esc", "back"},
	},
	ctxAliases: {
		{"/", "filter"},
		{"e", "edit config"},
		{"esc", "back"},
	},
	ctxEngines: {
		{"/", "filter"},
		{"i", "install engines"},
		{"r", "rescan"},
		{"esc", "back"},
	},
	ctxEngineInstall: {
		{"space", "toggle"},
		{"a", "select all missing"},
		{"enter", "install"},
		{"esc", "close"},
		{"x", "cancel"},
	},
	ctxStats: {
		{"j/k", "scroll"},
		{"esc", "back"},
	},
	ctxSettings: {
		{"j/k", "move"},
		{"enter", "edit"},
		{"t", "theme"},
		{"e", "config"},
		{"esc", "back"},
	},
	ctxThemes: {
		{"type", "filter"},
		{"ctrl+j/k", "move"},
		{"enter", "apply"},
		{"esc", "cancel"},
	},
	ctxEditor: {
		{"ctrl+s", "save"},
		{"ctrl+z", "undo"},
		{"ctrl+f", "find"},
		{"ctrl+n/p", "next / previous"},
		{"esc", "close"},
	},
	ctxPalette: {
		{"↑↓", "move"},
		{"ctrl+j/k", "move"},
		{"enter", "select"},
		{"ctrl+d", "delete pipeline"},
		{"esc", "close"},
	},
	ctxPicker: {
		{"space", "pick"},
		{"ctrl+s", "use files"},
		{"enter", "open / pick"},
		{"→", "open folder"},
		{"esc", "cancel"},
	},
	ctxPrompt: {
		{"tab", "complete"},
		{"enter", "submit"},
		{"esc", "cancel"},
	},
	ctxOnboarding: {
		{"space", "toggle"},
		{"i", "install engines"},
		{"enter", "finish"},
		{"esc", "skip"},
		{"q", "quit"},
	},
	ctxConfirm: {
		{"y", "confirm"},
		{"esc", "cancel"},
	},
	ctxHelp: {
		{"j/k", "scroll"},
		{"esc", "close"},
	},
}

func (m App) activeContext() keyContext {
	switch {
	case m.installer != nil:
		return ctxEngineInstall
	case m.confirmQuit:
		return ctxConfirm
	case m.confirm != nil:
		return ctxConfirm
	case m.prompt != nil:
		return ctxPrompt
	case m.logView != nil:
		return ctxJobLog
	case m.palette != nil:
		return ctxPalette
	case m.picker != nil:
		return ctxPicker
	case m.showHelp:
		return ctxHelp
	}
	if m.screen == screenEditor && m.editor != nil {
		return ctxEditor
	}
	if m.screen == screenThemes && m.themePicker != nil {
		return ctxThemes
	}
	switch m.screen {
	case screenHome:
		return ctxHome
	case screenOperations:
		return ctxOperations
	case screenCompose:
		if m.compose != nil {
			if m.compose.editing >= 0 {
				return ctxComposeStep
			}
			switch m.compose.focus {
			case focusInput:
				return ctxComposeInput
			case focusOutput:
				return ctxComposeOutput
			}
		}
		return ctxCompose
	case screenJobs:
		return ctxJobs
	case screenReview:
		return ctxReview
	case screenDetector:
		return ctxDetector
	case screenDetectorReport:
		return ctxDetectorReport
	case screenPreview:
		return ctxPreview
	case screenAliases:
		return ctxAliases
	case screenEngines:
		return ctxEngines
	case screenSettings:
		return ctxSettings
	case screenStats:
		return ctxStats
	case screenOnboarding:
		return ctxOnboarding
	}
	return ctxGlobal
}

// footerHints keeps the bottom bar to the three actions that matter on the
// current screen. The full list stays in the contextual help card.
const maxFooterHints = 3

func (m App) footerHints() []keyHint {
	if m.screen == screenCompose && m.compose != nil &&
		m.compose.focus == focusOutput && m.compose.isURLFlow() {
		return []keyHint{{"enter", "continue"}, {"esc", "back"}, {"ctrl+r", "run"}}
	}
	all := hints[m.activeContext()]
	if len(all) > maxFooterHints {
		all = all[:maxFooterHints]
	}
	return append([]keyHint(nil), all...)
}

func (m App) footerText() string {
	var text string
	switch {
	case m.installer != nil && m.installer.running:
		return "x cancel · esc cancel"
	case m.installer != nil:
		return joinHints(m.footerHints())
	case m.confirmQuit:
		return "y quit · q quit · esc stay"
	case m.confirm != nil:
		if m.confirm.showToggle {
			return "space toggle · y confirm · esc cancel"
		}
		return "y confirm · esc cancel"
	case m.prompt != nil:
		if m.prompt.action == promptOpenFile {
			return "↑↓ choose · tab complete · enter open"
		}
		return "enter save · esc cancel"
	case m.logView != nil && m.logView.searching:
		return "type search · enter find · esc cancel"
	case m.logView != nil:
		return joinHints(m.footerHints())
	case m.palette != nil:
		return joinHints(m.footerHints())
	case m.picker != nil:
		return joinHints(m.footerHints())
	case m.showHelp:
		return joinHints(m.footerHints())
	case m.screen == screenJobs && m.jobFiltering:
		return "type filter · enter open · esc clear"
	case m.screen == screenAliases && m.aliasFiltering:
		return "type filter · enter done · esc clear"
	case m.screen == screenEngines && m.engineFiltering:
		return "type filter · enter done · esc clear"
	case m.screen == screenEditor && m.editor != nil && m.editor.pendingClose:
		return "s save · d discard · esc cancel"
	case m.screen == screenEditor && m.editor != nil && m.editor.searching:
		return "type to find · enter next · esc cancel"
	default:
		text = joinHints(m.footerHints())
	}
	return text
}

func joinHints(hints []keyHint) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		parts = append(parts, h.Key+" "+h.Label)
	}
	return strings.Join(parts, " · ")
}

func (m App) helpSections() []helpSection {
	active := m.activeContext()
	out := make([]helpSection, 0, 2)
	for _, ctx := range helpSectionOrder {
		if ctx != ctxGlobal && ctx != active {
			continue
		}
		out = append(out, helpSection{Title: sectionTitles[ctx], Hints: hints[ctx]})
	}
	return out
}

// capturesText reports whether printable keys belong to a text field or a
// type-to-filter surface. Global single-letter commands must not run while it
// is true, or the user cannot type "q" into a filter.
func (m App) capturesText() bool {
	switch {
	case m.prompt != nil, m.palette != nil, m.editor != nil:
		return true
	case m.screen == screenThemes && m.themePicker != nil:
		return true
	case m.screen == screenOperations:
		return true
	case m.screen == screenAliases && m.aliasFiltering:
		return true
	case m.screen == screenEngines && m.engineFiltering:
		return true
	case m.screen == screenJobs && m.jobFiltering:
		return true
	case m.logView != nil && m.logView.searching:
		return true
	}
	if m.screen != screenCompose || m.compose == nil {
		return false
	}
	c := m.compose
	if c.editing >= 0 {
		return len(c.fields) > 0
	}
	return c.focus == focusInput || c.focus == focusOutput
}
