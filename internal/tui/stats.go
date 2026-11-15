package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/solomonadzape95/workbench/internal/core"
)

type statsRow struct {
	name       string
	jobs       int
	files      int
	in         int64
	out        int64
	saved      int64
	compressed bool
}

type statsDay struct {
	jobs   int
	failed int
	saved  int64
}

type kpiCell struct {
	label string
	value string
	style lipgloss.Style
}

func (m App) statsView(height int) string {
	width := min(m.width-4, 140)
	inner := max(width-4, 18)
	if len(m.jobs) == 0 {
		return m.panel("Statistics", m.theme.muted.Render("No jobs yet. Run something and come back."), width, height, true)
	}

	now := time.Now()
	rows := map[string]*statsRow{}
	byDay := map[string]*statsDay{}
	var totalIn, totalOut, totalSaved int64
	var totalFiles, settled, done, failedJobs int
	var totalDuration time.Duration
	var timedJobs int

	for _, job := range m.jobs {
		in, out := job.Metrics.BytesIn, job.Metrics.BytesOut
		saved := int64(0)
		compressed := isCompressionJob(job)
		if compressed && in > 0 && out > 0 {
			saved = in - out
		}
		totalIn += in
		totalOut += out
		totalSaved += saved
		totalFiles += len(job.Items)

		switch job.Status {
		case core.StatusDone:
			done++
			settled++
		case core.StatusPartial, core.StatusFailed, core.StatusCanceled:
			settled++
			if job.Status == core.StatusFailed {
				failedJobs++
			}
		}
		if !job.FinishedAt.IsZero() {
			totalDuration += job.Duration()
			timedJobs++
		}

		name := job.Name
		if name == "" {
			name = "unnamed"
		}
		row, ok := rows[name]
		if !ok {
			row = &statsRow{name: name}
			rows[name] = row
		}
		row.jobs++
		row.files += len(job.Items)
		row.in += in
		row.out += out
		row.saved += saved
		row.compressed = row.compressed || compressed

		when := job.FinishedAt
		if when.IsZero() {
			when = job.QueuedAt
		}
		if !when.IsZero() && now.Sub(when) < 7*24*time.Hour {
			key := when.Format("2006-01-02")
			day, ok := byDay[key]
			if !ok {
				day = &statsDay{}
				byDay[key] = day
			}
			day.jobs++
			if job.Status == core.StatusFailed {
				day.failed++
			}
			day.saved += saved
		}
	}

	success := "—"
	if settled > 0 {
		success = fmt.Sprintf("%d%%", done*100/settled)
	}
	avg := "—"
	if timedJobs > 0 {
		avg = humanDuration(totalDuration / time.Duration(timedJobs))
	}
	savedValue := "—"
	if totalSaved != 0 {
		savedValue = signedBytes(totalSaved)
	}

	cells := []kpiCell{
		{"jobs", fmt.Sprint(len(m.jobs)), m.theme.title},
		{"files", fmt.Sprint(totalFiles), m.theme.title},
		{"data in", humanBytes(totalIn), m.theme.accent},
		{"data out", humanBytes(totalOut), m.theme.accent},
		{"space saved", savedValue, m.theme.success},
		{"success", success, m.theme.success},
		{"avg time", avg, m.theme.accent},
	}
	if totalSaved < 0 {
		cells[4].style = m.theme.warning
	}

	kpiCols := 3
	if inner >= 68 {
		kpiCols = 4
	}
	lines := m.kpiGrid(cells, inner, kpiCols)
	lines = append(lines, "")

	ordered := make([]*statsRow, 0, len(rows))
	for _, row := range rows {
		ordered = append(ordered, row)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].jobs != ordered[j].jobs {
			return ordered[i].jobs > ordered[j].jobs
		}
		if ordered[i].saved != ordered[j].saved {
			return ordered[i].saved > ordered[j].saved
		}
		return ordered[i].name < ordered[j].name
	})

	if inner >= 100 {
		leftW := max(inner*58/100, 54)
		rightW := inner - leftW - 3
		left := []string{m.section("BY JOB", leftW), ""}
		left = append(left, m.statsTable(ordered, leftW, 7)...)
		right := []string{m.section("LAST 7 DAYS", rightW), ""}
		right = append(right, m.statsActivity(byDay, rightW, now)...)
		right = append(right, "", m.section("STATUS", rightW), "")
		right = append(right, m.statsStatus(rightW)...)
		lines = append(lines, joinColumns(left, right, leftW, 3)...)
	} else {
		lines = append(lines, m.section("BY JOB", inner), "")
		lines = append(lines, m.statsTable(ordered, inner, 4)...)
		lines = append(lines, "", m.section("LAST 7 DAYS", inner), "")
		lines = append(lines, m.statsActivity(byDay, inner, now)...)
		if inner >= 90 {
			lines = append(lines, "", m.section("STATUS", inner), "")
			lines = append(lines, m.statsStatus(inner)...)
		}
	}

	contentHeight := max(height-2, 4)
	if len(lines) > contentHeight {
		available := max(contentHeight-1, 1)
		start := clamp(m.statsScroll, 0, max(len(lines)-available, 0))
		end := min(start+available, len(lines))
		visible := append([]string(nil), lines[start:end]...)
		visible = append(visible, m.theme.muted.Render(fmt.Sprintf(
			"  j/k scroll · %d–%d of %d", start+1, end, len(lines))))
		lines = visible
	}
	return m.panel("Statistics", strings.Join(lines, "\n"), width, height, true)
}

