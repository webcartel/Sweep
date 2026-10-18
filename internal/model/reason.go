package model

// SkipReason says why a path or a rule was left alone. It reaches the report
// as the reason of a skipped item, so the values are the ones the
// specification names.
type SkipReason string

const (
	// ReasonInUse marks a file Windows would not let go.
	ReasonInUse SkipReason = "in_use"
	// ReasonAccessDenied marks a path this process may not touch.
	ReasonAccessDenied SkipReason = "access_denied"
	// ReasonProtected marks a path the guard refuses.
	ReasonProtected SkipReason = "protected"
	// ReasonAppRunning marks a rule whose program is still running.
	ReasonAppRunning SkipReason = "app_running"
	// ReasonNeedsAdmin marks a rule that wants an elevated process.
	ReasonNeedsAdmin SkipReason = "needs_admin"
	// ReasonOSTooOld marks a rule that wants a newer Windows.
	ReasonOSTooOld SkipReason = "os_too_old"
	// ReasonNotFound marks a path that is not there.
	ReasonNotFound SkipReason = "not_found"
	// ReasonSystemUnknown marks a rule that could not be asked what stands
	// in its way: a snapshot of the machine did not come through, and the
	// safe side of an unreadable machine is not to run.
	ReasonSystemUnknown SkipReason = "system_unknown"
)
