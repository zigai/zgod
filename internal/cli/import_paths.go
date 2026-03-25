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

const (
	windowsDrivePrefixLength = 3
	minSedScriptLength       = 4
)

type pathRequirement int

const (
	pathParentMustExist pathRequirement = iota
	pathMustExist
)

type pathCandidate struct {
	value       string
	requirement pathRequirement
}

func commandReferencesExistingPaths(command string, workingDirectory string) (bool, error) {
	tokens, err := splitCommandTokens(command)
	if err != nil {
		tokens = strings.Fields(command)
	}

	pathCandidates := extractPathCandidates(tokens, workingDirectory)
	if len(pathCandidates) == 0 {
		return true, nil
	}

	for _, candidate := range pathCandidates {
		resolvedPath, resolveErr := resolveCommandPath(candidate.value, workingDirectory)
		if resolveErr != nil {
			return false, fmt.Errorf("resolving path candidate %q: %w", candidate.value, resolveErr)
		}

		exists, existsErr := commandPathMatchesRequirement(resolvedPath, candidate.requirement)
		if existsErr != nil {
			return false, fmt.Errorf("checking path candidate %q: %w", candidate.value, existsErr)
		}

		if !exists {
			return false, nil
		}
	}

	return true, nil
}

func extractPathCandidates(tokens []string, workingDirectory string) []pathCandidate {
	commandName, commandIndex := primaryCommand(tokens)
	extractor := pathExtractor{
		commandName:           commandName,
		commandIndex:          commandIndex,
		workingDirectory:      workingDirectory,
		gitSubcommand:         "",
		afterDoubleDash:       false,
		expectSedExpression:   false,
		sedScriptConsumed:     false,
		seen:                  map[string]pathRequirement{},
		pendingRequirement:    0,
		hasPendingRequirement: false,
	}

	for index, rawToken := range tokens {
		extractor.consume(index, rawToken)
	}

	candidates := make([]pathCandidate, 0, len(extractor.seen))
	for value, requirement := range extractor.seen {
		candidates = append(candidates, pathCandidate{
			value:       value,
			requirement: requirement,
		})
	}

	return candidates
}

type pathExtractor struct {
	commandName           string
	commandIndex          int
	workingDirectory      string
	gitSubcommand         string
	afterDoubleDash       bool
	expectSedExpression   bool
	sedScriptConsumed     bool
	seen                  map[string]pathRequirement
	pendingRequirement    pathRequirement
	hasPendingRequirement bool
}

func (e *pathExtractor) consume(index int, rawToken string) {
	token := strings.TrimSpace(rawToken)
	if token == "" {
		return
	}

	if e.consumePendingRequirement(token) ||
		e.consumeRedirection(token) ||
		e.consumeDoubleDash(token) ||
		e.consumeCommandToken(index, token) ||
		e.consumeFlagPath(token) ||
		e.shouldSkipToken(token) ||
		e.consumeContextualPath(index, token) {
		return
	}

	if isPathLike(token) {
		addPathCandidate(e.seen, token, pathMustExist)
	}
}

func (e *pathExtractor) consumePendingRequirement(token string) bool {
	if !e.hasPendingRequirement {
		return false
	}

	addPathCandidate(e.seen, token, e.pendingRequirement)
	e.hasPendingRequirement = false

	return true
}

func (e *pathExtractor) consumeRedirection(token string) bool {
	if candidate, ok := pathCandidateFromInlineRedirection(token); ok {
		addPathCandidate(e.seen, candidate.value, candidate.requirement)
		return true
	}

	if requirement, ok := redirectionRequirement(token); ok {
		e.pendingRequirement = requirement
		e.hasPendingRequirement = true

		return true
	}

	return false
}

func (e *pathExtractor) consumeDoubleDash(token string) bool {
	if token != "--" {
		return false
	}

	e.afterDoubleDash = true

	return true
}

func (e *pathExtractor) consumeCommandToken(index int, token string) bool {
	if index == e.commandIndex && !strings.HasPrefix(token, "-") {
		if isPathLike(token) {
			addPathCandidate(e.seen, token, pathMustExist)
		}

		return true
	}

	if e.commandName == "git" && index > e.commandIndex && e.gitSubcommand == "" && !strings.HasPrefix(token, "-") {
		e.gitSubcommand = token
		return true
	}

	if e.commandName == "sed" && e.expectSedExpression {
		e.expectSedExpression = false
		return true
	}

	return false
}

func (e *pathExtractor) consumeFlagPath(token string) bool {
	if candidate, ok := pathCandidateFromFlagAssignment(token, e.workingDirectory); ok {
		addPathCandidate(e.seen, candidate.value, candidate.requirement)
		return true
	}

	candidate, ok := pathCandidateFromPathFlag(token)
	if !ok {
		return false
	}

	if candidate.value != "" {
		addPathCandidate(e.seen, candidate.value, candidate.requirement)
		return true
	}

	e.pendingRequirement = candidate.requirement
	e.hasPendingRequirement = true

	return true
}

