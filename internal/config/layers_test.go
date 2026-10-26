package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/BurntSushi/toml"
	"pgregory.net/rapid"
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

func configurationLayer(t *testing.T, check *rapid.T, layer string, cfg Config, want *Config) (map[string]map[string]any, []string) {
	t.Helper()

	sections := reflect.TypeFor[Config]()
	expected := reflect.ValueOf(want).Elem()

	var assignments []string

	generated := reflect.ValueOf(cfg)
	values := make(map[string]map[string]any)

	for section := range sections.Fields() {
		sectionName := section.Tag.Get("toml")
		for field := range section.Type.Fields() {
			key := field.Tag.Get("toml")

			name := sectionName + "." + key
			if !rapid.Bool().Draw(check, layer+"."+name+".present") {
				continue
			}

			fieldValue := generated.Field(section.Index[0]).Field(field.Index[0])
			expected.Field(section.Index[0]).Field(field.Index[0]).Set(fieldValue)
			value := fieldValue.Interface()

			if values[sectionName] == nil {
				values[sectionName] = make(map[string]any)
			}

			values[sectionName][key] = value

			var literal bytes.Buffer
			if err := toml.NewEncoder(&literal).Encode(map[string]any{"value": value}); err != nil {
				check.Fatal(err)
			}

			_, encoded, _ := strings.Cut(strings.TrimSpace(literal.String()), "=")
			encoded = strings.TrimSpace(encoded)

			if layer == "environment" {
				if text, ok := value.(string); ok {
					encoded = text
				}

				t.Setenv("ZGOD_"+strings.ToUpper(sectionName+"_"+key), encoded)
			}

			if layer == "override" {
				assignments = append(assignments, name+"="+encoded)
			}
		}
	}

	return values, assignments
}

func resetConfigurationEnvironment(t *testing.T, check *rapid.T) {
	t.Helper()

	for section := range reflect.TypeFor[Config]().Fields() {
		for field := range section.Type.Fields() {
			name := "ZGOD_" + strings.ToUpper(section.Tag.Get("toml")+"_"+field.Tag.Get("toml"))
			t.Setenv(name, "")

			if err := os.Unsetenv(name); err != nil {
				check.Fatal(err)
			}
		}
	}
}

func TestSparseConfigurationPrecedenceProperty(t *testing.T) {
	dir := t.TempDir()
	userPath := setTestHomes(t, dir)
	t.Chdir(dir)
	rapid.Check(t, func(check *rapid.T) {
		resetConfigurationEnvironment(t, check)

		want := Default()

		var overrides []string

		for _, layer := range []string{"user", "local", "environment", "override"} {
			cfg := canonicalConfiguration(configurationGenerator().Draw(check, layer))
			cfg.DB.Path = filepath.Join(dir, "history-"+rapid.StringMatching("[a-z]{0,16}").Draw(check, layer+".db-path"))
			values, assignments := configurationLayer(t, check, layer, cfg, &want)
			overrides = append(overrides, assignments...)

			if layer == "user" || layer == "local" {
				var data bytes.Buffer
				if err := toml.NewEncoder(&data).Encode(values); err != nil {
					check.Fatal(err)
				}

				path := userPath
				if layer == "local" {
					path = filepath.Join(dir, ".zgod.toml")
				}

				if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
					check.Fatal(err)
				}
			}
		}

		got, err := LoadWithOptions(LoadOptions{NoCreate: true, Overrides: overrides})
		if err != nil {
			check.Fatal(err)
		}

		if !reflect.DeepEqual(canonicalConfiguration(got), canonicalConfiguration(want)) {
			check.Fatalf("precedence result=%+v, want=%+v", got, want)
		}
	})
}

func TestDefaultPublicationProtectsExistingConfigProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.toml")

		existing := rapid.SliceOfN(rapid.Byte(), 0, 256).Draw(check, "existing-bytes")
		if err := os.WriteFile(path, existing, 0o600); err != nil {
			check.Fatal(err)
		}

		var workers sync.WaitGroup

		failures := make(chan error, 8)

		count := rapid.IntRange(1, 8).Draw(check, "writers")
		for range count {
			workers.Go(func() { failures <- saveConfigFile(path, Default(), false) })
		}

		workers.Wait()
		close(failures)

		for err := range failures {
			if err != nil {
				check.Fatal(err)
			}
		}

		got, err := os.ReadFile(path)
		if err != nil {
			check.Fatal(err)
		}

		if !bytes.Equal(got, existing) {
			check.Fatalf("overwrote protected config: got=%q, want=%q", got, existing)
		}

		files, err := os.ReadDir(dir)
		if err != nil {
			check.Fatal(err)
		}

		if len(files) != 1 || files[0].Name() != "config.toml" {
			check.Fatalf("temporary files leaked: %v", files)
		}
	})
}

func TestCommentedDefaultsPreserveEverySettingProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		cfg := configurationGenerator().Draw(check, "lower-priority")

		path := filepath.Join(t.TempDir(), "defaults.toml")
		if err := saveConfigFile(path, Default(), false); err != nil {
			check.Fatal(err)
		}

		got := cfg
		if err := mergeConfigFile(&got, path); err != nil {
			check.Fatal(err)
		}

		if !reflect.DeepEqual(got, cfg) {
			check.Fatalf("commented defaults changed settings: got=%+v, want=%+v", got, cfg)
		}
	})
}
