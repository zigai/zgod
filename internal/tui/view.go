package tui

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
	"github.com/zigai/zgod/internal/match"
)

const (
	panelBorderW         = 2
	panelBorderH         = 2
	panelPaddingX        = 1
	panelPaddingY        = 0
	resultsHeaderRows    = 1
	minInputWidth        = 20
	previewPaneHeight    = 4
	defaultSelectionChar = "▌ "
	failIncludeIndicator = "214"
)

type toggleIndicator struct {
	label  string
	bg     string
	active bool
}

func failToggleIndicator(mode db.FailFilterMode) toggleIndicator {
	indicator := toggleIndicator{label: "fails"}

	switch mode {
	case db.FailFilterInclude:
		indicator.bg = failIncludeIndicator
		indicator.active = true
	case db.FailFilterExclude:
	case db.FailFilterOnly:
		indicator.bg = "9"
		indicator.active = true
	}

	return indicator
}

func (m *Model) View() string {
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

	sections = append(sections, m.renderInputBar())
	sections = append(sections, m.renderResults())

	if m.cfg.Display.MultilinePreview == "preview_pane" {
		sections = append(sections, m.renderPreviewPane())
	}

	if m.cfg.Display.ShowHints {
		sections = append(sections, m.renderFooter())
	}

	body := strings.Join(sections, "\n")

	return m.styles.Border.
		Width(m.width+panelPaddingX*2).
		Padding(panelPaddingY, panelPaddingX).
		Render(body)
}

