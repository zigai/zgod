package shell

import "fmt"

type Shell int

const (
	Zsh Shell = iota
	Bash
	Fish
)

func Parse(name string) (Shell, error) {
	switch name {
	case "zsh":
		return Zsh, nil
	case "bash":
		return Bash, nil
	case "fish":
		return Fish, nil
	default:
		return 0, fmt.Errorf("unsupported shell: %s (supported: zsh, bash, fish)", name)
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
	default:
		return "unknown"
	}
}
