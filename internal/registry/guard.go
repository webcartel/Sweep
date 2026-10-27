package registry

import (
	"strings"
)

// The parent branches cleaning may read and change. Everything else is
// refused: a registry cleaner that can be pointed at any key is a way to
// break Windows.
var guardedParents = []string{
	`HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Uninstall`,
	`HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`,
	`HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
	`HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`,
	`HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`,
	`HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`,
	`HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\App Paths`,
	`HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths`,
	`HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\App Paths`,
}

// Guarded says whether a key sits inside one of the parent branches. The
// parent itself is guarded too: a Run value is deleted through its parent
// key, and the guard is asked about the key rather than the value.
func Guarded(key string) bool {
	normalized := normalizeKey(key)
	for _, parent := range guardedParents {
		parent = strings.ToLower(parent)
		if normalized == parent || strings.HasPrefix(normalized, parent+`\`) {
			return true
		}
	}
	return false
}

// normalizeKey folds a key path for comparison: the long hive names become
// the short ones, separators are one way, and case stops mattering.
func normalizeKey(key string) string {
	key = strings.ReplaceAll(strings.TrimSpace(key), "/", `\`)
	key = strings.TrimSuffix(key, `\`)
	upper := strings.ToUpper(key)
	switch {
	case strings.HasPrefix(upper, `HKEY_CURRENT_USER\`):
		key = `HKEY_CURRENT_USER` + key[len(`HKEY_CURRENT_USER`):]
	case upper == `HKCU`:
		key = `HKEY_CURRENT_USER`
	case strings.HasPrefix(upper, `HKCU\`):
		key = `HKEY_CURRENT_USER` + key[len(`HKCU`):]
	case strings.HasPrefix(upper, `HKEY_LOCAL_MACHINE\`):
		key = `HKEY_LOCAL_MACHINE` + key[len(`HKEY_LOCAL_MACHINE`):]
	case upper == `HKLM`:
		key = `HKEY_LOCAL_MACHINE`
	case strings.HasPrefix(upper, `HKLM\`):
		key = `HKEY_LOCAL_MACHINE` + key[len(`HKLM`):]
	}
	return strings.ToLower(key)
}
