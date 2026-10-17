package guard

import (
	"fmt"
	"strings"

	"github.com/webcartel/sweep/internal/platform"
)

// rangeExtent says how far a range of the protected list reaches.
type rangeExtent int

const (
	// extentSelf covers the path itself and nothing below it.
	extentSelf rangeExtent = iota
	// extentInside covers what is inside the path, but not the path itself.
	extentInside
	// extentBelow covers the path and everything below it.
	extentBelow
)

// rangeRule says what a range does with a path it covers.
type rangeRule int

const (
	// ruleForbid refuses the path, in every mode.
	ruleForbid rangeRule = iota
	// ruleFiles refuses the path unless the rule is `mode: files` with an
	// include filter and an age filter: that is the one way the specification
	// lets a rule reach into a root or a user directory.
	ruleFiles
	// ruleAllow lets the path through and stands in front of a wider range
	// that refuses.
	ruleAllow
)

// protectedRange is one entry of the protected list.
type protectedRange struct {
	path   string
	extent rangeExtent
	rule   rangeRule
}

// covers says whether a range decides a path.
func (r protectedRange) covers(path string) bool {
	same := pathKey(path) == pathKey(r.path)
	switch r.extent {
	case extentSelf:
		return same
	case extentInside:
		return !same && below(path, r.path)
	default:
		return same || below(path, r.path)
	}
}

// permits says whether a range lets a rule through. filtered is true for the
// one shape the specification allows into a filtered range: `mode: files`
// with an include filter and an age filter.
func (r protectedRange) permits(filtered bool) bool {
	switch r.rule {
	case ruleAllow:
		return true
	case ruleFiles:
		return filtered
	default:
		return false
	}
}

// userDirectories are the directories inside a profile that hold what a person
// put there, rather than what a program cached.
var userDirectories = []string{
	`Desktop`, `Documents`, `Pictures`, `Videos`, `Music`, `Downloads`, `OneDrive`,
}

// perDrive are the names every drive root carries. The drive is read from the
// path in front of the guard, not from a list of the machine's drives.
var perDrive = []string{`$Recycle.Bin`, `System Volume Information`, `EFI`, `Recovery`}

// rangesFor is the protected list that decides one path: the ranges of this
// machine, and the ones that stand at every drive root.
func (g *Guard) rangesFor(path string) []protectedRange {
	root, ok := driveRootOf(path)
	if !ok {
		return g.ranges
	}

	ranges := make([]protectedRange, 0, len(g.ranges)+len(perDrive)+1)
	ranges = append(ranges, g.ranges...)
	ranges = append(ranges, protectedRange{path: root, extent: extentSelf, rule: ruleFiles})
	for _, name := range perDrive {
		ranges = append(ranges, protectedRange{path: root + name, extent: extentBelow, rule: ruleForbid})
	}
	return ranges
}

// match finds the range that decides a path. The longest one wins, and at the
// same length the stricter one does, so that an exception cannot be widened by
// writing it a second time.
func (g *Guard) match(path string) (protectedRange, bool) {
	var best protectedRange
	found := false
	for _, r := range g.rangesFor(path) {
		if !r.covers(path) {
			continue
		}
		longer := found && len(r.path) > len(best.path)
		stricter := found && len(r.path) == len(best.path) && r.rule < best.rule
		if !found || longer || stricter {
			best, found = r, true
		}
	}
	return best, found
}

