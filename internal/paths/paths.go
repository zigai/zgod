package paths

import (
	"os"
	"path/filepath"
	"strings"
)

func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "zgod")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "zgod")
}

func DataDir() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "zgod")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "zgod")
}

func ConfigFile() string {
	if path := os.Getenv("ZGOD_CONFIG"); path != "" {
		return ExpandTilde(path)
	}
	return filepath.Join(ConfigDir(), "config.toml")
}

func DatabaseFile() string {
	return filepath.Join(DataDir(), "history.db")
}

func EnsureDirs() error {
	if err := os.MkdirAll(ConfigDir(), 0700); err != nil {
		return err
	}
	return os.MkdirAll(DataDir(), 0700)
}

func ExpandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, _ := os.UserHomeDir()
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return filepath.Join(home, path[1:])
}
