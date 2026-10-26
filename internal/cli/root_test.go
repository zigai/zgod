package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
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

func snapshotFileTree(root string) (map[string]string, error) {
	files := make(map[string]string)

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking test state: %w", err)
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("reading test file metadata: %w", err)
		}

		var data []byte
		if !entry.IsDir() {
			data, err = os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading test file contents: %w", err)
			}
		}

		files[path] = fmt.Sprintf("%o:%s", info.Mode(), data)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("snapshotting test state: %w", err)
	}

	return files, nil
}

func invalidNumericCLIArguments(check *rapid.T) []string {
	check.Helper()

	args := []string{"record", "--command", "example"}

	switch rapid.IntRange(0, 5).Draw(check, "numeric-kind") {
	case 0:
		flag, maximum := "height", int64(1000)
		if rapid.Bool().Draw(check, "limit") {
			flag, maximum = "limit", 100000
		}

		value := rapid.Int64Range(maximum+1, math.MaxInt64).Draw(check, "above-bound")
		if rapid.Bool().Draw(check, "below-bound") {
			value = rapid.Int64Range(math.MinInt64, 0).Draw(check, "below-bound-value")
		}

		return []string{"search", "--json", "--" + flag, strconv.FormatInt(value, 10)}
	case 1:
		args = append(args, "--duration", strconv.FormatInt(rapid.Int64Range(math.MinInt64, -2).Draw(check, "duration"), 10))
	case 2:
		args = append(args, "--ts", strconv.FormatInt(rapid.Int64Range(math.MaxInt64/1000+1, math.MaxInt64).Draw(check, "seconds"), 10)+"s")
	case 3:
		args = append(args, "--exit-code", strconv.FormatInt(rapid.Int64Range(math.MaxUint32+1, math.MaxInt64).Draw(check, "exit"), 10))
	case 4:
		args = append(args, "--duration", strings.Repeat("9", rapid.IntRange(20, 80).Draw(check, "overflow-digits")))
	default:
		args = append(args, "--ts", strconv.FormatInt(rapid.Int64Range(math.MinInt64, -1).Draw(check, "negative-timestamp"), 10))
	}

	return args
}

func invalidCLIInvocation(check *rapid.T) ([]string, int) {
	check.Helper()

	if rapid.Bool().Draw(check, "numeric") {
		return invalidNumericCLIArguments(check), 2
	}

	invalid := "invalid_" + rapid.StringMatching("[a-z]{1,20}").Draw(check, "invalid-text")
	switch rapid.IntRange(0, 6).Draw(check, "invalid-kind") {
	case 0:
		return []string{invalid}, 2
	case 1:
		return []string{"search", "--json", "--mode", invalid}, 2
	case 2:
		return []string{"search", "--json", "--mode", "regex", "--query", "(" + invalid}, 2
	case 3:
		return []string{"search", "--json", "--mode", "glob", "--query", "[" + invalid}, 2
	case 4:
		return []string{"record", "--command", "example", "--command-stdin"}, 2
	case 5:
		return []string{"config", "show", "--set", "display." + invalid + "=true"}, 2
	default:
		if rapid.Bool().Draw(check, "malformed-toml") {
			return []string{"config", "show", "--set", "theme.prompt=\"" + invalid}, 2
		}

		return []string{"config", "show", "--set", "display.default_mode=" + strconv.Quote(invalid)}, 1
	}
}

func TestCLIRejectionHasNoEffectsProperty(t *testing.T) {
	t.Chdir(t.TempDir())
	rapid.Check(t, func(check *rapid.T) {
		base := setCLITestHomes(t)
		dbPath := filepath.Join(base, "history.db")
		t.Setenv("ZGOD_DB_PATH", dbPath)

		if rapid.Bool().Draw(check, "existing-history") {
			database := openPropertyHistory(check, dbPath)
			storePropertyHistory(check, database, rapid.SliceOfN(historyRecordGenerator(), 1, 4).Draw(check, "history"))

			if err := database.Close(); err != nil {
				check.Fatal(err)
			}
		}

		protected := rapid.SliceOfN(rapid.Byte(), 0, 64).Draw(check, "protected")
		if err := os.WriteFile(filepath.Join(base, "unrelated"), protected, 0o600); err != nil {
			check.Fatal(err)
		}

		args, wantCode := invalidCLIInvocation(check)

		before, err := snapshotFileTree(base)
		if err != nil {
			check.Fatal(err)
		}

		var out, diagnostics bytes.Buffer
		if code := execute(args, strings.NewReader("incoming"), &out, &diagnostics); code != wantCode {
			check.Fatalf("arguments = %q: exit=%d, want %d; stderr=%q", args, code, wantCode, diagnostics.String())
		}

		if out.Len() != 0 || !strings.Contains(diagnostics.String(), documentationURL) {
			check.Fatalf("rejection streams: stdout=%q stderr=%q", out.String(), diagnostics.String())
		}

		after, err := snapshotFileTree(base)
		if err != nil || !reflect.DeepEqual(before, after) {
			check.Fatalf("rejected command changed state: before=%#v after=%#v, error: %v", before, after, err)
		}
	})
}