func (m *Model) renderIndicators() string {
	width := m.getWidth()

	key := indicatorCacheKey{
		width:      width,
		mode:       m.mode,
		cwdMode:    m.cwdMode,
		dedupe:     m.dedupe,
		failFilter: m.failFilter,
	}
	if m.indicatorCache.valid && m.indicatorCache.key == key {
		return m.indicatorCache.value
	}

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

	const searchModeIndicatorBg = "39"

	modes := []modeIndicator{
		{match.ModeFuzzy, "fuzzy", searchModeIndicatorBg, m.cfg.Display.EnableFuzzy},
		{match.ModeGlob, "glob", searchModeIndicatorBg, m.cfg.Display.EnableGlob},
		{match.ModeRegex, "regex", searchModeIndicatorBg, m.cfg.Display.EnableRegex},
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

	toggles := []toggleIndicator{
		{"cwd", "10", m.cwdMode},
		failToggleIndicator(m.failFilter),
		{"dedup", "11", m.dedupe},
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

	value := m.fitIndicators(indicators, width)
	m.indicatorCache = indicatorCache{
		key:   key,
		value: value,
		valid: true,
	}

	return value
}

func (m *Model) renderHeader() string {
	width := m.getWidth()
	indicatorStr := m.renderIndicators()

	fillWidth := max(width-lipgloss.Width(indicatorStr), 0)

	line := indicatorStr + strings.Repeat(" ", fillWidth)

	return m.styles.HeaderBar.Width(width).Render(line)
}

func (m *Model) isMerged() bool {
	width := m.getWidth()
	prompt := m.cfg.Theme.Prompt
	promptWidth := lipgloss.Width(m.styles.Prompt.Render(prompt))
	indicatorStr := m.renderIndicators()
	indicatorWidth := lipgloss.Width(indicatorStr)
	gap := 2
	remaining := width - promptWidth - indicatorWidth - gap

	return remaining >= minInputWidth
}

func (m *Model) chromeHeight() int {
	chrome := 1
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

func (m *Model) renderInputBar() string {
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

func (m *Model) renderInput() string {
	width := m.getWidth()
	prompt := m.styles.Prompt.Render(m.cfg.Theme.Prompt)
	input := m.input.View()

	contentWidth := lipgloss.Width(prompt) + lipgloss.Width(input)
	padding := max(width-contentWidth, 0)

	line := prompt + input + strings.Repeat(" ", padding)

	return m.styles.Input.Width(width).Render(line)
}

func (m *Model) emptyStateMessage() string {
	switch {
	case m.dbError != nil:
		return m.styles.ExitFail.Render("  Error: " + m.dbError.Error())
	case m.loadingHistory:
		return m.styles.Dimmed.Render("  loading history...")
	case m.input.Value() == "":
		return m.styles.Dimmed.Render("  No history entries found")
	default:
		return m.styles.Dimmed.Render("  No matches found")
	}
}

func (m *Model) renderEmptyState(headerRows int) string {
	msg := m.emptyStateMessage()
	fill := max(m.height-1-headerRows, 0)

	if headerRows > 0 {
		return m.renderResultsHeader() + "\n" + msg + strings.Repeat("\n", fill)
	}

	return msg + strings.Repeat("\n", fill)
}

func (m *Model) renderResults() string {
	width := m.getWidth()
	layout := m.calcResultLayout()
	now := time.Now()

	headerRows := resultsHeaderRows
	if m.height <= resultsHeaderRows {
		headerRows = 0
	}

	start, end := m.visibleResultRange()
	if start == end {
		return m.renderEmptyState(headerRows)
	}

	cacheKey := m.resultsBlockCacheKey(layout, now)
	if m.resultsCache.valid && m.resultsCache.key == cacheKey {
		return m.resultsCache.value
	}

	var lines []string
	if headerRows > 0 {
		lines = append(lines, m.renderResultsHeaderWithLayout(layout))
	}

	expandMode := m.cfg.Display.MultilinePreview == "expand"
	for idx := start; idx < end; idx++ {
		if len(lines) >= m.height {
			break
		}

		isSelected := idx == m.cursor

		if expandMode && isSelected && m.entryIsMultiline(idx) {
			expandedLines := m.renderExpandedResultLinesWithLayout(idx, layout, now)

			remaining := m.height - len(lines)
			if remaining <= 0 {
				break
			}

			if len(expandedLines) > remaining {
				expandedLines = expandedLines[:remaining]
			}

			lines = append(lines, expandedLines...)

			continue
		}

		line := m.renderResultLineWithLayout(idx, isSelected, layout, now)
		if lineWidth := lipgloss.Width(line); lineWidth < width {
			line += strings.Repeat(" ", width-lineWidth)
		}

		lines = append(lines, line)
	}

	for len(lines) < m.height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	result := strings.Join(lines, "\n")
	m.resultsCache = cachedResultsBlock{
		key:   cacheKey,
		value: result,
		valid: true,
	}

	return result
}

func (m *Model) resultsBlockCacheKey(layout resultLayout, now time.Time) resultsBlockCacheKey {
	return resultsBlockCacheKey{
		width:            layout.width,
		height:           m.height,
		cursor:           m.cursor,
		displayLen:       len(m.displayEntries),
		query:            m.input.Value(),
		mode:             m.mode,
		showDir:          layout.showDir,
		multilinePreview: m.cfg.Display.MultilinePreview,
		timeFormat:       m.cfg.Display.TimeFormat,
		durationFormat:   m.cfg.Display.DurationFormat,
		timeBucket:       resultLineTimeBucket(m.cfg.Display.TimeFormat, now),
	}
}

type resultLayout struct {
	width       int
	prefixWidth int
	exitWidth   int
	durWidth    int
	timeWidth   int
	dirWidth    int
	cmdWidth    int
	sep         string
	barChar     string
	showDir     bool
}

func (m *Model) calcResultLayout() resultLayout {
	width := m.getWidth()

	barChar := m.cfg.Theme.SelectionBarChar
	if barChar == "" {
		barChar = defaultSelectionChar
	}

	prefixWidth := lipgloss.Width(barChar)
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

	return resultLayout{
		width:       width,
		prefixWidth: prefixWidth,
		exitWidth:   exitWidth,
		durWidth:    durWidth,
		timeWidth:   timeWidth,
		dirWidth:    dirWidth,
		cmdWidth:    cmdWidth,
		sep:         sep,
		barChar:     barChar,
		showDir:     m.cfg.Display.ShowDirectory,
	}
}

func (m *Model) renderSelectionPrefix(layout resultLayout, fullLineBg bool, selBg lipgloss.TerminalColor) string {
	if !config.BoolDefault(m.cfg.Theme.SelectionBarShow, true) {
		if fullLineBg {
			return lipgloss.NewStyle().Background(selBg).Render(strings.Repeat(" ", layout.prefixWidth))
		}

		return strings.Repeat(" ", layout.prefixWidth)
	}

	barStyle := m.styles.SelectionBar
	if fullLineBg {
		barStyle = barStyle.Background(selBg)
	}

	return barStyle.Render(layout.barChar)
}

func (m *Model) renderResultLineWithLayout(entryIdx int, isSelected bool, layout resultLayout, now time.Time) string {
	if entryIdx >= len(m.displayEntries) {
		return strings.Repeat(" ", layout.width)
	}

	entry := m.displayEntries[entryIdx]

	cacheKey := m.resultLineCacheKey(entry, layout, now)
	if cached, ok := m.cachedResultLine(entryIdx, cacheKey, isSelected); ok {
		return cached
	}

	line := m.renderResultLineContent(entryIdx, entry, isSelected, layout, now)
	if !isSelected {
		line = strings.Repeat(" ", layout.prefixWidth) + line
		m.cacheResultLine(entryIdx, cacheKey, line)

		return line
	}

	return m.renderSelectedResultLine(line, layout)
}

func (m *Model) cachedResultLine(entryIdx int, cacheKey resultLineCacheKey, isSelected bool) (string, bool) {
	if isSelected {
		return "", false
	}

	cached, ok := m.lineCache[entryIdx]
	if ok && cached.key == cacheKey {
		return cached.line, true
	}

	return "", false
}

func (m *Model) renderResultLineContent(entryIdx int, entry history.ScoredEntry, isSelected bool, layout resultLayout, now time.Time) string {
	cmd, renderedCmd := m.renderResultCommand(entryIdx, entry, isSelected, layout)
	fullLineBg := isSelected && config.BoolDefault(m.cfg.Theme.SelectionFullLine, true)
	selBg := parseColor(m.cfg.Theme.SelectedBg)
	exitStyle, metaStyle := m.resultMetaStyles(entry.Entry.ExitCode, fullLineBg, selBg)

	exitStyled := exitStyle.Render(formatExit(entry.Entry.ExitCode, layout.exitWidth))
	durStyled := metaStyle.Render(formatDuration(entry.Entry.Duration, m.cfg.Display.DurationFormat, layout.durWidth))
	timeStyled := metaStyle.Render(formatWhenAt(entry.Entry.TSMs, m.cfg.Display.TimeFormat, layout.timeWidth, now))
	cmdStyled := padRenderedCell(renderedCmd, layout.cmdWidth, lipgloss.Width(cmd), fullLineBg, selBg)

	styledSep := layout.sep
	if fullLineBg {
		styledSep = lipgloss.NewStyle().Background(selBg).Render(layout.sep)
	}

	if !layout.showDir {
		return exitStyled + styledSep + durStyled + styledSep + timeStyled + styledSep + cmdStyled
	}

	dirStyled := metaStyle.Render(padLeft(formatDirectory(entry.Entry.Directory, layout.dirWidth, m.homeDir), layout.dirWidth))

	return exitStyled + styledSep + durStyled + styledSep + timeStyled + styledSep + cmdStyled + styledSep + dirStyled
}

func (m *Model) renderResultCommand(entryIdx int, entry history.ScoredEntry, isSelected bool, layout resultLayout) (string, string) {
	cmd := entry.Entry.Command
	matchInfo := m.cachedRenderMatchInfo(entryIdx, cmd)

	fullLineBg := isSelected && config.BoolDefault(m.cfg.Theme.SelectionFullLine, true)
	selBg := parseColor(m.cfg.Theme.SelectedBg)

	cmd, matchInfo = collapseMultiline(cmd, matchInfo, m.cfg.Display.MultilineCollapse)
	matchInfo = m.ensureRenderMatchRanges(cmd, matchInfo)
	cmd, matchInfo = truncateWithRanges(cmd, matchInfo, layout.cmdWidth)

	cmdStyle := m.styles.Cmd
	if isSelected {
		cmdStyle = m.styles.SelectedCmd
	}

	if fullLineBg {
		cmdStyle = cmdStyle.Background(selBg)
	}

	matchStyle := m.styles.Match
	if fullLineBg && m.cfg.Theme.MatchBg == "" {
		matchStyle = matchStyle.Background(selBg)
	}

	var renderedCmd string
	if matchInfo != nil && len(matchInfo.MatchedRanges) > 0 && m.input.Value() != "" {
		renderedCmd = m.highlightMatches(cmd, matchInfo.MatchedRanges, cmdStyle, matchStyle)
	} else {
		renderedCmd = cmdStyle.Render(cmd)
	}

	return cmd, renderedCmd
}

func (m *Model) resultMetaStyles(exitCode int, fullLineBg bool, selBg lipgloss.TerminalColor) (lipgloss.Style, lipgloss.Style) {
	exitStyle := m.styles.ExitOk
	if exitCode != 0 {
		exitStyle = m.styles.ExitFail
	}

	metaStyle := m.styles.Meta

	if fullLineBg {
		exitStyle = exitStyle.Background(selBg)
		metaStyle = metaStyle.Background(selBg)
	}

	return exitStyle, metaStyle
}

func (m *Model) renderSelectedResultLine(line string, layout resultLayout) string {
	fullLineBg := config.BoolDefault(m.cfg.Theme.SelectionFullLine, true)
	selBg := parseColor(m.cfg.Theme.SelectedBg)
	prefix := m.renderSelectionPrefix(layout, fullLineBg, selBg)

	fullLine := prefix + line
	if fullLineBg {
		if lineWidth := lipgloss.Width(fullLine); lineWidth < layout.width {
			fullLine += lipgloss.NewStyle().Background(selBg).Render(strings.Repeat(" ", layout.width-lineWidth))
		}
	}

	return fullLine
}

func (m *Model) resultLineCacheKey(entry history.ScoredEntry, layout resultLayout, now time.Time) resultLineCacheKey {
	return resultLineCacheKey{
		width:          layout.width,
		cmdWidth:       layout.cmdWidth,
		entryID:        entry.Entry.ID,
		query:          m.input.Value(),
		mode:           m.mode,
		showDir:        layout.showDir,
		timeFormat:     m.cfg.Display.TimeFormat,
		durationFormat: m.cfg.Display.DurationFormat,
		timeBucket:     resultLineTimeBucket(m.cfg.Display.TimeFormat, now),
	}
}

func resultLineTimeBucket(timeFormat string, now time.Time) int64 {
	if timeFormat == "absolute" {
		return 0
	}

	return now.Unix()
}

func (m *Model) cacheResultLine(entryIdx int, key resultLineCacheKey, line string) {
	if m.lineCache == nil {
		m.lineCache = make(map[int]cachedResultLine, m.pageSize()+1)
	}

	if len(m.lineCache) > max(m.pageSize()*4, 32) {
		clear(m.lineCache)
	}

	m.lineCache[entryIdx] = cachedResultLine{
		key:  key,
		line: line,
	}
}

func (m *Model) entryIsMultiline(idx int) bool {
	if idx >= len(m.displayEntries) {
		return false
	}

	return strings.Contains(m.displayEntries[idx].Entry.Command, "\n")
}

func (m *Model) renderExpandedFirstLineAt(entry *history.ScoredEntry, layout resultLayout, fullLineBg bool, selBg lipgloss.TerminalColor, cmdLine string, now time.Time) string {
	matchInfo := &entry.MatchInfo
	matchInfo = m.ensureRenderMatchRanges(cmdLine, matchInfo)
	cmdLine, matchInfo = truncateWithRanges(cmdLine, matchInfo, layout.cmdWidth)

	cmdStyle := m.styles.SelectedCmd
	if fullLineBg {
		cmdStyle = cmdStyle.Background(selBg)
	}

	matchStyle := m.styles.Match
	if fullLineBg && m.cfg.Theme.MatchBg == "" {
		matchStyle = matchStyle.Background(selBg)
	}

	var renderedCmd string
	if matchInfo != nil && len(matchInfo.MatchedRanges) > 0 && m.input.Value() != "" {
		renderedCmd = m.highlightMatches(cmdLine, matchInfo.MatchedRanges, cmdStyle, matchStyle)
	} else {
		renderedCmd = cmdStyle.Render(cmdLine)
	}

	exitStyle := m.styles.ExitOk
	if entry.Entry.ExitCode != 0 {
		exitStyle = m.styles.ExitFail
	}

	metaStyle := m.styles.Meta

	if fullLineBg {
		exitStyle = exitStyle.Background(selBg)
		metaStyle = metaStyle.Background(selBg)
	}

	exitStyled := exitStyle.Render(formatExit(entry.Entry.ExitCode, layout.exitWidth))
	durStyled := metaStyle.Render(formatDuration(entry.Entry.Duration, m.cfg.Display.DurationFormat, layout.durWidth))
	timeStyled := metaStyle.Render(formatWhenAt(entry.Entry.TSMs, m.cfg.Display.TimeFormat, layout.timeWidth, now))
	cmdStyled := padRenderedCell(renderedCmd, layout.cmdWidth, lipgloss.Width(cmdLine), fullLineBg, selBg)

	styledSep := layout.sep
	if fullLineBg {
		styledSep = lipgloss.NewStyle().Background(selBg).Render(layout.sep)
	}

	var line string

	if layout.showDir {
		dirStyled := metaStyle.Render(padLeft(formatDirectory(entry.Entry.Directory, layout.dirWidth, m.homeDir), layout.dirWidth))
		line = exitStyled + styledSep + durStyled + styledSep + timeStyled + styledSep + cmdStyled + styledSep + dirStyled
	} else {
		line = exitStyled + styledSep + durStyled + styledSep + timeStyled + styledSep + cmdStyled
	}

	prefix := m.renderSelectionPrefix(layout, fullLineBg, selBg)

	return prefix + line
}

func (m *Model) renderExpandedContinuationLine(layout resultLayout, fullLineBg bool, selBg lipgloss.TerminalColor, cmdLine string) string {
	cmdStyle := m.styles.SelectedCmd
	if fullLineBg {
		cmdStyle = cmdStyle.Background(selBg)
	}

	renderedCmd := cmdStyle.Render(cmdLine)

	metaWidth := layout.exitWidth + layout.durWidth + layout.timeWidth + (len(layout.sep) * 3)
	if layout.showDir {
		metaWidth += layout.dirWidth + len(layout.sep)
	}

	continuationChar := "│ "
	if !config.BoolDefault(m.cfg.Theme.SelectionBarShow, true) {
		continuationChar = "  "
	}

	padding := strings.Repeat(" ", metaWidth)
	cmdStyled := padRenderedCell(renderedCmd, layout.cmdWidth, lipgloss.Width(cmdLine), fullLineBg, selBg)
	lineContent := continuationChar + padding + cmdStyled

	if fullLineBg {
		return lipgloss.NewStyle().Background(selBg).Render(lineContent)
	}

	return lineContent
}

func (m *Model) padLine(line string, width int, fullLineBg bool, selBg lipgloss.TerminalColor) string {
	lineWidth := lipgloss.Width(line)
	if lineWidth >= width {
		return line
	}

	padding := strings.Repeat(" ", width-lineWidth)
	if fullLineBg {
		return line + lipgloss.NewStyle().Background(selBg).Render(padding)
	}

	return line + padding
}

func padRenderedCell(rendered string, width int, visibleWidth int, fullLineBg bool, bg lipgloss.TerminalColor) string {
	padding := width - visibleWidth
	if padding <= 0 {
		return rendered
	}

	spaces := strings.Repeat(" ", padding)
	if fullLineBg {
		spaces = lipgloss.NewStyle().Background(bg).Render(spaces)
	}

	return rendered + spaces
}

func padLeft(s string, width int) string {
	padding := width - lipgloss.Width(s)
	if padding <= 0 {
		return s
	}

	return strings.Repeat(" ", padding) + s
}

func (m *Model) renderExpandedResultLines(entryIdx int) []string {
	layout := m.calcResultLayout()
	return m.renderExpandedResultLinesWithLayout(entryIdx, layout, time.Now())
}

func (m *Model) renderExpandedResultLinesWithLayout(entryIdx int, layout resultLayout, now time.Time) []string {
	if entryIdx >= len(m.displayEntries) {
		return nil
	}

	entry := m.displayEntries[entryIdx]
	fullLineBg := config.BoolDefault(m.cfg.Theme.SelectionFullLine, true)
	selBg := parseColor(m.cfg.Theme.SelectedBg)

	cmdLines := strings.Split(entry.Entry.Command, "\n")
	result := make([]string, 0, len(cmdLines))

	for i, cmdLine := range cmdLines {
		cmdLine = strings.ReplaceAll(cmdLine, "\t", "    ")
		cmdLine = trimToWidth(cmdLine, layout.cmdWidth)

		var line string
		if i == 0 {
			line = m.renderExpandedFirstLineAt(&entry, layout, fullLineBg, selBg, cmdLine, now)
		} else {
			line = m.renderExpandedContinuationLine(layout, fullLineBg, selBg, cmdLine)
		}

		result = append(result, m.padLine(line, layout.width, fullLineBg, selBg))
	}

	return result
}

func (m *Model) renderFooter() string {
	width := m.getWidth()
	left := m.renderFooterLeft()
	right := m.styles.HelpDesc.Render(m.matchCountLabel())
	contentWidth := max(width-lipgloss.Width(m.styles.Footer.Render("")), 0)
	line := layoutFooterLine(left, right, contentWidth)

	return m.styles.Footer.Width(width).Render(line)
}

func (m *Model) matchCountLabel() string {
	label := formatMatchCountLabel(len(m.displayEntries))
	if !m.historyComplete && len(m.displayEntries) > 0 && len(m.allEntries) > 0 {
		return label + "+"
	}

	if m.loadingHistory {
		return "loading more..."
	}

	return label
}

func (m *Model) renderFooterLeft() string {
	showPreviewHint := m.cfg.Display.MultilinePreview == "popup" && m.selectedIsMultiline()
	if m.footerCache.valid && m.footerCache.showPreviewHint == showPreviewHint {
		return m.footerCache.left
	}

	keys := []struct {
		key  string
		desc string
	}{
		{m.cfg.Keys.Up + "/" + m.cfg.Keys.Down, "nav"},
		{m.cfg.Keys.Accept, "select"},
		{m.cfg.Keys.Cancel, "cancel"},
		{m.cfg.Keys.ModeNext, "mode"},
		{m.cfg.Keys.ToggleCWD, "cwd"},
		{m.cfg.Keys.ToggleDedupe, "dedup"},
		{m.cfg.Keys.Help, "help"},
	}

	parts := make([]string, 0, len(keys)+1)
	for _, k := range keys {
		key := m.styles.HelpKey.Render(k.key)
		desc := m.styles.HelpDesc.Render(k.desc)
		parts = append(parts, key+" "+desc)
	}

	if showPreviewHint {
		key := m.styles.HelpKey.Render(m.cfg.Keys.PreviewCommand)
		desc := m.styles.HelpDesc.Render("preview")
		parts = append(parts, key+" "+desc)
	}

	left := strings.Join(parts, "  ")
	m.footerCache = footerCache{
		showPreviewHint: showPreviewHint,
		left:            left,
		valid:           true,
	}

	return left
}

func formatMatchCountLabel(count int) string {
	return fmt.Sprintf("matches: %d", count)
}

func layoutFooterLine(left string, right string, width int) string {
	if width <= 0 {
		return ""
	}

	rightWidth := lipgloss.Width(right)
	if rightWidth >= width {
		return right
	}

	leftWidth := lipgloss.Width(left)
	if leftWidth+rightWidth <= width {
		return left + strings.Repeat(" ", width-leftWidth-rightWidth) + right
	}

	return strings.Repeat(" ", width-rightWidth) + right
}

func (m *Model) selectedIsMultiline() bool {
	if len(m.displayEntries) == 0 || m.cursor >= len(m.displayEntries) {
		return false
	}

	return strings.Contains(m.displayEntries[m.cursor].Entry.Command, "\n")
}

func (m *Model) renderPreviewPane() string {
	width := m.getWidth()

	if len(m.displayEntries) == 0 || m.cursor >= len(m.displayEntries) {
		emptyLine := strings.Repeat(" ", width)

		lines := make([]string, 0, previewPaneHeight)
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
		line = trimToWidth(line, width)

		if lineWidth := lipgloss.Width(line); lineWidth < width {
			line += strings.Repeat(" ", width-lineWidth)
		}

		displayLines = append(displayLines, m.styles.Dimmed.Render(line))
	}

	for len(displayLines) < contentHeight {
		displayLines = append(displayLines, strings.Repeat(" ", width))
	}

	return headerLine + "\n" + strings.Join(displayLines, "\n")
}

func (m *Model) renderHelp() string {
	width := m.getWidth()

	header := m.styles.Title.Render(" Keybindings ")

	bindings := []struct {
		key  string
		desc string
	}{
		{m.cfg.Keys.Up + "/" + m.cfg.Keys.Down, "Move up/down (also ctrl+p, ctrl+n, ctrl+r)"},
		{m.cfg.Keys.PageUp + "/" + m.cfg.Keys.PageDown, "Page up/down"},
		{m.cfg.Keys.Top + "/" + m.cfg.Keys.Bottom, "Jump to top/bottom"},
		{m.cfg.Keys.Accept, "Accept selection"},
		{m.cfg.Keys.Cancel, "Cancel / quit"},
		{m.cfg.Keys.ModeNext, "Cycle match mode (fuzzy/glob/regex)"},
		{m.cfg.Keys.ModeFuzzy, "Fuzzy match mode"},
		{m.cfg.Keys.ModeGlob, "Glob match mode"},
		{m.cfg.Keys.ModeRegex, "Regex match mode"},
		{m.cfg.Keys.ToggleCWD, "Filter to current directory"},
		{m.cfg.Keys.ToggleDedupe, "Toggle command deduplication"},
		{m.cfg.Keys.ToggleFails, "Cycle fail filter (include/exclude/only)"},
		{m.cfg.Keys.PreviewCommand, "Preview multiline command"},
		{m.cfg.Keys.Help, "Show/hide this help"},
	}

	lines := make([]string, 0, len(bindings))
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

func (m *Model) renderPreviewPopup() string {
	width := m.getWidth()

	header := m.styles.Title.Render(" Command Preview ")

	contentWidth := width - 8
	if contentWidth < 20 {
		contentWidth = width - 4
	}

	lines := strings.Split(m.previewCommand, "\n")
	wrappedLines := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.ReplaceAll(line, "\t", "    ")
		wrappedLines = append(wrappedLines, wrapToWidth(line, contentWidth)...)
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

func (m *Model) getWidth() int {
	return m.width
}

func (m *Model) visibleResultRange() (int, int) {
	count := len(m.displayEntries)
	if count == 0 {
		return 0, 0
	}

	maxVisible := min(m.pageSize(), count)

	if maxVisible == 0 {
		return 0, 0
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

	return start, end
}

func (m *Model) renderResultsHeader() string {
	return m.renderResultsHeaderWithLayout(m.calcResultLayout())
}

func (m *Model) renderResultsHeaderWithLayout(layout resultLayout) string {
	key := resultsHeaderCacheKey{
		width:      layout.width,
		cmdWidth:   layout.cmdWidth,
		dirWidth:   layout.dirWidth,
		timeWidth:  layout.timeWidth,
		showDir:    layout.showDir,
		barChar:    layout.barChar,
		prefixSize: layout.prefixWidth,
	}
	if m.headerCache.valid && m.headerCache.key == key {
		return m.headerCache.value
	}

	exit := m.styles.ColumnHeader.Width(layout.exitWidth).Align(lipgloss.Right).Render("exit")
	dur := m.styles.ColumnHeader.Width(layout.durWidth).Align(lipgloss.Right).Render("time")
	when := m.styles.ColumnHeader.Width(layout.timeWidth).Align(lipgloss.Right).Render("when")
	cmd := m.styles.ColumnHeader.Width(layout.cmdWidth).Render("command")

	prefix := strings.Repeat(" ", layout.prefixWidth)

	var line string

	if layout.showDir {
		dir := m.styles.ColumnHeader.Width(layout.dirWidth).Align(lipgloss.Right).Render("dir")
		line = prefix + strings.Join([]string{exit, dur, when, cmd, dir}, layout.sep)
	} else {
		line = prefix + strings.Join([]string{exit, dur, when, cmd}, layout.sep)
	}

	if lipgloss.Width(line) < layout.width {
		line += strings.Repeat(" ", layout.width-lipgloss.Width(line))
	}

	value := m.styles.ColumnHeaderBar.Width(layout.width).Render(line)
	m.headerCache = cachedResultsHeader{
		key:   key,
		value: value,
		valid: true,
	}

	return value
}

func (m *Model) fitIndicators(indicators []string, width int) string {
	if len(indicators) == 0 {
		return ""
	}

	best := strings.Join(indicators, " ")
	if lipgloss.Width(best) <= width {
		return best
	}

	for i := range slices.Backward(indicators) {
		candidate := strings.Join(indicators[:i], " ")
		if lipgloss.Width(candidate) <= width {
			return candidate
		}
	}

	return ""
}

func (m *Model) highlightMatches(text string, ranges []match.Range, baseStyle lipgloss.Style, matchStyle lipgloss.Style) string {
	if len(ranges) == 0 {
		return baseStyle.Render(text)
	}

	if isASCII(text) {
		return highlightASCIIMatches(text, ranges, baseStyle, matchStyle)
	}

	runes := []rune(text)

	var b strings.Builder

	pos := 0

	for _, r := range ranges {
		start := min(max(r.Start, 0), len(runes))

		end := min(max(r.End, start), len(runes))
		if start > pos {
			b.WriteString(baseStyle.Render(string(runes[pos:start])))
		}

		start = max(start, pos)
		if end > start {
			b.WriteString(matchStyle.Render(string(runes[start:end])))
			pos = end
		}
	}

	if pos < len(runes) {
		b.WriteString(baseStyle.Render(string(runes[pos:])))
	}

	return b.String()
}

func highlightASCIIMatches(text string, ranges []match.Range, baseStyle lipgloss.Style, matchStyle lipgloss.Style) string {
	var b strings.Builder

	pos := 0

	for _, r := range ranges {
		start := min(max(r.Start, 0), len(text))

		end := min(max(r.End, start), len(text))
		if start > pos {
			b.WriteString(baseStyle.Render(text[pos:start]))
		}

		start = max(start, pos)
		if end > start {
			b.WriteString(matchStyle.Render(text[start:end]))
			pos = end
		}
	}

	if pos < len(text) {
		b.WriteString(baseStyle.Render(text[pos:]))
	}

	return b.String()
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}

	return true
}

func (m *Model) cachedRenderMatchInfo(entryIdx int, text string) *match.Match {
	info := &m.displayEntries[entryIdx].MatchInfo
	if len(info.MatchedRanges) > 0 || m.mode != match.ModeFuzzy || strings.ContainsAny(text, "\n\r\t") {
		return info
	}

	ranges := fuzzyRenderRanges(m.input.Value(), text)
	if len(ranges) == 0 {
		return info
	}

	m.displayEntries[entryIdx].MatchInfo.MatchedRanges = ranges

	return &m.displayEntries[entryIdx].MatchInfo
}

func (m *Model) ensureRenderMatchRanges(text string, info *match.Match) *match.Match {
	if info == nil || len(info.MatchedRanges) > 0 {
		return info
	}

	var ranges []match.Range

	switch m.mode {
	case match.ModeFuzzy:
		ranges = fuzzyRenderRanges(m.input.Value(), text)
	case match.ModeRegex:
		ranges = m.regexRenderRanges(text)
	case match.ModeGlob:
		return info
	default:
		return info
	}

	if len(ranges) == 0 {
		return info
	}

	infoCopy := *info
	infoCopy.MatchedRanges = ranges

	return &infoCopy
}

func (m *Model) regexRenderRanges(text string) []match.Range {
	pattern := m.input.Value()
	if pattern == "" || text == "" {
		return nil
	}

	if match.IsLiteralRegex(pattern) {
		if ranges, ok := literalFoldRanges(pattern, text); ok {
			return ranges
		}
	}

	re := m.regexCache.re
	if !m.regexCache.valid || m.regexCache.pattern != pattern {
		compiled, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil
		}

		m.regexCache = regexRenderCache{
			pattern: pattern,
			re:      compiled,
			valid:   true,
		}
		re = compiled
	}

	return regexRanges(re, text)
}

func fuzzyRenderRanges(pattern string, text string) []match.Range {
	if pattern == "" || text == "" {
		return nil
	}

	patternRunes := []rune(pattern)
	patternIndex := 0
	rangeStart := -1
	previousMatch := -2
	ranges := make([]match.Range, 0, len(patternRunes))

	runeIndex := 0

	for _, r := range text {
		if equalFoldRune(r, patternRunes[patternIndex]) {
			if rangeStart < 0 {
				rangeStart = runeIndex
			} else if runeIndex != previousMatch+1 {
				ranges = append(ranges, match.Range{Start: rangeStart, End: previousMatch + 1})
				rangeStart = runeIndex
			}

			previousMatch = runeIndex

			patternIndex++
			if patternIndex == len(patternRunes) {
				return append(ranges, match.Range{Start: rangeStart, End: previousMatch + 1})
			}
		}

		runeIndex++
	}

	return nil
}

func regexRenderRanges(pattern string, text string) []match.Range {
	if pattern == "" || text == "" {
		return nil
	}

	if match.IsLiteralRegex(pattern) {
		if ranges, ok := literalFoldRanges(pattern, text); ok {
			return ranges
		}
	}

	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil
	}

	return regexRanges(re, text)
}

func literalFoldRanges(pattern string, text string) ([]match.Range, bool) {
	if !isASCII(pattern) || !isASCII(text) {
		return nil, false
	}

	lowerPattern := strings.ToLower(pattern)

	var ranges []match.Range

	for start := 0; start <= len(text)-len(lowerPattern); {
		idx := indexFoldASCII(text[start:], lowerPattern)
		if idx < 0 {
			break
		}

		matchStart := start + idx
		matchEnd := matchStart + len(lowerPattern)
		ranges = append(ranges, match.Range{Start: matchStart, End: matchEnd})
		start = matchEnd
	}

	return ranges, true
}

func indexFoldASCII(text string, lowerPattern string) int {
	if lowerPattern == "" {
		return 0
	}

	first := lowerPattern[0]

	limit := len(text) - len(lowerPattern)
	for i := 0; i <= limit; i++ {
		if toLowerASCII(text[i]) != first {
			continue
		}

		matched := true

		for j := 1; j < len(lowerPattern); j++ {
			if toLowerASCII(text[i+j]) != lowerPattern[j] {
				matched = false
				break
			}
		}

		if matched {
			return i
		}
	}

	return -1
}

func toLowerASCII(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}

	return b
}

func regexRanges(re *regexp.Regexp, text string) []match.Range {
	locs := re.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return nil
	}

	ranges := make([]match.Range, len(locs))
	if isASCII(text) {
		for i, loc := range locs {
			ranges[i] = match.Range{Start: loc[0], End: loc[1]}
		}

		return ranges
	}

	runeStarts := buildRuneByteOffsets(text)
	for i, loc := range locs {
		ranges[i] = match.Range{
			Start: byteOffsetToRuneIndex(runeStarts, loc[0]),
			End:   byteOffsetToRuneIndex(runeStarts, loc[1]),
		}
	}

	return ranges
}

func buildRuneByteOffsets(s string) []int {
	offsets := make([]int, 0, len([]rune(s))+1)
	for i := range s {
		offsets = append(offsets, i)
	}

	offsets = append(offsets, len(s))

	return offsets
}

func byteOffsetToRuneIndex(offsets []int, byteOffset int) int {
	return sort.SearchInts(offsets, byteOffset)
}

func equalFoldRune(a rune, b rune) bool {
	if a == b {
		return true
	}

	if a < b {
		a, b = b, a
	}

	if a < utf8.RuneSelf {
		return 'A' <= b && b <= 'Z' && a == b+'a'-'A'
	}

	r := unicode.SimpleFold(b)
	for r != b && r < a {
		r = unicode.SimpleFold(r)
	}

	return r == a
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
	if isHomeDirectoryPath(dir, home) {
		dir = "~" + dir[len(home):]
	}

	if width <= 0 {
		return ""
	}

	if lipgloss.Width(dir) <= width {
		return dir
	}

	runes := []rune(dir)

	remainingWidth := width - lipgloss.Width("…")
	if remainingWidth <= 0 {
		return "…"
	}

	start := len(runes)
	usedWidth := 0

	for start > 0 {
		runeWidth := lipgloss.Width(string(runes[start-1]))
		if usedWidth+runeWidth > remainingWidth {
			break
		}

		usedWidth += runeWidth
		start--
	}

	return "…" + string(runes[start:])
}

func isHomeDirectoryPath(dir string, home string) bool {
	if home == "" || !strings.HasPrefix(dir, home) {
		return false
	}

	if len(dir) == len(home) {
		return true
	}

	switch dir[len(home)] {
	case '/', '\\':
		return true
	default:
		return false
	}
}

func formatExit(code int, width int) string {
	return padLeftASCII(strconv.Itoa(code), width)
}

func formatDuration(ms int64, mode string, width int) string {
	var s string

	switch mode {
	case "ms":
		s = strconv.FormatInt(ms, 10) + "ms"
	case "s":
		s = strconv.FormatFloat(float64(ms)/1000.0, 'f', 2, 64) + "s"
	default:
		s = humanDuration(ms)
	}

	if len(s) > width {
		s = trimToWidth(s, width)
	}

	return padLeftASCII(s, width)
}

func formatWhenAt(tsMs int64, mode string, width int, now time.Time) string {
	tsMs = normalizeTimestampMs(tsMs)
	if tsMs <= 0 {
		return padLeftASCII("n/a", width)
	}

	t := time.UnixMilli(tsMs)

	var s string

	switch mode {
	case "absolute":
		s = t.Format("2006-01-02 15:04")
	default:
		s = humanSince(safeSub(now, t))
	}

	if len(s) > width {
		s = trimToWidth(s, width)
	}

	return padLeftASCII(s, width)
}

func humanDuration(ms int64) string {
	if ms < 1000 {
		return strconv.FormatInt(ms, 10) + "ms"
	}

	sec := float64(ms) / 1000.0
	if sec < 60 {
		return strconv.FormatFloat(sec, 'f', 1, 64) + "s"
	}

	minutes := sec / 60.0
	if minutes < 60 {
		return strconv.FormatFloat(minutes, 'f', 1, 64) + "m"
	}

	h := minutes / 60.0

	return strconv.FormatFloat(h, 'f', 1, 64) + "h"
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
		return strconv.Itoa(int(d.Seconds())) + "s ago"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	case d < 7*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	case d < 30*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/(24*7))) + "w ago"
	case d < 365*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/(24*30))) + "mo ago"
	default:
		return strconv.Itoa(int(d.Hours()/(24*365))) + "y ago"
	}
}

