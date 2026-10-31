package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
)

type statsRow struct {
	name  string
	jobs  int
	in    int64
	out   int64
	saved int64
}

func (m App) statsView(height int) string {
	width := max(m.width-4, 20)
	inner := max(width-4, 18)
	var lines []string

	var totalIn, totalOut, totalSaved int64
	rows := map[string]*statsRow{}
	byDay := map[string]int64{}
	now := time.Now()
	for _, job := range m.jobs {
		in, out := job.Metrics.BytesIn, job.Metrics.BytesOut
		saved := int64(0)
		if in > 0 && out > 0 {
			saved = in - out
		}
		totalIn += in
		totalOut += out
		totalSaved += saved
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
		row.in += in
		row.out += out
		row.saved += saved
		when := job.FinishedAt
		if when.IsZero() {
			when = job.QueuedAt
		}
		if !when.IsZero() && now.Sub(when) < 14*24*time.Hour {
			byDay[when.Format("2006-01-02")] += saved
		}
	}

	if len(m.jobs) == 0 {
		lines = append(lines, m.theme.muted.Render("No jobs yet. Run something and come back."))
		return m.panel("Statistics", strings.Join(lines, "\n"), width, height, true)
	}

	lines = append(lines,
		m.theme.title.Render(fmt.Sprintf("%d jobs", len(m.jobs)))+
			m.theme.muted.Render(fmt.Sprintf("  ·  %s in → %s out", humanBytes(totalIn), humanBytes(totalOut))),
		m.theme.success.Render("net saved "+signedBytes(totalSaved)),
		"",
	)

	lines = append(lines, m.section("SAVED BY PIPELINE", inner))
	ordered := make([]*statsRow, 0, len(rows))
	for _, row := range rows {
		ordered = append(ordered, row)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].saved == ordered[j].saved {
			return ordered[i].jobs > ordered[j].jobs
		}
		return ordered[i].saved > ordered[j].saved
	})
	limit := min(len(ordered), 8)
	for i := 0; i < limit; i++ {
		row := ordered[i]
		style := m.theme.success
		if row.saved < 0 {
			style = m.theme.warning
		}
		label := padRight(truncate(row.name, 25), 25)
		lines = append(lines, m.theme.muted.Render(label)+"  "+
			m.theme.accent.Render(fmt.Sprintf("%2d jobs", row.jobs))+"  "+
			style.Render(signedBytes(row.saved)))
	}
	if len(ordered) > limit {
		lines = append(lines, m.theme.muted.Render(fmt.Sprintf("  … %d more pipelines", len(ordered)-limit)))
	}

	lines = append(lines, "", m.section("SAVED OVER 14 DAYS", inner))
	days := 14
	samples := make([]int, 0, days)
	var maxSaved int64
	dayKeys := make([]string, 0, days)
	for i := days - 1; i >= 0; i-- {
		key := now.AddDate(0, 0, -i).Format("2006-01-02")
		dayKeys = append(dayKeys, key)
		if value := byDay[key]; value > maxSaved {
			maxSaved = value
		}
	}
	for _, key := range dayKeys {
		value := byDay[key]
		level := 0
		if maxSaved > 0 && value > 0 {
			level = int(value * 100 / maxSaved)
		}
		samples = append(samples, level)
	}
	lines = append(lines, "  "+m.sparklineStyled(samples, days))
	lines = append(lines, m.theme.muted.Render("  14 days ago"+
		strings.Repeat(" ", max(inner-26, 1))+"now"))

	lines = append(lines, "", m.section("STATUS", inner))
	counts := map[core.Status]int{}
	for _, job := range m.jobs {
		counts[job.Status]++
	}
	var statusParts []string
	for _, status := range []core.Status{core.StatusDone, core.StatusPartial, core.StatusFailed, core.StatusCanceled} {
		if counts[status] > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%s %d", status, counts[status]))
		}
	}
	lines = append(lines, m.theme.muted.Render("  "+strings.Join(statusParts, " · ")))

	contentHeight := max(height-2, 4)
	if len(lines) > contentHeight {
		lines = lines[:contentHeight]
	}
	return m.panel("Statistics", strings.Join(lines, "\n"), width, height, true)
}

func signedBytes(value int64) string {
	if value < 0 {
		return "-" + humanBytes(-value)
	}
	return "+" + humanBytes(value)
}
