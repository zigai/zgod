package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mouseInputBounds struct {
	x     int
	y     int
	width int
}

type footerShortcutAction int

const (
	footerShortcutNone footerShortcutAction = iota
	footerShortcutAccept
	footerShortcutCancel
	footerShortcutModeNext
	footerShortcutToggleCWD
	footerShortcutToggleDedupe
	footerShortcutHelp
	footerShortcutPreview
)

type footerShortcut struct {
	key    string
	desc   string
	action footerShortcutAction
}

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	ev := tea.MouseEvent(msg)

	if m.showPreview || m.showHelp {
		if ev.IsWheel() || ev.Action == tea.MouseActionPress {
			m.dismissTransientViews()
		}

		return m, nil
	}

	if ev.IsWheel() {
		m.handleMouseWheel(ev)

		return m, nil
	}

	if ev.Action == tea.MouseActionMotion {
		m.handleMouseHover(ev)

		return m, nil
	}

	if ev.Action != tea.MouseActionPress || ev.Button != tea.MouseButtonLeft {
		return m, nil
	}

	return m.handleMouseLeftPress(ev)
}

func (m *Model) handleMouseWheel(ev tea.MouseEvent) {
	if _, _, ok := m.mouseBodyPosition(ev); !ok {
		return
	}

	if ev.Button == tea.MouseButtonWheelUp {
		m.moveCursor(-1)
	}

	if ev.Button == tea.MouseButtonWheelDown {
		m.moveCursor(1)
	}
}

func (m *Model) handleMouseHover(ev tea.MouseEvent) {
	_, bodyY, ok := m.mouseBodyPosition(ev)
	if !ok {
		return
	}

	if resultIdx, resultOK := m.resultIndexAtBodyY(bodyY); resultOK {
		m.cursor = resultIdx
	}
}

func (m *Model) handleMouseLeftPress(ev tea.MouseEvent) (tea.Model, tea.Cmd) {
	bodyX, bodyY, ok := m.mouseBodyPosition(ev)
	if !ok {
		return m, nil
	}

	if m.handleMouseInputClick(bodyX, bodyY) {
		return m, nil
	}

	if resultIdx, resultOK := m.resultIndexAtBodyY(bodyY); resultOK {
		m.cursor = resultIdx
		if !m.acceptCurrentSelection() {
			return m, nil
		}

		m.quitting = true

		return m, tea.Quit
	}

	if action, actionOK := m.footerShortcutAt(bodyX, bodyY); actionOK {
		return m, m.triggerFooterShortcut(action)
	}

	return m, nil
}

func (m *Model) moveCursor(delta int) {
	if len(m.displayEntries) == 0 {
		return
	}

	m.cursor = min(max(m.cursor+delta, 0), len(m.displayEntries)-1)
}

func (m *Model) mouseBodyPosition(ev tea.MouseEvent) (int, int, bool) {
	viewY := ev.Y - m.viewOriginY()
	bodyY := viewY - 1 - panelPaddingY
	bodyX := ev.X - 1 - panelPaddingX

	if bodyX < 0 || bodyX >= m.width || bodyY < 0 || bodyY >= m.bodyHeight() {
		return 0, 0, false
	}

	return bodyX, bodyY, true
}

func (m *Model) viewOriginY() int {
	if m.terminalHeight <= 0 {
		return 0
	}

	return max(m.terminalHeight-m.viewHeight(), 0)
}

func (m *Model) viewHeight() int {
	return m.bodyHeight() + panelBorderH + (panelPaddingY * 2)
}

func (m *Model) bodyHeight() int {
	return m.inputRows() + m.height + m.previewPaneRows() + m.footerRows()
}

func (m *Model) inputRows() int {
	if m.isMerged() {
		return 1
	}

	return 2
}

func (m *Model) previewPaneRows() int {
	if m.cfg.Display.MultilinePreview == "preview_pane" {
		return previewPaneHeight
	}

	return 0
}

