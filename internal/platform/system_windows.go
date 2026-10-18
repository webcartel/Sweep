//go:build windows

package platform

import "golang.org/x/sys/windows"

// system answers for the running Windows.
type system struct{}

// NewSystem returns the system of the machine the process runs on.
func NewSystem() (System, error) { return system{}, nil }

// OSBuild gives the build number of the running Windows.
//
// RtlGetVersion is asked and not GetVersion: without a manifest that names
// the newer GUIDs, GetVersion reports 6.2 for every Windows since Windows 8,
// and os_min_build would then drop rules that do apply.
func (system) OSBuild() (int, error) {
	version := windows.RtlGetVersion()
	return int(version.BuildNumber), nil
}

// Elevated says whether the process runs with administrator rights, by the
// token of the process itself and not of the shell it came from.
func (system) Elevated() (bool, error) {
	return windows.GetCurrentProcessToken().IsElevated(), nil
}
