package match

import (
	"slices"
	"testing"
)

func TestFuzzyMatcher(t *testing.T) {
	m := &FuzzyMatcher{}
	candidates := []string{"git checkout", "git commit", "go build", "echo hello"}

	matches := m.Match("gco", candidates)
	if len(matches) == 0 {
		t.Fatal("expected fuzzy matches for 'gco'")
	}

	found := false

	for _, match := range matches {
		if candidates[match.Index] == "git checkout" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected 'git checkout' to match 'gco'")
	}
}

func TestFuzzyScoreASCIIMatchesRuneScorer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern   string
		candidate string
	}{
		{pattern: "gco", candidate: "git checkout"},
		{pattern: "gb", candidate: "go build"},
		{pattern: "gtp", candidate: "go test ./pkg"},
		{pattern: "fb", candidate: "foo-bar"},
		{pattern: "fb", candidate: "FooBarBaz"},
		{pattern: "fbb", candidate: "FooBarBaz"},
		{pattern: "zz", candidate: "git checkout"},
	}

	for _, tc := range tests {
		t.Run(tc.pattern+"/"+tc.candidate, func(t *testing.T) {
			t.Parallel()

			asciiScore, asciiOK, ascii := fuzzyScoreASCII(lowerASCIIBytes(tc.pattern), tc.candidate)
			if !ascii {
				t.Fatalf("fuzzyScoreASCII(%q, %q) reported non-ASCII", tc.pattern, tc.candidate)
			}

			runeScore, runeOK := fuzzyScore([]rune(tc.pattern), tc.candidate)
			if asciiScore != runeScore || asciiOK != runeOK {
				t.Fatalf(
					"fuzzyScoreASCII() = (%d, %t), fuzzyScore() = (%d, %t)",
					asciiScore,
					asciiOK,
					runeScore,
					runeOK,
				)
			}
		})
	}
}

func TestFuzzyMatcherMatchesCamelCase(t *testing.T) {
	t.Parallel()

	m := &FuzzyMatcher{}
	candidates := []string{"foo-bar_baz qux", "FooBarBaz", "git checkout"}

	matches := m.Match("fbb", candidates)
	if len(matches) != 2 {
		t.Fatalf("Match(%q) returned %d matches, want 2: %+v", "fbb", len(matches), matches)
	}

	if matches[0].Index != 0 || matches[1].Index != 1 {
		t.Fatalf("Match(%q) indexes = %d, %d; want 0, 1", "fbb", matches[0].Index, matches[1].Index)
	}
}

func TestFuzzyMatcherIndexedMatchesFullSearchSubset(t *testing.T) {
	t.Parallel()

	m := &FuzzyMatcher{}
	candidates := []string{
		"git checkout feature/login",
		"git commit -m initial",
		"go test ./internal/tui",
		"docker compose up -d",
		"git checkout bugfix/fuzzy-score",
	}
	indexes := []int{0, 2, 4}

	full := m.Match("gct", candidates)
	indexed := m.MatchIndexed("gct", candidates, indexes)

	var want []Match

	for _, match := range full {
		if slices.Contains(indexes, match.Index) {
			want = append(want, match)
		}
	}

	if len(indexed) != len(want) {
		t.Fatalf("MatchIndexed() returned %d matches, want %d: %+v", len(indexed), len(want), indexed)
	}

	for i := range want {
		if indexed[i].Index != want[i].Index || indexed[i].Score != want[i].Score {
			t.Fatalf("MatchIndexed()[%d] = %+v, want %+v", i, indexed[i], want[i])
		}
	}
}

func TestRegexMatcher(t *testing.T) {
	m := &RegexMatcher{}
	candidates := []string{"git checkout", "git commit", "go build", "echo hello"}

	matches := m.Match("^git", candidates)
	if len(matches) != 2 {
		t.Errorf("regex '^git' matched %d candidates, want 2", len(matches))
	}

	matches = m.Match("", candidates)
	if len(matches) != 0 {
		t.Error("empty regex should return no matches")
	}

	matches = m.Match("[invalid", candidates)
	if len(matches) != 0 {
		t.Error("invalid regex should return no matches")
	}
}

func TestRegexMatcherUsesRuneRanges(t *testing.T) {
	m := &RegexMatcher{}
	candidates := []string{"héllo"}

	matches := m.Match("é", candidates)
	if len(matches) != 1 {
		t.Fatalf("regex 'é' matched %d candidates, want 1", len(matches))
	}

	if len(matches[0].MatchedRanges) != 1 {
		t.Fatalf("matched ranges = %d, want 1", len(matches[0].MatchedRanges))
	}

	got := matches[0].MatchedRanges[0]

	want := Range{Start: 1, End: 2}
	if got != want {
		t.Fatalf("matched range = %+v, want %+v", got, want)
	}
}

func TestRegexMatcherLiteralFastPathMatchesCaseInsensitiveSubstring(t *testing.T) {
	m := &RegexMatcher{}
	candidates := []string{"Git Checkout", "go build", "docker compose"}

	matches := m.MatchCommands("git", candidates, nil)
	if len(matches) != 1 {
		t.Fatalf("literal regex matched %d candidates, want 1", len(matches))
	}

	if matches[0].Index != 0 {
		t.Fatalf("literal regex matched index %d, want 0", matches[0].Index)
	}
}

