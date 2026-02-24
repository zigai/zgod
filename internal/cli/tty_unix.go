//go:build !windows

package cli

import (
	"fmt"
	"os"
)

func openTTY() (*os.File, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening /dev/tty: %w", err)
	}
	return f, nil
}
