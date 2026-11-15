package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/runner"
)

// padStyled pads a string that already contains ANSI styles. Rune-based
// truncation would count escape bytes and corrupt selected rows.
func padStyled(s string, width int) string {
	s = xansi.Truncate(s, width, "…")
	if n := width - lipgloss.Width(s); n > 0 {
		s += strings.Repeat(" ", n)
	}
	return s
}

func (m App) selectedRow(s string, width int) string {
	// Keep the row's semantic foreground colors (green progress, red errors)
	// while painting the selected background behind every segment. Stripping
	// ANSI first made the progress bar lose its status color.
	bg := lipgloss.NewStyle().Background(m.theme.selected.GetBackground())
	open := ansiPrefix(bg)
	s = padStyled(xansi.Truncate(s, width, "…"), width)
	if open != "" {
		s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+open)
	}
	return open + s + "\x1b[0m"
}

func ansiPrefix(style lipgloss.Style) string {
	rendered := style.Render("x")
	if i := strings.IndexByte(rendered, 'x'); i >= 0 {
		return rendered[:i]
	}
	return ""
}

func (m App) filteredOps() []core.Operation {
	query := strings.ToLower(strings.TrimSpace(m.opQuery))
	if query == "" {
		return m.ops
	}
	out := make([]core.Operation, 0, len(m.ops))
	for _, op := range m.ops {
		spec := op.Spec()
		haystack := strings.ToLower(spec.ID + " " + spec.Title + " " + spec.Summary + " " + string(spec.Domain))
		if strings.Contains(haystack, query) {
			out = append(out, op)
		}
	}
	return out
}

func (m App) operationsView(height int) string {
	width := min(m.width-4, 120)
	contentWidth := width - 4
	var lines []string
	lines = append(lines, m.theme.accent.Render("Search  ")+m.opQuery+m.theme.accent.Render("▏"))
	lines = append(lines, m.theme.muted.Render("Type to filter · ctrl+j/k move · compound presets are in ctrl+p"), "")

	filtered := m.filteredOps()
	if len(filtered) == 0 {
		lines = append(lines, m.theme.muted.Render("No operations match."), "", m.theme.muted.Render("esc clear · q quit"))
		return m.panel("New job", strings.Join(lines, "\n"), width, height, true)
	}
	m.opIndex = clamp(m.opIndex, 0, len(filtered)-1)

	contentHeight := max(height-2, 6)
	// Header takes three rows (search, hint, gap); the selected summary needs
	// two to four. Each list row now occupies two rows because of the gap.
	listRows := max((contentHeight-8+1)/2, 2)
	start := 0
	if m.opIndex >= listRows {
		start = m.opIndex - listRows + 1
	}
	end := min(start+listRows, len(filtered))
	for i := start; i < end; i++ {
		op := filtered[i]
		spec := op.Spec()
		selected := i == m.opIndex
		titleStyle := lipgloss.NewStyle()
		if selected {
			titleStyle = m.theme.accent
		}
		mode := operationModeLabel(spec)
		meta := string(spec.Domain) + " · " + acceptsSummary(spec)
		meta = truncate(meta, max(contentWidth-28-lipgloss.Width(mode)-3, 8))
		line := titleStyle.Render(padRight(spec.Title, 26)) +
			m.theme.muted.Render(padRight(meta, max(contentWidth-28-lipgloss.Width(mode)-3, 8))) +
			m.theme.accent.Render(mode)
		lines = append(lines, m.listRow(line, contentWidth, selected))
		if i < end-1 {
			lines = append(lines, "")
		}
	}
	if end < len(filtered) {
		lines = append(lines, "", m.theme.muted.Render(fmt.Sprintf("  … %d more (ctrl+j/k)", len(filtered)-end)))
	}

	selected := filtered[clamp(m.opIndex, 0, len(filtered)-1)].Spec()
	lines = append(lines, "")
	if selected.Summary != "" {
		lines = append(lines, "  "+truncate(selected.Summary, contentWidth-2))
	}
	lines = append(lines, "  "+m.theme.accent.Render(operationModeLabel(selected))+
		m.theme.muted.Render(" · "+operationModeHint(selected)))
	if params := paramsSummary(selected); params != "" {
		lines = append(lines, "  "+m.theme.muted.Render("Options: "+params))
	}
	return m.panel("New job", strings.Join(lines, "\n"), width, height, true)
}

