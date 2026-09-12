package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteImportFromStdin(t *testing.T) {
	setCLITestHomes(t)
	dir := t.TempDir()
	t.Chdir(dir)
	sourcePath := filepath.Join(dir, "source.db")
	targetPath := filepath.Join(dir, "target.db")

	var out, diagnostics bytes.Buffer
	executeSuccessfully(t, []string{"record", "--no-config", "--set", fmt.Sprintf("db.path=%q", sourcePath), "--command=echo imported"}, "", &out, &diagnostics)

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	executeSuccessfully(t, []string{"import", "--no-config", "--set", fmt.Sprintf("db.path=%q", targetPath), "--json", "-"}, string(data), &out, &diagnostics)

	var summary map[string]int
	if err = json.Unmarshal(out.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}

	if summary["imported"] != 1 || diagnostics.Len() != 0 {
		t.Fatalf("summary=%v stderr=%q", summary, diagnostics.String())
	}

	out.Reset()
	executeSuccessfully(t, []string{"search", "--no-config", "--set", fmt.Sprintf("db.path=%q", targetPath)}, "", &out, &diagnostics)

	if out.String() != "echo imported\n" {
		t.Fatalf("imported commands=%q", out.String())
	}
}

func TestRedirectedSearchPrintsMatchingCommands(t *testing.T) {
	setCLITestHomes(t)
	t.Chdir(t.TempDir())

	for _, entry := range []struct {
		command   string
		timestamp string
	}{
		{"git status", "1700000000000"},
		{"git diff", "1700000001000"},
		{"echo ready", "1700000002000"},
	} {
		var out, diagnostics bytes.Buffer
		executeSuccessfully(t, []string{"record", "--no-config", "--command", entry.command, "--ts", entry.timestamp}, "", &out, &diagnostics)
	}

	for _, test := range []struct {
		query string
		want  string
	}{
		{"git", "git diff\n"},
		{"missing-command", ""},
	} {
		t.Run(test.query, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			executeSuccessfully(t, []string{"search", "--no-config", "--query", test.query, "--limit=1"}, "", &out, &diagnostics)

			if out.String() != test.want || diagnostics.Len() != 0 {
				t.Fatalf("stdout=%q stderr=%q, want %q and no diagnostics", out.String(), diagnostics.String(), test.want)
			}
		})
	}
}
