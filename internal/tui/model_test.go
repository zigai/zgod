package tui

import (
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
	"github.com/zigai/zgod/internal/match"
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

func TestHandleNavigationCtrlRMovesToNextEntry(t *testing.T) {
	t.Parallel()

	m := testNavModel(10, 8)

	handled := m.handleNavigation(tea.KeyMsg{Type: tea.KeyCtrlR})
	if !handled {
		t.Fatal("handleNavigation(ctrl+r) = false, want true")
	}

	if got, want := m.cursor, 1; got != want {
		t.Fatalf("cursor after ctrl+r = %d, want %d", got, want)
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
		{TSMs: 1000, ExitCode: 0, Command: "echo ok one"},
		{TSMs: 2000, ExitCode: 1, Command: "echo fail"},
		{TSMs: 3000, ExitCode: 0, Command: "echo ok two"},
	}

	for _, entry := range entries {
		if _, err = repo.Insert(entry); err != nil {
			t.Fatalf("repo.Insert(%q) error: %v", entry.Command, err)
		}
	}

	cfg := config.Default()
	m := NewModel(cfg, repo, "", "", 10, false, "")
	m.loadEntries()

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
		cmd, handled := m.handleToggle(tea.KeyMsg{Type: tea.KeyCtrlF})
		if !handled {
			t.Fatalf("handleToggle(%s) = false, want true", tc.name)
		}

		runHistoryLoadCmd(t, m, cmd)

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
		{TSMs: 1000, ExitCode: 0, Command: "echo ok"},
		{TSMs: 2000, ExitCode: 1, Command: "echo fail"},
	}

	for _, entry := range entries {
		if _, err = repo.Insert(entry); err != nil {
			t.Fatalf("repo.Insert(%q) error: %v", entry.Command, err)
		}
	}

	cfg := config.Default()
	cfg.Display.DefaultFailFilter = "exclude"

	m := NewModel(cfg, repo, "", "", 10, false, "")
	m.loadEntries()

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

func TestNewModelAppliesCWDFilterBeforeDedupe(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := db.NewHistoryRepo(database)
	entries := []db.HistoryEntry{
		{TSMs: 1000, Command: "repeat", Directory: "/repo"},
		{TSMs: 2000, Command: "repeat", Directory: "/elsewhere"},
	}

	for _, entry := range entries {
		if _, err = repo.Insert(entry); err != nil {
			t.Fatalf("repo.Insert(%q) error: %v", entry.Command, err)
		}
	}

	cfg := config.Default()
	m := NewModel(cfg, repo, "/repo", "", 10, true, "")
	m.loadEntries()

	if got, want := len(m.allEntries), 1; got != want {
		t.Fatalf("len(allEntries) = %d, want %d", got, want)
	}

	if got, want := m.allEntries[0].Directory, "/repo"; got != want {
		t.Fatalf("allEntries[0].Directory = %q, want %q", got, want)
	}
}

func TestNewModelSearchesBeyondTenThousandEntries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := db.NewHistoryRepo(database)
	if _, err = repo.Insert(db.HistoryEntry{TSMs: 1, Command: "old unique target"}); err != nil {
		t.Fatalf("repo.Insert(old target) error: %v", err)
	}

	for i := range 10000 {
		entry := db.HistoryEntry{
			TSMs:    int64(i + 2),
			Command: "newer filler command",
		}
		if _, err = repo.Insert(entry); err != nil {
			t.Fatalf("repo.Insert(filler %d) error: %v", i, err)
		}
	}

	cfg := config.Default()
	m := NewModel(cfg, repo, "", "", 10, false, "target")
	m.loadEntries()

	if len(m.displayEntries) == 0 {
		t.Fatal("displayEntries is empty, want old target to be searchable")
	}

	if got, want := m.displayEntries[0].Entry.Command, "old unique target"; got != want {
		t.Fatalf("displayEntries[0].Command = %q, want %q", got, want)
	}
}

func TestHistoryLoadsInLimitedBatchThenFullSet(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := db.NewHistoryRepo(database)
	for i, command := range []string{"old target", "middle command", "new command"} {
		if _, err = repo.Insert(db.HistoryEntry{TSMs: int64(i + 1), Command: command}); err != nil {
			t.Fatalf("repo.Insert(%q) error: %v", command, err)
		}
	}

	cfg := config.Default()
	cfg.Display.StartupLimit = 2

	m := NewModel(cfg, repo, "", "", 10, false, "")
	if len(m.allEntries) != 0 {
		t.Fatalf("len(allEntries) before loading = %d, want 0", len(m.allEntries))
	}

	if !m.loadingHistory {
		t.Fatal("loadingHistory before first batch = false, want true")
	}

	cmd := m.loadEntriesCmd(m.startupLimit(), false, m.historyLoadGen)
	next := runHistoryLoadCmd(t, m, cmd)

	if got, want := len(m.allEntries), 2; got != want {
		t.Fatalf("len(allEntries) after first batch = %d, want %d", got, want)
	}

	if m.historyComplete {
		t.Fatal("historyComplete after first batch = true, want false")
	}

	runHistoryLoadCmd(t, m, next)

	if got, want := len(m.allEntries), 3; got != want {
		t.Fatalf("len(allEntries) after full load = %d, want %d", got, want)
	}

	if !m.historyComplete {
		t.Fatal("historyComplete after full load = false, want true")
	}
}

func TestIncrementalFuzzyMatchMatchesFullSearch(t *testing.T) {
	t.Parallel()

	m := testFuzzySearchModel(1_000, "gc")
	m.input.SetValue("gct")
	incremental := m.matchCandidates("gct")

	fresh := testFuzzySearchModel(1_000, "")
	fresh.input.SetValue("gct")
	full := fresh.matchCandidates("gct")

	if len(incremental) != len(full) {
		t.Fatalf("incremental matches = %d, full matches = %d", len(incremental), len(full))
	}

	for i := range full {
		if incremental[i].Index != full[i].Index || incremental[i].Score != full[i].Score {
			t.Fatalf("match %d = %+v, want %+v", i, incremental[i], full[i])
		}
	}
}

func testFuzzySearchModel(entryCount int, query string) *Model {
	ti := textinput.New()
	ti.SetValue(query)

	cfg := config.Default()
	m := &Model{
		input: ti,
		cfg:   cfg,
		mode:  match.ModeFuzzy,
	}

	m.allEntries = make([]db.HistoryEntry, entryCount)

	m.candidates = make([]string, entryCount)
	for i := range entryCount {
		command := "echo filler"
		if i%2 == 0 {
			command = "git checkout target"
		}

		m.allEntries[i] = db.HistoryEntry{TSMs: int64(i), Command: command}
		m.candidates[i] = command
	}

	m.updateMatches()

	return m
}

func runHistoryLoadCmd(t *testing.T, m *Model, cmd tea.Cmd) tea.Cmd {
	t.Helper()

	if cmd == nil {
		t.Fatal("history load command is nil")
	}

	msg := cmd()
	_, next := m.Update(msg)

	return next
}

func testNavModel(entryCount int, height int) *Model {
	return &Model{
		cfg:            config.Default(),
		height:         height,
		displayEntries: make([]history.ScoredEntry, entryCount),
	}
}