func (m App) filteredJobs() []runner.JobView {
	query := strings.ToLower(strings.TrimSpace(m.jobQuery))
	out := make([]runner.JobView, 0, len(m.jobs))
	for _, job := range m.jobs {
		if query != "" {
			haystack := strings.ToLower(job.ID + " " + job.Name + " " + string(job.Status) + " " + job.Target.Dir)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		out = append(out, job)
	}
	switch m.jobSort {
	case 1:
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		})
	case 2:
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Status == out[j].Status {
				return out[i].QueuedAt.After(out[j].QueuedAt)
			}
			return out[i].Status < out[j].Status
		})
	default:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].QueuedAt.After(out[j].QueuedAt)
		})
	}
	return out
}

func jobSortLabel(sortMode int) string {
	switch sortMode {
	case 1:
		return "name"
	case 2:
		return "status"
	default:
		return "newest"
	}
}

func (m App) jobsView(height int) string {
	width := m.width - 4
	if width >= 96 {
		leftW := max(width*56/100, 46)
		rightW := width - leftW - 1
		left := m.panel("Jobs", m.jobListContent(leftW-4, max(height-2, 3)), leftW, height, true)
		right := m.panel("Job detail", m.jobDetailContent(rightW-4), rightW, height, false)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	}

	topH := min(max(len(m.jobs)+2, 5), max(height*45/100, 8))
	if topH > height-8 {
		topH = height - 8
	}
	bottomH := height - topH - 1
	return lipgloss.JoinVertical(lipgloss.Left,
		m.panel("Jobs", m.jobListContent(width-4, max(topH-2, 3)), width, topH, true),
		m.panel("Job detail", m.jobDetailContent(width-4), width, bottomH, false))
}

func (m App) jobListContent(width, height int) string {
	jobs := m.filteredJobs()
	if len(jobs) == 0 {
		if m.jobQuery != "" {
			return m.theme.muted.Render("No jobs match. esc clears the filter.")
		}
		return m.theme.muted.Render("No jobs yet — ctrl+p to start one.")
	}
	selected := 0
	for i, job := range jobs {
		if job.ID == m.selectedJob {
			selected = i
			break
		}
	}
	reserved := 0
	if m.jobFiltering || m.jobQuery != "" {
		reserved = 2
	}
	// Each job row is followed by a blank row; reserve one row for the
	// "N jobs total" footer when the list is clipped.
	visible := max((max(height-reserved, 2)+1)/2, 1)
	visible = min(visible, 12)
	start := 0
	if selected >= visible {
		start = selected - visible + 1
	}
	var lines []string
	if reserved > 0 {
		suffix := "▏"
		if !m.jobFiltering {
			suffix = ""
		}
		lines = append(lines, m.theme.accent.Render("Filter  ")+m.jobQuery+suffix, "")
	}
	end := min(start+visible, len(jobs))
	for i := start; i < end; i++ {
		lines = append(lines, m.jobLine(jobs[i], width, jobs[i].ID == m.selectedJob))
		if i < end-1 {
			lines = append(lines, "")
		}
	}
	if end < len(jobs) {
		lines = append(lines, "", m.theme.muted.Render(fmt.Sprintf("  … %d of %d jobs", end, len(jobs))))
	}
	return strings.Join(lines, "\n")
}

