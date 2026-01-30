package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/zigai/zgod/internal/match"
)

const (
	panelBorderW      = 2
	panelBorderH      = 2
	panelPaddingX     = 1
	panelPaddingY     = 0
	resultsHeaderRows = 1
	minInputWidth     = 20
	previewPaneHeight = 4
)

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.showPreview {
		return m.renderPreviewPopup()
	}
	if m.showHelp {
		return m.renderHelp()
	}

	var sections []string

	// Combined input bar (or split header + input on narrow terminals)
	sections = append(sections, m.renderInputBar())

	// Results list
	sections = append(sections, m.renderResults())

	// Preview pane (if enabled)
	if m.cfg.Display.MultilinePreview == "preview_pane" {
		sections = append(sections, m.renderPreviewPane())
	}

	// Footer with keybindings
	if m.cfg.Display.ShowHints {
		sections = append(sections, m.renderFooter())
	}

	body := strings.Join(sections, "\n")
	return m.styles.Border.
		Width(m.width+panelPaddingX*2).
		Padding(panelPaddingY, panelPaddingX).
		Render(body)
}

func (m Model) renderIndicators() string {
	width := m.getWidth()

	inactive := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Background(lipgloss.Color("237")).
		Padding(0, 1)

	var indicators []string

	type modeIndicator struct {
		mode    match.Mode
		label   string
		bg      string
		enabled bool
	}
	modes := []modeIndicator{
		{match.ModeFuzzy, "fuzzy", "207", m.cfg.Display.EnableFuzzy},
		{match.ModeRegex, "regex", "208", m.cfg.Display.EnableRegex},
		{match.ModeGlob, "glob", "39", m.cfg.Display.EnableGlob},
	}
	for _, mi := range modes {
		if !mi.enabled {
			continue
		}
		if m.mode == mi.mode {
			indicators = append(indicators, lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color(mi.bg)).
				Bold(true).
				Padding(0, 1).
				Render(mi.label))
		} else {
			indicators = append(indicators, inactive.Render(mi.label))
		}
	}

	type toggleIndicator struct {
		label  string
		bg     string
		active bool
	}
	toggles := []toggleIndicator{
		{"cwd", "10", m.cwdMode},
		{"dedup", "11", m.dedupe},
		{"fails", "9", m.onlyFails},
	}
	for _, ti := range toggles {
		if ti.active {
			indicators = append(indicators, lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color(ti.bg)).
				Bold(true).
				Padding(0, 1).
				Render(ti.label))
		} else {
			indicators = append(indicators, inactive.Render(ti.label))
		}
	}

	return m.fitIndicators(indicators, width)
}

func (m Model) renderHeader() string {
	width := m.getWidth()
	indicatorStr := m.renderIndicators()

	fillWidth := max(width-lipgloss.Width(indicatorStr), 0)

	line := indicatorStr + strings.Repeat(" ", fillWidth)
	return m.styles.HeaderBar.Width(width).Render(line)
}

func (m Model) isMerged() bool {
	width := m.getWidth()
	prompt := m.cfg.Theme.Prompt
	promptWidth := lipgloss.Width(m.styles.Prompt.Render(prompt))
	indicatorStr := m.renderIndicators()
	indicatorWidth := lipgloss.Width(indicatorStr)
	gap := 2
	remaining := width - promptWidth - indicatorWidth - gap
	return remaining >= minInputWidth
}

func (m Model) chromeHeight() int {
	chrome := 1 // footer
	if !m.cfg.Display.ShowHints {
		chrome = 0
	}
	if m.cfg.Display.MultilinePreview == "preview_pane" {
		chrome += previewPaneHeight
	}
	if m.isMerged() {
		return chrome + 1
	}
	return chrome + 2
}

func (m Model) renderInputBar() string {
	width := m.getWidth()
	prompt := m.styles.Prompt.Render(m.cfg.Theme.Prompt)
	indicatorStr := m.renderIndicators()

	promptWidth := lipgloss.Width(prompt)
	indicatorWidth := lipgloss.Width(indicatorStr)
	gap := 2

	remaining := width - promptWidth - indicatorWidth - gap
	if remaining < minInputWidth {
		return m.renderHeader() + "\n" + m.renderInput()
	}

	// Temporarily narrow the input so its View() doesn't pad to full width
	origWidth := m.input.Width
	inputWidth := remaining
	m.input.Width = inputWidth
	input := m.input.View()
	m.input.Width = origWidth

	leftContent := prompt + input
	leftWidth := lipgloss.Width(leftContent)
	fillWidth := max(width-leftWidth-indicatorWidth, 0)

	line := leftContent + strings.Repeat(" ", fillWidth) + indicatorStr
	return m.styles.Input.Width(width).Render(line)
}

