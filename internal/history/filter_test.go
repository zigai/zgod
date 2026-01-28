package history

import (
	"testing"

	"github.com/zigai/zgod/internal/config"
)

func TestFilterEmpty(t *testing.T) {
	f, _ := NewFilter(config.FilterConfig{})
	if f.ShouldRecord("", 0, "") {
		t.Error("empty command should not be recorded")
	}
	if f.ShouldRecord("   ", 0, "") {
		t.Error("whitespace-only command should not be recorded")
	}
}

func TestFilterLeadingSpace(t *testing.T) {
	f, _ := NewFilter(config.FilterConfig{IgnoreSpace: true})
	if f.ShouldRecord(" ls", 0, "") {
		t.Error("leading space command should not be recorded when IgnoreSpace=true")
	}
	if !f.ShouldRecord("ls", 0, "") {
		t.Error("normal command should be recorded")
	}
}

func TestFilterExitCode(t *testing.T) {
	f, _ := NewFilter(config.FilterConfig{ExitCode: []int{130}})
	if f.ShouldRecord("interrupted", 130, "") {
		t.Error("exit code 130 should be filtered")
	}
	if !f.ShouldRecord("success", 0, "") {
		t.Error("exit code 0 should not be filtered")
	}
}

func TestFilterGlob(t *testing.T) {
	f, _ := NewFilter(config.FilterConfig{CommandGlob: []string{"cd *", "ls", "exit"}})
	if f.ShouldRecord("cd /tmp", 0, "") {
		t.Error("'cd /tmp' should be filtered by glob 'cd *'")
	}
	if f.ShouldRecord("ls", 0, "") {
		t.Error("'ls' should be filtered by glob 'ls'")
	}
	if !f.ShouldRecord("git status", 0, "") {
		t.Error("'git status' should not be filtered")
	}
}

func TestFilterRegex(t *testing.T) {
	f, _ := NewFilter(config.FilterConfig{CommandRegex: []string{`^\s*$`}})
	if f.ShouldRecord("   ", 0, "") {
		t.Error("whitespace command should be filtered by regex")
	}
}

func TestFilterInvalidRegex(t *testing.T) {
	_, err := NewFilter(config.FilterConfig{CommandRegex: []string{"[invalid"}})
	if err == nil {
		t.Error("invalid regex should return error")
	}
}

func TestFilterCombined(t *testing.T) {
	f, _ := NewFilter(config.FilterConfig{
		IgnoreSpace: true,
		ExitCode:    []int{130},
		CommandGlob: []string{"exit"},
	})
	if !f.ShouldRecord("git commit -m 'test'", 0, "") {
		t.Error("normal command should be recorded")
	}
}

func TestFilterDirGlob(t *testing.T) {
	f, _ := NewFilter(config.FilterConfig{DirectoryGlob: []string{"/tmp/**"}})
	if f.ShouldRecord("ls", 0, "/tmp/foo/bar") {
		t.Error("command in /tmp/foo/bar should be filtered by dir glob '/tmp/**'")
	}
	if !f.ShouldRecord("ls", 0, "/home/user") {
		t.Error("command in /home/user should not be filtered")
	}
}

func TestFilterDirRegexp(t *testing.T) {
	f, _ := NewFilter(config.FilterConfig{DirectoryRegex: []string{`^/tmp`}})
	if f.ShouldRecord("ls", 0, "/tmp/test") {
		t.Error("command in /tmp/test should be filtered by dir regexp '^/tmp'")
	}
	if !f.ShouldRecord("ls", 0, "/home/user") {
		t.Error("command in /home/user should not be filtered")
	}
}

func TestFilterInvalidDirGlob(t *testing.T) {
	_, err := NewFilter(config.FilterConfig{DirectoryGlob: []string{"[invalid"}})
	if err == nil {
		t.Error("invalid directory glob should return error")
	}
}
