package exec

import (
	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/platform"
)

// Executor takes the plan of a scan, asks one last time for every path, and
// puts each of them away the way the rule and the options say.
type Executor struct {
	fs    platform.FS
	guard *guard.Guard
}

// NullProcs stands for a process list that always answers nothing, for the
// builders that have nothing to give.
type NullProcs struct{}

func (NullProcs) Running([]string) ([]string, error) { return nil, nil }