func padLeftASCII(s string, width int) string {
	padding := width - len(s)
	if padding <= 0 {
		return s
	}

	return strings.Repeat(" ", padding) + s
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

	if lipgloss.Width(s) <= width {
		return s
	}

	runes := []rune(s)
	end := prefixRuneCountForWidth(runes, width)

	return string(runes[:end])
}

func wrapToWidth(s string, width int) []string {
	if width <= 0 {
		return nil
	}

	runes := []rune(s)
	if len(runes) == 0 {
		return []string{""}
	}

	lines := make([]string, 0, (lipgloss.Width(s)+width-1)/width)
	for len(runes) > 0 {
		end := prefixRuneCountForWidth(runes, width)
		if end == 0 {
			end = 1
		}

		lines = append(lines, string(runes[:end]))
		runes = runes[end:]
	}

	return lines
}

func truncateWithRanges(text string, info *match.Match, maxLen int) (string, *match.Match) {
	if maxLen <= 0 || lipgloss.Width(text) <= maxLen {
		return text, info
	}

	runes := []rune(text)

	if info == nil || len(info.MatchedRanges) == 0 {
		return truncatePrefixRunes(runes, maxLen), info
	}

	return truncateAroundMatch(runes, info, maxLen)
}

func truncateAroundMatch(runes []rune, info *match.Match, maxLen int) (string, *match.Match) {
	const ellipsis = "..."

	ellipsisWidth := lipgloss.Width(ellipsis)
	if maxLen <= ellipsisWidth*2 {
		truncated := truncatePrefixRunes(runes, maxLen)

		infoCopy := *info
		infoCopy.MatchedRanges = nil

		return truncated, &infoCopy
	}

	firstMatchStart := len(runes)
	for _, r := range info.MatchedRanges {
		if r.Start < firstMatchStart {
			firstMatchStart = r.Start
		}
	}

	if firstMatchStart == len(runes) {
		truncated := truncatePrefixRunes(runes, maxLen)

		infoCopy := *info
		infoCopy.MatchedRanges = nil

		return truncated, &infoCopy
	}

	prefixWidth := 0
	suffixWidth := ellipsisWidth
	windowWidth := maxLen - suffixWidth
	start := 0

	if lipgloss.Width(string(runes[:firstMatchStart])) >= windowWidth {
		prefixWidth = ellipsisWidth
		windowWidth = maxLen - prefixWidth - suffixWidth
		start = suffixStartForWidth(runes[:firstMatchStart], windowWidth/3)
	}

	end := start + prefixRuneCountForWidth(runes[start:], windowWidth)
	if end == len(runes) {
		suffixWidth = 0
		windowWidth = maxLen - prefixWidth
		start = suffixStartForWidth(runes[:end], windowWidth)
	}

	truncatedRunes := make([]rune, 0, maxLen)
	if prefixWidth > 0 {
		truncatedRunes = append(truncatedRunes, []rune(ellipsis)...)
	}

	truncatedRunes = append(truncatedRunes, runes[start:end]...)
	if suffixWidth > 0 {
		truncatedRunes = append(truncatedRunes, []rune(ellipsis)...)
	}

	var ranges []match.Range

	for _, r := range info.MatchedRanges {
		if r.End <= start || r.Start >= end {
			continue
		}

		rangeStart := max(r.Start, start) - start + prefixWidth
		rangeEnd := min(r.End, end) - start + prefixWidth

		if rangeEnd > rangeStart {
			ranges = append(ranges, match.Range{Start: rangeStart, End: rangeEnd})
		}
	}

	infoCopy := *info
	infoCopy.MatchedRanges = ranges

	return string(truncatedRunes), &infoCopy
}

