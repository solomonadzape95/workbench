package tui

import (
	"fmt"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"

	"github.com/solomonadzape95/workbench/internal/core"
	aidetect "github.com/solomonadzape95/workbench/internal/detect"
	"github.com/solomonadzape95/workbench/internal/runner"
)

type detectorState struct {
	jobID    string
	report   aidetect.Report
	returnTo screen
	scroll   int
}

type detectorReportState struct {
	report aidetect.Report
	scroll int
}

// scoreGlyphs is the compact five-row display face used for the AI/human
// headline. It belongs to the same solid-block family as the ANSI Shadow
// wordmark, so the score reads as part of the Workbench identity rather than a
// generic chart label.
var scoreGlyphs = map[rune][]string{
	'0': {"█████", "█   █", "█   █", "█   █", "█████"},
	'1': {"  █  ", " ██  ", "  █  ", "  █  ", "█████"},
	'2': {"█████", "    █", "█████", "█    ", "█████"},
	'3': {"█████", "    █", "█████", "    █", "█████"},
	'4': {"█   █", "█   █", "█████", "    █", "    █"},
	'5': {"█████", "█    ", "█████", "    █", "█████"},
	'6': {"█████", "█    ", "█████", "█   █", "█████"},
	'7': {"█████", "    █", "   █ ", "  █  ", "  █  "},
	'8': {"█████", "█   █", "█████", "█   █", "█████"},
	'9': {"█████", "█   █", "█████", "    █", "█████"},
}

// bigScore renders a percentage in the five-row display face. The percent sign
// stays normal-sized at the top-right so it cannot be mistaken for a digit.
func bigScore(value float64, style lipgloss.Style) []string {
	text := fmt.Sprintf("%.0f", math.Round(value))
	lines := make([]string, 5)
	for _, r := range text {
		glyph, ok := scoreGlyphs[r]
		if !ok {
			continue
		}
		for i := 0; i < 5; i++ {
			if lines[i] != "" {
				lines[i] += " "
			}
			lines[i] += style.Render(glyph[i])
		}
	}
	if lines[0] != "" {
		lines[0] += " " + style.Render("%")
	}
	return lines
}

// scoreBlock is a label above a big percentage, padded to a fixed width so two
// blocks can sit side by side.
func scoreBlock(label string, value float64, style lipgloss.Style) ([]string, int) {
	num := bigScore(value, style)
	width := 0
	for _, line := range num {
		width = max(width, lipgloss.Width(line))
	}
	lines := make([]string, 0, len(num)+1)
	lines = append(lines, padStyled(
		lipgloss.PlaceHorizontal(width, lipgloss.Center, style.Bold(true).Render(label)), width))
	for _, line := range num {
		lines = append(lines, padStyled(line, width))
	}
	return lines, width
}

// detectorWide decides between the roomy dashboard and the stacked layout.
// The wide form wants enough columns for the gauge and verdict to share a row.
func detectorWide(width, height int) bool {
	return width >= 96 && height >= 30
}

func (m App) detectorMaxScroll() int {
	width := max(m.width-4, 20)
	inner := max(width-4, 20)
	height := max(m.height-4, 1)
	body := len(m.detectorBody(inner, !detectorWide(inner, height)))
	maxBody := max(height-4, 4)
	return max(body-maxBody, 0)
}

func (m App) detectorView(height int) string {
	if m.detector == nil {
		return m.theme.muted.Render("No detector report loaded.")
	}
	width := max(m.width-4, 20)
	inner := max(width-4, 20)
	compact := !detectorWide(inner, height)
	contentHeight := max(height-2, 6)
	maxBody := max(contentHeight-2, 4)

	body := m.detectorBody(inner, compact)
	start := clamp(m.detector.scroll, 0, max(len(body)-maxBody, 0))
	end := min(start+maxBody, len(body))
	visible := append([]string(nil), body[start:end]...)
	for len(visible) < maxBody {
		visible = append(visible, "")
	}
	visible = append(visible, "")
	visible = append(visible, m.detectorActions(inner))

	content := strings.Join(visible, "\n")
	title := "AI detection"
	if m.detector.report.Source.Name != "" {
		title += " · " + truncate(m.detector.report.Source.Name, max(width-20, 10))
	}
	return m.panel(title, content, width, height, true)
}