func (m App) jobDetailContent(width int) string {
	job, ok := m.selectedJobView()
	if !ok {
		return m.theme.muted.Render("Select a job to see its items and actions.")
	}
	mode := m.jobMode(job)
	counts := job.Counts()
	glyph, style := m.jobGlyph(job.Status)
	lines := []string{
		"  " + style.Render(glyph+" "+string(job.Status)) + "  " +
			m.theme.title.Render(truncate(job.Name, max(width-16, 8))),
	}

	files := fmt.Sprintf("%d/%d", counts.Done, counts.Total)
	if mode == core.ModeAggregate {
		files = fmt.Sprintf("%d inputs", counts.Total)
	}
	summary := []string{m.theme.muted.Render("Files ") + m.theme.accent.Render(files)}
	switch {
	case job.Metrics.BytesIn > 0 && job.Metrics.BytesOut > 0:
		summary = append(summary, m.theme.muted.Render("Size ")+
			m.theme.accent.Render(humanBytes(job.Metrics.BytesIn))+
			m.theme.muted.Render(" → ")+
			m.theme.accent.Render(humanBytes(job.Metrics.BytesOut)))
	case job.Metrics.BytesOut > 0:
		summary = append(summary, m.theme.muted.Render("Downloaded ")+
			m.theme.accent.Render(humanBytes(job.Metrics.BytesOut)))
	}
	if d := job.Duration(); d > 0 {
		summary = append(summary, m.theme.muted.Render("Time ")+m.theme.accent.Render(humanDuration(d)))
	}
	if isCompressionJob(job) && job.Metrics.BytesIn > 0 && job.Metrics.BytesOut > 0 {
		change := (1 - float64(job.Metrics.BytesOut)/float64(job.Metrics.BytesIn)) * 100
		style := m.theme.success
		label := fmt.Sprintf("space saved %.0f%%", change)
		if change < 0 {
			style = m.theme.warning
			label = fmt.Sprintf("space grew %.0f%%", -change)
		}
		summary = append(summary, style.Render(label))
	}
	lines = append(lines, truncateStyled("  "+strings.Join(summary, m.theme.muted.Render("  ·  ")), width))
	if out := m.jobOutputPath(job); out != "" {
		lines = append(lines, "  "+m.theme.muted.Render("Output  ")+truncate(out, max(width-12, 8)))
	}

	if isActive(job.Status) {
		lines = append(lines, "")
		barWidth := min(max(width-24, 10), 24)
		switch mode {
		case core.ModeAggregate, core.ModeDirect:
			label := "combining inputs…"
			if mode == core.ModeDirect {
				label = "working…"
			}
			lines = append(lines, "  "+m.indeterminateBar(barWidth, m.theme.accent)+" "+m.theme.muted.Render(label))
		default:
			lines = append(lines, "  "+m.blockBar(job.Percent(), barWidth, m.theme.accent)+
				" "+m.theme.muted.Render(fmt.Sprintf("%d%% · %d/%d files", job.Percent(), job.Settled(), counts.Total)))
		}
		if stage := currentStage(job); stage != "" {
			lines = append(lines, "  "+m.theme.muted.Render(truncate(stage, width-4)))
		}
	}

	jobErr := job.Err
	if jobErr == nil {
		for _, item := range job.Items {
			if item.Err != nil {
				jobErr = item.Err
				break
			}
		}
	}
	if jobErr != nil && job.Status != core.StatusCanceled {
		lines = append(lines, "", "  "+m.theme.danger.Render("! "+truncate(jobErr.Error(), width-4)))
		if hint := core.HintOf(jobErr); hint != "" {
			lines = append(lines, "  "+m.theme.warning.Render(truncate(hint, width-4)))
		}
	}

	if len(job.Steps) > 0 {
		lines = append(lines, "", "  "+m.theme.muted.Render("Steps"))
		titleW := max(width-18, 12)
		for _, step := range job.Steps {
			mark := m.theme.muted.Render("·")
			stepStyle := m.theme.muted
			switch step.Status {
			case core.StatusRunning:
				mark = m.theme.accent.Render("◐")
				stepStyle = m.theme.accent
			case core.StatusFailed:
				mark = m.theme.danger.Render("✗")
				stepStyle = m.theme.danger
			case core.StatusDone:
				mark = m.theme.success.Render("●")
			}
			line := "  " + mark + "  " + stepStyle.Render(padRight(truncate(step.Title, titleW), titleW))
			if step.Duration > 0 {
				line += " " + numericCell(humanDuration(step.Duration), 6)
			}
			lines = append(lines, line)
		}
	}

	if meta := detectorSummary(job); meta != nil {
		ai := metaFloat(meta, "ai_percent")
		human := metaFloat(meta, "human_percent")
		top := metaString(meta, "top_model")
		line := fmt.Sprintf("AI %.0f%% · human %.0f%%", ai, human)
		if top != "" {
			line += " · " + top
		}
		lines = append(lines, "", "  "+m.theme.danger.Render(truncate(line, width-4)))
	}

	items := job.Items
	itemTitle := "Files"
	if mode == core.ModeAggregate {
		itemTitle = "Inputs combined"
	}
	itemLimit := max(3, 10-len(job.Steps))
	if len(items) > itemLimit {
		items = items[:itemLimit]
	}
	if len(items) > 0 {
		lines = append(lines, "", "  "+m.theme.muted.Render(itemTitle))
		for _, item := range items {
			lines = append(lines, m.itemLine(item, width, mode == core.ModeAggregate))
		}
	}
	if len(job.Items) > len(items) {
		lines = append(lines, "  "+m.theme.muted.Render(fmt.Sprintf("… %d more", len(job.Items)-len(items))))
	}

	if len(job.Aggregate) > 0 {
		lines = append(lines, "", "  "+m.theme.accent.Render("Output"))
		for _, artifact := range job.Aggregate {
			lines = append(lines, "  "+m.theme.success.Render("✓ ")+truncate(artifact.Name(), max(width-6, 8)))
			lines = append(lines, "    "+m.theme.muted.Render(truncate(artifact.Path, max(width-6, 8))))
			lines = append(lines, "    "+m.theme.muted.Render(humanBytes(artifact.Size)))
		}
	}

	hint := "enter log · r rerun · R retry"
	if width >= 52 {
		hint += " · o reveal"
	}
	if detectorSummary(job) != nil {
		hint = "enter AI report · l log · R retry"
	}
	if job.Historical {
		hint = "history · " + hint
	}
	lines = append(lines, "", "  "+m.theme.muted.Render(truncate(hint, width-2)))
	return strings.Join(lines, "\n")
}

