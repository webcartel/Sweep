package model

// Risk says how much a rule can cost if it is wrong.
type Risk string

const (
	// RiskSafe marks data that comes back on its own.
	RiskSafe Risk = "safe"
	// RiskModerate marks data that comes back, but at a cost.
	RiskModerate Risk = "moderate"
	// RiskDangerous marks data the user may still want.
	RiskDangerous Risk = "dangerous"
)

// Kind says what a target names: a path pattern or an external command.
type Kind string

const (
	// KindPath is a path with %VARS% and glob masks.
	KindPath Kind = "path"
	// KindCommand is an external command.
	KindCommand Kind = "command"
)

// Mode says what is removed under a matched path.
type Mode string

const (
	// ModeContents removes what is inside the matched directories.
	ModeContents Mode = "contents"
	// ModeSelf removes the matched object itself.
	ModeSelf Mode = "self"
	// ModeFiles removes the files that match include.
	ModeFiles Mode = "files"
)

// Rule is one cleaning rule, as written in a rule file.
type Rule struct {
	ID, Name, Description, Category string
	Risk                            Risk
	DefaultEnabled, RequiresAdmin   bool
	Trash                           bool
	RequiresClosed, Tags            []string
	OSMinBuild                      int
	Targets                         []Target
	Source                          string // "builtin" or the path of the file it came from
}

// Target is one thing a rule removes.
type Target struct {
	Kind       Kind
	Path       string // as written in YAML, before %VARS% are expanded
	Mode       Mode
	Include    []string
	Exclude    []string
	MinAgeDays int
	MinSize    int64
	Cmd        string
	Args       []string
}
