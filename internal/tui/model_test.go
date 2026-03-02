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
