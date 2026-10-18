package model

import "time"

// The statuses a rule can leave a run with. A rule in a dry run finishes as
// dry_run; otherwise it is done, skipped or failed.
const (
	StatusDone    = "done"
	StatusSkipped = "skipped"
	StatusFailed  = "failed"
	StatusDryRun  = "dry_run"
)

// Selection is the user's choice over a fixed plan. Rules maps a rule id to
// whether it runs; Exclude lists item paths deselected in the detail view.
// The plan itself never changes after the scan.
type Selection struct {
	Rules   map[string]bool
	Exclude map[string]bool
}

// ExecOptions tunes one execution. Trash = nil keeps each rule's own choice;
// a pointer overrides it. Workers = 0 means runtime.NumCPU(). Now moves the
// clock for the report; nil means time.Now.
type ExecOptions struct {
	DryRun   bool
	Trash    *bool
	OnReboot bool
	Workers  int
	Now      func() time.Time
}

// RuleResult tells how one rule fared in a run.
type RuleResult struct {
	Rule    *Rule
	Status  string
	Bytes   int64 // freed on the disk, or the amount a dry run would free
	Files   int64
	Skipped []Skip
}

// Result is what one execution leaves behind. In a dry run the counters mean
// the amount that would be freed, and the tree stays untouched.
type Result struct {
	StartedAt  time.Time
	FinishedAt time.Time
	DryRun     bool
	// Items, Files and BytesFound come from the plan the run came out of:
	// what was on offer, as its scan counted it.
	Items         int
	Files         int64
	BytesFound    int64
	FreedBytes    int64
	TrashBytes    int64
	Removed       int
	Skipped       int
	Failed        int
	PerRule       []RuleResult
	PendingReboot []string
}

// ItemRemoved reports one path the executor took off the disk, or dealt
// with in a dry run.
type ItemRemoved struct {
	RuleID  string
	Path    string
	Bytes   int64
	ToTrash bool
}

// ExecDone carries the finished result. Exactly one ExecDone closes an
// execution that ran to the end.
type ExecDone struct{ Result *Result }

// RuleFinished tells how one rule fared, as the run of it closes. It reaches
// the stream so that a frontend shows the result while the others run on.
type RuleFinished struct{ Result RuleResult }

func (ItemRemoved) isEvent() {}
func (ExecDone) isEvent()    {}
