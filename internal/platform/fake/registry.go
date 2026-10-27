package fake

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/webcartel/sweep/internal/platform"
)

// Registry is a registry in memory: keys, their values, and the calls that
// changed them. Paths are compared the way Windows compares them: without
// case.
type Registry struct {
	mu sync.Mutex
	// values holds the values of each key by the folded key path.
	values map[string]map[string]platform.Value
	// names keeps the name of each key as it was added.
	names map[string]string
	// FailExport answers an error for the named key.
	FailExport map[string]error
	// Operations records what the caller did, in order: "export <key>",
	// "delete-value <key>\\<name>", "delete-key <key>".
	Operations []string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		values: map[string]map[string]platform.Value{},
		names:  map[string]string{},
	}
}

// AddKey creates a key and its parents.
func (r *Registry) AddKey(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addKey(key)
}

func (r *Registry) addKey(key string) {
	folded := fold(key)
	if _, ok := r.values[folded]; ok {
		return
	}
	r.values[folded] = map[string]platform.Value{}
	r.names[folded] = key
	if parent := parentKey(key); parent != "" {
		r.addKey(parent)
	}
}

// AddValue puts a value into a key, creating the key when it is not there.
func (r *Registry) AddValue(key, name string, value platform.Value) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addKey(key)
	value.Name = name
	r.values[fold(key)][fold(name)] = value
}

// SubKeys answers the keys directly under key, in name order, with the full
// path.
func (r *Registry) SubKeys(key string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []string{}
	for folded, name := range r.names {
		if parentKey(name) != "" && fold(parentKey(name)) == fold(key) && folded != fold(key) {
			out = append(out, name)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out, nil
}

// Values answers the values of a key, the default value first.
func (r *Registry) Values(key string) ([]platform.Value, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	values, ok := r.values[fold(key)]
	if !ok {
		return nil, fmt.Errorf("the key %q is not there", key)
	}
	out := make([]platform.Value, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == "" {
			return true
		}
		if out[j].Name == "" {
			return false
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// DeleteValue removes one value of a key.
func (r *Registry) DeleteValue(key, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.values[fold(key)]; !ok {
		return fmt.Errorf("the key %q is not there", key)
	}
	delete(r.values[fold(key)], fold(name))
	r.Operations = append(r.Operations, "delete-value "+key+`\`+name)
	return nil
}

// DeleteKey removes a key that has no subkeys.
func (r *Registry) DeleteKey(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range r.names {
		if parentKey(name) != "" && fold(parentKey(name)) == fold(key) && fold(name) != fold(key) {
			return fmt.Errorf("the key %q has subkeys", key)
		}
	}
	delete(r.values, fold(key))
	delete(r.names, fold(key))
	r.Operations = append(r.Operations, "delete-key "+key)
	return nil
}

// KeyExists says whether a key is there.
func (r *Registry) KeyExists(key string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.values[fold(key)]
	return ok, nil
}

// Export records the export and writes a .reg file with the branch name.
func (r *Registry) Export(key, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Operations = append(r.Operations, "export "+key)
	if err, ok := r.FailExport[key]; ok {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("Windows Registry Editor Version 5.00\r\n\r\n["+key+"]\r\n"), 0o600)
}

// parentKey answers the key one level up, or nothing for a hive.
func parentKey(key string) string {
	trimmed := strings.TrimSuffix(key, `\`)
	if i := strings.LastIndex(trimmed, `\`); i > 0 {
		return trimmed[:i]
	}
	return ""
}
