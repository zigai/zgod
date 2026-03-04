//go:build !windows

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

var errImportRequiresInteractiveTerminal = errors.New("import authentication requires an interactive terminal")

func requireImportAuthentication() error {
	tty, err := openTTY()
	if err != nil {
		return fmt.Errorf("%w: %w", errImportRequiresInteractiveTerminal, err)
	}

	_ = tty.Close()

	if _, err = exec.LookPath("sudo"); err != nil {
		return fmt.Errorf("sudo is required for import authentication: %w", err)
	}

	ctx := context.Background()

	if err = runSudoAuthCommand(ctx, "-k"); err != nil {
		return fmt.Errorf("resetting cached sudo credentials: %w", err)
	}

	if err = runSudoAuthCommand(ctx, "-v"); err != nil {
		return fmt.Errorf("validating sudo credentials: %w", err)
	}

	return nil
}

func runSudoAuthCommand(ctx context.Context, arg string) error {
	command := exec.CommandContext(ctx, "sudo", arg)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	if err := command.Run(); err != nil {
		return fmt.Errorf("running sudo %s: %w", arg, err)
	}

	return nil
}