func (m App) detectorBody(width int, compact bool) []string {
	r := m.detector.report
	var lines []string
	lines = append(lines, m.detectorHero(width, compact)...)
	lines = append(lines, "")

	if compact {
		lines = append(lines, m.sectionBlock("WHY THIS SCORE", width)...)
		lines = append(lines, m.signalGrid(r.Signals, width, 5)...)
		lines = append(lines, "")
		lines = append(lines, m.sectionBlock("FLAGGED SENTENCES", width)...)
		lines = append(lines, m.flaggedLines(r.Flagged, width, 2)...)
		return lines
	}

	lines = append(lines, m.sectionBlock("LIKELY SOURCE", width)...)
	lines = append(lines, m.detectorAttribution(width)...)
	lines = append(lines, "")

	lines = append(lines, m.sectionBlock("WHY THIS SCORE", width)...)
	lines = append(lines, m.signalGrid(r.Signals, width, 6)...)
	if len(r.Signals) > 6 {
		lines = append(lines, m.theme.muted.Render(fmt.Sprintf("  +%d more signals in the full report", len(r.Signals)-6)))
	}
	lines = append(lines, "")

	lines = append(lines, m.sectionBlock("FLAGGED SENTENCES", width)...)
	lines = append(lines, m.flaggedLines(r.Flagged, width, 3)...)
	lines = append(lines, "")

	if width >= 96 {
		col := (width - 4) / 2
		left := m.sectionBlock("SENTENCE SCORE DISTRIBUTION", col)
		left = append(left, m.sentenceHistogram(r.Stats.SentenceBuckets, col)...)
		right := m.sectionBlock("PARAGRAPHS", col)
		right = append(right, m.paragraphList(r.Paragraphs, col, 3)...)
		lines = append(lines, joinColumns(left, right, col, 4)...)
	} else {
		lines = append(lines, m.sectionBlock("SENTENCE SCORE DISTRIBUTION", width)...)
		lines = append(lines, m.sentenceHistogram(r.Stats.SentenceBuckets, width)...)
		lines = append(lines, "")
		lines = append(lines, m.sectionBlock("PARAGRAPHS", width)...)
		lines = append(lines, m.paragraphList(r.Paragraphs, width, 3)...)
	}
	return lines
}

// detectorHeadline is the compact score block shared by the dashboard and the
// full report: the verdict, the split gauge, and the confidence line.
func (m App) detectorHeadline(width int) []string {
	s := m.detector.report.Summary
	ai := math.Round(s.AI)
	strong := m.scoreStyle(s.AI).Bold(true)
	headline := strong.Render("AI") + " " +
		strong.Render(fmt.Sprintf("%.0f%%", ai)) + "  " +
		m.theme.muted.Render("·") + "  " +
		m.theme.title.Render(truncate(s.Verdict, max(width-24, 10)))
	confidence := m.confidenceBadge(s.Confidence)
	reliability := m.reliabilityLine(s.Reliability)
	lines := []string{truncateStyled(headline, width)}
	lines = append(lines, m.scoreGauge(s.AI, width)...)
	if lipgloss.Width(confidence)+lipgloss.Width(reliability)+3 > width {
		return append(lines, truncateStyled(confidence, width), truncateStyled(reliability, width))
	}
	return append(lines, truncateStyled(confidence+m.theme.muted.Render(" · ")+reliability, width))
}

func (m App) detectorHero(width int, compact bool) []string {
	r := m.detector.report
	s := r.Summary
	ai := math.Round(s.AI)
	strong := m.scoreStyle(s.AI).Bold(true)
	score := strong.Render("AI") + " " + strong.Render(fmt.Sprintf("%.0f%%", ai))
	verdict := m.theme.title.Render(truncate(s.Verdict, max(width-24, 10)))
	confidence := m.confidenceBadge(s.Confidence)
	reliability := m.reliabilityLine(s.Reliability)
	format := strings.ToUpper(r.Source.Format)

	if compact {
		meta := fmt.Sprintf("%s · %d words · %d sentences · %s profile", format, s.Words, s.Sentences, s.Profile)
		if s.AttributionTop != "" {
			meta = fmt.Sprintf("%s · %d words · %d sentences · likely %s", format, s.Words, s.Sentences, s.AttributionTop)
		}
		lines := []string{truncateStyled(score+m.theme.muted.Render("  ·  ")+verdict, width)}
		lines = append(lines, m.scoreGauge(s.AI, width)...)
		if lipgloss.Width(confidence)+lipgloss.Width(reliability)+3 > width {
			lines = append(lines,
				truncateStyled(confidence, width),
				truncateStyled(reliability, width))
		} else {
			lines = append(lines, truncateStyled(confidence+m.theme.muted.Render(" · ")+reliability, width))
		}
		lines = append(lines, m.theme.muted.Render(truncate(meta, width)))
		return m.appendWarning(lines, width)
	}

	source := ""
	if s.AttributionTop != "" {
		source = " · likely " + s.AttributionTop
	}
	aiBlock, aiWidth := scoreBlock("AI", ai, m.scoreStyle(s.AI))
	humanBlock, humanWidth := scoreBlock("HUMAN", 100-ai, m.theme.success)
	const (
		columnGap = 4
		divider   = "  │  "
	)
	leftWidth := aiWidth + columnGap + humanWidth
	rightWidth := max(width-leftWidth-lipgloss.Width(divider), 12)
	right := []string{
		m.theme.title.Render(truncate(s.Verdict, max(rightWidth, 10))),
		confidence,
		reliability,
		m.theme.muted.Render(truncate(fmt.Sprintf(
			"%d words · %d sentences · %d paragraphs", s.Words, s.Sentences, s.Paragraphs), rightWidth)),
		m.theme.muted.Render(truncate(fmt.Sprintf(
			"%.0f%% of prose sentences flagged", s.SentenceAI), rightWidth)),
		m.theme.muted.Render(truncate(fmt.Sprintf(
			"%s · %s profile%s", format, s.Profile, source), rightWidth)),
	}

	lines := []string{""}
	if rightWidth < 24 {
		for i := 0; i < max(len(aiBlock), len(humanBlock)); i++ {
			left := ""
			if i < len(aiBlock) {
				left = padStyled(aiBlock[i], aiWidth)
			}
			if i < len(humanBlock) {
				left = strings.TrimRight(left+strings.Repeat(" ", columnGap)+humanBlock[i], " ")
			}
			lines = append(lines, left)
		}
		lines = append(lines, "")
		lines = append(lines, right...)
	} else {
		for i := 0; i < max(max(len(aiBlock), len(humanBlock)), len(right)); i++ {
			left := ""
			if i < len(aiBlock) {
				left = padStyled(aiBlock[i], aiWidth)
			} else {
				left = strings.Repeat(" ", aiWidth)
			}
			if i < len(humanBlock) {
				left = strings.TrimRight(left+strings.Repeat(" ", columnGap)+humanBlock[i], " ")
			}
			rightLine := ""
			if i < len(right) {
				rightLine = right[i]
			}
			if rightLine == "" {
				lines = append(lines, left)
				continue
			}
			lines = append(lines, strings.TrimRight(
				padStyled(left, leftWidth)+m.theme.border.Render(divider)+rightLine, " "))
		}
	}

	lines = append(lines, "")
	lines = append(lines, m.scoreGauge(s.AI, width)...)
	return m.appendWarning(lines, width)
}

