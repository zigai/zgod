//go:build !windows

package db

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func tryLockDatabaseFile(file *os.File) error {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fmt.Errorf("flocking database lock file: %w", err)
	}

	return nil
}

func unlockDatabaseFile(file *os.File) error {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_UN); err != nil {
		return fmt.Errorf("unlocking database lock file: %w", err)
	}

	return nil
}

func isDatabaseFileLockBusy(err error) bool {
	return errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK)
}
