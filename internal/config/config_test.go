package config

import (
	"bytes"
	"embed"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"

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

	t.Setenv("ZGOD_CONFIG", "")

	configPath, err := paths.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}

	if err = os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}

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

	if !cfg.Display.NerdFont {
		t.Error("default NerdFont should be true")
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

func TestLoadMissingExplicitFileDoesNotCreateIt(t *testing.T) {
	dir := t.TempDir()
	setTestHomes(t, dir)

	configPath := filepath.Join(dir, "custom", "nested", "zgod.toml")
	t.Setenv("ZGOD_CONFIG", configPath)

	_, err := Load()
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load() = %v, want missing explicit config error", err)
	}

	if _, err = os.Stat(filepath.Dir(configPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("explicit config parent should not be created: %v", err)
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
nerd_font = false
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

	if cfg.Display.NerdFont {
		t.Error("NerdFont should be false from config")
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

//go:embed testdata/characters.txt
var configurationFixtureFS embed.FS

func configurationTextRunes(check *rapid.T) []rune {
	check.Helper()

	data, err := configurationFixtureFS.ReadFile("testdata/characters.txt")
	if err != nil {
		check.Fatal(err)
	}

	return []rune(string(data))
}

func configurationGenerator() *rapid.Generator[Config] {
	return rapid.Custom(func(check *rapid.T) Config {
		cfg := Default()
		text := rapid.StringOfN(rapid.RuneFrom(configurationTextRunes(check)), 0, 24, -1)
		cfg.Filters.IgnoreSpace = rapid.Bool().Draw(check, "ignore-space")
		cfg.Filters.ExitCode = rapid.SliceOfN(rapid.IntRange(0, 255), 0, 8).Draw(check, "exit-codes")
		cfg.Filters.CommandGlob = rapid.SliceOfN(text, 0, 4).Draw(check, "command-globs")
		literalRegex := rapid.Map(text, regexp.QuoteMeta)
		cfg.Filters.CommandRegex = rapid.SliceOfN(literalRegex, 0, 4).Draw(check, "command-regexes")
		cfg.Filters.DirectoryRegex = rapid.SliceOfN(literalRegex, 0, 4).Draw(check, "directory-regexes")
		cfg.Filters.DirectoryGlob = rapid.SliceOfN(rapid.StringMatching("[a-z]{0,12}"), 0, 4).Draw(check, "directory-globs")

		cfg.Filters.MaxCommandLength = rapid.IntRange(0, 1024).Draw(check, "max-command-length")
		for _, target := range []any{&cfg.Theme, &cfg.Keys} {
			value := reflect.ValueOf(target).Elem()
			for index := range value.NumField() {
				field := value.Field(index)

				label := value.Type().Field(index).Name
				if field.Kind() == reflect.String {
					field.SetString(text.Draw(check, label))
				}

				if field.Kind() == reflect.Pointer {
					field.Set(reflect.ValueOf(rapid.Ptr(rapid.Bool(), true).Draw(check, label)))
				}
			}
		}

		cfg.Display.TimeFormat = rapid.SampledFrom([]string{"", "relative", "absolute"}).Draw(check, "time-format")
		cfg.Display.DurationFormat = rapid.SampledFrom([]string{"", "auto", "ms", "s"}).Draw(check, "duration-format")
		cfg.Display.DefaultScope = rapid.SampledFrom([]string{"", "normal", "cwd"}).Draw(check, "scope")
		cfg.Display.DefaultMode = rapid.SampledFrom([]string{"", "fuzzy", "regex", "glob"}).Draw(check, "mode")
		cfg.Display.DefaultFailFilter = rapid.SampledFrom([]string{"include", "exclude", "only"}).Draw(check, "fail-filter")
		cfg.Display.MultilinePreview = rapid.SampledFrom([]string{"", "popup", "preview_pane", "expand", "collapsed"}).Draw(check, "preview")
		cfg.Display.MultilineCollapse = text.Draw(check, "collapse")
		cfg.Display.StartupLimit = rapid.IntRange(0, 100_000).Draw(check, "startup-limit")
		cfg.Display.CWDBoost = rapid.IntRange(-100, 100).Draw(check, "cwd-boost")
		cfg.Display.ShowHints = rapid.Bool().Draw(check, "hints")
		cfg.Display.ShowDirectory = rapid.Bool().Draw(check, "directories")
		cfg.Display.InstantExecute = rapid.Bool().Draw(check, "instant")
		cfg.Display.HideMultiline = rapid.Bool().Draw(check, "hide-multiline")
		cfg.Display.NerdFont = rapid.Bool().Draw(check, "nerd-font")

		return cfg
	})
}

func canonicalConfiguration(cfg Config) Config {
	for _, field := range []**bool{&cfg.Theme.MatchBold, &cfg.Theme.MatchUnderline, &cfg.Theme.SelectionBarShow, &cfg.Theme.SelectionFullLine} {
		if *field == nil {
			enabled := true
			*field = &enabled
		}
	}

	if cfg.Filters.ExitCode == nil {
		cfg.Filters.ExitCode = []int{}
	}

	if cfg.Filters.CommandGlob == nil {
		cfg.Filters.CommandGlob = []string{}
	}

	if cfg.Filters.CommandRegex == nil {
		cfg.Filters.CommandRegex = []string{}
	}

	if cfg.Filters.DirectoryGlob == nil {
		cfg.Filters.DirectoryGlob = []string{}
	}

	if cfg.Filters.DirectoryRegex == nil {
		cfg.Filters.DirectoryRegex = []string{}
	}

	return cfg
}

func TestConfigurationSaveLoadProperty(t *testing.T) {
	path := setTestHomes(t, t.TempDir())
	rapid.Check(t, func(check *rapid.T) {
		cfg := configurationGenerator().Draw(check, "configuration")
		if err := cfg.Save(); err != nil {
			check.Fatal(err)
		}

		got, err := LoadWithOptions(LoadOptions{Path: path, NoCreate: true})
		if err != nil {
			check.Fatal(err)
		}

		want := canonicalConfiguration(cfg)

		got = canonicalConfiguration(got)
		if !reflect.DeepEqual(got, want) {
			check.Fatalf("loaded=%+v, want=%+v", got, want)
		}
	})
}

func TestEnvironmentStringPreservationProperty(t *testing.T) {
	setTestHomes(t, t.TempDir())
	rapid.Check(t, func(check *rapid.T) {
		value := rapid.StringOfN(rapid.RuneFrom(configurationTextRunes(check)), 0, 48, -1).Draw(check, "value")
		t.Setenv("ZGOD_THEME_PROMPT", value)

		got, err := LoadWithOptions(LoadOptions{NoConfig: true, NoCreate: true})
		if err != nil {
			check.Fatal(err)
		}

		if got.Theme.Prompt != value {
			check.Fatalf("environment decoded=%q, want=%q", got.Theme.Prompt, value)
		}
	})
}

func TestConfigurationUnknownKeysHaveNoEffectsProperty(t *testing.T) {
	dir := t.TempDir()
	defaultPath := setTestHomes(t, dir)
	rapid.Check(t, func(check *rapid.T) {
		key := "unknown_" + rapid.StringMatching("[a-z]{1,24}").Draw(check, "key")
		section := rapid.SampledFrom([]string{"db", "filters", "theme", "display", "keys"}).Draw(check, "section")
		value := rapid.Int().Draw(check, "value")
		path := filepath.Join(t.TempDir(), "invalid.toml")

		content := []byte("[" + section + "]\n" + key + "=" + strconv.Itoa(value) + "\n")
		if err := os.WriteFile(path, content, 0o600); err != nil {
			check.Fatal(err)
		}

		_, err := LoadWithOptions(LoadOptions{Path: path})
		if !errors.Is(err, errInvalidSetting) {
			check.Fatalf("category=%v, want invalid setting", err)
		}

		after, err := os.ReadFile(path)
		if err != nil {
			check.Fatal(err)
		}

		if !bytes.Equal(after, content) {
			check.Fatal("validation modified input file")
		}

		if _, err = os.Stat(defaultPath); !errors.Is(err, os.ErrNotExist) {
			check.Fatalf("validation created defaults: %v", err)
		}
	})
}

func TestConfigurationInputSizeProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		size := rapid.OneOf(rapid.IntRange(0, 1<<20), rapid.IntRange((1<<20)-4096, (1<<20)+4096)).Draw(check, "size")
		path := filepath.Join(t.TempDir(), "config.toml")

		content := strings.Repeat(" ", size)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			check.Fatal(err)
		}

		_, err := LoadWithOptions(LoadOptions{Path: path, NoCreate: true})
		if size > 1<<20 {
			if !errors.Is(err, errInvalidSetting) {
				check.Fatalf("oversized config category=%v", err)
			}
		} else if err != nil {
			check.Fatalf("bounded config rejected: %v", err)
		}

		after, err := os.ReadFile(path)
		if err != nil {
			check.Fatal(err)
		}

		if string(after) != content {
			check.Fatal("loading altered config")
		}
	})
}

func TestConfigurationValidationProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		cfg := configurationGenerator().Draw(check, "valid-configuration")
		if err := cfg.Validate(); err != nil {
			check.Fatalf("valid config rejected: %v", err)
		}

		bad := cfg
		field := rapid.IntRange(0, 5).Draw(check, "invalid-field")
		invalid := "invalid-" + rapid.StringMatching("[a-z]{1,12}").Draw(check, "invalid-enum")

		var category error

		switch field {
		case 0:
			bad.Display.DefaultScope = invalid
			category = errInvalidDefaultScope
		case 1:
			bad.Display.DefaultMode = invalid
			category = errInvalidDefaultMode
		case 2:
			bad.Display.DefaultFailFilter = invalid
			category = errInvalidDefaultFailFilter
		case 3:
			bad.Display.MultilinePreview = invalid
			category = errInvalidMultilinePreview
		case 4:
			bad.Filters.MaxCommandLength = rapid.IntRange(-1000, -1).Draw(check, "negative-length")
			category = errInvalidSetting
		case 5:
			bad.Display.StartupLimit = rapid.IntRange(-1000, -1).Draw(check, "negative-startup")
			category = errInvalidSetting
		}

		if err := bad.Validate(); !errors.Is(err, category) {
			check.Fatalf("invalid field=%d category=%v, want=%v", field, err, category)
		}
	})
}
