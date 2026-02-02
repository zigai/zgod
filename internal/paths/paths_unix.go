//go:build !windows

package paths

import (
	"os"
	"path/filepath"
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
