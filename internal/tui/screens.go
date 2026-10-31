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
	listRows := max(contentHeight-7, 3)
	start := 0
	if m.opIndex >= listRows {
		start = m.opIndex - listRows + 1
	}
	end := min(start+listRows, len(filtered))
	for i := start; i < end; i++ {
		op := filtered[i]
		spec := op.Spec()
		marker := "  "
		titleStyle := lipgloss.NewStyle()
		if i == m.opIndex {
			marker = m.theme.accent.Render("▸ ")
			titleStyle = m.theme.accent
		}
		meta := string(spec.Domain) + " · " + acceptsSummary(spec)
		line := marker + titleStyle.Render(padRight(spec.Title, 26)) +
			m.theme.muted.Render(truncate(meta, max(contentWidth-30, 8)))
		if i == m.opIndex {
			line = m.selectedRow(line, contentWidth)
		}
		lines = append(lines, line)
	}
	if end < len(filtered) {
		lines = append(lines, m.theme.muted.Render(fmt.Sprintf("  … %d more (ctrl+j/k)", len(filtered)-end)))
	}

	selected := filtered[clamp(m.opIndex, 0, len(filtered)-1)].Spec()
	lines = append(lines, "")
	if selected.Summary != "" {
		lines = append(lines, truncate(selected.Summary, contentWidth))
	}
	if params := paramsSummary(selected); params != "" {
		lines = append(lines, m.theme.muted.Render("Parameters: "+params))
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
		left := m.panel("Jobs", m.jobListContent(leftW-4), leftW, height, true)
		right := m.panel("Job detail", m.jobDetailContent(rightW-4), rightW, height, false)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	}

	topH := min(max(len(m.jobs)+2, 5), max(height*45/100, 8))
	if topH > height-8 {
		topH = height - 8
	}
	bottomH := height - topH - 1
	return lipgloss.JoinVertical(lipgloss.Left,
		m.panel("Jobs", m.jobListContent(width-4), width, topH, true),
		m.panel("Job detail", m.jobDetailContent(width-4), width, bottomH, false))
}

func (m App) jobListContent(width int) string {
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
	const visible = 10
	start := 0
	if selected >= visible {
		start = selected - visible + 1
	}
	var lines []string
	if m.jobFiltering || m.jobQuery != "" {
		suffix := "▏"
		if !m.jobFiltering {
			suffix = ""
		}
		lines = append(lines, m.theme.accent.Render("Filter  ")+m.jobQuery+suffix)
	}
	for i := start; i < start+visible && i < len(jobs); i++ {
		lines = append(lines, m.jobLine(jobs[i], width, jobs[i].ID == m.selectedJob))
	}
	if len(jobs) > visible {
		lines = append(lines, m.theme.muted.Render(fmt.Sprintf("  … %d jobs total", len(jobs))))
	}
	return strings.Join(lines, "\n")
}

