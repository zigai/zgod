package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error: %v", err)
	}
	if got != "/tmp/xdg-config/zgod" {
		t.Errorf("ConfigDir() = %q, want /tmp/xdg-config/zgod", got)
	}
}

func TestDataDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
	got, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() error: %v", err)
	}
	if got != "/tmp/xdg-data/zgod" {
		t.Errorf("DataDir() = %q, want /tmp/xdg-data/zgod", got)
	}
}

func TestEnsureDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error: %v", err)
	}

	for _, sub := range []string{"config/zgod", "data/zgod"} {
		path := filepath.Join(dir, sub)
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
		got, err := ExpandTilde(tt.input)
		if err != nil {
			t.Fatalf("ExpandTilde(%q) error: %v", tt.input, err)
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
