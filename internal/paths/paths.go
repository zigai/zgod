package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	errHomeDirectoryEmpty = errors.New("home directory is empty")
	errUnsupportedTilde   = errors.New("unsupported tilde form")
)

func ConfigDir() (string, error) {
	if runtime.GOOS != "windows" {
		if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
			return filepath.Join(dir, "zgod"), nil
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("getting home directory: %w", err)
		}

		return filepath.Join(home, ".config", "zgod"), nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("getting user config directory: %w", err)
	}

	return filepath.Join(dir, "zgod"), nil
}

func ConfigFile() (string, error) {
	if path := os.Getenv("ZGOD_CONFIG"); path != "" {
		return ExpandTilde(path)
	}

	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, "config.toml")

	legacy := LegacyConfigFile()
	if _, err = os.Stat(path); errors.Is(err, os.ErrNotExist) && legacy != "" {
		if _, legacyErr := os.Stat(legacy); legacyErr == nil {
			return legacy, nil
		}
	}

	return path, nil
}

func DatabaseFile() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "history.db"), nil
}

func EnsureDirs() error {
	configDir, err := ConfigDir()
	if err != nil {
		return err
	}

	dataDir, err := DataDir()
	if err != nil {
		return err
	}

	if err = os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("creating config directory %q: %w", configDir, err)
	}

	if err = os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("creating data directory %q: %w", dataDir, err)
	}

	return nil
}

func EnsureParentDir(path string, mode os.FileMode) error {
	parentDir := filepath.Dir(path)
	if err := os.MkdirAll(parentDir, mode); err != nil {
		return fmt.Errorf("creating parent directory %q: %w", parentDir, err)
	}

	return nil
}

func ExpandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	if home == "" {
		return "", errHomeDirectoryEmpty
	}

	if path == "~" {
		return home, nil
	}

	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:]), nil
	}

	if runtime.GOOS == "windows" && strings.HasPrefix(path, `~\`) {
		return filepath.Join(home, path[2:]), nil
	}

	return "", fmt.Errorf("%w %q; use ~/", errUnsupportedTilde, path)
}
