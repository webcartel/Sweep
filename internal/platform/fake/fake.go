// Package fake holds in-memory stand-ins for the platform interfaces, so that
// the core can be tested without a disk, a process list and the environment of
// the machine that runs the tests.
package fake

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/webcartel/sweep/internal/platform"
)

// FS is a file system in memory. Paths are compared the way Windows compares
// them: without case, and with either separator.
type FS struct {
	entries map[string]platform.Info
	// kids holds the entries of a directory by the folded path of that
	// directory, so that ReadDir looks up its few instead of walking all the
	// entries of the file system for them.
	kids    map[string][]platform.Info
	denied  map[string]error
	long    map[string]string
	drives  map[string]platform.DriveKind
	removed []string
	trashed []string
	pending []string
}

// NewFS returns an empty file system.
func NewFS() *FS {
	return &FS{
		entries: map[string]platform.Info{},
		kids:    map[string][]platform.Info{},
		long:    map[string]string{},
		drives:  map[string]platform.DriveKind{},
	}
}

// Deny teaches the fake to fail at one path with the error given, the way a
// directory a process holds or a key that is missing does on a real machine.
// The error answers for every Lstat and ReadDir of that path.
func (f *FS) Deny(path string, with error) {
	if f.denied == nil {
		f.denied = map[string]error{}
	}
	f.denied[fold(path)] = with
}

// Add puts one entry in the file system. Path is taken as written; Name is
// filled from it when left empty. Adding the same path again overwrites it
// instead of leaving it twice in the list of its directory.
func (f *FS) Add(info platform.Info) {
	if info.Name == "" {
		info.Name = lastElement(info.Path)
	}
	key := fold(info.Path)
	held := false
	if _, ok := f.entries[key]; ok {
		held = true
	}
	f.entries[key] = info
	if parent := parentOf(info.Path); parent != "" {
		parentKey := fold(parent)
		if held {
			for i, entry := range f.kids[parentKey] {
				if fold(entry.Path) == key {
					f.kids[parentKey][i] = info
				}
			}
		} else {
			f.kids[parentKey] = append(f.kids[parentKey], info)
		}
	}
}

// MapLong teaches LongPath that short is spelled long.
func (f *FS) MapLong(short, long string) { f.long[fold(short)] = long }

// SetDrive says what kind of drive the root of the path is. A drive that is
// not set is fixed, which is what a test usually wants.
func (f *FS) SetDrive(root string, kind platform.DriveKind) { f.drives[fold(root)] = kind }

// Removed lists the paths Remove has deleted, in order.
func (f *FS) Removed() []string { return append([]string(nil), f.removed...) }

// Trashed lists the paths Trash has deleted, in order.
func (f *FS) Trashed() []string { return append([]string(nil), f.trashed...) }

// Pending lists the paths RemoveOnReboot has scheduled, in order.
func (f *FS) Pending() []string { return append([]string(nil), f.pending...) }

// Lstat describes an entry in the file system.
func (f *FS) Lstat(path string) (platform.Info, error) {
	if err, ok := f.denied[fold(path)]; ok {
		return platform.Info{}, &fs.PathError{Op: "lstat", Path: path, Err: err}
	}
	info, ok := f.entries[fold(path)]
	if !ok {
		return platform.Info{}, &fs.PathError{Op: "lstat", Path: path, Err: fs.ErrNotExist}
	}
	return info, nil
}

// ReadDir lists what is inside a directory.
func (f *FS) ReadDir(path string) ([]platform.Info, error) {
	if err, ok := f.denied[fold(path)]; ok {
		return nil, &fs.PathError{Op: "readdir", Path: path, Err: err}
	}
	info, ok := f.entries[fold(path)]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: path, Err: fs.ErrNotExist}
	}
	if !info.IsDir {
		return nil, &fs.PathError{Op: "readdir", Path: path, Err: fs.ErrInvalid}
	}

	dir := fold(path)
	entries := make([]platform.Info, len(f.kids[dir]))
	copy(entries, f.kids[dir])
	sort.Slice(entries, func(i, j int) bool {
		a, b := strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name)
		if a == b {
			return entries[i].Name < entries[j].Name
		}
		return a < b
	})
	return entries, nil
}

// Glob returns the paths the mask takes.
func (f *FS) Glob(mask string) ([]string, error) { return platform.GlobPaths(f, mask) }

