package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitCommandTokens(t *testing.T) {
	tokens, err := splitCommandTokens(`cp "file with spaces.txt" ./dest`)
	if err != nil {
		t.Fatalf("splitCommandTokens() error: %v", err)
	}

	if len(tokens) != 3 {
		t.Fatalf("splitCommandTokens() returned %d tokens, want 3", len(tokens))
	}

	if tokens[1] != "file with spaces.txt" {
		t.Fatalf("token[1] = %q, want %q", tokens[1], "file with spaces.txt")
	}
}

func TestSplitCommandTokensFallsBackOnUnterminatedQuote(t *testing.T) {
	_, err := splitCommandTokens(`cp "unterminated`)
	if err == nil {
		t.Fatal("splitCommandTokens() should fail for unterminated quote")
	}
}

func TestSplitCommandTokensPreservesWindowsBackslashes(t *testing.T) {
	tokens, err := splitCommandTokens(`type C:\Users\me\file.txt`)
	if err != nil {
		t.Fatalf("splitCommandTokens() error: %v", err)
	}

	if len(tokens) != 2 {
		t.Fatalf("splitCommandTokens() returned %d tokens, want 2", len(tokens))
	}

	if tokens[1] != `C:\Users\me\file.txt` {
		t.Fatalf("token[1] = %q, want %q", tokens[1], `C:\Users\me\file.txt`)
	}
}

func TestCommandReferencesExistingPathsNoPathTokens(t *testing.T) {
	ok, err := commandReferencesExistingPaths("echo hello", t.TempDir())
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if !ok {
		t.Fatal("expected command with no path tokens to pass")
	}
}

func TestCommandReferencesExistingPathsRelativePath(t *testing.T) {
	baseDir := t.TempDir()
	fileName := "existing.txt"

	filePath := filepath.Join(baseDir, fileName)
	if writeErr := os.WriteFile(filePath, []byte("ok"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile() error: %v", writeErr)
	}

	ok, err := commandReferencesExistingPaths("./"+fileName, baseDir)
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if !ok {
		t.Fatal("expected existing relative path to pass")
	}
}

func TestCommandReferencesExistingPathsFlagValue(t *testing.T) {
	baseDir := t.TempDir()

	filePath := filepath.Join(baseDir, "input.txt")
	if writeErr := os.WriteFile(filePath, []byte("ok"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile() error: %v", writeErr)
	}

	command := "--input=" + filepath.ToSlash(filePath)

	ok, err := commandReferencesExistingPaths(command, baseDir)
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if !ok {
		t.Fatal("expected flag path value to pass when file exists")
	}
}

func TestCommandReferencesExistingPathsMissingPath(t *testing.T) {
	baseDir := t.TempDir()

	ok, err := commandReferencesExistingPaths("./missing.txt", baseDir)
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if ok {
		t.Fatal("expected missing path to fail")
	}
}

func TestCommandReferencesExistingPathsMissingWindowsPath(t *testing.T) {
	ok, err := commandReferencesExistingPaths(`type C:\Users\me\missing.txt`, t.TempDir())
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if ok {
		t.Fatal("expected missing Windows path to fail")
	}
}

func TestCommandReferencesExistingPathsIgnoresURLs(t *testing.T) {
	ok, err := commandReferencesExistingPaths("curl https://example.com/file.txt", t.TempDir())
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if !ok {
		t.Fatal("expected URL-only command to pass")
	}
}

func TestCommandReferencesExistingPathsIgnoresGitCheckoutRef(t *testing.T) {
	ok, err := commandReferencesExistingPaths("git checkout feature/foo", t.TempDir())
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if !ok {
		t.Fatal("expected git ref to not be treated as a missing path")
	}
}

func TestCommandReferencesExistingPathsSedCommandUsesFileArgument(t *testing.T) {
	baseDir := t.TempDir()

	filePath := filepath.Join(baseDir, "input.txt")
	if writeErr := os.WriteFile(filePath, []byte("a\n"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile() error: %v", writeErr)
	}

	ok, err := commandReferencesExistingPaths(`sed 's/a/b/' input.txt`, baseDir)
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if !ok {
		t.Fatal("expected sed script token to be ignored and input file to be checked")
	}
}

func TestCommandReferencesExistingPathsBareChangeDirectoryTarget(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(baseDir, "src"), 0o755); err != nil {
		t.Fatalf("Mkdir() error: %v", err)
	}

	ok, err := commandReferencesExistingPaths("cd src", baseDir)
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if !ok {
		t.Fatal("expected bare cd target to be treated as a path")
	}
}

func TestCommandReferencesExistingPathsMakeDirectoryFlag(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(baseDir, "build"), 0o755); err != nil {
		t.Fatalf("Mkdir() error: %v", err)
	}

	ok, err := commandReferencesExistingPaths("make -C build", baseDir)
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if !ok {
		t.Fatal("expected make -C target to be treated as a path")
	}
}

func TestCommandReferencesExistingPathsOutputRedirectionParentExists(t *testing.T) {
	ok, err := commandReferencesExistingPaths("> out.txt", t.TempDir())
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if !ok {
		t.Fatal("expected output redirection to allow creating a new file")
	}
}

func TestCommandReferencesExistingPathsEditorAllowsCreateableFile(t *testing.T) {
	ok, err := commandReferencesExistingPaths("vim foo.txt", t.TempDir())
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if !ok {
		t.Fatal("expected editor command to allow creating a new file")
	}
}

func TestCommandReferencesExistingPathsGlob(t *testing.T) {
	baseDir := t.TempDir()

	filePath := filepath.Join(baseDir, "sample.log")
	if writeErr := os.WriteFile(filePath, []byte("ok"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile() error: %v", writeErr)
	}

	pattern := filepath.Join(baseDir, "*.log")
	pattern = strings.ReplaceAll(pattern, "\\", "/")

	ok, err := commandReferencesExistingPaths(pattern, baseDir)
	if err != nil {
		t.Fatalf("commandReferencesExistingPaths() error: %v", err)
	}

	if !ok {
		t.Fatal("expected glob to pass when at least one match exists")
	}
}
