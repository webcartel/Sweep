//go:build !windows

package platform

// NewProcs answers with ErrNotWindows: only the Windows build can read the
// process list.
func NewProcs() (Procs, error) { return nil, ErrNotWindows }
