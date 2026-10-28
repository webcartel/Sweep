//go:build windows

package platform

import (
	"strings"

	"golang.org/x/sys/windows"
)

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

// muiLanguageName asks GetUserPreferredUILanguages for BCP-47 names
// rather than for raw LANGID numbers.
const muiLanguageName = 0x8

// UILanguage answers the language of the Windows user interface as "en" or
// "ru". Windows names its preferred languages in order, and the first one
// is the language the user reads the interface in: "ru-RU" here, with
// "en-US" after it. Only the primary subtag is asked, so every Russian
// locale counts as Russian. Every other language answers "en": sweep
// speaks English when nothing says it speaks Russian.
func (system) UILanguage() string {
	names, err := windows.GetUserPreferredUILanguages(muiLanguageName)
	if err != nil || len(names) == 0 {
		return "en"
	}
	primary, _, _ := strings.Cut(names[0], "-")
	if strings.EqualFold(primary, "ru") {
		return "ru"
	}
	return "en"
}
