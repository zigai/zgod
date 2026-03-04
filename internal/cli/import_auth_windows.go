//go:build windows

package cli

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

var errImportRequiresElevatedTerminal = errors.New("import requires an elevated terminal; rerun as Administrator")

func requireImportAuthentication() error {
	elevated, err := isCurrentProcessElevated()
	if err != nil {
		return fmt.Errorf("checking process elevation: %w", err)
	}

	if !elevated {
		return errImportRequiresElevatedTerminal
	}

	return nil
}

func isCurrentProcessElevated() (bool, error) {
	adminSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false, fmt.Errorf("creating administrators SID: %w", err)
	}

	member, err := windows.GetCurrentProcessToken().IsMember(adminSID)
	if err != nil {
		return false, fmt.Errorf("checking administrators group membership: %w", err)
	}

	return member, nil
}