func prefixRuneCountForWidth(runes []rune, width int) int {
	if width <= 0 {
		return 0
	}

	for i := range runes {
		if lipgloss.Width(string(runes[:i+1])) > width {
			return i
		}
	}

	return len(runes)
}

func suffixStartForWidth(runes []rune, width int) int {
	if width <= 0 {
		return len(runes)
	}

	for i := range slices.Backward(runes) {
		if lipgloss.Width(string(runes[i:])) > width {
			return i + 1
		}
	}

	return 0
}

func truncatePrefixRunes(runes []rune, maxWidth int) string {
	const ellipsis = "..."

	if maxWidth <= 0 {
		return ""
	}

	tail := trimToWidth(ellipsis, maxWidth)

	availableWidth := maxWidth - lipgloss.Width(tail)
	if availableWidth <= 0 {
		return tail
	}

	end := prefixRuneCountForWidth(runes, availableWidth)

	return string(runes[:end]) + tail
}

func collapseRunes(textRunes []rune, symbolRunes []rune) ([]rune, []int) {
	collapsed := make([]rune, 0, len(textRunes))
	runeMap := make([]int, 0, len(textRunes))

	for i, r := range textRunes {
		switch r {
		case '\n', '\r':
			for _, sr := range symbolRunes {
				collapsed = append(collapsed, sr)
				runeMap = append(runeMap, i)
			}
		case '\t':
			for range 4 {
				collapsed = append(collapsed, ' ')
				runeMap = append(runeMap, i)
			}
		default:
			collapsed = append(collapsed, r)
			runeMap = append(runeMap, i)
		}
	}

	return collapsed, runeMap
}