func (e *pathExtractor) shouldSkipToken(token string) bool {
	if shouldSkipGitRefToken(e.commandName, e.gitSubcommand, token, e.afterDoubleDash) {
		return true
	}

	return shouldSkipSedToken(
		e.commandName,
		token,
		&e.sedScriptConsumed,
		&e.expectSedExpression,
	)
}

func (e *pathExtractor) consumeContextualPath(index int, token string) bool {
	requirement, ok := contextualPathRequirement(
		e.commandName,
		token,
		index,
		e.commandIndex,
		e.workingDirectory,
		e.sedScriptConsumed,
	)
	if !ok {
		return false
	}

	addPathCandidate(e.seen, token, requirement)

	return true
}

func addPathCandidate(seen map[string]pathRequirement, value string, requirement pathRequirement) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}

	if existing, ok := seen[value]; ok && existing >= requirement {
		return
	}

	seen[value] = requirement
}

func isPathLike(path string) bool {
	if path == "" || shouldIgnorePathLikeToken(path) {
		return false
	}

	if hasExplicitPathPrefix(path) {
		return true
	}

	return strings.ContainsRune(path, '/') || strings.ContainsRune(path, '\\')
}

func shouldIgnorePathLikeToken(path string) bool {
	return strings.Contains(path, "://") ||
		looksLikeRemotePath(path) ||
		looksLikeSedScript(path) ||
		looksLikeCompositeOptionValue(path)
}

func hasExplicitPathPrefix(path string) bool {
	return strings.HasPrefix(path, "/") ||
		strings.HasPrefix(path, "./") ||
		strings.HasPrefix(path, "../") ||
		strings.HasPrefix(path, "~/") ||
		path == "." ||
		path == ".." ||
		hasWindowsDrivePrefix(path)
}

func looksLikeRemotePath(path string) bool {
	if hasWindowsDrivePrefix(path) {
		return false
	}

	colonIndex := strings.IndexRune(path, ':')
	if colonIndex <= 0 || colonIndex >= len(path)-1 {
		return false
	}

	prefix := path[:colonIndex]
	if strings.ContainsRune(prefix, '/') || strings.ContainsRune(prefix, '\\') {
		return false
	}

	remainder := path[colonIndex+1:]

	return strings.ContainsRune(remainder, '/') || strings.ContainsRune(remainder, '\\')
}

func looksLikeSedScript(path string) bool {
	if len(path) < minSedScriptLength {
		return false
	}

	return strings.HasPrefix(path, "s/") && strings.Count(path, "/") >= 3
}

func looksLikeCompositeOptionValue(path string) bool {
	return strings.ContainsRune(path, ',') && strings.ContainsRune(path, '=')
}

func primaryCommand(tokens []string) (string, int) {
	for index, rawToken := range tokens {
		token := strings.TrimSpace(rawToken)
		if token == "" {
			continue
		}

		if token == "sudo" ||
			token == "command" ||
			token == "builtin" ||
			token == "nohup" ||
			token == "time" ||
			token == "env" ||
			isEnvironmentAssignment(token) {
			continue
		}

		return filepath.Base(token), index
	}

	return "", -1
}

func isEnvironmentAssignment(token string) bool {
	if token == "" || strings.HasPrefix(token, "=") {
		return false
	}

	name, _, found := strings.Cut(token, "=")
	if !found || name == "" {
		return false
	}

	for index, char := range name {
		if index == 0 {
			if !unicode.IsLetter(char) && char != '_' {
				return false
			}

			continue
		}

		if !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '_' {
			return false
		}
	}

	return true
}

func pathCandidateFromFlagAssignment(token string, workingDirectory string) (pathCandidate, bool) {
	if !strings.HasPrefix(token, "-") || !strings.Contains(token, "=") {
		return pathCandidate{}, false
	}

	name, value, found := strings.Cut(token, "=")
	if !found {
		return pathCandidate{}, false
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return pathCandidate{}, false
	}

	if requirement, ok := pathRequirementForFlag(name); ok {
		return pathCandidate{value: value, requirement: requirement}, true
	}

	if isPathLike(value) || bareTokenResolvesToExistingPath(value, workingDirectory) {
		return pathCandidate{value: value, requirement: pathMustExist}, true
	}

	return pathCandidate{}, false
}

func pathCandidateFromPathFlag(token string) (pathCandidate, bool) {
	if token == "-C" || token == "--directory" {
		return pathCandidate{requirement: pathMustExist}, true
	}

	if strings.HasPrefix(token, "-C") && len(token) > 2 {
		return pathCandidate{value: token[2:], requirement: pathMustExist}, true
	}

	if after, ok := strings.CutPrefix(token, "--directory="); ok {
		return pathCandidate{
			value:       strings.TrimSpace(after),
			requirement: pathMustExist,
		}, true
	}

	return pathCandidate{}, false
}

