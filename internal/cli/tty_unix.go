//go:build !windows

package cli

import (
	"fmt"
	"os"
)

func openTTY() (input *os.File, output *os.File, cleanup func(), err error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("opening /dev/tty: %w", err)
	}

	cleanup = func() {
		_ = f.Close()
	}

	return f, f, cleanup, nil
}