// appendWarning surfaces the first caveat in the hero instead of burying it at
// the bottom of a scroll view. The full report still lists every warning.
func (m App) appendWarning(lines []string, width int) []string {
	warnings := m.detector.report.Warnings
	if len(warnings) == 0 {
		return lines
	}
	lines = append(lines, "")
	lines = append(lines, m.theme.warning.Render("! ")+
		m.theme.muted.Render(truncate(warnings[0], max(width-3, 10))))
	return lines
}

// scoreGauge is the one place the AI/human split is drawn. Wide layouts get a
// two-row bar with a centered caption; compact layouts keep one row so the
// signals and flagged sentences stay above the fold.
func (m App) scoreGauge(ai float64, width int) []string {
	ai = clampFloat(ai, 0, 100)
	legend := m.scoreStyle(ai).Render(fmt.Sprintf("%.0f%% AI", ai)) +
		m.theme.muted.Render(" · ") +
		m.theme.success.Render(fmt.Sprintf("%.0f%% HUMAN", 100-ai))
	style := m.scoreStyle(ai)
	if width < 96 {
		// If the legend and a readable bar cannot share a row, put the legend
		// underneath instead of letting the panel clip it.
		if lipgloss.Width(legend)+14 <= width {
			barWidth := width - lipgloss.Width(legend) - 2
			return []string{m.dotBar(ai, barWidth, style) + "  " + legend}
		}
		return []string{
			m.dotBar(ai, max(width-2, 4), style),
			legend,
		}
	}
	bar := m.dotBar(ai, width, style)
	return []string{
		bar,
		bar,
		lipgloss.PlaceHorizontal(width, lipgloss.Center, legend),
	}
}

func (m App) scoreStyle(score float64) lipgloss.Style {
	switch {
	case score >= 60:
		return m.theme.danger
	case score >= 30:
		return m.theme.warning
	default:
		return m.theme.success
	}
}

func (m App) section(title string, width int) string {
	title = strings.ToUpper(title)
	style := m.theme.accent.Bold(true)
	remaining := width - lipgloss.Width(title) - 3
	if remaining < 1 {
		return style.Render(truncate(title, max(width, 1)))
	}
	return m.theme.border.Render("─ ") + style.Render(title) + " " +
		m.theme.border.Render(strings.Repeat("─", remaining))
}

// sectionBlock keeps a blank row between a section label and its content so
// dense dashboards get room to breathe.
func (m App) sectionBlock(title string, width int) []string {
	return []string{m.section(title, width), ""}
}

func (m App) confidenceBadge(confidence string) string {
	label := strings.ToLower(strings.TrimSpace(confidence))
	if label == "" {
		label = "unknown"
	}
	style := m.theme.muted
	switch label {
	case "high":
		style = m.theme.success
	case "medium":
		style = m.theme.warning
	case "low":
		style = m.theme.danger
	}
	return style.Render("● " + label + " confidence")
}

func (m App) reliabilityLine(reliability float64) string {
	style := m.theme.success
	switch {
	case reliability < 45:
		style = m.theme.danger
	case reliability < 70:
		style = m.theme.warning
	}
	return m.theme.muted.Render("reliability ") +
		m.dotBar(reliability, 10, style) + " " +
		style.Render(fmt.Sprintf("%.0f/100", reliability))
}

