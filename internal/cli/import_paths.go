package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/zigai/zgod/internal/paths"
)

var (
	errUnterminatedSingleQuote = errors.New("unterminated single quote")
	errUnterminatedDoubleQuote = errors.New("unterminated double quote")
)

const windowsDrivePrefixLength = 3

func commandReferencesExistingPaths(command string, workingDirectory string) (bool, error) {
	tokens, err := splitCommandTokens(command)
	if err != nil {
		tokens = strings.Fields(command)
	}

	pathCandidates := extractPathCandidates(tokens)
	if len(pathCandidates) == 0 {
		return true, nil
	}

	for _, candidate := range pathCandidates {
		resolvedPath, resolveErr := resolveCommandPath(candidate, workingDirectory)
		if resolveErr != nil {
			return false, fmt.Errorf("resolving path candidate %q: %w", candidate, resolveErr)
		}

		exists, existsErr := commandPathExists(resolvedPath)
		if existsErr != nil {
			return false, fmt.Errorf("checking path candidate %q: %w", candidate, existsErr)
		}

		if !exists {
			return false, nil
		}
	}

	return true, nil
}

func extractPathCandidates(tokens []string) []string {
	seen := map[string]bool{}
	candidates := make([]string, 0, len(tokens))

	for _, token := range tokens {
		pathCandidate, ok := pathCandidateFromToken(token)
		if !ok || seen[pathCandidate] {
			continue
		}

		seen[pathCandidate] = true
		candidates = append(candidates, pathCandidate)
	}

	return candidates
}

func pathCandidateFromToken(token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" || strings.Contains(token, "://") {
		return "", false
	}

	candidate := token
	if strings.HasPrefix(candidate, "-") && strings.Contains(candidate, "=") {
		_, value, found := strings.Cut(candidate, "=")
		if !found {
			return "", false
		}

		candidate = strings.TrimSpace(value)
	}

	if !isPathLike(candidate) {
		return "", false
	}

	return candidate, true
}

func isPathLike(path string) bool {
	if path == "" {
		return false
	}

	if strings.HasPrefix(path, "/") ||
		strings.HasPrefix(path, "./") ||
		strings.HasPrefix(path, "../") ||
		strings.HasPrefix(path, "~/") ||
		hasWindowsDrivePrefix(path) {
		return true
	}

	return strings.ContainsRune(path, '/') || strings.ContainsRune(path, '\\')
}

func hasWindowsDrivePrefix(path string) bool {
	if len(path) < windowsDrivePrefixLength {
		return false
	}

	letter := path[0]
	if (letter < 'a' || letter > 'z') && (letter < 'A' || letter > 'Z') {
		return false
	}

	if path[1] != ':' {
		return false
	}

	return path[2] == '\\' || path[2] == '/'
}

func resolveCommandPath(pathCandidate string, workingDirectory string) (string, error) {
	expanded := os.ExpandEnv(pathCandidate)

	expandedPath, err := paths.ExpandTilde(expanded)
	if err != nil {
		return "", fmt.Errorf("expanding home directory in path %q: %w", pathCandidate, err)
	}

	if hasWindowsDrivePrefix(expandedPath) || filepath.IsAbs(expandedPath) {
		return filepath.Clean(expandedPath), nil
	}

	if workingDirectory == "" {
		workingDirectory = "."
	}

	return filepath.Clean(filepath.Join(workingDirectory, expandedPath)), nil
}

func commandPathExists(path string) (bool, error) {
	if hasGlobMeta(path) {
		matches, err := filepath.Glob(path)
		if err != nil {
			return false, fmt.Errorf("expanding glob path %q: %w", path, err)
		}

		return len(matches) > 0, nil
	}

	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("stating path %q: %w", path, err)
}

func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func splitCommandTokens(command string) ([]string, error) {
	tokenizer := commandTokenizer{
		tokens:        make([]string, 0, len(command)),
		current:       strings.Builder{},
		inSingleQuote: false,
		inDoubleQuote: false,
		escaped:       false,
	}

	for _, char := range command {
		tokenizer.consumeRune(char)
	}

	if tokenizer.escaped {
		tokenizer.current.WriteRune('\\')
	}

	if tokenizer.inSingleQuote {
		return nil, errUnterminatedSingleQuote
	}

	if tokenizer.inDoubleQuote {
		return nil, errUnterminatedDoubleQuote
	}

	tokenizer.appendToken()

	return tokenizer.tokens, nil
}

type commandTokenizer struct {
	tokens        []string
	current       strings.Builder
	inSingleQuote bool
	inDoubleQuote bool
	escaped       bool
}

func (t *commandTokenizer) consumeRune(char rune) {
	if t.consumeEscapedRune(char) {
		return
	}

	if t.consumeSingleQuotedRune(char) {
		return
	}

	if t.consumeDoubleQuotedRune(char) {
		return
	}

	t.consumeUnquotedRune(char)
}

func (t *commandTokenizer) consumeEscapedRune(char rune) bool {
	if !t.escaped {
		return false
	}

	if t.inDoubleQuote {
		if char != '\\' && char != '"' && char != '$' && char != '`' {
			t.current.WriteRune('\\')
		}

		t.current.WriteRune(char)
		t.escaped = false

		return true
	}

	if !unicode.IsSpace(char) && char != '\\' && char != '\'' && char != '"' {
		t.current.WriteRune('\\')
	}

	t.current.WriteRune(char)
	t.escaped = false

	return true
}

func (t *commandTokenizer) consumeSingleQuotedRune(char rune) bool {
	if !t.inSingleQuote {
		return false
	}

	if char == '\'' {
		t.inSingleQuote = false
		return true
	}

	t.current.WriteRune(char)

	return true
}

func (t *commandTokenizer) consumeDoubleQuotedRune(char rune) bool {
	if !t.inDoubleQuote {
		return false
	}

	switch char {
	case '\\':
		t.escaped = true
	case '"':
		t.inDoubleQuote = false
	default:
		t.current.WriteRune(char)
	}

	return true
}

func (t *commandTokenizer) consumeUnquotedRune(char rune) {
	if unicode.IsSpace(char) {
		t.appendToken()
		return
	}

	switch char {
	case '\\':
		t.escaped = true
	case '\'':
		t.inSingleQuote = true
	case '"':
		t.inDoubleQuote = true
	default:
		t.current.WriteRune(char)
	}
}

func (t *commandTokenizer) appendToken() {
	if t.current.Len() == 0 {
		return
	}

	t.tokens = append(t.tokens, t.current.String())
	t.current.Reset()
}
