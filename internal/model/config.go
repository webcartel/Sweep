package model

// Config is the user's config file: the rules it switches on or off, the paths
// it never touches, whether the rules a user wrote may run commands, and the
// profiles it adds to the built-in ones.
type Config struct {
	Rules             map[string]RuleToggle
	ExcludePaths      []string
	AllowUserCommands bool
	Profiles          map[string]Profile
}

// RuleToggle switches one rule on or off by its id.
type RuleToggle struct {
	Enabled bool
}

// Profile picks rules out of the merged set.
type Profile struct {
	Include           []string // id masks
	IncludeTags       []string
	IncludeCategories []string
	MaxRisk           Risk     // empty takes every risk
	Exclude           []string // id masks that win over the selectors
	// DefaultOnly adds the rules of the default profile, the ones written with
	// default_enabled: true, to the selection. Only the built-in profiles set
	// it; a profile written in the config does not.
	DefaultOnly bool
}