func (m Model) renderInput() string {
	width := m.getWidth()
	prompt := m.styles.Prompt.Render(m.cfg.Theme.Prompt)
	input := m.input.View()

	// Calculate remaining width for padding
	contentWidth := lipgloss.Width(prompt) + lipgloss.Width(input)
	padding := max(width-contentWidth, 0)

	line := prompt + input + strings.Repeat(" ", padding)
	return m.styles.Input.Width(width).Render(line)
}

func (m Model) renderResults() string {
	width := m.getWidth()
	headerRows := resultsHeaderRows
	if m.height <= resultsHeaderRows {
		headerRows = 0
	}
	visible := m.visibleResults()

	if len(visible) == 0 {
		header := ""
		if headerRows > 0 {
			header = m.renderResultsHeader()
		}
		if m.dbError != nil {
			msg := m.styles.ExitFail.Render("  Error: " + m.dbError.Error())
			fill := m.height - 1
			if headerRows > 0 {
				fill = m.height - 2
			}
			if fill < 0 {
				fill = 0
			}
			if headerRows > 0 {
				return header + "\n" + msg + strings.Repeat("\n", fill)
			}
			return msg + strings.Repeat("\n", fill)
		}
		if m.input.Value() == "" {
			msg := m.styles.Dimmed.Render("  No history entries found")
			fill := m.height - 1
			if headerRows > 0 {
				fill = m.height - 2
			}
			if fill < 0 {
				fill = 0
			}
			if headerRows > 0 {
				return header + "\n" + msg + strings.Repeat("\n", fill)
			}
			return msg + strings.Repeat("\n", fill)
		}
		msg := m.styles.Dimmed.Render("  No matches found")
		fill := m.height - 1
		if headerRows > 0 {
			fill = m.height - 2
		}
		if fill < 0 {
			fill = 0
		}
		if headerRows > 0 {
			return header + "\n" + msg + strings.Repeat("\n", fill)
		}
		return msg + strings.Repeat("\n", fill)
	}

	var lines []string
	if headerRows > 0 {
		lines = append(lines, m.renderResultsHeader())
	}

	expandMode := m.cfg.Display.MultilinePreview == "expand"
	for _, idx := range visible {
		isSelected := idx == m.cursor

		if expandMode && isSelected && m.entryIsMultiline(idx) {
			expandedLines := m.renderExpandedResultLines(idx)
			lines = append(lines, expandedLines...)
		} else {
			line := m.renderResultLine(idx, isSelected)
			lineWidth := lipgloss.Width(line)
			if lineWidth < width {
				line += strings.Repeat(" ", width-lineWidth)
			}
			lines = append(lines, line)
		}
	}

	// Fill remaining lines
	for len(lines) < m.height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderResultLine(entryIdx int, isSelected bool) string {
	if entryIdx >= len(m.displayEntries) {
		return strings.Repeat(" ", m.getWidth())
	}

	entry := m.displayEntries[entryIdx]
	cmd := entry.Entry.Command
	width := m.getWidth()

	prefixWidth := 2 // "▌ " or "  "
	exitWidth := 4
	durWidth := 8
	timeWidth := timeColumnWidth(m.cfg.Display.TimeFormat)
	sep := "  "

	var dirWidth int
	if m.cfg.Display.ShowDirectory {
		dirWidth = dirColumnWidth(width)
	}

	columnsWidth := prefixWidth + exitWidth + durWidth + timeWidth + (len(sep) * 3)
	if m.cfg.Display.ShowDirectory {
		columnsWidth += dirWidth + len(sep)
	}
	cmdWidth := width - columnsWidth
	if cmdWidth < 10 {
		cmdWidth = width
	}

	// Handle selection styling
	var matchInfo *match.Match
	if entryIdx < len(m.displayEntries) {
		matchInfo = &entry.MatchInfo
	}

	var renderedCmd string
	cmd, matchInfo = collapseMultiline(cmd, matchInfo, m.cfg.Display.MultilineCollapse)
	cmd, matchInfo = truncateWithRanges(cmd, matchInfo, cmdWidth)
	if matchInfo != nil && len(matchInfo.MatchedRanges) > 0 && m.input.Value() != "" {
		renderedCmd = m.highlightMatches(cmd, matchInfo.MatchedRanges)
	} else {
		renderedCmd = cmd
	}
	if isSelected {
		renderedCmd = m.styles.SelectedCmd.Render(renderedCmd)
	} else {
		renderedCmd = m.styles.Cmd.Render(renderedCmd)
	}

	exitStr := formatExit(entry.Entry.ExitCode, exitWidth)
	durStr := formatDuration(entry.Entry.Duration, m.cfg.Display.DurationFormat, durWidth)
	timeStr := formatWhen(entry.Entry.TsMs, m.cfg.Display.TimeFormat, timeWidth)

	exitStyle := m.styles.ExitOk
	if entry.Entry.ExitCode != 0 {
		exitStyle = m.styles.ExitFail
	}
	exitStyled := exitStyle.Width(exitWidth).Align(lipgloss.Right).Render(exitStr)
	durStyled := m.styles.Meta.Width(durWidth).Align(lipgloss.Right).Render(durStr)
	timeStyled := m.styles.Meta.Width(timeWidth).Align(lipgloss.Right).Render(timeStr)
	cmdStyled := lipgloss.NewStyle().Width(cmdWidth).Render(renderedCmd)

	// Build the line
	var line string
	if m.cfg.Display.ShowDirectory {
		dirStr := formatDirectory(entry.Entry.Directory, dirWidth, m.homeDir)
		dirStyled := m.styles.Meta.Width(dirWidth).Align(lipgloss.Right).Render(dirStr)
		line = strings.Join([]string{exitStyled, durStyled, timeStyled, cmdStyled, dirStyled}, sep)
	} else {
		line = strings.Join([]string{exitStyled, durStyled, timeStyled, cmdStyled}, sep)
	}

	if isSelected {
		bar := m.styles.SelectionBar.Render("▌ ")
		line = bar + line
		return line
	}

	return "  " + line
}

func (m Model) entryIsMultiline(idx int) bool {
	if idx >= len(m.displayEntries) {
		return false
	}
	return strings.Contains(m.displayEntries[idx].Entry.Command, "\n")
}

func (m Model) renderExpandedResultLines(entryIdx int) []string {
	if entryIdx >= len(m.displayEntries) {
		return nil
	}

	entry := m.displayEntries[entryIdx]
	cmd := entry.Entry.Command
	width := m.getWidth()

	prefixWidth := 2 // "▌ " or "  "
	exitWidth := 4
	durWidth := 8
	timeWidth := timeColumnWidth(m.cfg.Display.TimeFormat)
	sep := "  "

	var dirWidth int
	if m.cfg.Display.ShowDirectory {
		dirWidth = dirColumnWidth(width)
	}

	columnsWidth := prefixWidth + exitWidth + durWidth + timeWidth + (len(sep) * 3)
	if m.cfg.Display.ShowDirectory {
		columnsWidth += dirWidth + len(sep)
	}
	cmdWidth := width - columnsWidth
	if cmdWidth < 10 {
		cmdWidth = width
	}

	cmdLines := strings.Split(cmd, "\n")
	var result []string

	for i, cmdLine := range cmdLines {
		cmdLine = strings.ReplaceAll(cmdLine, "\t", "    ")
		if len(cmdLine) > cmdWidth {
			cmdLine = cmdLine[:cmdWidth]
		}

		var renderedCmd string
		if i == 0 {
			matchInfo := &entry.MatchInfo
			cmdLine, matchInfo = truncateWithRanges(cmdLine, matchInfo, cmdWidth)
			if matchInfo != nil && len(matchInfo.MatchedRanges) > 0 && m.input.Value() != "" {
				renderedCmd = m.highlightMatches(cmdLine, matchInfo.MatchedRanges)
			} else {
				renderedCmd = cmdLine
			}
		} else {
			renderedCmd = cmdLine
		}
		renderedCmd = m.styles.SelectedCmd.Render(renderedCmd)

		var line string
		if i == 0 {
			exitStr := formatExit(entry.Entry.ExitCode, exitWidth)
			durStr := formatDuration(entry.Entry.Duration, m.cfg.Display.DurationFormat, durWidth)
			timeStr := formatWhen(entry.Entry.TsMs, m.cfg.Display.TimeFormat, timeWidth)

			exitStyle := m.styles.ExitOk
			if entry.Entry.ExitCode != 0 {
				exitStyle = m.styles.ExitFail
			}
			exitStyled := exitStyle.Width(exitWidth).Align(lipgloss.Right).Render(exitStr)
			durStyled := m.styles.Meta.Width(durWidth).Align(lipgloss.Right).Render(durStr)
			timeStyled := m.styles.Meta.Width(timeWidth).Align(lipgloss.Right).Render(timeStr)
			cmdStyled := lipgloss.NewStyle().Width(cmdWidth).Render(renderedCmd)

			if m.cfg.Display.ShowDirectory {
				dirStr := formatDirectory(entry.Entry.Directory, dirWidth, m.homeDir)
				dirStyled := m.styles.Meta.Width(dirWidth).Align(lipgloss.Right).Render(dirStr)
				line = strings.Join([]string{exitStyled, durStyled, timeStyled, cmdStyled, dirStyled}, sep)
			} else {
				line = strings.Join([]string{exitStyled, durStyled, timeStyled, cmdStyled}, sep)
			}

			bar := m.styles.SelectionBar.Render("▌ ")
			line = bar + line
		} else {
			metaWidth := exitWidth + durWidth + timeWidth + (len(sep) * 3)
			if m.cfg.Display.ShowDirectory {
				metaWidth += dirWidth + len(sep)
			}
			padding := strings.Repeat(" ", metaWidth+2)
			cmdStyled := lipgloss.NewStyle().Width(cmdWidth).Render(renderedCmd)
			line = "│ " + padding[:metaWidth] + cmdStyled
		}

		lineWidth := lipgloss.Width(line)
		if lineWidth < width {
			line += strings.Repeat(" ", width-lineWidth)
		}
		result = append(result, line)
	}

	return result
}

func (m Model) renderFooter() string {
	width := m.getWidth()
	keys := []struct {
		key  string
		desc string
	}{
		{"↑↓", "nav"},
		{"enter", "select"},
		{"esc", "cancel"},
		{"ctrl+s", "mode"},
		{"ctrl+g", "cwd"},
		{"ctrl+d", "dedup"},
		{"?", "help"},
	}

	var parts []string
	for _, k := range keys {
		key := m.styles.HelpKey.Render(k.key)
		desc := m.styles.HelpDesc.Render(k.desc)
		parts = append(parts, key+" "+desc)
	}

	if m.cfg.Display.MultilinePreview == "popup" && m.selectedIsMultiline() {
		key := m.styles.HelpKey.Render(m.cfg.Keys.PreviewCommand)
		desc := m.styles.HelpDesc.Render("preview")
		parts = append(parts, key+" "+desc)
	}

	return m.styles.Footer.Width(width).Render(strings.Join(parts, "  "))
}

func (m Model) selectedIsMultiline() bool {
	if len(m.displayEntries) == 0 || m.cursor >= len(m.displayEntries) {
		return false
	}
	return strings.Contains(m.displayEntries[m.cursor].Entry.Command, "\n")
}

func (m Model) renderPreviewPane() string {
	width := m.getWidth()

	if len(m.displayEntries) == 0 || m.cursor >= len(m.displayEntries) {
		emptyLine := strings.Repeat(" ", width)
		var lines []string
		for range previewPaneHeight {
			lines = append(lines, emptyLine)
		}
		return strings.Join(lines, "\n")
	}

	cmd := m.displayEntries[m.cursor].Entry.Command

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Bold(true)
	header := headerStyle.Render("─ Preview ─")
	headerLine := header + strings.Repeat("─", max(width-lipgloss.Width(header), 0))

	cmd = strings.ReplaceAll(cmd, "\t", "    ")
	cmdLines := strings.Split(cmd, "\n")

	contentHeight := previewPaneHeight - 1
	var displayLines []string
	for i := 0; i < contentHeight && i < len(cmdLines); i++ {
		line := cmdLines[i]
		if len(line) > width {
			line = line[:width]
		}
		if len(line) < width {
			line += strings.Repeat(" ", width-len(line))
		}
		displayLines = append(displayLines, m.styles.Dimmed.Render(line))
	}

	for len(displayLines) < contentHeight {
		displayLines = append(displayLines, strings.Repeat(" ", width))
	}

	return headerLine + "\n" + strings.Join(displayLines, "\n")
}

func (m Model) renderHelp() string {
	width := m.getWidth()

	header := m.styles.Title.Render(" Keybindings ")

	bindings := []struct {
		key  string
		desc string
	}{
		{m.cfg.Keys.Up + "/" + m.cfg.Keys.Down, "Move up/down"},
		{m.cfg.Keys.PageUp + "/" + m.cfg.Keys.PageDown, "Page up/down"},
		{m.cfg.Keys.Top + "/" + m.cfg.Keys.Bottom, "Jump to top/bottom"},
		{m.cfg.Keys.Accept, "Accept selection"},
		{m.cfg.Keys.Cancel, "Cancel / quit"},
		{m.cfg.Keys.ModeNext, "Cycle match mode (fuzzy/regex/glob)"},
		{m.cfg.Keys.ModeFuzzy, "Fuzzy match mode"},
		{m.cfg.Keys.ModeRegex, "Regex match mode"},
		{m.cfg.Keys.ModeGlob, "Glob match mode"},
		{m.cfg.Keys.ToggleCWD, "Filter to current directory"},
		{m.cfg.Keys.ToggleDedupe, "Toggle command deduplication"},
		{m.cfg.Keys.ToggleFails, "Show only failed commands"},
		{m.cfg.Keys.PreviewCommand, "Preview multiline command"},
		{m.cfg.Keys.Help, "Show/hide this help"},
	}

	var lines []string
	for _, bind := range bindings {
		key := m.styles.HelpKey.Render(fmt.Sprintf("%-16s", bind.key))
		desc := m.styles.HelpDesc.Render(bind.desc)
		lines = append(lines, "  "+key+"  "+desc)
	}

	content := strings.Join(lines, "\n")
	footer := m.styles.Dimmed.Render("  Press any key to dismiss")

	boxContent := header + "\n\n" + content + "\n\n" + footer
	boxWidth := width - 4
	if boxWidth < 10 {
		boxWidth = width
	}
	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(boxWidth).
		Render(boxContent)

	return box
}

func (m Model) renderPreviewPopup() string {
	width := m.getWidth()

	header := m.styles.Title.Render(" Command Preview ")

	contentWidth := width - 8
	if contentWidth < 20 {
		contentWidth = width - 4
	}

	lines := strings.Split(m.previewCommand, "\n")
	var wrappedLines []string
	for _, line := range lines {
		line = strings.ReplaceAll(line, "\t", "    ")
		if len(line) > contentWidth {
			for len(line) > contentWidth {
				wrappedLines = append(wrappedLines, line[:contentWidth])
				line = line[contentWidth:]
			}
			if len(line) > 0 {
				wrappedLines = append(wrappedLines, line)
			}
		} else {
			wrappedLines = append(wrappedLines, line)
		}
	}

	content := strings.Join(wrappedLines, "\n")
	footer := m.styles.Dimmed.Render("  Press any key to dismiss")

	boxContent := header + "\n\n" + content + "\n\n" + footer
	boxWidth := width - 4
	if boxWidth < 10 {
		boxWidth = width
	}
	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(boxWidth).
		Render(boxContent)

	return box
}

func (m Model) getWidth() int {
	return m.width
}

func (m Model) visibleResults() []int {
	count := len(m.displayEntries)
	if count == 0 {
		return nil
	}

	headerRows := resultsHeaderRows
	if m.height <= resultsHeaderRows {
		headerRows = 0
	}
	maxVisible := min(m.height-headerRows, count)
	if maxVisible < 1 {
		maxVisible = 0
	}
	if maxVisible == 0 {
		return nil
	}

	// Window scrolling
	start := 0
	if m.cursor >= maxVisible {
		start = m.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > count {
		end = count
		start = max(end-maxVisible, 0)
	}

	indices := make([]int, end-start)
	for i := range indices {
		indices[i] = start + i
	}
	return indices
}

func (m Model) renderResultsHeader() string {
	width := m.getWidth()
	prefixWidth := 2 // "▌ " or "  "
	exitWidth := 4
	durWidth := 8
	timeWidth := timeColumnWidth(m.cfg.Display.TimeFormat)
	sep := "  "

	var dirWidth int
	if m.cfg.Display.ShowDirectory {
		dirWidth = dirColumnWidth(width)
	}

	columnsWidth := prefixWidth + exitWidth + durWidth + timeWidth + (len(sep) * 3)
	if m.cfg.Display.ShowDirectory {
		columnsWidth += dirWidth + len(sep)
	}
	cmdWidth := width - columnsWidth
	if cmdWidth < 10 {
		cmdWidth = width
	}

	exit := m.styles.ColumnHeader.Width(exitWidth).Align(lipgloss.Right).Render("exit")
	dur := m.styles.ColumnHeader.Width(durWidth).Align(lipgloss.Right).Render("time")
	when := m.styles.ColumnHeader.Width(timeWidth).Align(lipgloss.Right).Render("when")
	cmd := m.styles.ColumnHeader.Width(cmdWidth).Render("command")

	var line string
	if m.cfg.Display.ShowDirectory {
		dir := m.styles.ColumnHeader.Width(dirWidth).Align(lipgloss.Right).Render("dir")
		line = "  " + strings.Join([]string{exit, dur, when, cmd, dir}, sep)
	} else {
		line = "  " + strings.Join([]string{exit, dur, when, cmd}, sep)
	}

	if lipgloss.Width(line) < width {
		line += strings.Repeat(" ", width-lipgloss.Width(line))
	}
	return m.styles.ColumnHeaderBar.Width(width).Render(line)
}

func (m Model) fitIndicators(indicators []string, width int) string {
	if len(indicators) == 0 {
		return ""
	}
	best := strings.Join(indicators, " ")
	if lipgloss.Width(best) <= width {
		return best
	}
	for i := len(indicators) - 1; i >= 0; i-- {
		candidate := strings.Join(indicators[:i], " ")
		if lipgloss.Width(candidate) <= width {
			return candidate
		}
	}
	return ""
}

func (m Model) highlightMatches(text string, ranges []match.Range) string {
	if len(ranges) == 0 {
		return text
	}

	runes := []rune(text)
	inMatch := map[int]bool{}
	for _, r := range ranges {
		for i := r.Start; i < r.End && i < len(runes); i++ {
			inMatch[i] = true
		}
	}

	var b strings.Builder
	inRun := false
	runStart := 0

	for i := 0; i <= len(runes); i++ {
		current := i < len(runes) && inMatch[i]
		if i == len(runes) || current != inRun {
			if i > runStart {
				chunk := string(runes[runStart:i])
				if inRun {
					b.WriteString(m.styles.Match.Render(chunk))
				} else {
					b.WriteString(chunk)
				}
			}
			inRun = current
			runStart = i
		}
	}
	return b.String()
}

func timeColumnWidth(mode string) int {
	switch mode {
	case "absolute":
		return 16
	default:
		return 8
	}
}

func dirColumnWidth(width int) int {
	w := width / 5
	if w < 12 {
		return 12
	}
	if w > 30 {
		return 30
	}
	return w
}

func formatDirectory(dir string, width int, home string) string {
	if home != "" && strings.HasPrefix(dir, home) {
		dir = "~" + dir[len(home):]
	}
	if len(dir) <= width {
		return dir
	}
	runes := []rune(dir)
	return "…" + string(runes[len(runes)-width+1:])
}

func formatExit(code int, width int) string {
	return fmt.Sprintf("%*d", width, code)
}

func formatDuration(ms int64, mode string, width int) string {
	var s string
	switch mode {
	case "ms":
		s = fmt.Sprintf("%dms", ms)
	case "s":
		s = fmt.Sprintf("%.2fs", float64(ms)/1000.0)
	default:
		s = humanDuration(ms)
	}
	s = trimToWidth(s, width)
	return fmt.Sprintf("%*s", width, s)
}

func formatWhen(tsMs int64, mode string, width int) string {
	tsMs = normalizeTimestampMs(tsMs)
	if tsMs <= 0 {
		return fmt.Sprintf("%*s", width, trimToWidth("n/a", width))
	}
	t := time.UnixMilli(tsMs)
	now := time.Now()
	var s string
	switch mode {
	case "absolute":
		s = t.Format("2006-01-02 15:04")
	default:
		s = humanSince(safeSub(now, t))
	}
	s = trimToWidth(s, width)
	return fmt.Sprintf("%*s", width, s)
}

func humanDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	sec := float64(ms) / 1000.0
	if sec < 60 {
		return fmt.Sprintf("%.1fs", sec)
	}
	min := sec / 60.0
	if min < 60 {
		return fmt.Sprintf("%.1fm", min)
	}
	h := min / 60.0
	return fmt.Sprintf("%.1fh", h)
}

func humanSince(d time.Duration) string {
	if d == math.MinInt64 {
		d = math.MaxInt64
	}
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}

func safeSub(a, b time.Time) time.Duration {
	d := a.Sub(b)
	if d == math.MinInt64 {
		return math.MaxInt64
	}
	return d
}

func normalizeTimestampMs(tsMs int64) int64 {
	if tsMs <= 0 {
		return tsMs
	}
	nowMs := time.Now().UnixMilli()
	if tsMs > nowMs*1000 {
		if tsMs > nowMs*1_000_000 {
			tsMs /= 1_000_000
		} else {
			tsMs /= 1000
		}
	}
	maxUnixMs := int64(math.MaxInt64) / int64(time.Millisecond)
	if tsMs > maxUnixMs {
		return nowMs
	}
	return tsMs
}

func trimToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return string(runes[:width])
}