func (m App) kpiGrid(cells []kpiCell, width, cols int) []string {
	if cols < 1 {
		cols = 1
	}
	cellW := max((width-2)/cols, 8)
	var lines []string
	for start := 0; start < len(cells); start += cols {
		end := min(start+cols, len(cells))
		var labels, values strings.Builder
		for i := start; i < end; i++ {
			labels.WriteString(padStyled(m.theme.muted.Render(strings.ToUpper(cells[i].label)), cellW))
			values.WriteString(padStyled(cells[i].style.Render(cells[i].value), cellW))
		}
		lines = append(lines, "  "+strings.TrimRight(labels.String(), " "), "  "+strings.TrimRight(values.String(), " "))
	}
	return lines
}

func (m App) statsTable(rows []*statsRow, width, limit int) []string {
	showFiles := width >= 48
	showInOut := width >= 72
	runsW, savedW := 5, 11
	filesW, inW, outW := 0, 0, 0
	if showFiles {
		filesW = 6
	}
	if showInOut {
		inW, outW = 10, 10
	}
	gaps := 2 // name/runs and runs/saved
	if showFiles {
		gaps++
	}
	if showInOut {
		gaps += 2
	}
	nameW := width - 2 - runsW - savedW - filesW - inW - outW - gaps
	if nameW < 12 && showInOut {
		showInOut = false
		inW, outW = 0, 0
		gaps -= 2
		nameW = width - 2 - runsW - savedW - filesW - gaps
	}
	if nameW < 12 && showFiles {
		showFiles = false
		filesW = 0
		gaps--
		nameW = width - 2 - runsW - savedW - gaps
	}
	nameW = max(nameW, 10)

	header := "  " + padRight("JOB", nameW) + " " + numericCell("RUNS", runsW)
	if showFiles {
		header += " " + numericCell("FILES", filesW)
	}
	if showInOut {
		header += " " + numericCell("IN", inW) + " " + numericCell("OUT", outW)
	}
	header += " " + numericCell("SAVED", savedW)
	lines := []string{m.theme.muted.Render(header)}

	limit = min(len(rows), limit)
	for _, row := range rows[:limit] {
		line := "  " + m.theme.muted.Render(padRight(truncate(row.name, nameW), nameW)) +
			" " + m.theme.accent.Render(numericCell(fmt.Sprint(row.jobs), runsW))
		if showFiles {
			line += " " + m.theme.muted.Render(numericCell(fmt.Sprint(row.files), filesW))
		}
		if showInOut {
			line += " " + m.theme.muted.Render(numericCell(humanBytes(row.in), inW)) +
				" " + m.theme.muted.Render(numericCell(humanBytes(row.out), outW))
		}
		saved := "—"
		style := m.theme.muted
		if row.compressed && row.saved != 0 {
			saved = signedBytes(row.saved)
			style = m.theme.success
			if row.saved < 0 {
				style = m.theme.warning
			}
		}
		line += " " + style.Render(numericCell(saved, savedW))
		lines = append(lines, line)
	}
	if len(rows) > limit {
		lines = append(lines, "  "+m.theme.muted.Render(fmt.Sprintf("… %d more", len(rows)-limit)))
	}
	return lines
}