func (m App) jobDetailContent(width int) string {
	job, ok := m.selectedJobView()
	if !ok {
		return m.theme.muted.Render("Select a job to see its items and actions.")
	}
	glyph, style := m.jobGlyph(job.Status)
	lines := []string{
		style.Render(glyph+" "+string(job.Status)) + "  " + truncate(job.Name, max(width-10, 8)),
	}
	metrics := m.jobMetricsLine(job)
	if metrics != "" {
		lines = append(lines, xansi.Truncate(metrics, width, "…"))
	}
	if meta := detectorSummary(job); meta != nil {
		ai := metaFloat(meta, "ai_percent")
		human := metaFloat(meta, "human_percent")
		top := metaString(meta, "top_model")
		confidence := metaString(meta, "confidence")
		line := fmt.Sprintf("AI %.0f%% · human %.0f%%", ai, human)
		if top != "" {
			line += " · " + top
		}
		if confidence != "" {
			line += " · " + confidence + " confidence"
		}
		lines = append(lines, m.theme.danger.Render(xansi.Truncate(line, width, "…")))
	}
	lines = append(lines, "")
	for _, step := range job.Steps {
		mark := m.theme.muted.Render("·")
		if step.Status == core.StatusRunning {
			mark = m.theme.accent.Render("◐")
		} else if step.Status == core.StatusFailed {
			mark = m.theme.danger.Render("✗")
		} else if step.Status == core.StatusDone {
			mark = m.theme.success.Render("●")
		}
		lines = append(lines, fmt.Sprintf("  %s step %d/%d  %s", mark, step.Index+1, len(job.Steps), step.Title))
	}
	lines = append(lines, "")
	items := job.Items
	itemLimit := max(4, 12-len(job.Steps))
	if len(items) > itemLimit {
		items = items[:itemLimit]
	}
	for _, item := range items {
		lines = append(lines, m.itemLine(item, width))
	}
	if len(job.Items) > len(items) {
		lines = append(lines, m.theme.muted.Render(fmt.Sprintf("  … %d more items", len(job.Items)-len(items))))
	}
	if len(job.Aggregate) > 0 {
		lines = append(lines, "", m.theme.accent.Render("Aggregate output"))
		for _, artifact := range job.Aggregate {
			lines = append(lines, "  "+m.theme.success.Render("✓ ")+truncate(artifact.Name(), max(width-4, 8)))
			lines = append(lines, m.theme.muted.Render("    "+truncate(artifact.Path, max(width-6, 8))))
		}
	}
	if job.Err != nil && job.Status != core.StatusCanceled {
		line := job.Err.Error()
		if hint := core.HintOf(job.Err); hint != "" {
			line = hint
		}
		lines = append(lines, "", m.theme.danger.Render(truncate("! "+line, width)))
	}
	hint := "l log · R retry · r rerun · o reveal"
	if detectorSummary(job) != nil {
		hint = "enter AI report · " + hint
	} else {
		hint = "enter log · " + hint
	}
	if job.Historical {
		hint = "history · " + hint
	}
	lines = append(lines, "", m.theme.muted.Render(hint))
	return strings.Join(lines, "\n")
}

func (m App) jobLine(job runner.JobView, width int, selected bool) string {
	marker := "  "
	if selected {
		marker = m.theme.accent.Render("▸ ")
	}
	glyph, glyphStyle := m.jobGlyph(job.Status)

	counts := job.Counts()
	percent := job.Percent()
	var stats string
	switch job.Status {
	case core.StatusQueued:
		stats = fmt.Sprintf("  0%% · %d queued", counts.Total)
	case core.StatusRunning, core.StatusCancelling:
		stats = fmt.Sprintf("%3d%%", percent)
		if counts.Failed > 0 {
			stats += fmt.Sprintf(" · %d failed", counts.Failed)
		}
		if counts.Canceled > 0 {
			stats += fmt.Sprintf(" · %d cancelled", counts.Canceled)
		}
		if counts.Active > 0 {
			stats += fmt.Sprintf(" · %d active", counts.Active)
		}
	default:
		stats = fmt.Sprintf("%3d%% · %d/%d", percent, job.Settled(), counts.Total)
		if counts.Failed > 0 {
			stats += fmt.Sprintf(" · %d failed", counts.Failed)
		}
		if counts.Canceled > 0 {
			stats += fmt.Sprintf(" · %d cancelled", counts.Canceled)
		}
	}
	if d := job.Duration(); d > 0 {
		stats += " · " + humanDuration(d)
	}
	if job.Historical {
		stats += " · history"
	}

	statsWidth := lipgloss.Width(stats)
	barWidth := clamp((width-6-statsWidth)/2, 10, 64)
	nameWidth := max(width-6-barWidth-statsWidth, 8)
	name := padRight(truncate(job.Name, nameWidth), nameWidth)
	fill := m.theme.accent
	switch job.Status {
	case core.StatusDone:
		fill = m.theme.success
	case core.StatusFailed:
		fill = m.theme.danger
	case core.StatusCanceled:
		fill = m.theme.muted
	}
	bar := m.progressBarStyle(percent, 100, barWidth, fill)
	line := marker + glyphStyle.Render(glyph) + " " + name + " " + bar + " " + m.theme.muted.Render(stats)
	if selected {
		line = m.selectedRow(line, width)
	}
	return line
}

