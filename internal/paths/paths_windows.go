//go:build windows

package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

func DataDir() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("getting home directory: %w", err)
		}
		if home == "" {
			return "", fmt.Errorf("home directory is empty")
		}
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localAppData, "zgod"), nil
}