func buildReverseMap(runeMap []int) map[int]int {
	reverseMap := make(map[int]int, len(runeMap))
	for newIdx, oldIdx := range runeMap {
		reverseMap[oldIdx] = newIdx
	}

	return reverseMap
}

func findMappedStart(reverseMap map[int]int, start int, textLen int) (int, bool) {
	if newStart, ok := reverseMap[start]; ok {
		return newStart, true
	}

	for i := start; i < textLen; i++ {
		if ns, ok := reverseMap[i]; ok {
			return ns, true
		}
	}

	return 0, false
}

func findMappedEnd(reverseMap map[int]int, start int, end int, textLen int, newStart int) int {
	newEnd := newStart

	for i := start; i < end && i < textLen; i++ {
		if ne, ok := reverseMap[i]; ok {
			newEnd = ne + 1
		}
	}

	return newEnd
}

func remapMatchRanges(ranges []match.Range, runeMap []int, textLen int) []match.Range {
	reverseMap := buildReverseMap(runeMap)

	var newRanges []match.Range

	for _, r := range ranges {
		newStart, ok := findMappedStart(reverseMap, r.Start, textLen)
		if !ok {
			continue
		}

		newEnd := findMappedEnd(reverseMap, r.Start, r.End, textLen, newStart)
		if newEnd > newStart {
			newRanges = append(newRanges, match.Range{Start: newStart, End: newEnd})
		}
	}

	return newRanges
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
	collapsed, runeMap := collapseRunes(textRunes, symbolRunes)

	if info == nil || len(info.MatchedRanges) == 0 {
		return string(collapsed), info
	}

	infoCopy := *info
	infoCopy.MatchedRanges = remapMatchRanges(info.MatchedRanges, runeMap, len(textRunes))

	return string(collapsed), &infoCopy
}
