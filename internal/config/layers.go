package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/zigai/zgod/internal/paths"
)

const maxConfigBytes = 1 << 20

var errInvalidSetting = errors.New("invalid configuration setting")

type LoadOptions struct {
	Path      string
	NoConfig  bool
	NoCreate  bool
	Overrides []string
}

// LoadWithOptions resolves files, environment and explicit TOML assignments.
// Explicit config paths replace file discovery; --no-config skips every file.
func LoadWithOptions(opts LoadOptions) (Config, error) {
	cfg := Default()
	if err := ValidateOverrides(opts.Overrides); err != nil {
		return cfg, err
	}

	createPath, err := mergeConfigSources(&cfg, opts)
	if err != nil {
		return cfg, err
	}

	if err := mergeEnvironment(&cfg); err != nil {
		return cfg, err
	}

	for _, assignment := range opts.Overrides {
		if err := mergeTOML(&cfg, assignment); err != nil {
			return cfg, err
		}
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	if createPath != "" && !opts.NoCreate {
		if err := saveConfigFile(createPath, Default(), false); err != nil {
			return cfg, err
		}
	}

	return cfg, nil
}

func EnsureDefault(opts LoadOptions) error {
	if opts.NoConfig {
		return nil
	}

	_, path, err := configSources(opts.Path)
	if err != nil || path == "" {
		return err
	}

	return saveConfigFile(path, Default(), false)
}

func ValidateOverrides(assignments []string) error {
	for _, assignment := range assignments {
		key, _, found := strings.Cut(assignment, "=")
		if !found || strings.Count(strings.TrimSpace(key), ".") != 1 {
			return fmt.Errorf("%w: --set requires section.key=TOML-value", errInvalidSetting)
		}

		cfg := Default()
		if err := mergeTOML(&cfg, assignment); err != nil {
			return err
		}
	}

	return nil
}

func mergeConfigSources(cfg *Config, opts LoadOptions) (string, error) {
	if opts.NoConfig {
		return "", nil
	}

	files, createPath, err := configSources(opts.Path)
	if err != nil {
		return "", err
	}

	for _, file := range files {
		if file == paths.LegacyConfigFile() {
			if _, err = fmt.Fprintf(os.Stderr, "Warning: config %s is deprecated; move it to the XDG zgod/config.toml path.\n", file); err != nil {
				return "", fmt.Errorf("writing config migration warning: %w", err)
			}
		}

		if err = mergeConfigFile(cfg, file); err != nil {
			return "", err
		}
	}

	return createPath, nil
}

func configSources(explicit string) ([]string, string, error) {
	if explicit == "" {
		explicit = os.Getenv("ZGOD_CONFIG")
	}

	if explicit != "" {
		path, err := paths.ExpandTilde(explicit)
		if err != nil {
			return nil, "", fmt.Errorf("expanding config path: %w", err)
		}

		return []string{path}, "", nil
	}

	userDir, err := paths.ConfigDir()
	if err != nil {
		return nil, "", fmt.Errorf("resolving config directory: %w", err)
	}

	userPath := filepath.Join(userDir, "config.toml")
	candidates := []string{paths.SystemConfigFile()}

	legacy := paths.LegacyConfigFile()
	if _, err = os.Stat(userPath); errors.Is(err, os.ErrNotExist) && legacy != "" {
		candidates = append(candidates, legacy)
	}

	candidates = append(candidates, userPath, ".zgod.toml")

	files, err := existingConfigFiles(candidates)
	if err != nil {
		return nil, "", err
	}

	createPath := userPath
	if slices.Contains(files, userPath) || slices.Contains(files, legacy) {
		createPath = ""
	}

	return files, createPath, nil
}

func existingConfigFiles(candidates []string) ([]string, error) {
	var files []string

	for _, path := range candidates {
		if path == "" {
			continue
		}

		_, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("checking config %q: %w", path, err)
		}

		files = append(files, path)
	}

	return files, nil
}

