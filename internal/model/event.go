package model

// Event is one step of a scan or an execution. The set is closed: only the
// kinds below send on the stream, and the frontend reads until the channel
// closes.
type Event interface{ isEvent() }

func (RuleStarted) isEvent()  {}
func (RuleProgress) isEvent() {}
func (RuleScanned) isEvent()  {}
func (ScanDone) isEvent()     {}
func (RuleFinished) isEvent() {}
func (ItemSkipped) isEvent()  {}
func (ItemFailed) isEvent()   {}
func (Warning) isEvent()      {}

// RuleStarted comes before the scanner touches a rule.
type RuleStarted struct{ RuleID string }

// RuleProgress tells how much one rule has accumulated. The scanner emits it
// at most once every 100 ms per rule.
type RuleProgress struct {
	RuleID string
	Files  int
	Bytes  int64
}

// RuleScanned carries the finished plan of one rule.
type RuleScanned struct{ Plan RulePlan }

// ScanDone carries the whole plan. Exactly one ScanDone closes a scan that
// ran to the end.
type ScanDone struct{ Plan *Plan }

// ItemSkipped reports one path the scanner or the executor could not take.
type ItemSkipped struct {
	RuleID string
	Skip   Skip
}

// ItemFailed reports one path whose removal went wrong.
type ItemFailed struct {
	RuleID string
	Path   string
	Err    error
}

// Warning carries a remark about one rule. Warnings go to stderr when the
// frontend prints JSON.
type Warning struct {
	RuleID  string
	Message string
}