func (m App) jobOutputPath(job runner.JobView) string {
	if len(job.Aggregate) > 0 && job.Aggregate[0].Path != "" {
		return filepath.Dir(job.Aggregate[0].Path)
	}
	if job.Target.BesideInput {
		return "beside each input"
	}
	return job.Target.Dir
}

func currentStage(job runner.JobView) string {
	for _, item := range job.Items {
		if item.Status == core.ItemStatusRunning && item.Stage != "" {
			return item.Stage
		}
	}
	return ""
}

func (m App) jobLine(job runner.JobView, width int, selected bool) string {
	glyph, glyphStyle := m.jobGlyph(job.Status)
	mode := m.jobMode(job)
	result := m.jobResult(job, mode)

	duration := ""
	if d := job.Duration(); d > 0 {
		duration = humanDuration(d)
	}
	age := ""
	if job.Historical {
		age = "history"
	} else if !job.QueuedAt.IsZero() {
		age = humanAge(job.QueuedAt)
	}

	// Fixed columns keep names, results, durations, and ages aligned. The
	// age column is the first to go when the list gets narrow.
	resultW, durW, ageW := 18, 7, 0
	if width >= 68 {
		ageW = 8
	}
	nameW := width - 7 - resultW - durW - ageW
	if nameW < 12 {
		durW = 0
		ageW = 0
		nameW = width - 7 - resultW
	}
	if nameW < 10 {
		resultW = 14
		nameW = width - 7 - resultW
	}
	if nameW < 8 {
		resultW = 10
		nameW = width - 7 - resultW
	}
	nameW = max(nameW, 6)

	var b strings.Builder
	b.WriteString(glyphStyle.Render(glyph))
	b.WriteString(" ")
	b.WriteString(padRight(truncate(job.Name, nameW), nameW))
	b.WriteString(" ")
	b.WriteString(padStyled(result, resultW))
	if durW > 0 {
		b.WriteString(" ")
		b.WriteString(numericCell(duration, durW))
	}
	if ageW > 0 {
		b.WriteString(" ")
		b.WriteString(numericCell(age, ageW))
	}
	return m.listRow(b.String(), width, selected)
}

