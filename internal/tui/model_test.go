package tui

import (
	"path/filepath"
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

func TestHandleToggleFailsCyclesFailFilterModesAndReloadsEntries(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := db.NewHistoryRepo(database)
	entries := []db.HistoryEntry{
		{TsMs: 1000, ExitCode: 0, Command: "echo ok one"},
		{TsMs: 2000, ExitCode: 1, Command: "echo fail"},
		{TsMs: 3000, ExitCode: 0, Command: "echo ok two"},
	}

	for _, entry := range entries {
		if _, err = repo.Insert(entry); err != nil {
			t.Fatalf("repo.Insert(%q) error: %v", entry.Command, err)
		}
	}

	cfg := config.Default()
	m := NewModel(cfg, repo, "", "", 10, false, "")

	if got, want := m.failFilter, db.FailFilterInclude; got != want {
		t.Fatalf("initial failFilter = %v, want %v", got, want)
	}

	if got, want := len(m.allEntries), 3; got != want {
		t.Fatalf("initial len(allEntries) = %d, want %d", got, want)
	}

	tests := []struct {
		name       string
		wantMode   db.FailFilterMode
		wantLength int
	}{
		{name: "exclude", wantMode: db.FailFilterExclude, wantLength: 2},
		{name: "only", wantMode: db.FailFilterOnly, wantLength: 1},
		{name: "include", wantMode: db.FailFilterInclude, wantLength: 3},
	}

	for _, tc := range tests {
		handled := m.handleToggle(tea.KeyMsg{Type: tea.KeyCtrlF})
		if !handled {
			t.Fatalf("handleToggle(%s) = false, want true", tc.name)
		}

		if got := m.failFilter; got != tc.wantMode {
			t.Fatalf("failFilter after %s = %v, want %v", tc.name, got, tc.wantMode)
		}

		if got := len(m.allEntries); got != tc.wantLength {
			t.Fatalf("len(allEntries) after %s = %d, want %d", tc.name, got, tc.wantLength)
		}
	}
}

func TestNewModelUsesConfiguredDefaultFailFilter(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := db.NewHistoryRepo(database)
	entries := []db.HistoryEntry{
		{TsMs: 1000, ExitCode: 0, Command: "echo ok"},
		{TsMs: 2000, ExitCode: 1, Command: "echo fail"},
	}

	for _, entry := range entries {
		if _, err = repo.Insert(entry); err != nil {
			t.Fatalf("repo.Insert(%q) error: %v", entry.Command, err)
		}
	}

	cfg := config.Default()
	cfg.Display.DefaultFailFilter = "exclude"

	m := NewModel(cfg, repo, "", "", 10, false, "")

	if got, want := m.failFilter, db.FailFilterExclude; got != want {
		t.Fatalf("failFilter = %v, want %v", got, want)
	}

	if got, want := len(m.allEntries), 1; got != want {
		t.Fatalf("len(allEntries) = %d, want %d", got, want)
	}

	if got, want := m.allEntries[0].Command, "echo ok"; got != want {
		t.Fatalf("allEntries[0].Command = %q, want %q", got, want)
	}
}

func testNavModel(entryCount int, height int) *Model {
	return &Model{
		cfg:            config.Default(),
		height:         height,
		displayEntries: make([]history.ScoredEntry, entryCount),
	}
}
