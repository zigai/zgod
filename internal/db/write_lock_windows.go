//go:build windows

package db

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func tryLockDatabaseFile(file *os.File) error {
	var overlapped windows.Overlapped

	if err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	); err != nil {
		return fmt.Errorf("locking database lock file with LockFileEx: %w", err)
	}

	return nil
}

func unlockDatabaseFile(file *os.File) error {
	var overlapped windows.Overlapped

	if err := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped); err != nil {
		return fmt.Errorf("unlocking database lock file with UnlockFileEx: %w", err)
	}

	return nil
}

func isDatabaseFileLockBusy(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
