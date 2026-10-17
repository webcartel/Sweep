//go:build !windows

package platform

// NewFS answers with ErrNotWindows: only the Windows build can touch a file
// system, and the other builds exist so that the packages without WinAPI keep
// compiling and their tests keep running.
func NewFS() (FS, error) { return nil, ErrNotWindows }
