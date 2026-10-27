package registry

import (
	"strings"
)

// Kind names the three branches cleaning knows.
type Kind string

const (
	// KindUninstall is a key under Uninstall whose uninstall command is gone.
	KindUninstall Kind = "uninstall"
	// KindRun is one value of a Run key whose program is gone.
	KindRun Kind = "run"
	// KindAppPath is a key under App Paths whose default program is gone.
	KindAppPath Kind = "apppath"
)

// Finding is one dead registry entry, with everything the screen and the
// report need to explain it.
type Finding struct {
	Kind Kind
	ID   string
	Name string
	// Key is the full path of the key the finding is in.
	Key string
	// Value is the value name; empty for uninstall and App Paths.
	Value string
	// Path is the command or program path that is gone.
	Path string
	// Reason explains why the finding is a candidate.
	Reason string
	// Admin says the key lives under HKLM and needs elevation.
	Admin bool
	// HasSubkeys says the key cannot be deleted whole.
	HasSubkeys bool
	// DeleteValue says the cleaner deletes one value, not the key.
	DeleteValue bool
}

// findingID is the stable id of a finding: kind, key and value, folded.
func findingID(kind Kind, key, value string) string {
	return string(kind) + "|" + strings.ToLower(key) + "|" + strings.ToLower(value)
}

// isAdminKey says whether a key needs the elevated process.
func isAdminKey(key string) bool {
	upper := strings.ToUpper(key)
	return strings.HasPrefix(upper, `HKEY_LOCAL_MACHINE`) || strings.HasPrefix(upper, `HKLM\`)
}
