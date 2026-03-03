package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
)

func TestHandleKeyAcceptUsesSelectedEntryWhenAvailable(t *testing.T) {
	t.Parallel()

	ti := textinput.New()
	ti.SetValue("typed command")

	cfg := config.Default()
	m := &Model{
		input: ti,
		cfg:   cfg,
		displayEntries: []history.ScoredEntry{
			{Entry: db.HistoryEntry{Command: "selected from history"}},
		},
	}

	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if got, want := m.Selected(), "selected from history"; got != want {
		t.Fatalf("Selected() = %q, want %q", got, want)
	}

	if !m.quitting {
		t.Fatal("quitting = false, want true")
	}
}

func TestHandleKeyAcceptFallsBackToTypedCommandWhenNoMatches(t *testing.T) {
	t.Parallel()

	ti := textinput.New()
	ti.SetValue("typed command")

	cfg := config.Default()
	m := &Model{
		input: ti,
		cfg:   cfg,
	}

	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if got, want := m.Selected(), "typed command"; got != want {
		t.Fatalf("Selected() = %q, want %q", got, want)
	}

	if !m.quitting {
		t.Fatal("quitting = false, want true")
	}
}

func TestHandleNavigationPageDownMovesByVisiblePage(t *testing.T) {
	t.Parallel()

	m := testNavModel(30, 8)

	handled := m.handleNavigation(tea.KeyMsg{Type: tea.KeyPgDown})
	if !handled {
		t.Fatal("handleNavigation(pgdown) = false, want true")
	}

	if got, want := m.cursor, 7; got != want {
		t.Fatalf("cursor after pgdown = %d, want %d", got, want)
	}
}

func TestHandleNavigationPageUpMovesByVisiblePage(t *testing.T) {
	t.Parallel()

	m := testNavModel(30, 8)
	m.cursor = 14

	handled := m.handleNavigation(tea.KeyMsg{Type: tea.KeyPgUp})
	if !handled {
		t.Fatal("handleNavigation(pgup) = false, want true")
	}

	if got, want := m.cursor, 7; got != want {
		t.Fatalf("cursor after pgup = %d, want %d", got, want)
	}
}

func TestHandleNavigationPageDownClampsAtBottom(t *testing.T) {
	t.Parallel()

	m := testNavModel(30, 8)
	m.cursor = 27

	handled := m.handleNavigation(tea.KeyMsg{Type: tea.KeyPgDown})
	if !handled {
		t.Fatal("handleNavigation(pgdown) = false, want true")
	}

	if got, want := m.cursor, 29; got != want {
		t.Fatalf("cursor after pgdown clamp = %d, want %d", got, want)
	}
}

func TestHandleNavigationPageUpClampsAtTop(t *testing.T) {
	t.Parallel()

	m := testNavModel(30, 8)
	m.cursor = 3

	handled := m.handleNavigation(tea.KeyMsg{Type: tea.KeyPgUp})
	if !handled {
		t.Fatal("handleNavigation(pgup) = false, want true")
	}

	if got, want := m.cursor, 0; got != want {
		t.Fatalf("cursor after pgup clamp = %d, want %d", got, want)
	}
}

func TestHandleNavigationPageDownUsesSingleStepAtMinimumHeight(t *testing.T) {
	t.Parallel()

	m := testNavModel(10, 1)
	m.cursor = 2

	handled := m.handleNavigation(tea.KeyMsg{Type: tea.KeyPgDown})
	if !handled {
		t.Fatal("handleNavigation(pgdown) = false, want true")
	}

	if got, want := m.cursor, 3; got != want {
		t.Fatalf("cursor after pgdown at height=1 = %d, want %d", got, want)
	}
}

func testNavModel(entryCount int, height int) *Model {
	return &Model{
		cfg:            config.Default(),
		height:         height,
		displayEntries: make([]history.ScoredEntry, entryCount),
	}
}
