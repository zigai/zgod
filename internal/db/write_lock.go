package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/zigai/zgod/internal/paths"
)

var ErrDatabaseWriteLockTimeout = errors.New("timed out waiting for database write lock")

const databaseWriteLockPollInterval = 25 * time.Millisecond

type databaseWriteLock struct {
	file *os.File
}

func WithDatabaseWriteLock(ctx context.Context, dbPath string, fn func() error) (err error) {
	lock, err := acquireDatabaseWriteLock(ctx, dbPath)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := lock.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("releasing database write lock: %w", closeErr))
		}
	}()

	return fn()
}

func DatabaseWriteLockPath(dbPath string) string {
	return dbPath + ".lock"
}

func acquireDatabaseWriteLock(ctx context.Context, dbPath string) (*databaseWriteLock, error) {
	lockPath := DatabaseWriteLockPath(dbPath)
	if err := paths.EnsureParentDir(lockPath, 0o700); err != nil {
		return nil, fmt.Errorf("ensuring database lock parent directory for %q: %w", lockPath, err)
	}

	// #nosec G703 -- the lock file intentionally follows the configured database path.
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening database lock file %q: %w", lockPath, err)
	}

	if err = lockDatabaseFile(ctx, file); err != nil {
		_ = file.Close()
		return nil, err
	}

	return &databaseWriteLock{file: file}, nil
}

func lockDatabaseFile(ctx context.Context, file *os.File) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: %w", ErrDatabaseWriteLockTimeout, err)
		}

		err := tryLockDatabaseFile(file)
		if err == nil {
			return nil
		}

		if !isDatabaseFileLockBusy(err) {
			return fmt.Errorf("locking database lock file %q: %w", file.Name(), err)
		}

		timer := time.NewTimer(databaseWriteLockPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%w: %w", ErrDatabaseWriteLockTimeout, ctx.Err())
		case <-timer.C:
		}
	}
}

func (l *databaseWriteLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	file := l.file
	l.file = nil

	return errors.Join(unlockDatabaseFile(file), file.Close())
}