func (m *Model) footerRows() int {
	if m.cfg.Display.ShowHints {
		return 1
	}

	return 0
}

func (m *Model) resultIndexAtBodyY(bodyY int) (int, bool) {
	resultY := bodyY - m.inputRows()
	if resultY < 0 || resultY >= m.height {
		return 0, false
	}

	headerRows := resultsHeaderRows
	if m.height <= resultsHeaderRows {
		headerRows = 0
	}

	if resultY < headerRows {
		return 0, false
	}

	start, end := m.visibleResultRange()
	if start == end {
		return 0, false
	}

	row := headerRows
	expandMode := m.cfg.Display.MultilinePreview == "expand"

	for idx := start; idx < end && row < m.height; idx++ {
		rowCount := 1
		if expandMode && idx == m.cursor && m.entryIsMultiline(idx) {
			rowCount = m.expandedResultRowCount(idx, m.height-row)
		}

		if resultY >= row && resultY < row+rowCount {
			return idx, true
		}

		row += rowCount
	}

	return 0, false
}

func (m *Model) expandedResultRowCount(idx int, remaining int) int {
	if idx < 0 || idx >= len(m.displayEntries) || remaining <= 0 {
		return 0
	}

	rows := strings.Count(m.displayEntries[idx].Entry.Command, "\n") + 1

	return min(max(rows, 1), remaining)
}

func (m *Model) handleMouseInputClick(bodyX int, bodyY int) bool {
	bounds, ok := m.mouseInputBounds()
	if !ok || bodyY != bounds.y || bodyX < bounds.x || bodyX >= bounds.x+bounds.width {
		return false
	}

	m.input.SetCursor(m.inputCursorPositionAtCell(bodyX-bounds.x, bounds.width))

	return true
}

func (m *Model) mouseInputBounds() (mouseInputBounds, bool) {
	promptWidth := lipgloss.Width(m.styles.Prompt.Render(m.cfg.Theme.Prompt))

	if m.isMerged() {
		indicatorWidth := lipgloss.Width(m.renderIndicators())
		width := m.width - promptWidth - indicatorWidth - 2

		return mouseInputBounds{x: promptWidth, y: 0, width: max(width, 0)}, width > 0
	}

	width := min(m.input.Width, max(m.width-promptWidth, 0))

	return mouseInputBounds{x: promptWidth, y: 1, width: width}, width > 0
}

func (m *Model) inputCursorPositionAtCell(cell int, width int) int {
	value := []rune(m.input.Value())
	if len(value) == 0 || cell <= 0 {
		return m.inputVisibleStart(width)
	}

	start := m.inputVisibleStart(width)
	pos := start
	cells := 0

	for pos < len(value) {
		runeWidth := lipgloss.Width(string(value[pos]))
		next := cells + runeWidth

		if cell < next {
			return pos
		}

		if cell == next {
			return pos + 1
		}

		cells = next
		pos++

		if width > 0 && cells >= width {
			break
		}
	}

	return pos
}

func (m *Model) inputVisibleStart(width int) int {
	value := []rune(m.input.Value())
	if width <= 0 || lipgloss.Width(string(value)) <= width {
		return 0
	}

	pos := min(m.input.Position(), len(value))
	if pos <= 0 {
		return 0
	}

	start := pos
	cells := 0

	for start > 0 {
		runeWidth := lipgloss.Width(string(value[start-1]))
		if cells+runeWidth >= width {
			break
		}

		cells += runeWidth
		start--
	}

	return start
}

