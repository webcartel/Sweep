package platform

// ValueKind is the type of one registry value, in the few kinds cleaning
// needs to tell apart.
type ValueKind int

const (
	// ValueString is REG_SZ: a plain string.
	ValueString ValueKind = iota
	// ValueExpandString is REG_EXPAND_SZ: a string with %VARS% in it.
	ValueExpandString
	// ValueDWord is REG_DWORD.
	ValueDWord
	// ValueOther is every type cleaning does not read.
	ValueOther
)

// Value is one registry value: its name, the data as text and its kind.
// The default value has the empty name, and a DWORD carries its decimal
// spelling in Data.
type Value struct {
	Name string
	Data string
	Kind ValueKind
}

// Registry is the part of the Windows registry cleaning needs: reading the
// branches it knows and removing the values and keys the user picked. The
// implementation for Windows lives behind a build tag, and the tests use an
// in-memory one.
//
// Keys are written in full, with the hive first: `HKEY_CURRENT_USER\...`.
// The Windows implementation reads them through the 64-bit view.
type Registry interface {
	// SubKeys answers the keys directly under key, in name order.
	SubKeys(key string) ([]string, error)
	// Values answers the values of key, the default value first.
	Values(key string) ([]Value, error)
	// DeleteValue removes one value of a key, leaving the key itself.
	DeleteValue(key, name string) error
	// DeleteKey removes a key that has no subkeys.
	DeleteKey(key string) error
	// KeyExists says whether a key is there.
	KeyExists(key string) (bool, error)
	// Export writes the branch to a .reg file, the way regedit restores it.
	Export(key, path string) error
}