func (m App) detectorAttribution(width int) []string {
	r := m.detector.report
	if len(r.Attribution) == 0 {
		return []string{m.theme.muted.Render("No attribution available.")}
	}
	top := r.Attribution[0]
	style := m.theme.accent
	if top.Family == "human" {
		style = m.theme.success
	}
	barWidth := clamp(width/6, 10, 20)
	meta := fmt.Sprintf("%.0f/100 · %s", top.Score, strings.ToLower(top.Confidence))
	if top.Family == "human" {
		meta = "human-leaning rhythm and vocabulary"
	}
	lines := []string{
		m.theme.title.Render(top.Label) + "  " +
			m.dotBar(top.Score, barWidth, style) + "  " +
			m.theme.muted.Render(meta),
	}
	if len(top.Evidence) > 0 {
		lines = append(lines, m.theme.muted.Render(truncate(
			"evidence: "+strings.Join(top.Evidence, ", "), max(width-2, 12))))
	}
	if top.Notes != "" && width >= 80 {
		lines = append(lines, m.theme.muted.Render(truncate(top.Notes, max(width-2, 12))))
	}
	if len(r.Attribution) > 1 {
		var others []string
		for _, entry := range r.Attribution[1:min(len(r.Attribution), 4)] {
			others = append(others, fmt.Sprintf("%s %.0f", shortFamily(entry.Label), entry.Score))
		}
		lines = append(lines, m.theme.muted.Render(truncate(
			"also: "+strings.Join(others, " · "), max(width-2, 12))))
	}
	return lines
}

// dotBar uses the same braille dot-matrix as job progress: bright dots for the
// value, a dim low-dot track for the remainder.
func (m App) dotBar(score float64, width int, style lipgloss.Style) string {
	return m.progressBarStyle(int(math.Round(score)), 100, width, style)
}

func (m App) signalGrid(signals []aidetect.Signal, width, limit int) []string {
	if len(signals) == 0 {
		return []string{m.theme.muted.Render("No signals recorded.")}
	}
	if limit <= 0 || limit > len(signals) {
		limit = len(signals)
	}
	signals = signals[:limit]
	if width < 96 {
		lines := make([]string, 0, len(signals)*2)
		for i, signal := range signals {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, m.signalCard(signal, width))
		}
		return lines
	}
	colWidth := (width - 3) / 2
	var lines []string
	for i := 0; i < len(signals); i += 2 {
		if i > 0 {
			lines = append(lines, "")
		}
		left := m.signalCard(signals[i], colWidth)
		line := padStyled(left, colWidth)
		if i+1 < len(signals) {
			line += "   " + m.signalCard(signals[i+1], width-colWidth-3)
		}
		lines = append(lines, line)
	}
	return lines
}

func (m App) signalCard(signal aidetect.Signal, width int) string {
	style := m.scoreStyle(signal.AIScore)
	display := truncate(signal.Display, 10)
	if width < 48 {
		lines := []string{
			m.theme.muted.Render(truncate(signal.Label, max(width-6, 8))) + "  " +
				style.Render(fmt.Sprintf("%.0f", signal.AIScore)),
			"  " + m.dotBar(signal.AIScore, max(width-2, 6), style),
		}
		if display != "" {
			lines = append(lines, "  "+m.theme.muted.Render(display))
		}
		return strings.Join(lines, "\n")
	}
	labelWidth := clamp(width/3, 14, 22)
	barWidth := clamp(width-labelWidth-lipgloss.Width(display)-7, 6, 40)
	label := padRight(truncate(signal.Label, labelWidth), labelWidth)
	return m.theme.muted.Render(label) + " " +
		m.dotBar(signal.AIScore, barWidth, style) + " " +
		style.Render(fmt.Sprintf("%3.0f", signal.AIScore)) + " " +
		m.theme.muted.Render(display)
}

func (m App) sentenceHistogram(buckets [10]int, width int) []string {
	maxCount := 0
	total := 0
	for _, count := range buckets {
		total += count
		if count > maxCount {
			maxCount = count
		}
	}
	if total == 0 {
		return []string{m.theme.muted.Render("No sentences to chart.")}
	}

	const height = 6
	barWide := clamp((width-3-(len(buckets)-1))/len(buckets), 1, 7)
	chartWidth := len(buckets)*barWide + len(buckets) - 1
	var lines []string
	for row := height - 1; row >= 0; row-- {
		var b strings.Builder
		b.WriteString(m.theme.border.Render(fmt.Sprintf("%d ┤", row+1)))
		for i, count := range buckets {
			filled := count*height >= (row+1)*maxCount
			if filled {
				b.WriteString(m.bucketStyle(i).Render(strings.Repeat("⣿", barWide)))
			} else {
				b.WriteString(strings.Repeat(" ", barWide))
			}
			if i < len(buckets)-1 {
				b.WriteByte(' ')
			}
		}
		lines = append(lines, b.String())
	}
	lines = append(lines, m.theme.border.Render("  └"+strings.Repeat("─", chartWidth)))

	labelRow := []rune(strings.Repeat(" ", chartWidth))
	labelStep := 1
	if barWide < 2 {
		labelStep = 2
	}
	for i := range buckets {
		if i%labelStep != 0 {
			continue
		}
		label := fmt.Sprintf("%d", i*10)
		pos := i * (barWide + 1)
		for j, r := range label {
			if pos+j < len(labelRow) {
				labelRow[pos+j] = r
			}
		}
	}
	lines = append(lines, "   "+m.theme.muted.Render(strings.TrimRight(string(labelRow), " ")))
	lines = append(lines, "   "+m.theme.muted.Render(truncate(
		fmt.Sprintf("%d sentences · each column is a 10-point band", total), max(width-3, 10))))
	return lines
}

