package shell

import (
	"errors"
	"fmt"
)

type Shell int

const (
	Zsh Shell = iota
	Bash
	Fish
	PowerShell
)

var errUnsupportedShell = errors.New("unsupported shell")

func Parse(name string) (Shell, error) {
	switch name {
	case "zsh":
		return Zsh, nil
	case "bash":
		return Bash, nil
	case "fish":
		return Fish, nil
	case "powershell", "pwsh":
		return PowerShell, nil
	default:
		return 0, fmt.Errorf("%w: %s (supported: zsh, bash, fish, powershell)", errUnsupportedShell, name)
	}
}

func (s Shell) String() string {
	switch s {
	case Zsh:
		return "zsh"
	case Bash:
		return "bash"
	case Fish:
		return "fish"
	case PowerShell:
		return "powershell"
	default:
		return "unknown"
	}
}