func (m App) jobMode(job runner.JobView) core.Mode {
	if len(job.Aggregate) > 0 {
		return core.ModeAggregate
	}
	ops := make([]string, 0, len(job.Pipeline.Steps)+len(job.Steps))
	for _, step := range job.Pipeline.Steps {
		ops = append(ops, step.Op)
	}
	if len(ops) == 0 {
		for _, step := range job.Steps {
			ops = append(ops, step.Op)
		}
	}
	if m.opts.Operations != nil {
		for _, id := range ops {
			op, ok := m.opts.Operations.Get(id)
			if !ok {
				continue
			}
			if op.Spec().Aggregate() {
				return core.ModeAggregate
			}
			if op.Spec().Direct() {
				return core.ModeDirect
			}
		}
	}
	return core.ModePerItem
}

func (m App) jobResult(job runner.JobView, mode core.Mode) string {
	counts := job.Counts()
	percent := job.Percent()
	switch job.Status {
	case core.StatusQueued:
		return fmt.Sprintf("queued · %d files", counts.Total)
	case core.StatusRunning, core.StatusCancelling:
		switch mode {
		case core.ModeAggregate:
			return m.indeterminateBar(6, m.theme.accent) + " combining"
		case core.ModeDirect:
			return m.indeterminateBar(6, m.theme.accent) + " working"
		default:
			return m.blockBar(percent, 6, m.theme.accent) + fmt.Sprintf(" %d%% · %d/%d", percent, job.Settled(), counts.Total)
		}
	case core.StatusDone:
		if mode == core.ModeAggregate {
			return fmt.Sprintf("%d → 1 file", counts.Total)
		}
		return fmt.Sprintf("done · %d/%d", counts.Done, counts.Total)
	case core.StatusPartial:
		return fmt.Sprintf("%d/%d · %d failed", job.Settled(), counts.Total, counts.Failed)
	case core.StatusFailed:
		if counts.Failed > 0 {
			return fmt.Sprintf("failed · %d/%d", job.Settled(), counts.Total)
		}
		return "failed"
	case core.StatusCanceled:
		return "canceled"
	default:
		return string(job.Status)
	}
}

func humanAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

func (m App) blockBar(percent, width int, style lipgloss.Style) string {
	if width < 1 {
		return ""
	}
	percent = clamp(percent, 0, 100)
	filled := (percent*width + 50) / 100
	return style.Render(strings.Repeat("█", filled)) +
		m.theme.border.Render(strings.Repeat("░", width-filled))
}

// indeterminateBar gives aggregate and direct stages something to show while
// the exact fraction is unknown. It relies on job progress events to repaint.
func (m App) indeterminateBar(width int, style lipgloss.Style) string {
	if width < 1 {
		return ""
	}
	pos := int(time.Now().UnixMilli()/140)%(width+3) - 3
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i >= pos && i < pos+3 {
			b.WriteString(style.Render("█"))
		} else {
			b.WriteString(m.theme.border.Render("░"))
		}
	}
	return b.String()
}

func (m App) itemLine(item runner.ItemView, width int, aggregate bool) string {
	glyph, style := m.itemGlyph(item.Status)
	status := itemStatusLabel(item.Status)
	detail := ""

	if aggregate {
		status = "input"
		detail = humanBytes(item.Input.Size)
	} else {
		switch item.Status {
		case core.ItemStatusDone:
			switch {
			case len(item.Outputs) > 0 && item.Outputs[0].Size > 0:
				detail = humanBytes(item.Outputs[0].Size)
			case item.Input.Size > 0:
				detail = humanBytes(item.Input.Size)
			default:
				detail = "—"
			}
		case core.ItemStatusSkipped:
			detail = "skipped"
		case core.ItemStatusCanceled:
			detail = "canceled"
		case core.ItemStatusPending:
			detail = "pending"
		case core.ItemStatusRunning:
			detail = item.Stage
			if item.Total > 0 {
				detail += fmt.Sprintf(" %d%%", min(item.Done*100/item.Total, 100))
			}
		case core.ItemStatusFailed:
			if item.Err != nil {
				detail = item.Err.Error()
			}
		}
		if detail == "" {
			detail = "—"
		}
	}

	nameW := clamp(width-24, 12, 28)
	statusW := 9
	const glyphW = 4 // two spaces + glyph + trailing space
	detailW := max(width-glyphW-nameW-1-statusW-1, 8)
	line := "  " + style.Render(glyph) + " " +
		padRight(truncate(item.Input.Name(), nameW), nameW) + " " +
		m.theme.muted.Render(padRight(truncate(status, statusW), statusW)) + " " +
		m.theme.muted.Render(truncate(detail, detailW))
	return xansi.Truncate(line, width, "…")
}

