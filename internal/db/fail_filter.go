package db

type FailFilterMode int

const (
	FailFilterInclude FailFilterMode = iota
	FailFilterExclude
	FailFilterOnly
)

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