func (m App) statsActivity(byDay map[string]*statsDay, width int, now time.Time) []string {
	days := make([]statsDay, 7)
	labels := make([]string, 7)
	maxJobs := 0
	for i := 6; i >= 0; i-- {
		when := now.AddDate(0, 0, -i)
		key := when.Format("2006-01-02")
		idx := 6 - i
		labels[idx] = when.Format("Mon")
		if day, ok := byDay[key]; ok {
			days[idx] = *day
			if day.jobs > maxJobs {
				maxJobs = day.jobs
			}
		}
	}
	if maxJobs == 0 {
		return []string{"  " + m.theme.muted.Render("No jobs in the last 7 days.")}
	}
	barW := clamp(width-28, 8, 20)
	lines := make([]string, 0, len(days))
	for i, day := range days {
		filled := day.jobs * barW / maxJobs
		bar := m.theme.accent.Render(strings.Repeat("█", filled)) +
			m.theme.border.Render(strings.Repeat("░", barW-filled))
		line := "  " + m.theme.muted.Render(padRight(labels[i], 4)) + " " + bar +
			" " + m.theme.muted.Render(jobCount(day.jobs))
		if day.failed > 0 {
			line += " " + m.theme.danger.Render(fmt.Sprintf("· %d failed", day.failed))
		}
		lines = append(lines, line)
	}
	return lines
}

func (m App) statsStatus(width int) []string {
	counts := map[core.Status]int{}
	total := 0
	for _, job := range m.jobs {
		counts[job.Status]++
		total++
	}
	if total == 0 {
		return nil
	}
	order := []core.Status{
		core.StatusDone, core.StatusPartial, core.StatusFailed,
		core.StatusCanceled, core.StatusRunning, core.StatusQueued,
	}
	styles := map[core.Status]lipgloss.Style{
		core.StatusDone:     m.theme.success,
		core.StatusPartial:  m.theme.warning,
		core.StatusFailed:   m.theme.danger,
		core.StatusCanceled: m.theme.muted,
		core.StatusRunning:  m.theme.accent,
		core.StatusQueued:   m.theme.border,
	}
	barW := max(width-2, 8)
	used := 0
	nonZero := make([]core.Status, 0, len(order))
	for _, status := range order {
		if counts[status] > 0 {
			nonZero = append(nonZero, status)
		}
	}
	var bar strings.Builder
	for i, status := range nonZero {
		n := counts[status] * barW / total
		if n < 1 {
			n = 1
		}
		if i == len(nonZero)-1 {
			n = max(barW-used, 1)
		}
		used += n
		bar.WriteString(styles[status].Render(strings.Repeat("█", n)))
	}
	if pad := barW - used; pad > 0 {
		bar.WriteString(m.theme.border.Render(strings.Repeat("░", pad)))
	}

	var parts []string
	for _, status := range nonZero {
		parts = append(parts, styles[status].Render(fmt.Sprintf("%s %d", status, counts[status])))
	}
	return []string{"  " + bar.String(), "  " + strings.Join(parts, m.theme.muted.Render(" · "))}
}

func jobCount(n int) string {
	if n == 1 {
		return "1 job"
	}
	return fmt.Sprintf("%d jobs", n)
}

func signedBytes(value int64) string {
	if value < 0 {
		return "-" + humanBytes(-value)
	}
	return "+" + humanBytes(value)
}