func itemStatusLabel(status core.ItemStatus) string {
	switch status {
	case core.ItemStatusDone:
		return "done"
	case core.ItemStatusRunning:
		return "running"
	case core.ItemStatusFailed:
		return "failed"
	case core.ItemStatusCanceled:
		return "canceled"
	case core.ItemStatusSkipped:
		return "skipped"
	default:
		return "pending"
	}
}

func (m App) jobGlyph(status core.Status) (string, lipgloss.Style) {
	switch status {
	case core.StatusRunning:
		return "◐", m.theme.accent
	case core.StatusCancelling:
		return "◒", m.theme.warning
	case core.StatusDone:
		return "●", m.theme.success
	case core.StatusPartial:
		return "◑", m.theme.warning
	case core.StatusFailed:
		return "✗", m.theme.danger
	case core.StatusCanceled:
		return "–", m.theme.muted
	default:
		return "○", m.theme.muted
	}
}

func (m App) itemGlyph(status core.ItemStatus) (string, lipgloss.Style) {
	switch status {
	case core.ItemStatusRunning:
		return "◐", m.theme.accent
	case core.ItemStatusDone:
		return "●", m.theme.success
	case core.ItemStatusFailed:
		return "✗", m.theme.danger
	case core.ItemStatusCanceled, core.ItemStatusSkipped:
		return "–", m.theme.muted
	default:
		return "○", m.theme.muted
	}
}

// progressBarStyle renders a dot-matrix bar: bright braille dots for work
// completed, dim low dots for the remaining track.
func (m App) progressBarStyle(done, total, width int, style lipgloss.Style) string {
	if total <= 0 {
		total = 1
	}
	if width < 1 {
		return ""
	}
	done = clamp(done, 0, total)
	filled := done * width / total
	return style.Render(strings.Repeat("⣿", filled)) +
		m.theme.border.Render(strings.Repeat("⣀", width-filled))
}

