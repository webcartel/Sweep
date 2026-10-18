package model

import "time"

// Item is one unit of deletion. Without file filters it is a top level entry
// (a whole directory, with size and file count accumulated); with filters
// (include, exclude, min_age_days, min_size) it is a single file.
type Item struct {
	Path    string
	Size    int64
	Files   int
	IsDir   bool
	ModTime time.Time
	Reparse bool // a junction or a symlink: the link itself is removed, not the target
}

// Skip says why one path did not make it into the plan.
type Skip struct {
	Path   string
	Reason SkipReason
	Detail string
}

// CommandPlan is the plan of a rule whose target runs a command. Size is not
// known in advance.
type CommandPlan struct {
	Command string
	Args    []string
}

// RulePlan is what one rule would remove.
type RulePlan struct {
	Rule    *Rule
	Items   []Item
	Bytes   int64
	Files   int
	Skipped []Skip
	Command *CommandPlan // for kind: command
}

// Plan is what the scanner hands the executor. After ScanDone it does not
// change: the user's choice lives separately, in a Selection.
type Plan struct {
	Rules     []RulePlan
	CreatedAt time.Time
}
