package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var errHomeDirectoryEmpty = errors.New("home directory is empty")

func ConfigFile() (string, error) {
	if path := os.Getenv("ZGOD_CONFIG"); path != "" {
		return ExpandTilde(path)
	}

	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.toml"), nil
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

	return filepath.Join(home, path[1:]), nil
}