// LongPath gives the long spelling of a path: the 8.3 names it was taught are
// expanded at every level, the way Windows expands them.
func (f *FS) LongPath(path string) (string, error) {
	if long, ok := f.long[fold(path)]; ok {
		return long, nil
	}
	parent := parentOf(path)
	if parent == "" || fold(parent) == fold(path) {
		return path, nil
	}
	long, err := f.LongPath(parent)
	if err != nil {
		return "", err
	}
	if long == parent {
		return path, nil
	}
	return long + `\` + lastElement(path), nil
}

// DriveType answers what kind of drive holds the path.
func (f *FS) DriveType(path string) (platform.DriveKind, error) {
	if kind, ok := f.drives[fold(rootOf(path))]; ok {
		return kind, nil
	}
	return platform.DriveFixed, nil
}

// Remove deletes an entry, and only an empty directory: that is what os.Remove
// does, and the engine counts on the difference.
func (f *FS) Remove(path string) error { return f.delete(path, &f.removed) }

// Trash moves an entry to the recycle bin, which takes a directory together
// with everything inside it: unlike Remove, it does not ask for an empty one.
func (f *FS) Trash(path string) error {
	key := fold(path)
	if _, ok := f.entries[key]; !ok {
		return &fs.PathError{Op: "trash", Path: path, Err: fs.ErrNotExist}
	}
	f.drop(key)
	f.trashed = append(f.trashed, path)
	return nil
}

// RemoveOnReboot schedules an entry for the next start of Windows. It is not
// deleted now, which is the point of it.
func (f *FS) RemoveOnReboot(path string) error {
	f.pending = append(f.pending, path)
	return nil
}

// delete takes one entry out of the file system and writes it to a log.
func (f *FS) delete(path string, log *[]string) error {
	key := fold(path)
	if _, ok := f.entries[key]; !ok {
		return &fs.PathError{Op: "remove", Path: path, Err: fs.ErrNotExist}
	}
	if len(f.kids[key]) != 0 {
		return fmt.Errorf("remove %s: the directory is not empty", path)
	}
	f.unhook(key)
	delete(f.entries, key)
	*log = append(*log, path)
	return nil
}

// unhook takes one entry out of the list of children its directory holds.
func (f *FS) unhook(key string) {
	parent := fold(parentOf(f.entries[key].Path))
	kept := f.kids[parent][:0]
	for _, entry := range f.kids[parent] {
		if fold(entry.Path) != key {
			kept = append(kept, entry)
		}
	}
	f.kids[parent] = kept
}

// drop takes an entry and everything below it out of the file system.
func (f *FS) drop(key string) {
	for other := range f.entries {
		if other == key || strings.HasPrefix(other, key+`\`) {
			delete(f.entries, other)
		}
	}
	for dir, list := range f.kids {
		if dir == key || strings.HasPrefix(dir, key+`\`) {
			delete(f.kids, dir)
			continue
		}
		kept := list[:0]
		for _, entry := range list {
			if _, held := f.entries[fold(entry.Path)]; held {
				kept = append(kept, entry)
			}
		}
		f.kids[dir] = kept
	}
}

// System is the running Windows in memory.
type System struct {
	build    int
	elevated bool
}

// NewSystem returns a system that reports build 0 until SetBuild is called,
// and an elevated process until SetElevated(false) is called.
func NewSystem() *System { return &System{elevated: true} }

// SetBuild says what build the system runs.
func (s *System) SetBuild(build int) { s.build = build }

// OSBuild gives the build SetBuild set.
func (s *System) OSBuild() (int, error) { return s.build, nil }

// Elevated says whether the process runs with administrator rights. A system
// starts elevated because the tests of rules that require admin want to pass
// by default; SetElevated takes it away.
func (s *System) Elevated() (bool, error) { return s.elevated, nil }

// SetElevated says whether the system reports an elevated process.
func (s *System) SetElevated(v bool) { s.elevated = v }

// Procs is a process list in memory.
type Procs struct {
	running map[string]string
}

// NewProcs returns an empty process list.
func NewProcs() *Procs { return &Procs{running: map[string]string{}} }

// Set makes the named processes the running ones.
func (p *Procs) Set(names ...string) {
	for _, name := range names {
		p.running[strings.ToLower(name)] = name
	}
}

// Running returns those of names that are running.
func (p *Procs) Running(names []string) ([]string, error) {
	var found []string
	seen := map[string]bool{}
	for _, name := range names {
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		if _, ok := p.running[key]; !ok {
			continue
		}
		seen[key] = true
		found = append(found, name)
	}
	return found, nil
}

// Env is an environment in memory. Names are compared without case, the way
// Windows reads them.
type Env struct {
	values map[string]string
}

// NewEnv returns an empty environment.
func NewEnv() *Env { return &Env{values: map[string]string{}} }

// Set gives a variable a value.
func (e *Env) Set(name, value string) { e.values[strings.ToUpper(name)] = value }

// Get reads a variable.
func (e *Env) Get(name string) (string, bool) {
	value, ok := e.values[strings.ToUpper(name)]
	return value, ok
}

// fold is the key a path is stored under: separators become backslashes, a
// trailing separator goes away and case stops mattering, as on Windows.
func fold(path string) string {
	trimmed := strings.TrimRight(strings.ReplaceAll(path, "/", `\`), `\`)
	if trimmed == "" {
		return `\`
	}
	if strings.HasSuffix(trimmed, ":") {
		return strings.ToLower(trimmed) + `\`
	}
	return strings.ToLower(trimmed)
}

// lastElement is the name of the last segment of a path.
func lastElement(path string) string {
	trimmed := strings.TrimRight(strings.ReplaceAll(path, "/", `\`), `\`)
	if index := strings.LastIndex(trimmed, `\`); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}

// parentOf is the directory that holds a path.
func parentOf(path string) string {
	trimmed := strings.TrimRight(strings.ReplaceAll(path, "/", `\`), `\`)
	index := strings.LastIndex(trimmed, `\`)
	if index < 0 {
		return ""
	}
	if index == 2 && strings.HasSuffix(trimmed[:2], ":") {
		return trimmed[:3]
	}
	return trimmed[:index]
}

// rootOf is what DriveType can be asked about: the drive root of a lettered
// path, and the share of a UNC path.
func rootOf(path string) string {
	cleaned := strings.ReplaceAll(path, "/", `\`)
	if len(cleaned) >= 2 && cleaned[1] == ':' {
		return cleaned[:2] + `\`
	}
	if strings.HasPrefix(cleaned, `\\`) {
		parts := strings.SplitN(cleaned[2:], `\`, 3)
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			return `\\` + parts[0] + `\` + parts[1]
		}
	}
	return cleaned
}
