package model

import "testing"

func TestExecEventsLiveInEventSet(t *testing.T) {
	var _ Event = ItemRemoved{}
	var _ Event = ExecDone{}
}

func TestStatusConstantsMatchTheSpec(t *testing.T) {
	for status, got := range map[string]string{
		"done":    StatusDone,
		"skipped": StatusSkipped,
		"failed":  StatusFailed,
		"dry_run": StatusDryRun,
	} {
		if got != status {
			t.Errorf("Status constant = %q, want %q", got, status)
		}
	}
}

func TestExecOptionsZeroWorkersAndNilNowAreTheDefaults(t *testing.T) {
	var opt ExecOptions
	if opt.Workers != 0 || opt.Now != nil || opt.DryRun || opt.OnReboot {
		t.Errorf("zero ExecOptions = %+v, want all defaults", opt)
	}
	if opt.Trash != nil {
		t.Error("zero ExecOptions.Trash = non-nil, want nil")
	}
}
