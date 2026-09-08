package tui

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
)

func BenchmarkNewModelConstructor(b *testing.B) {
	cfg := config.Default()

	b.ResetTimer()

	for b.Loop() {
		_ = NewModel(cfg, nil, "/repo/01", "", 10, false, "")
	}
}

func BenchmarkInitialHistoryLoadAndRender(b *testing.B) {
	dbPath := filepath.Join(b.TempDir(), "bench.db")

	database, err := db.Open(dbPath)
	if err != nil {
		b.Fatalf("db.Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	tx, err := database.Begin()
	if err != nil {
		b.Fatalf("database.Begin() error: %v", err)
	}

	stmt, err := tx.Prepare(`INSERT INTO history (ts_ms, duration, exit_code, command, directory, session_id, hostname) VALUES (?, ?, ?, ?, ?, '', '')`)
	if err != nil {
		b.Fatalf("tx.Prepare() error: %v", err)
	}
	defer stmt.Close()

	stmtLatest, err := tx.Prepare(`INSERT OR REPLACE INTO latest_command (command, history_id, ts_ms, duration, exit_code, directory) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		b.Fatalf("tx.Prepare() latest error: %v", err)
	}
	defer stmtLatest.Close()

	for i := range 10_000 {
		cmdStr := fmt.Sprintf("echo repeated command %05d", i%2_000)
		dir := fmt.Sprintf("/repo/%02d", i%25)
		ts := int64(i + 1)
		dur := int64(i % 10_000)
		exitCode := i % 3

		res, err := stmt.Exec(ts, dur, exitCode, cmdStr, dir)
		if err != nil {
			b.Fatalf("stmt.Exec(%d) error: %v", i, err)
		}

		id, _ := res.LastInsertId()
		if _, err = stmtLatest.Exec(cmdStr, id, ts, dur, exitCode, dir); err != nil {
			b.Fatalf("stmtLatest.Exec(%d) error: %v", i, err)
		}
	}

	if err = tx.Commit(); err != nil {
		b.Fatalf("tx.Commit() error: %v", err)
	}

	repo := db.NewHistoryRepo(database)
	cfg := config.Default()

	b.ResetTimer()

	for b.Loop() {
		m := NewModel(cfg, repo, "/repo/01", "", 10, false, "")
		m.terminalHeight = 24
		m.height = 10
		m.width = 80

		cmd := m.loadEntriesCmd(m.startupLimit(), false, m.historyLoadGen)
		if cmd == nil {
			b.Fatal("loadEntriesCmd returned nil")
		}

		msg := cmd()
		_, _ = m.Update(msg)

		view := m.View()
		if len(view) == 0 {
			b.Fatal("m.View() returned empty")
		}

		if len(m.allEntries) == 0 {
			b.Fatal("no entries loaded")
		}
	}
}
