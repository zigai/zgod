package config

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/bmatcuk/doublestar/v4"
)

func (c Config) validateValues() error {
	for _, setting := range []struct {
		key     string
		value   string
		choices []string
	}{
		{"display.time_format", c.Display.TimeFormat, []string{"", "relative", "absolute"}},
		{"display.duration_format", c.Display.DurationFormat, []string{"", "auto", "ms", "s"}},
	} {
		if !slices.Contains(setting.choices, setting.value) {
			return fmt.Errorf("%w: %s must be one of %q", errInvalidSetting, setting.key, setting.choices)
		}
	}

	if c.Display.StartupLimit < 0 {
		return fmt.Errorf("%w: display.startup_limit must be nonnegative (0 loads all history)", errInvalidSetting)
	}

	if c.Filters.MaxCommandLength < 0 {
		return fmt.Errorf("%w: filters.max_command_length must be nonnegative (0 disables the limit)", errInvalidSetting)
	}

	return c.validateFilterPatterns()
}

func (c Config) validateFilterPatterns() error {
	for _, setting := range []struct {
		key      string
		patterns []string
	}{
		{"filters.command_regex", c.Filters.CommandRegex},
		{"filters.directory_regex", c.Filters.DirectoryRegex},
	} {
		for _, pattern := range setting.patterns {
			if _, err := regexp.Compile(pattern); err != nil {
				return fmt.Errorf("%w: %s: %w", errInvalidSetting, setting.key, err)
			}
		}
	}

	for _, pattern := range c.Filters.DirectoryGlob {
		if !doublestar.ValidatePattern(pattern) {
			return fmt.Errorf("%w: filters.directory_glob contains an invalid glob", errInvalidSetting)
		}
	}

	return nil
}