func (m App) confirmView(active int) string {
	lines := []string{
		m.theme.warning.Render("Jobs are still running"),
		"",
		fmt.Sprintf("%d active job(s) will be canceled when you quit.", active),
		"",
		m.theme.muted.Render("y / q  quit and cancel"),
		m.theme.muted.Render("esc / n  stay"),
	}
	return m.theme.card.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func operationModeLabel(spec core.Spec) string {
	switch {
	case spec.Aggregate():
		return "combine"
	case spec.Direct():
		return "in place"
	default:
		return "per file"
	}
}

func operationModeHint(spec core.Spec) string {
	switch {
	case spec.Aggregate():
		return "Combines every selected input into one output."
	case spec.Direct():
		return "Updates each input file directly."
	default:
		return "Runs once per input; each file gets its own output."
	}
}

func acceptsSummary(spec core.Spec) string {
	if spec.SourceKind() == core.SourceURLs {
		return "URL"
	}
	if len(spec.Accepts) == 0 {
		return "any file"
	}
	formats := spec.Accepts[0].Formats
	if len(formats) == 0 {
		return "any " + string(spec.Accepts[0].Kind)
	}
	if len(formats) > 4 {
		formats = formats[:4]
	}
	return strings.Join(formats, ", ")
}

func paramsSummary(spec core.Spec) string {
	names := make([]string, 0, len(spec.Params))
	for _, p := range spec.Params {
		names = append(names, labelOf(p))
	}
	return strings.Join(names, ", ")
}

func humanDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PB", value/unit)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m App) previewView() string {
	p := m.preview
	if p == nil {
		return ""
	}
	width := max(m.width-4, 10)
	var b strings.Builder
	b.WriteString(m.theme.title.Render("Preview · " + filepath.Base(p.path)))
	b.WriteString("\n")
	if len(p.data.Meta) > 0 {
		b.WriteString(m.theme.muted.Render(truncate(strings.Join(p.data.Meta, " · "), width)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	switch {
	case p.loading:
		b.WriteString(m.theme.muted.Render("loading preview…"))
	case p.err != nil:
		b.WriteString(m.theme.danger.Render(truncate("! "+p.err.Error(), width)))
	case p.data.ANSI != "":
		b.WriteString(p.data.ANSI)
	case p.data.Text != "":
		b.WriteString(p.data.Text)
	default:
		b.WriteString(m.theme.muted.Render("no preview available"))
	}
	return b.String()
}

func (m App) enginesView(height int) string {
	width := min(m.width-4, 120)
	contentWidth := width - 4
	var b strings.Builder
	if m.engineFiltering || m.engineQuery != "" {
		suffix := "▏"
		if !m.engineFiltering {
			suffix = ""
		}
		b.WriteString(m.theme.accent.Render("Filter  ") + m.engineQuery + suffix)
		b.WriteString("\n\n")
	}
	var infos []engines.Info
	query := strings.ToLower(strings.TrimSpace(m.engineQuery))
	for _, info := range m.engines {
		if query != "" && !strings.Contains(strings.ToLower(string(info.ID)+" "+info.Name+" "+info.Detail+" "+info.Hint), query) {
			continue
		}
		infos = append(infos, info)
	}
	var ready, missing int
	for _, info := range infos {
		if info.Available {
			ready++
		} else {
			missing++
		}
	}
	b.WriteString(m.theme.muted.Render(fmt.Sprintf("%d engines · %d ready · %d missing", len(infos), ready, missing)))
	b.WriteString("\n\n")

	if m.probing {
		b.WriteString(m.theme.muted.Render("scanning…"))
		return m.panel("Engines", b.String(), width, height, true)
	}
	if len(infos) == 0 {
		b.WriteString(m.theme.warning.Render("no engines match"))
		return m.panel("Engines", b.String(), width, height, true)
	}

	nameW, statusW, versionW := 11, 8, 12
	detailW := max(contentWidth-nameW-statusW-versionW-3, 10)
	b.WriteString(m.theme.muted.Render(
		padRight("ENGINE", nameW) + " " + padRight("STATUS", statusW) + " " +
			padRight("VERSION", versionW) + " DETAIL"))
	b.WriteString("\n")
	b.WriteString(m.theme.border.Render(strings.Repeat("─", contentWidth)))
	b.WriteString("\n")
	for i, info := range infos {
		status := m.theme.success.Render(padRight("ready", statusW))
		version := info.Version
		detail := info.Detail
		if !info.Available {
			status = m.theme.warning.Render(padRight("missing", statusW))
			version = "—"
			detail = info.Hint
		}
		if version == "" {
			version = "—"
		}
		name := string(info.ID)
		if info.ID == engines.Internal {
			name = "internal"
		}
		line := m.theme.accent.Render(padRight(truncate(name, nameW), nameW)) + " " +
			status + " " + m.theme.muted.Render(padRight(truncate(version, versionW), versionW)) + " " +
			m.theme.muted.Render(truncate(detail, detailW))
		b.WriteString(line)
		b.WriteString("\n")
		if info.Path != "" && info.Path != "—" && contentWidth > 90 {
			b.WriteString("  " + m.theme.muted.Render(truncate(info.Path, contentWidth-2)))
			b.WriteString("\n")
		}
		if i < len(infos)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("/ filter · i install engines · r rescan · esc back"))
	return m.panel("Engines", b.String(), width, height, true)
}

func isCompressionJob(job runner.JobView) bool {
	for _, step := range job.Steps {
		switch step.Op {
		case "image.compress", "audio.compress", "video.compress", "file.archive":
			return true
		}
	}
	name := strings.ToLower(job.Name)
	return strings.Contains(name, "compress") || strings.Contains(name, "archive")
}
