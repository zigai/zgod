package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/paths"
)

func TestConfigShowPrintsInvalidConfig(t *testing.T) {
	setConfigHomes(t)

	configPath, err := paths.ConfigFile()
	if err != nil {
		t.Fatalf("ConfigFile() error: %v", err)
	}

	if err = os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	const brokenConfig = "not = [valid\n"
	if err = os.WriteFile(configPath, []byte(brokenConfig), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cmd := &cobra.Command{}

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err = configShowCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("config show error: %v", err)
	}

	if stdout.String() != brokenConfig {
		t.Fatalf("config show output = %q, want %q", stdout.String(), brokenConfig)
	}
}

func TestConfigEditAllowsInvalidConfigAndParsesEditorArgs(t *testing.T) {
	setConfigHomes(t)

	configPath, err := paths.ConfigFile()
	if err != nil {
		t.Fatalf("ConfigFile() error: %v", err)
	}

	if err = os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	if err = os.WriteFile(configPath, []byte("not = [valid\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	t.Setenv("EDITOR", "code -w")
	t.Setenv("VISUAL", "")

	var (
		gotName string
		gotArgs []string
	)

	oldRunner := runEditorProcess
	runEditorProcess = func(name string, args []string) error {
		gotName = name

		gotArgs = append([]string(nil), args...)

		return nil
	}

	t.Cleanup(func() {
		runEditorProcess = oldRunner
	})

	if err = configEditCmd.RunE(&cobra.Command{}, nil); err != nil {
		t.Fatalf("config edit error: %v", err)
	}

	if gotName != "code" {
		t.Fatalf("editor name = %q, want %q", gotName, "code")
	}

	wantArgs := []string{"-w", configPath}
	if strings.Join(gotArgs, "\n") != strings.Join(wantArgs, "\n") {
		t.Fatalf("editor args = %q, want %q", gotArgs, wantArgs)
	}
}

func TestSplitCommandLine(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "single executable",
			input: "vim",
			want:  []string{"vim"},
		},
		{
			name:  "editor flags",
			input: "code -w",
			want:  []string{"code", "-w"},
		},
		{
			name:  "quoted executable path",
			input: "\"C:/Program Files/VS Code/Code.exe\" -w",
			want:  []string{"C:/Program Files/VS Code/Code.exe", "-w"},
		},
		{
			name:    "unterminated quote",
			input:   "\"code -w",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitCommandLine(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("splitCommandLine(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Fatalf("splitCommandLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func setConfigHomes(t *testing.T) {
	t.Helper()

	baseDir := t.TempDir()

	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", filepath.Join(baseDir, "config"))
		t.Setenv("LOCALAPPDATA", filepath.Join(baseDir, "data"))

		return
	}

	t.Setenv("XDG_CONFIG_HOME", filepath.Join(baseDir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(baseDir, "data"))
}
