package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/zigai/zgod/internal/paths"
)

type Config struct {
	DB      DBConfig      `toml:"db"`
	Filters FilterConfig  `toml:"filters"`
	Theme   ThemeConfig   `toml:"theme"`
	Display DisplayConfig `toml:"display"`
	Keys    KeyConfig     `toml:"keys"`
}

type DBConfig struct {
	Path string `toml:"path"`
}

type FilterConfig struct {
	IgnoreSpace      bool     `toml:"ignore_space"`
	ExitCode         []int    `toml:"exit_code"`
	CommandGlob      []string `toml:"command_glob"`
	CommandRegex     []string `toml:"command_regex"`
	DirectoryGlob    []string `toml:"directory_glob"`
	DirectoryRegex   []string `toml:"directory_regex"`
	MaxCommandLength int      `toml:"max_command_length"`
}

var (
	errNoMatchModeEnabled      = errors.New("at least one match mode must be enabled")
	errInvalidDefaultScope     = errors.New("invalid default_scope")
	errDefaultModeNotEnabled   = errors.New("default_mode is not enabled")
	errInvalidDefaultMode      = errors.New("invalid default_mode")
	errInvalidMultilinePreview = errors.New("invalid multiline_preview")
)

func Default() Config {
	return Config{
		DB: DBConfig{
			Path: "",
		},
		Filters: FilterConfig{
			IgnoreSpace:      true,
			ExitCode:         []int{130},
			CommandGlob:      []string{},
			CommandRegex:     []string{},
			DirectoryGlob:    []string{},
			DirectoryRegex:   []string{},
			MaxCommandLength: 0,
		},
		Theme:   DefaultTheme(),
		Display: DefaultDisplay(),
		Keys:    DefaultKeys(),
	}
}

func Load() (Config, error) {
	cfg := Default()
	configPath, err := paths.ConfigFile()
	if err != nil {
		return cfg, fmt.Errorf("resolving config file path: %w", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = cfg.Save()
			if err != nil {
				return cfg, err
			}
			return cfg, nil
		}
		return cfg, err
	}
	if _, err = toml.Decode(string(data), &cfg); err != nil {
		return cfg, fmt.Errorf("decoding config TOML: %w", err)
	}
	if err = cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	err := c.validateEnabledModes()
	if err != nil {
		return err
	}
	err = c.validateDefaultScope()
	if err != nil {
		return err
	}
	err = c.validateDefaultMode()
	if err != nil {
		return err
	}
	return c.validateMultilinePreview()
}

func (c Config) Save() error {
	if err := paths.EnsureDirs(); err != nil {
		return fmt.Errorf("ensuring config directories: %w", err)
	}
	configPath, err := paths.ConfigFile()
	if err != nil {
		return fmt.Errorf("resolving config file path: %w", err)
	}
	f, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("opening config file %q: %w", configPath, err)
	}
	defer func() { _ = f.Close() }()
	if err = toml.NewEncoder(f).Encode(c); err != nil {
		return fmt.Errorf("encoding config TOML: %w", err)
	}
	return nil
}

func (c Config) DatabasePath() (string, error) {
	if c.DB.Path != "" {
		path, err := paths.ExpandTilde(c.DB.Path)
		if err != nil {
			return "", fmt.Errorf("expanding database path %q: %w", c.DB.Path, err)
		}
		return path, nil
	}
	path, err := paths.DatabaseFile()
	if err != nil {
		return "", fmt.Errorf("resolving default database path: %w", err)
	}
	return path, nil
}

func (c Config) validateEnabledModes() error {
	if !c.Display.EnableFuzzy && !c.Display.EnableRegex && !c.Display.EnableGlob {
		return errNoMatchModeEnabled
	}
	return nil
}

func (c Config) validateDefaultScope() error {
	switch c.Display.DefaultScope {
	case "", "normal", "cwd":
		return nil
	default:
		return fmt.Errorf("%w %q: must be \"normal\" or \"cwd\"", errInvalidDefaultScope, c.Display.DefaultScope)
	}
}

func (c Config) validateDefaultMode() error {
	switch c.Display.DefaultMode {
	case "", "fuzzy":
		if c.Display.DefaultMode == "fuzzy" && !c.Display.EnableFuzzy {
			return fmt.Errorf("%w: %q", errDefaultModeNotEnabled, c.Display.DefaultMode)
		}
		return nil
	case "regex":
		if !c.Display.EnableRegex {
			return fmt.Errorf("%w: %q", errDefaultModeNotEnabled, c.Display.DefaultMode)
		}
		return nil
	case "glob":
		if !c.Display.EnableGlob {
			return fmt.Errorf("%w: %q", errDefaultModeNotEnabled, c.Display.DefaultMode)
		}
		return nil
	default:
		return fmt.Errorf("%w %q: must be \"fuzzy\", \"regex\", or \"glob\"", errInvalidDefaultMode, c.Display.DefaultMode)
	}
}

func (c Config) validateMultilinePreview() error {
	switch c.Display.MultilinePreview {
	case "", "popup", "preview_pane", "expand", "collapsed":
		return nil
	default:
		return fmt.Errorf(
			"%w %q: must be \"popup\", \"preview_pane\", \"expand\", or \"collapsed\"",
			errInvalidMultilinePreview,
			c.Display.MultilinePreview,
		)
	}
}