func mergeConfigFile(cfg *Config, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("reading config %q: %w", path, err)
	}

	if info.Size() > maxConfigBytes {
		return fmt.Errorf("%w: config %q exceeds 1 MiB", errInvalidSetting, path)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening config %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return fmt.Errorf("reading config %q: %w", path, err)
	}

	if len(data) > maxConfigBytes {
		return fmt.Errorf("%w: config %q exceeds 1 MiB", errInvalidSetting, path)
	}

	if err = mergeTOML(cfg, string(data)); err != nil {
		return fmt.Errorf("reading config %q: %w", path, err)
	}

	return nil
}

func mergeTOML(cfg *Config, input string) error {
	metadata, err := toml.Decode(input, cfg)
	if err != nil {
		return fmt.Errorf("decoding config TOML: %w", err)
	}

	if unknown := metadata.Undecoded(); len(unknown) != 0 {
		return fmt.Errorf("%w: unknown key %s", errInvalidSetting, unknown[0])
	}

	return nil
}

func mergeEnvironment(cfg *Config) error {
	sections := reflect.TypeFor[Config]()
	for section := range sections.Fields() {
		name := section.Tag.Get("toml")
		for field := range section.Type.Fields() {
			key := field.Tag.Get("toml")
			envName := "ZGOD_" + strings.ToUpper(name+"_"+key)

			value, set := os.LookupEnv(envName)
			if !set {
				continue
			}

			assignment, err := environmentAssignment(name, key, field.Type, value)
			if err != nil {
				return fmt.Errorf("%s: %w", envName, err)
			}

			if err = mergeTOML(cfg, assignment); err != nil {
				return fmt.Errorf("%s: %w", envName, err)
			}
		}
	}

	return nil
}

func environmentAssignment(section, key string, fieldType reflect.Type, value string) (string, error) {
	if fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}

	if fieldType.Kind() == reflect.String {
		var output bytes.Buffer
		if err := toml.NewEncoder(&output).Encode(map[string]map[string]string{section: {key: value}}); err != nil {
			return "", fmt.Errorf("encoding environment setting: %w", err)
		}

		return output.String(), nil
	}

	if fieldType.Kind() == reflect.Bool {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return "", fmt.Errorf("expected true or false: %w", err)
		}

		value = strconv.FormatBool(parsed)
	}

	return section + "." + key + " = " + value, nil
}

func saveConfigFile(path string, cfg Config, replace bool) error {
	if err := paths.EnsureParentDir(path, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	file, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return fmt.Errorf("creating config temporary file: %w", err)
	}

	defer func() { _ = os.Remove(file.Name()) }()

	if err = writeConfig(file, cfg, !replace); err != nil {
		_ = file.Close()
		return err
	}

	if err = file.Close(); err != nil {
		return fmt.Errorf("closing config: %w", err)
	}

	if replace {
		err = os.Rename(file.Name(), path)
	} else {
		// A hard link publishes a complete file without overwriting a concurrent
		// first-run writer. The temporary name is removed by the owner above.
		err = os.Link(file.Name(), path)
		if errors.Is(err, os.ErrExist) {
			return nil
		}
	}

	if err != nil {
		return fmt.Errorf("publishing config %q: %w", path, err)
	}

	return nil
}

func writeConfig(file *os.File, cfg Config, commented bool) error {
	const intro = "# zgod configuration. All fields are optional.\n# Overrides: CLI --set > ZGOD_SECTION_KEY > ./.zgod.toml > this file > system config.\n# Arrays replace lower-priority lists. Boolean values can be true or false.\n# Docs: https://github.com/zigai/zgod#configuration\n\n"
	if _, err := file.WriteString(intro); err != nil {
		return fmt.Errorf("writing config comments: %w", err)
	}

	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).Encode(cfg); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	content := encoded.String()
	if commented {
		// Generated defaults document settings without overriding system values on
		// the next run. Explicit Save calls still persist active assignments.
		content = "# " + strings.ReplaceAll(content, "\n", "\n# ")
	}

	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("syncing config: %w", err)
	}

	return nil
}
