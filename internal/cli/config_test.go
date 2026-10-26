package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"pgregory.net/rapid"

	"github.com/zigai/zgod/internal/paths"
)

func TestConfigShowRejectsInvalidConfig(t *testing.T) {
	setCLITestHomes(t)

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

	if err = runConfigShow(cmd, nil); err == nil {
		t.Fatal("config show error = nil, want invalid config error")
	}

	if stdout.String() != "" {
		t.Fatalf("config show output = %q, want empty output", stdout.String())
	}
}

func TestConfigShowRawPrintsInvalidConfig(t *testing.T) {
	setCLITestHomes(t)

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
	cmd.Flags().Bool("raw", false, "")

	if err = cmd.Flags().Set("raw", "true"); err != nil {
		t.Fatalf("setting raw flag: %v", err)
	}

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err = runConfigShow(cmd, nil); err != nil {
		t.Fatalf("config show --raw error: %v", err)
	}

	if stdout.String() != brokenConfig {
		t.Fatalf("config show --raw output = %q, want %q", stdout.String(), brokenConfig)
	}
}

func TestConfigEditAllowsInvalidConfigAndParsesEditorArgs(t *testing.T) {
	setCLITestHomes(t)

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

	if err = runConfigEdit(&cobra.Command{}, nil); err != nil {
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
		{
			name:  "preserves empty quoted argument",
			input: `code --profile ""`,
			want:  []string{"code", "--profile", ""},
		},
		{
			name:    "rejects empty quoted executable",
			input:   `"" code`,
			wantErr: true,
		},
		{
			name:    "rejects lone empty quotes",
			input:   `""`,
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

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitCommandLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEditorArgumentProcess(t *testing.T) {
	capture := os.Getenv("ZGOD_TEST_EDITOR_CAPTURE")
	if capture == "" {
		return
	}

	var arguments []string

	for index, argument := range os.Args {
		if argument == "--" {
			arguments = os.Args[index+1:]
			break
		}
	}

	data, err := json.Marshal(arguments)
	if err != nil {
		os.Exit(90)
	}

	if err = os.WriteFile(capture, data, 0o600); err != nil {
		os.Exit(91)
	}

	os.Exit(0)
}

func TestEditorArgumentRoundTripProperty(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	rapid.Check(t, func(check *rapid.T) {
		dir := t.TempDir()
		capture := filepath.Join(dir, "arguments.json")
		t.Setenv("ZGOD_TEST_EDITOR_CAPTURE", capture)
		t.Setenv("GORACE", "atexit_sleep_ms=0")

		text := rapid.StringOfN(rapid.RuneFrom(historyTextRunes(check)), 0, 32, -1)
		arguments := rapid.SliceOfN(text, 0, 8).Draw(check, "arguments")
		arguments = append(arguments, "")
		path := filepath.Join(dir, "config-"+text.Draw(check, "config"))

		words := append([]string{binary, "-test.run=TestEditorArgumentProcess", "--"}, arguments...)

		escaping := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
		for index := range words {
			words[index] = `"` + escaping.Replace(words[index]) + `"`
		}

		if err := openEditor(strings.Join(words, " "), path); err != nil {
			check.Fatal(err)
		}

		data, err := os.ReadFile(capture)
		if err != nil {
			check.Fatal(err)
		}

		var got []string
		if err = json.Unmarshal(data, &got); err != nil {
			check.Fatal(err)
		}

		want := append(append([]string(nil), arguments...), path)
		if !reflect.DeepEqual(got, want) {
			check.Fatalf("editor arguments = %#v, want %#v", got, want)
		}
	})
}

func TestConfigEditPreservesEmptyQuotedEditorArgument(t *testing.T) {
	setCLITestHomes(t)

	configPath, err := paths.ConfigFile()
	if err != nil {
		t.Fatalf("ConfigFile() error: %v", err)
	}

	if err = os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	t.Setenv("EDITOR", `code --profile ""`)
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

	if err = runConfigEdit(&cobra.Command{}, nil); err != nil {
		t.Fatalf("config edit error: %v", err)
	}

	if gotName != "code" {
		t.Fatalf("editor name = %q, want %q", gotName, "code")
	}

	wantArgs := []string{"--profile", "", configPath}
	if strings.Join(gotArgs, "\n") != strings.Join(wantArgs, "\n") {
		t.Fatalf("editor args = %q, want %q", gotArgs, wantArgs)
	}
}

func TestCLITestHomesIsolatesFromInheritedOverride(t *testing.T) {
	t.Setenv("ZGOD_CONFIG", "/sentinel/inherited/override/config.toml")
	baseDir := setCLITestHomes(t)

	configPath, err := paths.ConfigFile()
	if err != nil {
		t.Fatalf("paths.ConfigFile() error: %v", err)
	}

	rel, err := filepath.Rel(baseDir, configPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("paths.ConfigFile() = %q, want path within %q", configPath, baseDir)
	}
}

func setCLITestHomes(t *testing.T) string {
	t.Helper()

	baseDir := t.TempDir()

	t.Setenv("HOME", baseDir)
	t.Setenv("USERPROFILE", baseDir)

	configDir := filepath.Join(baseDir, "config")
	dataDir := filepath.Join(baseDir, "data")

	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", configDir)
		t.Setenv("LOCALAPPDATA", dataDir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", configDir)
		t.Setenv("XDG_DATA_HOME", dataDir)
	}

	t.Setenv("ZGOD_CONFIG", "")

	resolved, err := paths.ConfigFile()
	if err != nil {
		t.Fatalf("resolving isolated ConfigFile: %v", err)
	}

	rel, err := filepath.Rel(baseDir, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("ConfigFile %q is not within isolated test root %q", resolved, baseDir)
	}

	return baseDir
}