func (m App) bucketStyle(index int) lipgloss.Style {
	switch {
	case index >= 7:
		return m.theme.danger
	case index >= 4:
		return m.theme.warning
	default:
		return m.theme.success
	}
}

func (m App) paragraphList(paras []aidetect.Paragraph, width, limit int) []string {
	if len(paras) == 0 {
		return []string{m.theme.muted.Render("No paragraph detail available.")}
	}
	sorted := append([]aidetect.Paragraph(nil), paras...)
	sortParagraphsByAI(sorted)
	if limit > 0 && len(sorted) > limit {
		sorted = sorted[:limit]
	}
	var lines []string
	for i, para := range sorted {
		if i > 0 {
			lines = append(lines, "")
		}
		style := m.scoreStyle(para.AI)
		meta := fmt.Sprintf("%d/%d flagged", para.Flagged, para.Sentences)
		head := fmt.Sprintf("#%d  ", para.Index+1) +
			m.dotBar(para.AI, 8, style) + " " +
			style.Render(fmt.Sprintf("%3.0f%%", para.AI)) + "  " +
			m.theme.muted.Render(meta)
		if previewWidth := width - lipgloss.Width(head) - 2; previewWidth >= 18 {
			lines = append(lines, "  "+head+"  "+m.theme.muted.Render(truncate(para.Preview, previewWidth)))
		} else {
			lines = append(lines, "  "+head)
			lines = append(lines, "    "+m.theme.muted.Render(truncate(para.Preview, max(width-4, 8))))
		}
	}
	return lines
}

func (m App) flaggedLines(sentences []aidetect.Sentence, width int, limit int) []string {
	if len(sentences) == 0 {
		return []string{m.theme.muted.Render("No individual sentence crossed the threshold.")}
	}
	if len(sentences) < limit {
		limit = len(sentences)
	}
	var lines []string
	for i := 0; i < limit; i++ {
		sentence := sentences[i]
		if i > 0 {
			lines = append(lines, "")
		}
		style := m.scoreStyle(sentence.AI)
		prefix := style.Render(fmt.Sprintf("%3.0f%%", sentence.AI)) +
			m.theme.muted.Render(fmt.Sprintf(" ¶%d  ", sentence.Paragraph+1))
		wrapped := wrapText(sentence.Text, max(width-10, 12))
		if len(wrapped) == 0 {
			continue
		}
		lines = append(lines, prefix+wrapped[0])
		for _, extra := range wrapped[1:] {
			lines = append(lines, "      "+extra)
		}
		if len(sentence.Reasons) > 0 {
			lines = append(lines, "      "+m.theme.muted.Render(
				truncate(strings.Join(sentence.Reasons, " · "), max(width-8, 10))))
		}
	}
	return lines
}

func (m App) detectorActions(width int) string {
	if width < 34 {
		return m.theme.accent.Render("▸ enter") + " " + m.theme.title.Render("report")
	}
	if width < 48 {
		return m.theme.accent.Render("▸ enter") + " " +
			m.theme.title.Render("report") + "  " +
			m.theme.accent.Render("h") + " " + m.theme.muted.Render("humanize")
	}
	primary := m.theme.accent.Render("▸ enter") + " " + m.theme.title.Render("Full report")
	secondary := m.theme.accent.Render("h") + " " + m.theme.muted.Render("Humanize this text")
	if lipgloss.Width(primary)+lipgloss.Width(secondary)+6 > width {
		return primary + "\n" + secondary
	}
	return primary + "    " + secondary
}

func (m App) detectorReportView(height int) string {
	if m.detectorReport == nil {
		return m.theme.muted.Render("No report loaded.")
	}
	width := max(m.width-4, 20)
	inner := max(width-4, 20)
	contentHeight := max(height-2, 6)
	maxBody := max(contentHeight-2, 4)

	body := m.detectorReportBody(inner)
	start := clamp(m.detectorReport.scroll, 0, max(len(body)-maxBody, 0))
	end := min(start+maxBody, len(body))
	visible := append([]string(nil), body[start:end]...)
	for len(visible) < maxBody {
		visible = append(visible, "")
	}
	visible = append(visible, "")
	visible = append(visible, m.detectorReportActions(inner))

	content := strings.Join(visible, "\n")
	title := "AI report"
	if m.detectorReport.report.Source.Name != "" && width >= 60 {
		title += " · " + truncate(m.detectorReport.report.Source.Name, max(width-28, 10))
	}
	if len(body) > 0 {
		title += fmt.Sprintf(" · %d–%d/%d", start+1, max(end, start+1), len(body))
	}
	return m.panel(title, content, width, height, true)
}

