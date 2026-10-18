package model

import (
	"context"
	"time"
)

// ScanOptions says how a scan runs. Workers of 0 mean runtime.NumCPU(),
// Throttle of 0 means 100 ms, and Now is read on every throttle decision, so
// tests can hand a clock of their own.
type ScanOptions struct {
	Workers  int
	Throttle time.Duration
	Now      func() time.Time
}

// Scanner turns rules into a plan without touching anything on disk. The
// returned stream is buffered, closed by the scanner itself, and the plan it
// hands over in ScanDone stays the same from then on.
type Scanner interface {
	Scan(ctx context.Context, rules []*Rule, opt ScanOptions) <-chan Event
}
