//go:build !windows

package cli

import "os"

func openTTY() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}