func (m App) detectorReportActions(width int) string {
	if width < 34 {
		return m.theme.accent.Render("▸ h") + " " + m.theme.title.Render("humanize")
	}
	if width < 48 {
		return m.theme.accent.Render("▸ h") + " " +
			m.theme.title.Render("humanize") + "  " +
			m.theme.accent.Render("esc") + " " + m.theme.muted.Render("back")
	}
	primary := m.theme.accent.Render("▸ h") + " " + m.theme.title.Render("Humanize this text")
	secondary := m.theme.accent.Render("esc") + " " + m.theme.muted.Render("Back to dashboard")
	if lipgloss.Width(primary)+lipgloss.Width(secondary)+6 > width {
		return primary + "\n" + secondary
	}
	return primary + "    " + secondary
}

func (m App) detectorReportBody(width int) []string {
	r := m.detectorReport.report
	s := r.Summary
	var lines []string

	lines = append(lines, m.sectionBlock("VERDICT", width)...)
	lines = append(lines, m.detectorHeadline(width)...)
	lines = append(lines, m.theme.muted.Render(truncate(fmt.Sprintf(
		"%s · %s · %d words · %d sentences · %d paragraphs · %s profile",
		r.Source.Name, strings.ToUpper(r.Source.Format), s.Words, s.Sentences, s.Paragraphs, s.Profile), width)))
	lines = append(lines, "")

	lines = append(lines, m.sectionBlock("LIKELY SOURCE", width)...)
	lines = append(lines, m.detectorAttributionAll(width)...)
	lines = append(lines, "")

	lines = append(lines, m.sectionBlock("SIGNALS", width)...)
	for _, signal := range r.Signals {
		style := m.scoreStyle(signal.AIScore)
		if width < 60 {
			lines = append(lines,
				m.theme.muted.Render(truncate(signal.Label, max(width-8, 8)))+"  "+
					style.Render(fmt.Sprintf("%.0f", signal.AIScore)))
			barWidth := min(14, max(width-18, 4))
			meta := truncate(fmt.Sprintf("%s · w=%.0f%%", signal.Display, signal.Weight*100),
				max(width-barWidth-4, 6))
			lines = append(lines,
				"  "+m.dotBar(signal.AIScore, barWidth, style)+"  "+
					m.theme.muted.Render(meta))
		} else {
			lines = append(lines,
				m.theme.muted.Render(padRight(truncate(signal.Label, 20), 20))+"  "+
					m.dotBar(signal.AIScore, 20, style)+"  "+
					style.Render(fmt.Sprintf("%3.0f", signal.AIScore))+"  "+
					m.theme.muted.Render(fmt.Sprintf("%-9s w=%2.0f%%", signal.Display, signal.Weight*100)))
		}
		for _, detail := range wrapText(signal.Detail, width-4) {
			lines = append(lines, m.theme.muted.Render("  "+detail))
		}
	}
	lines = append(lines, "")

	lines = append(lines, m.sectionBlock("SENTENCE SCORE DISTRIBUTION", width)...)
	lines = append(lines, m.sentenceHistogram(r.Stats.SentenceBuckets, width)...)
	lines = append(lines, "")

	lines = append(lines, m.sectionBlock("PARAGRAPHS", width)...)
	lines = append(lines, m.paragraphList(r.Paragraphs, width, 20)...)
	lines = append(lines, "")

	lines = append(lines, m.sectionBlock("FLAGGED SENTENCES", width)...)
	lines = append(lines, m.flaggedLines(r.Flagged, width, 25)...)
	lines = append(lines, "")

	if len(r.Warnings) > 0 {
		lines = append(lines, m.sectionBlock("READ THIS", width)...)
		for _, warning := range r.Warnings {
			for _, line := range wrapText(warning, width-3) {
				lines = append(lines, m.theme.warning.Render("! ")+m.theme.muted.Render(line))
			}
		}
		lines = append(lines, "")
	}

	lines = append(lines, m.sectionBlock("HOW THIS WAS SCORED", width)...)
	for _, item := range r.Methodology {
		for _, line := range wrapText(item, width-3) {
			lines = append(lines, m.theme.muted.Render("· "+line))
		}
	}
	lines = append(lines, "")

	lines = append(lines, m.sectionBlock("LIMITATIONS", width)...)
	for _, item := range []string{
		"No detector can prove authorship. This is a diagnostic, not a gate.",
		"False positives are more likely on formulaic human writing, non-native English, and heavily edited text.",
		"Paraphrasing, translation, and humanizing tools can reduce or erase the signals.",
		"Short texts are much less reliable. Treat a score below 300 words as weak.",
		"Model attribution is a phrase/style fingerprint, not an exact identification.",
	} {
		for _, line := range wrapText(item, width-3) {
			lines = append(lines, m.theme.muted.Render("· "+line))
		}
	}
	lines = append(lines, "")

	lines = append(lines, m.sectionBlock("REFERENCES", width)...)
	for _, ref := range r.References {
		lines = append(lines, m.theme.muted.Render(fmt.Sprintf("%s (%s) · %s", ref.Authors, ref.Year, ref.Title)))
		lines = append(lines, m.theme.border.Render("  "+ref.URL))
	}
	return lines
}

