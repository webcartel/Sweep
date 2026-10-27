//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// NewRegistry returns the registry of this machine.
func NewRegistry() (Registry, error) { return windowsRegistry{}, nil }

// windowsRegistry is the registry as Windows shows it: the 64-bit view of
// every key, so that WOW6432Node is a plain path rather than a second
// enumeration.
type windowsRegistry struct{}

// openKey reads the hive and the rest of a full key path.
func openKey(key string, access uint32) (registry.Key, error) {
	hive, rest, err := splitHive(key)
	if err != nil {
		return 0, err
	}
	return registry.OpenKey(hive, rest, access|registry.WOW64_64KEY)
}

// splitHive answers the hive constant and the path under it.
func splitHive(key string) (registry.Key, string, error) {
	parts := strings.SplitN(strings.TrimPrefix(key, `\`), `\`, 2)
	if len(parts) == 0 {
		return 0, "", fmt.Errorf("the key %q has no hive", key)
	}
	rest := ""
	if len(parts) == 2 {
		rest = parts[1]
	}
	switch strings.ToUpper(parts[0]) {
	case "HKEY_CURRENT_USER", "HKCU":
		return registry.CURRENT_USER, rest, nil
	case "HKEY_LOCAL_MACHINE", "HKLM":
		return registry.LOCAL_MACHINE, rest, nil
	case "HKEY_CLASSES_ROOT", "HKCR":
		return registry.CLASSES_ROOT, rest, nil
	case "HKEY_USERS", "HKU":
		return registry.USERS, rest, nil
	case "HKEY_CURRENT_CONFIG", "HKCC":
		return registry.CURRENT_CONFIG, rest, nil
	}
	return 0, "", fmt.Errorf("the key %q names no hive", key)
}

// SubKeys answers the keys directly under key, in name order, with the
// parent path in front.
func (windowsRegistry) SubKeys(key string) ([]string, error) {
	open, err := openKey(key, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, err
	}
	defer func() { _ = open.Close() }()
	names, err := open.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, joinKey(key, name))
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out, nil
}

// Values answers the values of key: the default value first, then the rest
// in name order.
func (windowsRegistry) Values(key string) ([]Value, error) {
	open, err := openKey(key, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer func() { _ = open.Close() }()
	names, err := open.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == "" {
			return true
		}
		if names[j] == "" {
			return false
		}
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	out := make([]Value, 0, len(names))
	for _, name := range names {
		value, kind := readValue(open, name)
		out = append(out, Value{Name: name, Data: value, Kind: kind})
	}
	return out, nil
}

// readValue reads one value in the kind cleaning understands; a value of
// another kind comes back as ValueOther with no data.
func readValue(key registry.Key, name string) (string, ValueKind) {
	_, kind, err := key.GetValue(name, nil)
	if err != nil {
		return "", ValueOther
	}
	switch kind {
	case registry.SZ, registry.EXPAND_SZ:
		data, _, err := key.GetStringValue(name)
		if err != nil {
			return "", ValueOther
		}
		if kind == registry.EXPAND_SZ {
			return data, ValueExpandString
		}
		return data, ValueString
	case registry.DWORD:
		data, _, err := key.GetIntegerValue(name)
		if err != nil {
			return "", ValueOther
		}
		return fmt.Sprintf("%d", data), ValueDWord
	}
	return "", ValueOther
}

// DeleteValue removes one value and leaves the key.
func (windowsRegistry) DeleteValue(key, name string) error {
	open, err := openKey(key, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = open.Close() }()
	return open.DeleteValue(name)
}

// DeleteKey removes a key that has no subkeys.
func (windowsRegistry) DeleteKey(key string) error {
	hive, rest, err := splitHive(key)
	if err != nil {
		return err
	}
	return registry.DeleteKey(hive, rest)
}

// KeyExists says whether a key opens for reading.
func (windowsRegistry) KeyExists(key string) (bool, error) {
	open, err := openKey(key, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	_ = open.Close()
	return true, nil
}

// Export writes the branch to a .reg file with reg.exe, which runs as the
// same elevation as this process.
func (windowsRegistry) Export(key, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	cmd := exec.Command("reg.exe", "export", key, path, "/y")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reg export %s: %v: %s", key, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// joinKey puts a subkey name under its parent.
func joinKey(parent, name string) string {
	return strings.TrimSuffix(parent, `\`) + `\` + name
}