func pathRequirementForFlag(name string) (pathRequirement, bool) {
	switch name {
	case "--dir", "--directory", "--file", "--input", "--path":
		return pathMustExist, true
	case "--output", "--out":
		return pathParentMustExist, true
	default:
		return 0, false
	}
}

func shouldSkipGitRefToken(commandName string, gitSubcommand string, token string, afterDoubleDash bool) bool {
	if commandName != "git" || afterDoubleDash {
		return false
	}

	switch gitSubcommand {
	case "checkout", "switch":
		return !strings.HasPrefix(token, "/") &&
			!strings.HasPrefix(token, "./") &&
			!strings.HasPrefix(token, "../") &&
			!strings.HasPrefix(token, "~/") &&
			!hasWindowsDrivePrefix(token)
	default:
		return false
	}
}

func shouldSkipSedToken(
	commandName string,
	token string,
	sedScriptConsumed *bool,
	expectSedExpression *bool,
) bool {
	if commandName != "sed" {
		return false
	}

	if token == "-e" || token == "--expression" {
		*expectSedExpression = true

		return true
	}

	if token == "-f" || token == "--file" {
		return false
	}

	if strings.HasPrefix(token, "-") {
		return true
	}

	if !*sedScriptConsumed {
		*sedScriptConsumed = true

		return true
	}

	return false
}

func contextualPathRequirement(
	commandName string,
	token string,
	index int,
	commandIndex int,
	workingDirectory string,
	sedScriptConsumed bool,
) (pathRequirement, bool) {
	if token == "" || token == "-" {
		return 0, false
	}

	if commandName == "cd" || commandName == "pushd" {
		if index == commandIndex+1 {
			return pathMustExist, true
		}
	}

	if isEditorCommand(commandName) && !strings.HasPrefix(token, "-") {
		return pathParentMustExist, true
	}

	if commandName == "sed" && sedScriptConsumed && !strings.HasPrefix(token, "-") {
		return pathMustExist, true
	}

	if bareTokenResolvesToExistingPath(token, workingDirectory) {
		return pathMustExist, true
	}

	return 0, false
}

func isEditorCommand(commandName string) bool {
	switch commandName {
	case "code", "emacs", "nano", "nvim", "vi", "vim":
		return true
	default:
		return false
	}
}

func bareTokenResolvesToExistingPath(token string, workingDirectory string) bool {
	if token == "" ||
		token == "-" ||
		strings.HasPrefix(token, "-") ||
		strings.ContainsRune(token, '=') ||
		strings.ContainsRune(token, ':') ||
		strings.ContainsRune(token, ',') {
		return false
	}

	resolvedPath, err := resolveCommandPath(token, workingDirectory)
	if err != nil {
		return false
	}

	exists, err := commandPathExists(resolvedPath)

	return err == nil && exists
}

func redirectionRequirement(token string) (pathRequirement, bool) {
	switch token {
	case "<", "0<":
		return pathMustExist, true
	case ">", ">>", "1>", "1>>", "2>", "2>>", "&>", "&>>":
		return pathParentMustExist, true
	default:
		return 0, false
	}
}

func pathCandidateFromInlineRedirection(token string) (pathCandidate, bool) {
	if strings.HasPrefix(token, "<<") || strings.HasPrefix(token, "<<<") {
		return pathCandidate{}, false
	}

	operators := []struct {
		prefix      string
		requirement pathRequirement
	}{
		{prefix: "&>>", requirement: pathParentMustExist},
		{prefix: "1>>", requirement: pathParentMustExist},
		{prefix: "2>>", requirement: pathParentMustExist},
		{prefix: ">>", requirement: pathParentMustExist},
		{prefix: "&>", requirement: pathParentMustExist},
		{prefix: "1>", requirement: pathParentMustExist},
		{prefix: "2>", requirement: pathParentMustExist},
		{prefix: "0<", requirement: pathMustExist},
		{prefix: ">", requirement: pathParentMustExist},
		{prefix: "<", requirement: pathMustExist},
	}

	for _, operator := range operators {
		if !strings.HasPrefix(token, operator.prefix) || len(token) == len(operator.prefix) {
			continue
		}

		return pathCandidate{
			value:       strings.TrimSpace(token[len(operator.prefix):]),
			requirement: operator.requirement,
		}, true
	}

	return pathCandidate{}, false
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

func commandPathMatchesRequirement(path string, requirement pathRequirement) (bool, error) {
	if requirement == pathParentMustExist {
		parent := filepath.Dir(path)

		return commandPathExists(parent)
	}

	return commandPathExists(path)
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
