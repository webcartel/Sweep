//go:build !windows

package platform

// system holds nothing: sweep cleans only Windows.
type system struct{}

// NewSystem answers with ErrNotWindows: only the Windows build can tell the
// build of the running system.
func NewSystem() (System, error) { return nil, ErrNotWindows }
