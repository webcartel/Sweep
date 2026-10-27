//go:build !windows

package platform

// NewRegistry answers the error of the platform away from Windows.
func NewRegistry() (Registry, error) { return nil, ErrNotWindows }
