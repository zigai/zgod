package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zigai/zgod/internal/paths"
)

func TestUsageErrorsHaveNoStateSideEffects(t *testing.T) {
	base := setCLITestHomes(t)
	t.Chdir(t.TempDir())

	for _, args := range [][]string{
		{"unknown"},
		{"init"},
		{"init", "bash", "extra"},
		{"install", "invalid"},
		{"import"},
		{"search", "--height", "-1"},
		{"search", "--height", "1001"},
		{"search", "--shell"},
		{"search", "--json", "--shell"},
		{"search", "--json", "--mode", "invalid"},
		{"search", "--json", "--mode", "regex", "--query", "["},
		{"record", "--command", "example", "--ts", "garbage"},
		{"record", "--command", "example", "--ts", "9223372036854775807s"},
		{"record", "--command", "example", "--duration", "-2"},
		{"config", "show", "--set", "display.unknown=true"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			if code := execute(args, strings.NewReader(""), &out, &diagnostics); code != 2 {
				t.Fatalf("exit=%d, stdout=%q, stderr=%q", code, out.String(), diagnostics.String())
			}

			if out.Len() != 0 || !strings.Contains(diagnostics.String(), documentationURL) {
				t.Fatalf("stdout=%q stderr=%q", out.String(), diagnostics.String())
			}

			for _, dir := range []string{"config", "data"} {
				if _, err := os.Stat(filepath.Join(base, dir)); !os.IsNotExist(err) {
					t.Fatalf("usage error created %s: %v", dir, err)
				}
			}
		})
	}
}

func TestVersionAndHelpStreams(t *testing.T) {
	setCLITestHomes(t)

	for _, arg := range []string{"--version", "-V", "--help"} {
		var out, diagnostics bytes.Buffer
		if code := execute([]string{arg}, strings.NewReader(""), &out, &diagnostics); code != 0 {
			t.Fatalf("%s exit=%d stderr=%q", arg, code, diagnostics.String())
		}

		if out.Len() == 0 || diagnostics.Len() != 0 {
			t.Fatalf("%s stdout=%q stderr=%q", arg, out.String(), diagnostics.String())
		}
	}

	var out, diagnostics bytes.Buffer
	if code := execute([]string{"-v"}, strings.NewReader(""), &out, &diagnostics); code != 0 {
		t.Fatalf("legacy version exit=%d: %s", code, &diagnostics)
	}

	if !strings.Contains(diagnostics.String(), "deprecated") || !strings.Contains(out.String(), "zgod") {
		t.Fatalf("legacy version stdout=%q stderr=%q", out.String(), diagnostics.String())
	}
}

func TestNoConfigRecordAndJSONSearch(t *testing.T) {
	setCLITestHomes(t)
	t.Chdir(t.TempDir())

	const command = "printf 'hello λ'\nprintf 'second line'"

	var out, diagnostics bytes.Buffer

	args := []string{"record", "--no-config", "--command-stdin", "--duration=0", "--directory", "/elsewhere"}
	executeSuccessfully(t, args, command, &out, &diagnostics)

	t.Run("record streams", func(t *testing.T) {
		if out.Len() != 0 || diagnostics.Len() != 0 {
			t.Fatalf("record polluted streams: %q %q", out.String(), diagnostics.String())
		}
	})

	configPath, err := paths.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}

	if _, err = os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("--no-config wrote configuration: %v", err)
	}

	args = []string{"search", "--no-config", "--json", "--mode=regex", "--query=hello", "--set", "display.default_scope=\"cwd\"", "--cwd=false"}
	executeSuccessfully(t, args, "", &out, &diagnostics)

	var records []historyRecord
	if err = json.Unmarshal(out.Bytes(), &records); err != nil {
		t.Fatal(err)
	}

	if len(records) != 1 || records[0].Command != command || records[0].Directory != "/elsewhere" {
		t.Fatalf("records=%+v", records)
	}

	if !strings.Contains(out.String(), "\n  {\n    \"id\"") || diagnostics.Len() != 0 {
		t.Fatalf("JSON streams: %q %q", out.String(), diagnostics.String())
	}
}

func executeSuccessfully(t *testing.T, args []string, input string, out, diagnostics *bytes.Buffer) {
	t.Helper()

	if code := execute(args, strings.NewReader(input), out, diagnostics); code != 0 {
		t.Fatalf("%v exit=%d: %s", args, code, diagnostics)
	}
}

func TestConfiguredRegexIsValidatedBeforeCreatingDefaults(t *testing.T) {
	base := setCLITestHomes(t)
	t.Chdir(t.TempDir())
	t.Setenv("ZGOD_DISPLAY_DEFAULT_MODE", "regex")

	var out, diagnostics bytes.Buffer
	if code := execute([]string{"search", "--json", "--query", "["}, strings.NewReader(""), &out, &diagnostics); code != 2 {
		t.Fatalf("exit=%d: %s", code, &diagnostics)
	}

	if _, err := os.Stat(filepath.Join(base, "config")); !os.IsNotExist(err) {
		t.Fatalf("invalid configured regex created config directory: %v", err)
	}
}
