package match

import "testing"

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
