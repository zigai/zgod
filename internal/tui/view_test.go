package tui

import (
	"strings"
	"testing"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
)

func TestFormatMatchCountLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count int
		want  string
	}{
		{name: "zero", count: 0, want: "matches: 0"},
		{name: "one", count: 1, want: "matches: 1"},
		{name: "many", count: 2, want: "matches: 2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := formatMatchCountLabel(tc.count)
			if got != tc.want {
				t.Fatalf("formatMatchCountLabel(%d) = %q, want %q", tc.count, got, tc.want)
			}
		})
	}
}

func TestLayoutFooterLineFitsBoth(t *testing.T) {
	t.Parallel()

	got := layoutFooterLine("left", "right", 12)

	want := "left   right"
	if got != want {
		t.Fatalf("layoutFooterLine fit = %q, want %q", got, want)
	}
}

func TestLayoutFooterLineFallsBackToRightOnly(t *testing.T) {
	t.Parallel()

	got := layoutFooterLine("left-side", "count", 7)

	want := "  count"
	if got != want {
		t.Fatalf("layoutFooterLine fallback = %q, want %q", got, want)
	}
}

func TestRenderFooterShowsMatchCountAtNarrowWidth(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	m := &Model{
		cfg:            cfg,
		styles:         NewStyles(cfg.Theme),
		width:          12,
		displayEntries: make([]history.ScoredEntry, 3),
	}

	rendered := m.renderFooter()
	if !strings.Contains(rendered, "matches: 3") {
		t.Fatalf("renderFooter() = %q, expected to contain %q", rendered, "matches: 3")
	}
}

func TestFailToggleIndicator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mode       db.FailFilterMode
		wantBG     string
		wantActive bool
	}{
		{name: "include", mode: db.FailFilterInclude},
		{name: "exclude", mode: db.FailFilterExclude, wantBG: failExcludeIndicator, wantActive: true},
		{name: "only", mode: db.FailFilterOnly, wantBG: "9", wantActive: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := failToggleIndicator(tc.mode)
			if got.label != "fails" {
				t.Fatalf("failToggleIndicator(%v).label = %q, want %q", tc.mode, got.label, "fails")
			}

			if got.bg != tc.wantBG {
				t.Fatalf("failToggleIndicator(%v).bg = %q, want %q", tc.mode, got.bg, tc.wantBG)
			}

			if got.active != tc.wantActive {
				t.Fatalf("failToggleIndicator(%v).active = %t, want %t", tc.mode, got.active, tc.wantActive)
			}
		})
	}
}

func TestRenderHelpShowsFailFilterCycle(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	m := &Model{
		cfg:    cfg,
		styles: NewStyles(cfg.Theme),
		width:  80,
	}

	rendered := m.renderHelp()
	if !strings.Contains(rendered, "Cycle fail filter (include/exclude/only)") {
		t.Fatalf("renderHelp() = %q, expected fail filter help text", rendered)
	}
}
