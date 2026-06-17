package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
	"github.com/zigai/zgod/internal/match"
)

func TestHandleMouseWheelMovesCursor(t *testing.T) {
	t.Parallel()

	m := testMouseModel(4, 5)
	m.cursor = 1
	x, y := testMouseBodyCell(m, 2, testMouseFirstResultBodyY(m))

	_, _ = m.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
	})

	if got, want := m.cursor, 0; got != want {
		t.Fatalf("cursor after wheel up = %d, want %d", got, want)
	}

	_, _ = m.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	if got, want := m.cursor, 1; got != want {
		t.Fatalf("cursor after wheel down = %d, want %d", got, want)
	}
}

func TestHandleMouseClickResultSelectsAndQuits(t *testing.T) {
	t.Parallel()

	m := testMouseModel(4, 5)
	x, y := testMouseBodyCell(m, 2, testMouseFirstResultBodyY(m)+1)

	_, cmd := m.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if got, want := m.cursor, 1; got != want {
		t.Fatalf("cursor after result click = %d, want %d", got, want)
	}

	if got, want := m.Selected(), "command 1"; got != want {
		t.Fatalf("Selected() after result click = %q, want %q", got, want)
	}

	if !m.quitting {
		t.Fatal("quitting after result click = false, want true")
	}

	if cmd == nil {
		t.Fatal("mouse result click returned nil command, want tea.Quit")
	}

	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("mouse result click command = %T, want tea.QuitMsg", msg)
	}
}

func TestHandleMouseHoverSelectsResult(t *testing.T) {
	t.Parallel()

	m := testMouseModel(5, 6)
	x, y := testMouseBodyCell(m, 2, testMouseFirstResultBodyY(m)+2)

	_, _ = m.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonNone,
		Action: tea.MouseActionMotion,
	})

	if got, want := m.cursor, 2; got != want {
		t.Fatalf("cursor after result hover = %d, want %d", got, want)
	}

	if m.Selected() != "" {
		t.Fatalf("Selected() after result hover = %q, want empty", m.Selected())
	}
}

func TestHandleMouseFooterShortcutCyclesMode(t *testing.T) {
	t.Parallel()

	m := testMouseModel(2, 4)
	m.width = 200
	m.input.Width = max(m.width-4, 1)
	m.terminalHeight = m.viewHeight() + 3
	x, y := testMouseBodyCell(m, testMouseFooterShortcutBodyX(t, m, footerShortcutModeNext), m.footerBodyY())

	_, cmd := m.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if cmd != nil {
		t.Fatalf("mode shortcut returned command %T, want nil", cmd)
	}

	if got, want := m.mode, match.ModeGlob; got != want {
		t.Fatalf("mode after footer shortcut click = %v, want %v", got, want)
	}
}

func TestHandleMouseInputClickMovesCursor(t *testing.T) {
	t.Parallel()

	m := testMouseModel(0, 4)
	m.input.SetValue("abcdef")
	m.input.SetCursor(0)

	bounds, ok := m.mouseInputBounds()
	if !ok {
		t.Fatal("mouseInputBounds() = false, want true")
	}

	x, y := testMouseBodyCell(m, bounds.x+3, bounds.y)
	_, cmd := m.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if cmd != nil {
		t.Fatalf("input click returned command %T, want nil", cmd)
	}

	if got, want := m.input.Position(), 3; got != want {
		t.Fatalf("input cursor after click = %d, want %d", got, want)
	}
}

func testMouseModel(entryCount int, height int) *Model {
	cfg := config.Default()
	m := NewModel(cfg, nil, "", "", height, false, "")
	m.loadingHistory = false
	m.historyComplete = true
	m.terminalHeight = m.viewHeight() + 3

	m.displayEntries = make([]history.ScoredEntry, entryCount)
	for i := range entryCount {
		m.displayEntries[i] = history.ScoredEntry{
			Entry: db.HistoryEntry{
				ID:      int64(i + 1),
				Command: fmt.Sprintf("command %d", i),
			},
		}
	}

	return m
}

func testMouseBodyCell(m *Model, bodyX int, bodyY int) (int, int) {
	return 1 + panelPaddingX + bodyX, m.viewOriginY() + 1 + panelPaddingY + bodyY
}

func testMouseFirstResultBodyY(m *Model) int {
	headerRows := resultsHeaderRows
	if m.height <= resultsHeaderRows {
		headerRows = 0
	}

	return m.inputRows() + headerRows
}

func testMouseFooterShortcutBodyX(t *testing.T, m *Model, action footerShortcutAction) int {
	t.Helper()

	x := 1
	for _, shortcut := range m.footerShortcuts() {
		if shortcut.action == action {
			return x
		}

		x += m.footerShortcutWidth(shortcut) + 2
	}

	t.Fatalf("footer shortcut action %v not found", action)

	return 0
}
