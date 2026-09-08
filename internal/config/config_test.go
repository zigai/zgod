package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zigai/zgod/internal/paths"
)

func setTestHomes(t *testing.T, dir string) string {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("LOCALAPPDATA", dir)

	configPath := filepath.Join(dir, "config.toml")
	t.Setenv("ZGOD_CONFIG", configPath)

	return configPath
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

	if cfg.Keys.Select1 != "ctrl+1" {
		t.Errorf("default Select1 = %q, want 'ctrl+1'", cfg.Keys.Select1)
	}

	if cfg.Keys.Select0 != "ctrl+0" {
		t.Errorf("default Select0 = %q, want 'ctrl+0'", cfg.Keys.Select0)
	}

	if cfg.Keys.SortHistory != "alt+t" {
		t.Errorf("default SortHistory = %q, want 'alt+t'", cfg.Keys.SortHistory)
	}

	if cfg.Display.DefaultFailFilter != "include" {
		t.Errorf("default DefaultFailFilter = %q, want 'include'", cfg.Display.DefaultFailFilter)
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	configPath := setTestHomes(t, dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.Filters.IgnoreSpace {
		t.Error("missing file should return defaults")
	}

	if _, err = os.Stat(configPath); err != nil {
		t.Fatalf("expected missing config file to be created at %q: %v", configPath, err)
	}
}

func TestLoadMissingFileCreatesCustomConfigParentDir(t *testing.T) {
	dir := t.TempDir()
	setTestHomes(t, dir)

	configPath := filepath.Join(dir, "custom", "nested", "zgod.toml")
	t.Setenv("ZGOD_CONFIG", configPath)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.Filters.IgnoreSpace {
		t.Error("missing custom config should return defaults")
	}

	if _, err = os.Stat(configPath); err != nil {
		t.Fatalf("expected custom config file to be created: %v", err)
	}
}

func TestLoadTOML(t *testing.T) {
	dir := t.TempDir()
	configPath := setTestHomes(t, dir)

	tomlContent := `
[filters]
ignore_space = false
exit_code = [1, 2]

[theme]
prompt = "$ "

[display]
default_fail_filter = "exclude"
`
	// #nosec G306 -- test file doesn't need restricted permissions
	if err := os.WriteFile(configPath, []byte(tomlContent), 0o644); err != nil {
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

	if cfg.Display.DefaultFailFilter != "exclude" {
		t.Errorf("DefaultFailFilter = %q, want 'exclude'", cfg.Display.DefaultFailFilter)
	}
}

func TestLoadPlatformDefaultPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ZGOD_CONFIG", "")
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("LOCALAPPDATA", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.Filters.IgnoreSpace {
		t.Error("missing file should return defaults")
	}

	configPath, err := paths.ConfigFile()
	if err != nil {
		t.Fatalf("paths.ConfigFile() error: %v", err)
	}

	if _, err = os.Stat(configPath); err != nil {
		t.Fatalf("expected platform default config file to be created at %q: %v", configPath, err)
	}
}

func TestValidateDefaultFailFilter(t *testing.T) {
	cfg := Default()
	cfg.Display.DefaultFailFilter = "bad"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want invalid default_fail_filter")
	}

	if !errors.Is(err, errInvalidDefaultFailFilter) {
		t.Fatalf("Validate() error = %v, want errInvalidDefaultFailFilter", err)
	}
}

func TestValidateRejectsRelativeDatabasePath(t *testing.T) {
	cfg := Default()
	cfg.DB.Path = "relative/history.db"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want invalid db.path")
	}

	if !errors.Is(err, errInvalidDBPath) {
		t.Fatalf("Validate() error = %v, want errInvalidDBPath", err)
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

	customPath := filepath.Join(t.TempDir(), "custom", "path.db")
	cfg.DB.Path = customPath

	path, err = cfg.DatabasePath()
	if err != nil {
		t.Fatalf("DatabasePath() error: %v", err)
	}

	if path != customPath {
		t.Errorf("DatabasePath() = %q, want %q", path, customPath)
	}
}

func TestDatabasePathRejectsRelativePath(t *testing.T) {
	cfg := Default()
	cfg.DB.Path = "relative/history.db"

	_, err := cfg.DatabasePath()
	if err == nil {
		t.Fatal("DatabasePath() error = nil, want invalid db.path")
	}

	if !errors.Is(err, errInvalidDBPath) {
		t.Fatalf("DatabasePath() error = %v, want errInvalidDBPath", err)
	}
}
