package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func setConfigHome(t *testing.T, dir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", dir)
		return filepath.Join(dir, "zgod")
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "zgod")
}

func setDataHome(t *testing.T, dir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", dir)
		return filepath.Join(dir, "zgod")
	}
	t.Setenv("XDG_DATA_HOME", dir)
	return filepath.Join(dir, "zgod")
}

func TestConfigDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "xdg-config")
	want := setConfigHome(t, dir)
	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error: %v", err)
	}
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "xdg-data")
	want := setDataHome(t, dir)
	got, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() error: %v", err)
	}
	if got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
}

func TestEnsureDirs(t *testing.T) {
	dir := t.TempDir()
	configHome := filepath.Join(dir, "config")
	dataHome := filepath.Join(dir, "data")
	configDir := setConfigHome(t, configHome)
	dataDir := setDataHome(t, dataHome)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error: %v", err)
	}

	for _, path := range []string{configDir, dataDir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected dir %s to exist: %v", path, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", path)
		}
	}
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error: %v", err)
	}
	tests := []struct {
		input string
		want  string
	}{
		{"~/foo", filepath.Join(home, "foo")},
		{"/absolute/path", "/absolute/path"},
		{"relative", "relative"},
	}
	for _, tt := range tests {
		got, expandErr := ExpandTilde(tt.input)
		if expandErr != nil {
			t.Fatalf("ExpandTilde(%q) error: %v", tt.input, expandErr)
		}
		if got != tt.want {
			t.Errorf("ExpandTilde(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	got, err := ConfigFile()
	if err != nil {
		t.Fatalf("ConfigFile() error: %v", err)
	}
	if !strings.HasSuffix(got, "config.toml") {
		t.Errorf("ConfigFile() = %q, expected config.toml suffix", got)
	}
}

func TestDatabaseFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg")
	got, err := DatabaseFile()
	if err != nil {
		t.Fatalf("DatabaseFile() error: %v", err)
	}
	if !strings.HasSuffix(got, "history.db") {
		t.Errorf("DatabaseFile() = %q, expected history.db suffix", got)
	}
}
