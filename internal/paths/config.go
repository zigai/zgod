package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

func SystemConfigFile() string {
	if runtime.GOOS == "windows" {
		if dir := os.Getenv("PROGRAMDATA"); dir != "" {
			return filepath.Join(dir, "zgod", "config.toml")
		}

		return ""
	}

	return "/etc/zgod/config.toml"
}

func LegacyConfigFile() string {
	if runtime.GOOS != "darwin" {
		return ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, "Library", "Application Support", "zgod", "config.toml")
}
