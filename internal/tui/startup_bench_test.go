package tui

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
)

func BenchmarkNewModelStartup(b *testing.B) {
	dbPath := filepath.Join(b.TempDir(), "bench.db")

	database, err := db.Open(dbPath)
	if err != nil {
		b.Fatalf("db.Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := db.NewHistoryRepo(database)

	for i := range 50_000 {
		entry := db.HistoryEntry{
			TSMs:      int64(i + 1),
			Duration:  int64(i % 10_000),
			ExitCode:  i % 3,
			Command:   fmt.Sprintf("echo repeated command %05d", i%10_000),
			Directory: fmt.Sprintf("/repo/%02d", i%25),
		}
		if _, err = repo.Insert(entry); err != nil {
			b.Fatalf("repo.Insert(%d) error: %v", i, err)
		}
	}

	cfg := config.Default()

	b.ResetTimer()

	for b.Loop() {
		_ = NewModel(cfg, repo, "/repo/01", "", 10, false, "")
	}
}
