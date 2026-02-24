package history

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/zigai/zgod/internal/config"
)

var errInvalidDirectoryGlobPattern = errors.New("invalid directory glob pattern")

type Filter struct {
	ignoreSpace      bool
	exitCode         map[int]bool
	commandGlob      []*regexp.Regexp
	commandRegex     []*regexp.Regexp
	directoryGlob    []string
	directoryRegex   []*regexp.Regexp
	maxCommandLength int
}

func NewFilter(cfg config.FilterConfig) (*Filter, error) {
	codes := map[int]bool{}
	for _, c := range cfg.ExitCode {
		codes[c] = true
	}

	cmdGlobs := make([]*regexp.Regexp, 0, len(cfg.CommandGlob))
	for _, g := range cfg.CommandGlob {
		re, err := globToRegexp(g)
		if err != nil {
			return nil, err
		}

		cmdGlobs = append(cmdGlobs, re)
	}

	cmdRegexps := make([]*regexp.Regexp, 0, len(cfg.CommandRegex))
	for _, pattern := range cfg.CommandRegex {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("compiling command regex %q: %w", pattern, err)
		}

		cmdRegexps = append(cmdRegexps, re)
	}

	for _, g := range cfg.DirectoryGlob {
		if !doublestar.ValidatePattern(g) {
			return nil, fmt.Errorf("%w: %s", errInvalidDirectoryGlobPattern, g)
		}
	}

	dirRegexps := make([]*regexp.Regexp, 0, len(cfg.DirectoryRegex))
	for _, pattern := range cfg.DirectoryRegex {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("compiling directory regex %q: %w", pattern, err)
		}

		dirRegexps = append(dirRegexps, re)
	}

	return &Filter{
		ignoreSpace:      cfg.IgnoreSpace,
		exitCode:         codes,
		commandGlob:      cmdGlobs,
		commandRegex:     cmdRegexps,
		directoryGlob:    cfg.DirectoryGlob,
		directoryRegex:   dirRegexps,
		maxCommandLength: cfg.MaxCommandLength,
	}, nil
}

func (f *Filter) ShouldRecord(command string, exitCode int, directory string) bool {
	if strings.TrimSpace(command) == "" {
		return false
	}

	if f.maxCommandLength > 0 && len(command) > f.maxCommandLength {
		return false
	}

	if f.ignoreSpace && strings.HasPrefix(command, " ") {
		return false
	}

	if f.exitCode[exitCode] {
		return false
	}

	if f.matchesCommandFilters(command) {
		return false
	}

	if f.matchesDirectoryFilters(directory) {
		return false
	}

	return true
}

func (f *Filter) matchesCommandFilters(command string) bool {
	for _, glob := range f.commandGlob {
		if glob.MatchString(command) {
			return true
		}
	}

	for _, re := range f.commandRegex {
		if re.MatchString(command) {
			return true
		}
	}

	return false
}

func (f *Filter) matchesDirectoryFilters(directory string) bool {
	for _, g := range f.directoryGlob {
		if matched, _ := doublestar.Match(g, directory); matched {
			return true
		}
	}

	for _, re := range f.directoryRegex {
		if re.MatchString(directory) {
			return true
		}
	}

	return false
}

func globToRegexp(glob string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")

	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '.', '+', '(', ')', '{', '}', '[', ']', '^', '$', '|', '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}

	b.WriteString("$")

	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("compiling generated regex for glob %q: %w", glob, err)
	}

	return re, nil
}
