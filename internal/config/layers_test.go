package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigurationLayersAndExplicitPresence(t *testing.T) {
	dir := t.TempDir()
	userPath := setTestHomes(t, dir)
	t.Chdir(dir)

	if err := os.WriteFile(userPath, []byte("[display]\ncwd_boost=10\nshow_hints=false\n[filters]\nexit_code=[1,2]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, ".zgod.toml"), []byte("[display]\ncwd_boost=20\n[filters]\nexit_code=[3]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ZGOD_DISPLAY_CWD_BOOST", "30")
	t.Setenv("ZGOD_DISPLAY_SHOW_HINTS", "true")

	opts := LoadOptions{Path: "", NoConfig: false, NoCreate: true, Overrides: []string{"display.cwd_boost=0", "display.show_hints=false"}}

	cfg := loadTestConfiguration(t, opts)

	if cfg.Display.CWDBoost != 0 || cfg.Display.ShowHints || !reflect.DeepEqual(cfg.Filters.ExitCode, []int{3}) {
		t.Fatalf("CLI presence/local replacement failed: %+v %+v", cfg.Display, cfg.Filters)
	}

	t.Run("environment overrides files", func(t *testing.T) {
		opts.Overrides = nil

		cfg = loadTestConfiguration(t, opts)

		if cfg.Display.CWDBoost != 30 || !cfg.Display.ShowHints {
			t.Fatalf("env did not win: %+v", cfg.Display)
		}
	})
	t.Setenv("ZGOD_DISPLAY_CWD_BOOST", "40")

	opts.Path = userPath

	cfg = loadTestConfiguration(t, opts)

	if cfg.Display.CWDBoost != 40 || !reflect.DeepEqual(cfg.Filters.ExitCode, []int{1, 2}) {
		t.Fatalf("explicit file should replace discovery while yielding to env: %+v", cfg)
	}
}

func TestGeneratedDefaultsPreserveLowerPriorityValues(t *testing.T) {
	path := setTestHomes(t, t.TempDir())
	loadTestConfiguration(t, LoadOptions{Path: "", NoConfig: false, NoCreate: false, Overrides: nil})

	lower := Default()
	lower.Display.CWDBoost = 123

	lower.Display.ShowHints = false
	if err := mergeConfigFile(&lower, path); err != nil {
		t.Fatal(err)
	}

	if lower.Display.CWDBoost != 123 || lower.Display.ShowHints {
		t.Fatalf("first-run file overwrote lower-priority settings: %+v", lower.Display)
	}
}

func loadTestConfiguration(t *testing.T, opts LoadOptions) Config {
	t.Helper()

	cfg, err := LoadWithOptions(opts)
	if err != nil {
		t.Fatal(err)
	}

	return cfg
}

func TestNoConfigSkipsInvalidFilesAndDoesNotCreateDefaults(t *testing.T) {
	dir := t.TempDir()
	path := setTestHomes(t, dir)
	t.Chdir(dir)
	t.Setenv("ZGOD_CONFIG", filepath.Join(dir, "missing", "explicit.toml"))

	if err := os.WriteFile(filepath.Join(dir, ".zgod.toml"), []byte("invalid = ["), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithOptions(LoadOptions{Path: "", NoConfig: true, NoCreate: false, Overrides: []string{"display.show_hints=false", "filters.exit_code=[]"}})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Display.ShowHints || len(cfg.Filters.ExitCode) != 0 {
		t.Fatalf("overrides=%+v", cfg)
	}

	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("default config created: %v", err)
	}
}

func TestInvalidEnvironmentDoesNotCreateConfig(t *testing.T) {
	path := setTestHomes(t, t.TempDir())
	t.Setenv("ZGOD_DISPLAY_SHOW_HINTS", "not-a-boolean")

	if _, err := Load(); err == nil {
		t.Fatal("invalid environment accepted")
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid environment created config: %v", err)
	}
}