func (m App) detectorAttributionAll(width int) []string {
	r := m.detectorReport.report
	if len(r.Attribution) == 0 {
		return []string{m.theme.muted.Render("No attribution available.")}
	}
	var lines []string
	for _, attribution := range r.Attribution {
		style := m.theme.accent
		if attribution.Family == "human" {
			style = m.theme.success
		}
		if width < 60 {
			barWidth := min(14, max(width-18, 6))
			lines = append(lines,
				style.Render(truncate(attribution.Label, max(width-2, 8))))
			lines = append(lines,
				"  "+m.dotBar(attribution.Score, barWidth, style)+"  "+
					m.theme.muted.Render(fmt.Sprintf("%.0f/100 · %s",
						attribution.Score, titleCase(attribution.Confidence))))
		} else {
			lines = append(lines,
				style.Render(padRight(truncate(attribution.Label, 28), 28))+"  "+
					m.dotBar(attribution.Score, 20, style)+"  "+
					m.theme.muted.Render(fmt.Sprintf("%3.0f/100  %s", attribution.Score, titleCase(attribution.Confidence))))
		}
		if len(attribution.Evidence) > 0 {
			for _, line := range wrapText("evidence: "+strings.Join(attribution.Evidence, ", "), width-4) {
				lines = append(lines, m.theme.muted.Render("  "+line))
			}
		}
		if attribution.Notes != "" && width > 80 {
			for _, line := range wrapText(attribution.Notes, width-4) {
				lines = append(lines, m.theme.muted.Render("  "+line))
			}
		}
	}
	return lines
}

func (m App) detectorReportMaxScroll() int {
	width := max(m.width-4, 20)
	inner := max(width-4, 20)
	height := max(m.height-4, 1)
	body := len(m.detectorReportBody(inner))
	maxBody := max(height-4, 4)
	return max(body-maxBody, 0)
}

func sortParagraphsByAI(paras []aidetect.Paragraph) {
	for i := 1; i < len(paras); i++ {
		for j := i; j > 0 && paras[j].AI > paras[j-1].AI; j-- {
			paras[j], paras[j-1] = paras[j-1], paras[j]
		}
	}
}

func wrapText(text string, width int) []string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" || width < 4 {
		return nil
	}
	var lines []string
	var current string
	for _, word := range strings.Fields(text) {
		if len([]rune(word)) > width {
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			for len([]rune(word)) > width {
				lines = append(lines, string([]rune(word)[:width]))
				word = string([]rune(word)[width:])
			}
			if word != "" {
				current = word
			}
			continue
		}
		if current == "" {
			current = word
			continue
		}
		if len([]rune(current))+1+len([]rune(word)) <= width {
			current += " " + word
		} else {
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func joinColumns(left, right []string, colWidth, gutter int) []string {
	lines := make([]string, 0, max(len(left), len(right)))
	for i := 0; i < max(len(left), len(right)); i++ {
		line := ""
		if i < len(left) {
			line = padStyled(left[i], colWidth)
		} else {
			line = strings.Repeat(" ", colWidth)
		}
		if i < len(right) {
			line += strings.Repeat(" ", gutter) + right[i]
		}
		lines = append(lines, strings.TrimRight(line, " "))
	}
	return lines
}

func shortFamily(label string) string {
	if i := strings.Index(label, " /"); i > 0 {
		return label[:i]
	}
	return label
}

func titleCase(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}

func truncateStyled(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	return padStyled(s, width)
}

func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func (m App) handleDetectorKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.detector == nil {
		m.screen = screenJobs
		return m, nil
	}
	key := msg.String()
	switch key {
	case "esc":
		m.screen = m.detector.returnTo
		m.detector = nil
	case "j", "down":
		m.detector.scroll++
	case "k", "up":
		if m.detector.scroll > 0 {
			m.detector.scroll--
		}
	case "pgdown", "ctrl+d":
		m.detector.scroll += 8
	case "pgup", "ctrl+u":
		m.detector.scroll -= 8
	case "g", "home":
		m.detector.scroll = 0
	case "G", "end":
		m.detector.scroll = m.detectorMaxScroll()
	case "h":
		return m.humanizeFromDetector()
	case "enter", "o":
		m.detectorReport = &detectorReportState{report: m.detector.report}
		m.screen = screenDetectorReport
	case "c", "y":
		text := aidetect.SummaryText(m.detector.report)
		if err := clipboard.WriteAll(text); err != nil {
			m.toast = "clipboard: " + err.Error()
			m.toastLevel = core.LevelError
		} else {
			m.toast = "AI detection summary copied"
			m.toastLevel = core.LevelInfo
		}
	case "r":
		return m.retryDetector()
	}
	return m, nil
}

func (m App) handleDetectorReportKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.detectorReport == nil {
		m.screen = screenDetector
		return m, nil
	}
	key := msg.String()
	switch key {
	case "esc":
		m.detectorReport = nil
		m.screen = screenDetector
	case "j", "down":
		m.detectorReport.scroll++
	case "k", "up":
		if m.detectorReport.scroll > 0 {
			m.detectorReport.scroll--
		}
	case "pgdown", "ctrl+d":
		m.detectorReport.scroll += 10
	case "pgup", "ctrl+u":
		m.detectorReport.scroll -= 10
	case "g", "home":
		m.detectorReport.scroll = 0
	case "G", "end":
		m.detectorReport.scroll = m.detectorReportMaxScroll()
	case "h":
		return m.humanizeFromDetector()
	case "c", "y":
		text := aidetect.SummaryText(m.detectorReport.report)
		if err := clipboard.WriteAll(text); err != nil {
			m.toast = "clipboard: " + err.Error()
			m.toastLevel = core.LevelError
		} else {
			m.toast = "AI detection summary copied"
			m.toastLevel = core.LevelInfo
		}
	}
	return m, nil
}