func TestRegexMatcherLiteralUnicodeFoldDifferential(t *testing.T) {
	m := &RegexMatcher{}

	candidates := []string{
		"baſic command",     // long s: 'ſ' folds with 's'
		"greek word: βάσιϛ", // Greek final sigma: 'ς' folds with 'σ'
		"regular basic",
		"invalid utf8: \xff\xfe abc",
	}

	tests := []struct {
		name    string
		pattern string
	}{
		{name: "ascii s matches long s", pattern: "basic"},
		{name: "long s matches ascii s", pattern: "baſic"},
		{name: "sigma matches final sigma", pattern: "βάσισ"},
		{name: "final sigma matches sigma", pattern: "βάσιϛ"},
		{name: "pattern in invalid utf8 candidate", pattern: "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullMatches := m.Match(tt.pattern, candidates)
			literalMatches := m.MatchCommands(tt.pattern, candidates, nil)

			if len(literalMatches) != len(fullMatches) {
				t.Fatalf("literalMatches count = %d, fullMatches count = %d", len(literalMatches), len(fullMatches))
			}

			for i := range fullMatches {
				if literalMatches[i].Index != fullMatches[i].Index {
					t.Fatalf("literalMatches[%d].Index = %d, fullMatches[%d].Index = %d",
						i, literalMatches[i].Index, i, fullMatches[i].Index)
				}

				if literalMatches[i].Score != 100 {
					t.Fatalf("literalMatches[%d].Score = %d, want 100", i, literalMatches[i].Score)
				}

				if literalMatches[i].MatchedRanges != nil {
					t.Fatalf("literalMatches[%d].MatchedRanges = %+v, want nil", i, literalMatches[i].MatchedRanges)
				}
			}
		})
	}
}

func TestGlobMatcher(t *testing.T) {
	m := &GlobMatcher{}
	candidates := []string{"git checkout", "git commit", "go build", "echo hello"}

	matches := m.Match("git *", candidates)
	if len(matches) != 2 {
		t.Errorf("glob 'git *' matched %d candidates, want 2", len(matches))
	}

	matches = m.Match("", candidates)
	if len(matches) != 0 {
		t.Error("empty glob should return no matches")
	}
}

func TestGlobMatcherSimpleStarFastPath(t *testing.T) {
	m := &GlobMatcher{}
	candidates := []string{
		"git checkout feature && go test internal-package",
		"go test internal-package",
		"git status",
	}

	matches := m.Match("git*test*", candidates)
	if len(matches) != 1 {
		t.Fatalf("glob 'git*test*' matched %d candidates, want 1", len(matches))
	}

	if matches[0].Index != 0 {
		t.Fatalf("glob 'git*test*' matched index %d, want 0", matches[0].Index)
	}
}

func TestGlobMatcherRepeatedTerminalLiterals(t *testing.T) {
	m := &GlobMatcher{}
	candidates := []string{
		"foo bar foo",
		"git commit git",
		"a b a c a",
		"bar a",
	}

	matches := m.Match("*foo*foo", candidates)
	if len(matches) != 1 || matches[0].Index != 0 {
		t.Fatalf("glob '*foo*foo' matches = %+v, want index 0", matches)
	}

	matches = m.Match("*git*git", candidates)
	if len(matches) != 1 || matches[0].Index != 1 {
		t.Fatalf("glob '*git*git' matches = %+v, want index 1", matches)
	}

	matches = m.Match("*a*a", candidates)
	if len(matches) != 2 {
		t.Fatalf("glob '*a*a' matches = %+v, want 2 matches", matches)
	}
}

func TestModeNext(t *testing.T) {
	all := []Mode{ModeFuzzy, ModeRegex, ModeGlob}
	if ModeFuzzy.Next(all) != ModeRegex {
		t.Error("fuzzy.Next() should be regex")
	}

	if ModeRegex.Next(all) != ModeGlob {
		t.Error("regex.Next() should be glob")
	}

	if ModeGlob.Next(all) != ModeFuzzy {
		t.Error("glob.Next() should be fuzzy")
	}

	partial := []Mode{ModeFuzzy, ModeGlob}
	if ModeFuzzy.Next(partial) != ModeGlob {
		t.Error("fuzzy.Next(partial) should be glob")
	}

	if ModeGlob.Next(partial) != ModeFuzzy {
		t.Error("glob.Next(partial) should be fuzzy")
	}
}

func TestNew(t *testing.T) {
	if _, ok := New(ModeFuzzy).(*FuzzyMatcher); !ok {
		t.Error("New(ModeFuzzy) should return *FuzzyMatcher")
	}

	if _, ok := New(ModeRegex).(*RegexMatcher); !ok {
		t.Error("New(ModeRegex) should return *RegexMatcher")
	}

	if _, ok := New(ModeGlob).(*GlobMatcher); !ok {
		t.Error("New(ModeGlob) should return *GlobMatcher")
	}
}
