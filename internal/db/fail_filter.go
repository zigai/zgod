package db

type FailFilterMode int

const (
	FailFilterInclude FailFilterMode = iota
	FailFilterExclude
	FailFilterOnly
)

func (m FailFilterMode) String() string {
	switch m {
	case FailFilterInclude:
		return "include"
	case FailFilterExclude:
		return "exclude"
	case FailFilterOnly:
		return "only"
	}

	return "include"
}

func ParseFailFilterMode(s string) (FailFilterMode, bool) {
	switch s {
	case "", "include":
		return FailFilterInclude, true
	case "exclude":
		return FailFilterExclude, true
	case "only":
		return FailFilterOnly, true
	default:
		return FailFilterInclude, false
	}
}

func (m FailFilterMode) Next() FailFilterMode {
	switch m {
	case FailFilterInclude:
		return FailFilterExclude
	case FailFilterExclude:
		return FailFilterOnly
	case FailFilterOnly:
		return FailFilterInclude
	}

	return FailFilterInclude
}
