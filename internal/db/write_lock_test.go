package db

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const databaseWriteLockHelperEnv = "ZGOD_DATABASE_WRITE_LOCK_HELPER"

func TestWithDatabaseWriteLockSerializesProcesses(t *testing.T) {
	cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cmdCancel()

	dbPath := filepath.Join(t.TempDir(), "history.db")
	cmd := exec.CommandContext(cmdCtx, os.Args[0], "-test.run=TestDatabaseWriteLockHelperProcess", "--", dbPath)

	cmd.Env = append(os.Environ(), databaseWriteLockHelperEnv+"=1")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe() error: %v", err)
	}

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	if err = cmd.Start(); err != nil {
		t.Fatalf("starting lock helper: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		t.Fatalf("lock helper did not report readiness; stderr=%s", stderr.String())
	}

	if got, want := scanner.Text(), "locked"; got != want {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		t.Fatalf("lock helper reported %q, want %q; stderr=%s", got, want, stderr.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = WithDatabaseWriteLock(ctx, dbPath, func() error {
		return nil
	})

	cancel()

	if !errors.Is(err, ErrDatabaseWriteLockTimeout) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		t.Fatalf("WithDatabaseWriteLock() error = %v, want ErrDatabaseWriteLockTimeout", err)
	}

	if err = cmd.Wait(); err != nil {
		t.Fatalf("lock helper exited with error: %v; stderr=%s", err, stderr.String())
	}

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = WithDatabaseWriteLock(ctx, dbPath, func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("WithDatabaseWriteLock() after helper exit error: %v", err)
	}
}

func TestDatabaseWriteLockHelperProcess(t *testing.T) {
	if os.Getenv(databaseWriteLockHelperEnv) != "1" {
		return
	}

	dbPath := os.Args[len(os.Args)-1]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	err := WithDatabaseWriteLock(ctx, dbPath, func() error {
		fmt.Println("locked")
		time.Sleep(500 * time.Millisecond)

		return nil
	})

	cancel()

	if err != nil {
		fmt.Fprintf(os.Stderr, "WithDatabaseWriteLock() error: %v\n", err)
		os.Exit(2)
	}

	os.Exit(0)
}
