package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setTestHomes(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("LOCALAPPDATA", dir)
}

func TestDefault(t *testing.T) {
	cfg := Default()
	if !cfg.Filters.IgnoreSpace {
		t.Error("default IgnoreSpace should be true")
	}

	if cfg.Theme.Prompt != "> " {
		t.Errorf("default prompt = %q, want '> '", cfg.Theme.Prompt)
	}

	if cfg.Keys.ModeNext != "ctrl+s" {
		t.Errorf("default ModeNext = %q, want 'ctrl+s'", cfg.Keys.ModeNext)
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	setTestHomes(t, dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.Filters.IgnoreSpace {
		t.Error("missing file should return defaults")
	}
}

func TestLoadTOML(t *testing.T) {
	dir := t.TempDir()
	setTestHomes(t, dir)

	zgodDir := filepath.Join(dir, "zgod")
	if err := os.MkdirAll(zgodDir, 0700); err != nil {
		t.Fatal(err)
	}

	tomlContent := `
[filters]
ignore_space = false
exit_code = [1, 2]

[theme]
prompt = "$ "
`
	// #nosec G306 -- test file doesn't need restricted permissions
	if err := os.WriteFile(filepath.Join(zgodDir, "config.toml"), []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Filters.IgnoreSpace {
		t.Error("IgnoreSpace should be false from config")
	}

	if len(cfg.Filters.ExitCode) != 2 {
		t.Errorf("ExitCode length = %d, want 2", len(cfg.Filters.ExitCode))
	}

	if cfg.Theme.Prompt != "$ " {
		t.Errorf("Prompt = %q, want '$ '", cfg.Theme.Prompt)
	}
}

func TestDatabasePath(t *testing.T) {
	cfg := Default()
	cfg.DB.Path = ""

	path, err := cfg.DatabasePath()
	if err != nil {
		t.Fatalf("DatabasePath() error: %v", err)
	}

	if path == "" {
		t.Error("DatabasePath() should not be empty with default config")
	}

	cfg.DB.Path = "/custom/path.db"

	path, err = cfg.DatabasePath()
	if err != nil {
		t.Fatalf("DatabasePath() error: %v", err)
	}

	if path != "/custom/path.db" {
		t.Errorf("DatabasePath() = %q, want /custom/path.db", path)
	}
}
