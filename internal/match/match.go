package match

type Mode int

const (
	ModeFuzzy Mode = iota
	ModeRegex
	ModeGlob
)

func (m Mode) String() string {
	switch m {
	case ModeFuzzy:
		return "fuzzy"
	case ModeRegex:
		return "regex"
	case ModeGlob:
		return "glob"
	default:
		return "unknown"
	}
}

func ParseMode(s string) (Mode, bool) {
	switch s {
	case "fuzzy", "":
		return ModeFuzzy, true
	case "regex":
		return ModeRegex, true
	case "glob":
		return ModeGlob, true
	default:
		return ModeFuzzy, false
	}
}

func (m Mode) Next(enabled []Mode) Mode {
	if len(enabled) == 0 {
		return m
	}
	for i, mode := range enabled {
		if mode == m {
			return enabled[(i+1)%len(enabled)]
		}
	}
	return enabled[0]
}

type Range struct {
	Start int
	End   int
}

type Match struct {
	Index         int
	Score         int
	MatchedRanges []Range
}

type Matcher interface {
	Match(pattern string, candidates []string) []Match
}

func New(mode Mode) Matcher {
	switch mode {
	case ModeFuzzy:
		return &FuzzyMatcher{}
	case ModeRegex:
		return &RegexMatcher{}
	case ModeGlob:
		return &GlobMatcher{}
	default:
		return &FuzzyMatcher{}
	}
}