func (m App) humanizeFromDetector() (tea.Model, tea.Cmd) {
	if m.detector == nil || m.detector.report.Source.Path == "" {
		m.toast = "detector report has no source path"
		m.toastLevel = core.LevelWarn
		return m, nil
	}
	humanizeOp, ok := m.opts.Operations.Get("doc.humanize")
	if !ok {
		m.toast = "humanize operation is not available"
		m.toastLevel = core.LevelWarn
		return m, nil
	}
	source := m.detector.report.Source.Path
	sourceFormat := strings.ToLower(m.detector.report.Source.Format)
	var c *compose
	if sourceFormat == "pdf" {
		convertOp, ok := m.opts.Operations.Get("doc.convert")
		if !ok {
			m.toast = "PDF humanization needs the document converter"
			m.toastLevel = core.LevelWarn
			return m, nil
		}
		pipeline := core.Pipeline{
			Name:    "Humanize PDF",
			Version: core.PipelineVersion,
			Steps: []core.Step{
				{Op: convertOp.Spec().ID, Params: core.Values{"format": "docx", "engine": "auto"}},
				{Op: humanizeOp.Spec().ID, Params: core.Values{"style": "professional", "model": "auto"}},
			},
		}
		c = m.newComposeFromPipeline(pipeline)
	} else {
		c = m.newCompose(humanizeOp)
	}
	c.input = source
	c.focus = focusInput
	m.compose = c
	m.screen = screenCompose
	m.detector = nil
	m.detectorReport = nil
	m.toast = "humanize: " + source
	m.toastLevel = core.LevelInfo
	return m, c.afterTextChange(m.opts.Config)
}

func (m App) retryDetector() (tea.Model, tea.Cmd) {
	if m.detector == nil {
		return m, nil
	}
	op, ok := m.opts.Operations.Get("doc.detect_ai")
	if !ok {
		m.toast = "detector operation is not available"
		m.toastLevel = core.LevelWarn
		return m, nil
	}
	c := m.newCompose(op)
	c.input = m.detector.report.Source.Path
	c.focus = focusInput
	m.compose = c
	m.screen = screenCompose
	m.detector = nil
	m.detectorReport = nil
	return m, c.afterTextChange(m.opts.Config)
}

func detectorJSONPath(job runner.JobView) string {
	for _, item := range job.Items {
		for _, output := range item.Outputs {
			if output.Format == "json" && output.Meta["kind"] == "ai-detection-json" {
				return output.Path
			}
		}
	}
	return ""
}

func detectorSummary(job runner.JobView) map[string]any {
	for _, item := range job.Items {
		for _, output := range item.Outputs {
			if output.Meta["kind"] == "ai-detection" {
				return output.Meta
			}
		}
	}
	return nil
}

func (m App) openDetectorForJob(job runner.JobView, returnTo screen) (App, bool) {
	path := detectorJSONPath(job)
	if path == "" {
		return m, false
	}
	report, err := aidetect.LoadReport(path)
	if err != nil {
		m.toast = "AI report: " + err.Error()
		m.toastLevel = core.LevelError
		return m, false
	}
	m.detector = &detectorState{
		jobID:    job.ID,
		report:   report,
		returnTo: returnTo,
	}
	m.detectorReport = nil
	m.screen = screenDetector
	return m, true
}

func metaFloat(meta map[string]any, key string) float64 {
	switch value := meta[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}

func metaString(meta map[string]any, key string) string {
	if value, ok := meta[key].(string); ok {
		return value
	}
	return ""
}