func (m *Model) footerShortcuts() []footerShortcut {
	shortcuts := []footerShortcut{
		{key: m.cfg.Keys.Up + "/" + m.cfg.Keys.Down, desc: "nav", action: footerShortcutNone},
		{key: m.cfg.Keys.Accept, desc: "select", action: footerShortcutAccept},
		{key: m.cfg.Keys.Cancel, desc: "cancel", action: footerShortcutCancel},
		{key: m.cfg.Keys.ModeNext, desc: "mode", action: footerShortcutModeNext},
		{key: m.cfg.Keys.ToggleCWD, desc: "cwd", action: footerShortcutToggleCWD},
		{key: m.cfg.Keys.ToggleDedupe, desc: "dedup", action: footerShortcutToggleDedupe},
		{key: m.cfg.Keys.Help, desc: "help", action: footerShortcutHelp},
	}

	if m.cfg.Display.MultilinePreview == "popup" && m.selectedIsMultiline() {
		shortcuts = append(shortcuts, footerShortcut{
			key:    m.cfg.Keys.PreviewCommand,
			desc:   "preview",
			action: footerShortcutPreview,
		})
	}

	return shortcuts
}

func (m *Model) footerShortcutAt(bodyX int, bodyY int) (footerShortcutAction, bool) {
	if bodyY != m.footerBodyY() {
		return footerShortcutNone, false
	}

	contentWidth := max(m.width-lipgloss.Width(m.styles.Footer.Render("")), 0)
	rightWidth := lipgloss.Width(m.styles.HelpDesc.Render(m.matchCountLabel()))
	leftWidth := m.footerShortcutLineWidth()

	if leftWidth+rightWidth > contentWidth {
		return footerShortcutNone, false
	}

	contentX := bodyX - 1
	if contentX < 0 || contentX >= contentWidth {
		return footerShortcutNone, false
	}

	x := 0

	for _, shortcut := range m.footerShortcuts() {
		partWidth := m.footerShortcutWidth(shortcut)
		if contentX >= x && contentX < x+partWidth {
			return shortcut.action, shortcut.action != footerShortcutNone
		}

		x += partWidth + 2
	}

	return footerShortcutNone, false
}

func (m *Model) footerBodyY() int {
	if !m.cfg.Display.ShowHints {
		return -1
	}

	return m.inputRows() + m.height + m.previewPaneRows()
}

func (m *Model) footerShortcutLineWidth() int {
	shortcuts := m.footerShortcuts()
	if len(shortcuts) == 0 {
		return 0
	}

	width := 0

	for i, shortcut := range shortcuts {
		if i > 0 {
			width += 2
		}

		width += m.footerShortcutWidth(shortcut)
	}

	return width
}

func (m *Model) footerShortcutWidth(shortcut footerShortcut) int {
	return lipgloss.Width(m.styles.HelpKey.Render(shortcut.key)) +
		1 +
		lipgloss.Width(m.styles.HelpDesc.Render(shortcut.desc))
}

func (m *Model) triggerFooterShortcut(action footerShortcutAction) tea.Cmd {
	switch action {
	case footerShortcutNone:
		return nil
	case footerShortcutAccept:
		if !m.acceptCurrentSelection() {
			return nil
		}

		m.quitting = true

		return tea.Quit
	case footerShortcutCancel:
		m.quitting = true
		m.canceled = true

		return tea.Quit
	case footerShortcutModeNext:
		m.mode = m.mode.Next(m.enabledModes)
		m.updateMatches()

		return nil
	case footerShortcutToggleCWD:
		m.cwdMode = !m.cwdMode

		return m.startLoadingEntries()
	case footerShortcutToggleDedupe:
		m.dedupe = !m.dedupe

		return m.startLoadingEntries()
	case footerShortcutHelp:
		m.showHelp = true

		return nil
	case footerShortcutPreview:
		m.showCurrentPreview()

		return nil
	default:
		return nil
	}
}

func (m *Model) showCurrentPreview() {
	if m.cfg.Display.MultilinePreview != "popup" {
		return
	}

	cmd, ok := m.currentResultCommand()
	if !ok || !strings.Contains(cmd, "\n") {
		return
	}

	m.showPreview = true
	m.previewCommand = cmd
}