func assertRecordedStdin(check *rapid.T, path, command string) {
	check.Helper()

	database, err := db.OpenReadOnly(path)
	if err != nil {
		check.Fatal(err)
	}

	defer func() { _ = database.Close() }()

	entries := readPropertyHistory(check, database)
	if len(entries) != 1 {
		check.Fatalf("valid stdin stored %d records, want 1", len(entries))
	}

	if entries[0].Command != command || entries[0].TimestampMS != 1_234_000 || entries[0].DurationMS != 0 {
		check.Fatalf("valid stdin was not preserved: record=%#v", entries[0])
	}
}

func TestRecordStdinByteLimitProperty(t *testing.T) {
	t.Chdir(t.TempDir())
	rapid.Check(t, func(check *rapid.T) {
		base := setCLITestHomes(t)
		dbPath := filepath.Join(base, "history.db")
		t.Setenv("ZGOD_DB_PATH", dbPath)

		size := (1 << 20) + rapid.IntRange(-64, 64).Draw(check, "size-offset")
		letter := rapid.SampledFrom([]string{"é", "a", "$", "\n", "\t"}).Draw(check, "content")
		command := "c" + strings.Repeat(letter, (size-1)/len(letter)) + strings.Repeat("a", (size-1)%len(letter))

		before, err := snapshotFileTree(base)
		if err != nil {
			check.Fatal(err)
		}

		var out, diagnostics bytes.Buffer

		args := []string{"record", "--no-config", "--set", "filters.max_command_length=0", "--command-stdin", "--ts", "1234s", "--duration", "0"}

		code := execute(args, strings.NewReader(command), &out, &diagnostics)
		if size > 1<<20 {
			if code != 2 || out.Len() != 0 {
				check.Fatalf("oversized command: exit=%d stdout=%q stderr=%q", code, out.String(), diagnostics.String())
			}

			after, err := snapshotFileTree(base)
			if err != nil || !reflect.DeepEqual(after, before) {
				check.Fatalf("oversized input changed state: before=%#v after=%#v error=%v", before, after, err)
			}

			return
		}

		if code != 0 || out.Len() != 0 || diagnostics.Len() != 0 {
			check.Fatalf("valid command: exit=%d stdout=%q stderr=%q", code, out.String(), diagnostics.String())
		}

		assertRecordedStdin(check, dbPath, command)
	})
}

func TestCLIExplicitOverridesProperty(t *testing.T) {
	setCLITestHomes(t)
	t.Chdir(t.TempDir())
	rapid.Check(t, func(check *rapid.T) {
		hints := rapid.Bool().Draw(check, "show-hints")
		prompt := rapid.StringOfN(rapid.RuneFrom(historyTextRunes(check)), 0, 32, -1).Draw(check, "prompt")

		t.Setenv("ZGOD_DISPLAY_SHOW_HINTS", strconv.FormatBool(!hints))
		t.Setenv("ZGOD_THEME_PROMPT", "lower-priority")

		args := []string{"config", "show", "--json", "--no-config", "--set", "display.show_hints=" + strconv.FormatBool(hints), "--set", "theme.prompt=" + strconv.Quote(prompt)}

		var out, diagnostics bytes.Buffer
		if code := execute(args, strings.NewReader(""), &out, &diagnostics); code != 0 || diagnostics.Len() != 0 {
			check.Fatalf("explicit override rejected: exit=%d stderr=%q", code, diagnostics.String())
		}

		var got config.Config
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			check.Fatal(err)
		}

		if got.Display.ShowHints != hints || got.Theme.Prompt != prompt {
			check.Fatalf("CLI overrides lost: hints=%t prompt=%q, want %t %q", got.Display.ShowHints, got.Theme.Prompt, hints, prompt)
		}
	})
}

func searchBoundsAccept(height, limit int) bool {
	return height >= 1 && height <= 1000 && limit >= 1 && limit <= 100000
}

func TestSearchNumericBoundariesProperty(t *testing.T) {
	t.Chdir(t.TempDir())
	rapid.Check(t, func(check *rapid.T) {
		base := setCLITestHomes(t)
		t.Setenv("ZGOD_DB_PATH", filepath.Join(base, "history.db"))

		height := rapid.SampledFrom([]int{1, 1000}).Draw(check, "height-boundary") + rapid.IntRange(-2, 2).Draw(check, "height-offset")
		limit := rapid.SampledFrom([]int{1, 100000}).Draw(check, "limit-boundary") + rapid.IntRange(-2, 2).Draw(check, "limit-offset")
		valid := searchBoundsAccept(height, limit)

		var out, diagnostics bytes.Buffer

		args := []string{"search", "--no-config", "--json", "--height", strconv.Itoa(height), "--limit", strconv.Itoa(limit)}

		code := execute(args, strings.NewReader(""), &out, &diagnostics)
		if !valid {
			if code != 2 || out.Len() != 0 {
				check.Fatalf("invalid bounds accepted: height=%d limit=%d exit=%d", height, limit, code)
			}
		} else {
			if code != 0 || diagnostics.Len() != 0 {
				check.Fatalf("valid bounds rejected: height=%d limit=%d exit=%d stderr=%q", height, limit, code, diagnostics.String())
			}

			var records []historyRecord
			if err := json.Unmarshal(out.Bytes(), &records); err != nil || len(records) != 0 {
				check.Fatalf("empty search output=%q, error=%v", out.String(), err)
			}
		}

		files, err := os.ReadDir(base)
		if err != nil || len(files) != 0 {
			check.Fatalf("search created state: files=%v, error=%v", files, err)
		}
	})
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