func truncateWithRanges(text string, info *match.Match, max int) (string, *match.Match) {
	if max <= 0 || len(text) <= max {
		return text, info
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text, info
	}
	ellipsis := "..."
	cutoff := max - len(ellipsis)
	if cutoff < 0 {
		cutoff = 0
	}
	truncated := string(runes[:cutoff]) + ellipsis
	if info == nil || len(info.MatchedRanges) == 0 {
		return truncated, info
	}
	var ranges []match.Range
	for _, r := range info.MatchedRanges {
		if r.Start >= cutoff {
			continue
		}
		end := min(r.End, cutoff)
		if end > r.Start {
			ranges = append(ranges, match.Range{Start: r.Start, End: end})
		}
	}
	infoCopy := *info
	infoCopy.MatchedRanges = ranges
	return truncated, &infoCopy
}

func collapseMultiline(text string, info *match.Match, collapseSymbol string) (string, *match.Match) {
	if !strings.ContainsAny(text, "\n\r\t") {
		return text, info
	}

	symbolRunes := []rune(collapseSymbol)
	if len(symbolRunes) == 0 {
		symbolRunes = []rune{' '}
	}

	textRunes := []rune(text)
	var collapsed []rune
	runeMap := make([]int, 0, len(textRunes))

	for i, r := range textRunes {
		switch r {
		case '\n', '\r':
			collapsed = append(collapsed, symbolRunes...)
			for range symbolRunes {
				runeMap = append(runeMap, i)
			}
		case '\t':
			collapsed = append(collapsed, ' ', ' ', ' ', ' ')
			for range 4 {
				runeMap = append(runeMap, i)
			}
		default:
			collapsed = append(collapsed, r)
			runeMap = append(runeMap, i)
		}
	}

	if info == nil || len(info.MatchedRanges) == 0 {
		return string(collapsed), info
	}

	reverseMap := make(map[int]int)
	for newIdx, oldIdx := range runeMap {
		reverseMap[oldIdx] = newIdx
	}

	var newRanges []match.Range
	for _, r := range info.MatchedRanges {
		newStart, okStart := reverseMap[r.Start]
		if !okStart {
			for i := r.Start; i < len(textRunes); i++ {
				if ns, ok := reverseMap[i]; ok {
					newStart = ns
					okStart = true
					break
				}
			}
		}
		newEnd := newStart
		for i := r.Start; i < r.End && i < len(textRunes); i++ {
			if ne, ok := reverseMap[i]; ok {
				newEnd = ne + 1
			}
		}
		if okStart && newEnd > newStart {
			newRanges = append(newRanges, match.Range{Start: newStart, End: newEnd})
		}
	}

	infoCopy := *info
	infoCopy.MatchedRanges = newRanges
	return string(collapsed), &infoCopy
}