// protectedRanges is the protected list of a machine. The environment says
// where Windows and the user live; a variable that is not set falls back to
// the place Windows puts it by default.
func protectedRanges(env platform.Env, extra []string) []protectedRange {
	value := func(name, fallback string) string {
		if set, ok := env.Get(name); ok && set != "" {
			// A value that ends with a separator or is written with the other
			// one is the same place, and the comparison below is literal.
			return cleanPath(deviceFree(set))
		}
		return fallback
	}

	systemRoot := value(`SystemRoot`, `C:\Windows`)
	profile := value(`USERPROFILE`, ``)
	programFiles := value(`ProgramFiles`, `C:\Program Files`)
	programFilesX86 := value(`ProgramFiles(x86)`, `C:\Program Files (x86)`)

	ranges := []protectedRange{
		// The roots themselves: they may not be deleted and may not be
		// emptied, and only a `mode: files` rule with both filters reaches
		// into one.
		{path: programFiles, extent: extentSelf, rule: ruleFiles},
		{path: programFilesX86, extent: extentSelf, rule: ruleFiles},
		{path: value(`ProgramData`, `C:\ProgramData`), extent: extentSelf, rule: ruleFiles},
		// Windows itself is refused whole, and the directories named below
		// are the only way in: the specification calls that list an explicit
		// one on top of the ban on C:\Windows, and extending it is a change to
		// this file with the tests that go with it (346).
		{path: systemRoot, extent: extentBelow, rule: ruleForbid},
		// C:\Users is refused whole, and the profile in front of the guard is
		// the one exception inside it: the other profiles are not this
		// machine's to clean.
		{path: `C:\Users`, extent: extentBelow, rule: ruleForbid},

		// The subtrees of Windows, and everything inside the program
		// directories: nothing there is touched, in any mode. WinSxS is
		// DISM's to clean, and the rest is Windows' own.
		{path: joinPath(systemRoot, `System32`), extent: extentBelow, rule: ruleForbid},
		{path: joinPath(systemRoot, `SysWOW64`), extent: extentBelow, rule: ruleForbid},
		{path: joinPath(systemRoot, `WinSxS`), extent: extentBelow, rule: ruleForbid},
		{path: joinPath(systemRoot, `Boot`), extent: extentBelow, rule: ruleForbid},
		{path: joinPath(systemRoot, `Fonts`), extent: extentBelow, rule: ruleForbid},
		{path: joinPath(systemRoot, `Recovery`), extent: extentBelow, rule: ruleForbid},
		{path: programFiles, extent: extentInside, rule: ruleForbid},
		{path: programFilesX86, extent: extentInside, rule: ruleForbid},

		// The explicit list inside Windows: the places the specification
		// names as safe to clean. Extending it is a change to this file, with
		// the tests that go with it.
		{path: joinPath(systemRoot, `Temp`), extent: extentBelow, rule: ruleAllow},
		{path: joinPath(systemRoot, `Logs`), extent: extentBelow, rule: ruleAllow},
		{path: joinPath(systemRoot, `Minidump`), extent: extentBelow, rule: ruleAllow},
		{path: joinPath(systemRoot, `SoftwareDistribution\Download`), extent: extentBelow, rule: ruleAllow},
		{path: joinPath(systemRoot, `ServiceProfiles\NetworkService\AppData\Local\Microsoft\Windows\DeliveryOptimization`), extent: extentBelow, rule: ruleAllow},
		// The one file in the system root the specification cleans, and the
		// reason the ban above is on the tree rather than on the file: the
		// rule names it by its own name, in `mode: self`.
		{path: joinPath(systemRoot, `MEMORY.DMP`), extent: extentSelf, rule: ruleAllow},
	}

	if profile != "" {
		ranges = append(ranges,
			// The profile itself is a root like the others, and what is
			// inside it is the user's own: the exception that makes the
			// profile usable at all.
			protectedRange{path: profile, extent: extentSelf, rule: ruleFiles},
			protectedRange{path: profile, extent: extentBelow, rule: ruleAllow},
		)
		for _, name := range userDirectories {
			ranges = append(ranges, protectedRange{path: joinPath(profile, name), extent: extentBelow, rule: ruleFiles})
		}
		ranges = append(ranges,
			protectedRange{path: value(`APPDATA`, joinPath(profile, `AppData\Roaming`)), extent: extentSelf, rule: ruleFiles},
			protectedRange{path: value(`LOCALAPPDATA`, joinPath(profile, `AppData\Local`)), extent: extentSelf, rule: ruleFiles},
		)
		if oneDrive := value(`OneDrive`, ``); oneDrive != "" {
			ranges = append(ranges, protectedRange{path: oneDrive, extent: extentBelow, rule: ruleFiles})
		}
	}

	// The config's own exclusions join the same list and can only forbid:
	// the mechanism has no way to allow with them.
	for _, path := range extra {
		ranges = append(ranges, protectedRange{path: path, extent: extentBelow, rule: ruleForbid})
	}
	return ranges
}

// exclusion turns a path from the config into the prefix it forbids. Unlike a
// target it does not have to exist, or to sit on a fixed disk: it only takes
// part in a comparison.
func (g *Guard) exclusion(entry string) (string, error) {
	expanded := platform.Expand(entry, g.env)
	if names := platform.EnvNames(expanded); len(names) > 0 {
		return "", fmt.Errorf("exclude_paths %s names %%%s%%, which this machine does not set", quote(entry), names[0])
	}

	text := deviceFree(expanded)
	cleaned := cleanPath(text)
	if cleaned == "" {
		return "", fmt.Errorf("exclude_paths %s names no place at all", quote(entry))
	}
	// An entry is a path, not a mask: the specification lets the config add to
	// the refusal list and nothing more. A mask that refuses nothing would be
	// a config that loads and quietly does not do what it says.
	if strings.ContainsAny(cleaned, `*?`) {
		return "", fmt.Errorf("exclude_paths %s is a mask, and the list takes paths", quote(entry))
	}
	if !isAbsPath(text) {
		cleaned = g.absolute(text, cleaned)
	}
	if long, err := g.fs.LongPath(cleaned); err == nil {
		cleaned = long
	}
	return cleaned, nil
}
