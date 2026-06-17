package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
	"github.com/zigai/zgod/internal/match"
)

const wideTestRune = "\u754c"

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

func TestRenderFooterUsesDefaultConfiguredKeys(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	m := &Model{
		cfg:            cfg,
		styles:         NewStyles(cfg.Theme),
		width:          200,
		displayEntries: make([]history.ScoredEntry, 1),
	}

	rendered := m.renderFooter()
	for _, needle := range []string{
		"ctrl+d",
		"cwd",
		"ctrl+g",
		"dedup",
		"ctrl+s",
		"mode",
	} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("renderFooter() = %q, expected to contain %q", rendered, needle)
		}
	}
}

func TestRenderFooterUsesRemappedKeys(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Keys.ToggleCWD = "alt+c"
	cfg.Keys.ToggleDedupe = "alt+d"
	cfg.Keys.ModeNext = "alt+m"

	m := &Model{
		cfg:            cfg,
		styles:         NewStyles(cfg.Theme),
		width:          200,
		displayEntries: make([]history.ScoredEntry, 1),
	}

	rendered := m.renderFooter()
	for _, needle := range []string{"alt+c", "alt+d", "alt+m"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("renderFooter() = %q, expected to contain %q", rendered, needle)
		}
	}

	for _, needle := range []string{"ctrl+d cwd", "ctrl+g dedup", "ctrl+s mode"} {
		if strings.Contains(rendered, needle) {
			t.Fatalf("renderFooter() = %q, should not contain stale footer hint %q", rendered, needle)
		}
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
		{name: "include", mode: db.FailFilterInclude, wantBG: failIncludeIndicator, wantActive: true},
		{name: "exclude", mode: db.FailFilterExclude},
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

func TestRenderPreviewPanePreservesUTF8(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	m := &Model{
		cfg:    cfg,
		styles: NewStyles(cfg.Theme),
		width:  1,
		cursor: 0,
		displayEntries: []history.ScoredEntry{
			{Entry: db.HistoryEntry{Command: "ž"}},
		},
	}

	rendered := m.renderPreviewPane()
	if !utf8.ValidString(rendered) {
		t.Fatalf("renderPreviewPane() produced invalid UTF-8: %q", rendered)
	}
}

func TestRenderExpandedResultLinesPreservesUTF8(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	m := &Model{
		cfg:    cfg,
		styles: NewStyles(cfg.Theme),
		width:  1,
		displayEntries: []history.ScoredEntry{
			{Entry: db.HistoryEntry{Command: "ž"}},
		},
	}

	rendered := strings.Join(m.renderExpandedResultLines(0), "\n")
	if !utf8.ValidString(rendered) {
		t.Fatalf("renderExpandedResultLines() produced invalid UTF-8: %q", rendered)
	}
}

func TestRenderResultLineFitsWideUnicodeCommand(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	m := &Model{
		cfg:    cfg,
		styles: NewStyles(cfg.Theme),
		width:  40,
		displayEntries: []history.ScoredEntry{
			{Entry: db.HistoryEntry{Command: strings.Repeat(wideTestRune, 10)}},
		},
	}

	layout := m.calcResultLayout()

	rendered := m.renderResultLineWithLayout(0, false, layout, time.Unix(0, 0))
	if got := lipgloss.Width(rendered); got > layout.width {
		t.Fatalf("renderResultLineWithLayout() width = %d, want <= %d: %q", got, layout.width, rendered)
	}
}

func TestTrimToWidthUsesCellWidth(t *testing.T) {
	t.Parallel()

	want := strings.Repeat(wideTestRune, 2)

	got := trimToWidth(want+"a", 4)
	if got != want {
		t.Fatalf("trimToWidth() = %q, want %q", got, want)
	}

	if width := lipgloss.Width(got); width > 4 {
		t.Fatalf("trimToWidth() width = %d, want <= 4", width)
	}
}

func TestWrapToWidthUsesCellWidth(t *testing.T) {
	t.Parallel()

	wide := strings.Repeat(wideTestRune, 2)
	got := wrapToWidth(wide+"ab", 4)

	want := []string{wide, "ab"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("wrapToWidth() = %q, want %q", got, want)
	}

	for _, line := range got {
		if width := lipgloss.Width(line); width > 4 {
			t.Fatalf("wrapToWidth() line width = %d, want <= 4: %q", width, line)
		}
	}
}

func TestFormatDirectoryTruncatesUnicodeWithoutPanic(t *testing.T) {
	t.Parallel()

	got := formatDirectory(strings.Repeat("é", 10), 12, "")
	if !utf8.ValidString(got) {
		t.Fatalf("formatDirectory() produced invalid UTF-8: %q", got)
	}
}

func TestFormatDirectoryOnlyAbbreviatesActualHome(t *testing.T) {
	t.Parallel()

	got := formatDirectory("/home/me2/project", 80, "/home/me")
	if got != "/home/me2/project" {
		t.Fatalf("formatDirectory() = %q, want original path", got)
	}

	got = formatDirectory("/home/me/project", 80, "/home/me")
	if got != "~/project" {
		t.Fatalf("formatDirectory() = %q, want home abbreviation", got)
	}

	got = formatDirectory(`C:\Users\me2\project`, 80, `C:\Users\me`)
	if got != `C:\Users\me2\project` {
		t.Fatalf("formatDirectory() = %q, want original Windows path", got)
	}

	got = formatDirectory(`C:\Users\me\project`, 80, `C:\Users\me`)
	if got != `~\project` {
		t.Fatalf("formatDirectory() = %q, want Windows home abbreviation", got)
	}
}

func TestRenderPreviewPopupPreservesUTF8(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	m := &Model{
		cfg:            cfg,
		styles:         NewStyles(cfg.Theme),
		width:          5,
		showPreview:    true,
		previewCommand: "žž",
	}

	rendered := m.renderPreviewPopup()
	if !utf8.ValidString(rendered) {
		t.Fatalf("renderPreviewPopup() produced invalid UTF-8: %q", rendered)
	}
}

func TestRenderResultsClipsExpandedMultilineToHeight(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Display.MultilinePreview = "expand"

	m := &Model{
		cfg:    cfg,
		styles: NewStyles(cfg.Theme),
		width:  80,
		height: 3,
		cursor: 0,
		displayEntries: []history.ScoredEntry{
			{Entry: db.HistoryEntry{Command: "one\ntwo\nthree\nfour"}},
			{Entry: db.HistoryEntry{Command: "tail"}},
		},
	}

	rendered := m.renderResults()
	if got := len(strings.Split(rendered, "\n")); got != m.height {
		t.Fatalf("renderResults() returned %d lines, want %d", got, m.height)
	}
}

func TestFuzzyRenderRanges(t *testing.T) {
	t.Parallel()

	got := fuzzyRenderRanges("gch", "git checkout")
	want := []match.Range{
		{Start: 0, End: 1},
		{Start: 4, End: 6},
	}

	if len(got) != len(want) {
		t.Fatalf("len(fuzzyRenderRanges()) = %d, want %d: %+v", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fuzzyRenderRanges()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestRegexRenderRangesUsesRuneRanges(t *testing.T) {
	t.Parallel()

	got := regexRenderRanges("é", "héllo")
	want := []match.Range{{Start: 1, End: 2}}

	if len(got) != len(want) {
		t.Fatalf("len(regexRenderRanges()) = %d, want %d: %+v", len(got), len(want), got)
	}

	if got[0] != want[0] {
		t.Fatalf("regexRenderRanges()[0] = %+v, want %+v", got[0], want[0])
	}
}

func TestRegexRenderRangesLiteralFastPath(t *testing.T) {
	t.Parallel()

	got := regexRenderRanges("git", "Git git")
	want := []match.Range{
		{Start: 0, End: 3},
		{Start: 4, End: 7},
	}

	if len(got) != len(want) {
		t.Fatalf("len(regexRenderRanges()) = %d, want %d: %+v", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("regexRenderRanges()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestTruncateWithRangesCentersHiddenMatch(t *testing.T) {
	t.Parallel()

	info := match.Match{
		MatchedRanges: []match.Range{{Start: 29, End: 35}},
	}

	got, gotInfo := truncateWithRanges("prefix text that is too long needle suffix", &info, 20)

	if !strings.Contains(got, "needle") {
		t.Fatalf("truncateWithRanges() text = %q, expected to contain hidden match", got)
	}

	if !strings.HasPrefix(got, "...") {
		t.Fatalf("truncateWithRanges() text = %q, expected prefix ellipsis", got)
	}

	if gotInfo == nil || len(gotInfo.MatchedRanges) != 1 {
		t.Fatalf("truncateWithRanges() ranges = %+v, want one remapped range", gotInfo)
	}

	r := gotInfo.MatchedRanges[0]
	if got[r.Start:r.End] != "needle" {
		t.Fatalf("remapped range points to %q, want needle in %q", got[r.Start:r.End], got)
	}
}

func TestTruncateWithRangesUsesCellWidth(t *testing.T) {
	t.Parallel()

	want := wideTestRune + "..."

	got, gotInfo := truncateWithRanges(strings.Repeat(wideTestRune, 3)+"a", nil, 5)
	if got != want {
		t.Fatalf("truncateWithRanges() text = %q, want %q", got, want)
	}

	if gotInfo != nil {
		t.Fatalf("truncateWithRanges() match info = %+v, want nil", gotInfo)
	}

	if width := lipgloss.Width(got); width > 5 {
		t.Fatalf("truncateWithRanges() width = %d, want <= 5", width)
	}
}

func TestMatchCountLabelShowsPartialZeroMatchesWhenFullHistoryPending(t *testing.T) {
	t.Parallel()

	m := &Model{
		allEntries: []db.HistoryEntry{
			{Command: "recent command"},
		},
		historyComplete: false,
	}

	if got, want := m.matchCountLabel(), "matches: 0+"; got != want {
		t.Fatalf("matchCountLabel() = %q, want %q", got, want)
	}
}
