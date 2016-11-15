package main

type FlagType int

const (
	_ FlagType = iota
	FlagUnknown
	FlagError
	FlagSkipped
	FlagImage
	FlagVideo
)

var flagTypeValues = []string{
	FlagUnknown: "?",
	FlagError:   "X",
	FlagSkipped: ".",
	FlagImage:   "i",
	FlagVideo:   "v",
}

func (t FlagType) String() string {
	if t <= 0 || int(t) >= len(flagTypeValues) {
		return ""
	}
	return flagTypeValues[t]
}

func (t FlagType) IsLoggable() bool {
	return t == FlagUnknown || t == FlagError || t == FlagSkipped
}

func (t FlagType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}