func (m App) jobMetricsLine(job runner.JobView) string {
	var parts []string
	if job.Metrics.BytesIn > 0 && job.Metrics.BytesOut > 0 {
		parts = append(parts, m.theme.muted.Render("size ")+
			m.theme.accent.Render(humanBytes(job.Metrics.BytesIn))+
			m.theme.muted.Render(" → ")+
			m.theme.accent.Render(humanBytes(job.Metrics.BytesOut)))
		if isCompressionJob(job) {
			change := (1 - float64(job.Metrics.BytesOut)/float64(job.Metrics.BytesIn)) * 100
			if change >= 1 {
				parts = append(parts, m.theme.success.Render(fmt.Sprintf("space saved %.0f%%", change)))
			} else if change <= -1 {
				parts = append(parts, m.theme.warning.Render(fmt.Sprintf("space grew %.0f%%", -change)))
			}
		}
	} else if job.Metrics.BytesOut > 0 {
		parts = append(parts, m.theme.muted.Render("downloaded ")+m.theme.accent.Render(humanBytes(job.Metrics.BytesOut)))
	}
	if len(job.Samples) > 1 {
		parts = append(parts, m.theme.muted.Render("progress ")+m.sparklineStyled(job.Samples, 16))
	}
	if d := job.Duration(); d > 0 {
		parts = append(parts, m.theme.muted.Render(humanDuration(d)))
	}
	return strings.Join(parts, m.theme.muted.Render("  ·  "))
}

func (m App) itemLine(item runner.ItemView, width int) string {
	glyph, style := m.itemGlyph(item.Status)
	nameWidth := clamp(width-20, 12, 28)
	name := truncate(item.Input.Name(), nameWidth)
	detail := item.Stage
	switch item.Status {
	case core.ItemStatusDone:
		detail = "done"
	case core.ItemStatusSkipped:
		detail = "skipped"
	case core.ItemStatusCanceled:
		detail = "cancelled"
	case core.ItemStatusPending:
		detail = "pending"
	case core.ItemStatusFailed:
		if item.Err != nil {
			detail = item.Err.Error()
		}
	}
	if detail == "" {
		detail = string(item.Status)
	}
	progress := ""
	if item.Total > 0 {
		progress = fmt.Sprintf(" %d%%", min(item.Done*100/item.Total, 100))
	}
	detailWidth := max(width-2-1-1-nameWidth-1-len(progress), 8)
	line := "  " + style.Render(glyph) + " " + padRight(name, nameWidth) + " " +
		m.theme.muted.Render(padRight(truncate(detail, detailWidth), detailWidth)) + progress
	return xansi.Truncate(line, width, "…")
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

func (m App) progressBar(done, total, width int) string {
	if done >= total {
		return m.progressBarStyle(done, total, width, m.theme.success)
	}
	return m.progressBarStyle(done, total, width, m.theme.accent)
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
	for _, info := range infos {
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
	}
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("/ filter · r rescan · esc back"))
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

func (m App) sparklineStyled(samples []int, width int) string {
	glyphs := []rune("▁▂▃▄▅▆▇█")
	if len(samples) == 0 || width <= 0 {
		return ""
	}
	if len(samples) > width {
		samples = samples[len(samples)-width:]
	}
	var b strings.Builder
	for _, value := range samples {
		value = clamp(value, 0, 100)
		index := value * (len(glyphs) - 1) / 100
		style := m.theme.muted
		switch {
		case value >= 66:
			style = m.theme.success
		case value >= 33:
			style = m.theme.accent
		}
		b.WriteString(style.Render(string(glyphs[index])))
	}
	return b.String()
}

func sparkline(samples []int, width int) string {
	glyphs := []rune("▁▂▃▄▅▆▇█")
	if len(samples) == 0 || width <= 0 {
		return ""
	}
	if len(samples) > width {
		samples = samples[len(samples)-width:]
	}
	var b strings.Builder
	for _, value := range samples {
		value = clamp(value, 0, 100)
		index := value * (len(glyphs) - 1) / 100
		b.WriteRune(glyphs[index])
	}
	return b.String()
}
