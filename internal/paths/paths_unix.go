//go:build !windows

package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

func DataDir() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "zgod"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	if home == "" {
		return "", errHomeDirectoryEmpty
	}

	return filepath.Join(home, ".local", "share", "zgod"), nil
}
