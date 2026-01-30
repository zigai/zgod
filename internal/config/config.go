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

func Default() Config {
	return Config{
		DB: DBConfig{},
		Filters: FilterConfig{
			IgnoreSpace:    true,
			ExitCode:       []int{130},
			CommandGlob:    []string{},
			CommandRegex:   []string{},
			DirectoryGlob:  []string{},
			DirectoryRegex: []string{},
		},
		Theme:   DefaultTheme(),
		Display: DefaultDisplay(),
		Keys:    DefaultKeys(),
	}
}

func Load() (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(paths.ConfigFile())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err = cfg.Save(); err != nil {
				return cfg, err
			}
			return cfg, nil
		}
		return cfg, err
	}
	if _, err = toml.Decode(string(data), &cfg); err != nil {
		return cfg, err
	}
	if err = cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if !c.Display.EnableFuzzy && !c.Display.EnableRegex && !c.Display.EnableGlob {
		return fmt.Errorf("at least one match mode must be enabled")
	}

	switch c.Display.DefaultScope {
	case "", "normal", "cwd":
	default:
		return fmt.Errorf("invalid default_scope %q: must be \"normal\" or \"cwd\"", c.Display.DefaultScope)
	}

	switch c.Display.DefaultMode {
	case "", "fuzzy":
		if c.Display.DefaultMode == "fuzzy" && !c.Display.EnableFuzzy {
			return fmt.Errorf("default_mode %q is not enabled", c.Display.DefaultMode)
		}
	case "regex":
		if !c.Display.EnableRegex {
			return fmt.Errorf("default_mode %q is not enabled", c.Display.DefaultMode)
		}
	case "glob":
		if !c.Display.EnableGlob {
			return fmt.Errorf("default_mode %q is not enabled", c.Display.DefaultMode)
		}
	default:
		return fmt.Errorf("invalid default_mode %q: must be \"fuzzy\", \"regex\", or \"glob\"", c.Display.DefaultMode)
	}

	switch c.Display.MultilinePreview {
	case "", "popup", "preview_pane", "expand", "collapsed":
	default:
		return fmt.Errorf("invalid multiline_preview %q: must be \"popup\", \"preview_pane\", \"expand\", or \"collapsed\"", c.Display.MultilinePreview)
	}

	return nil
}

func (c Config) Save() error {
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	f, err := os.OpenFile(paths.ConfigFile(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return toml.NewEncoder(f).Encode(c)
}

func (c Config) DatabasePath() string {
	if c.DB.Path != "" {
		return paths.ExpandTilde(c.DB.Path)
	}
	return paths.DatabaseFile()
}
