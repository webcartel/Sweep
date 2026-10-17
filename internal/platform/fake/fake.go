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
		long:    map[string]string{},
		drives:  map[string]platform.DriveKind{},
	}
}

// Add puts one entry in the file system. Path is taken as written; Name is
// filled from it when left empty.
func (f *FS) Add(info platform.Info) {
	if info.Name == "" {
		info.Name = lastElement(info.Path)
	}
	f.entries[fold(info.Path)] = info
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
	info, ok := f.entries[fold(path)]
	if !ok {
		return platform.Info{}, &fs.PathError{Op: "lstat", Path: path, Err: fs.ErrNotExist}
	}
	return info, nil
}

// ReadDir lists what is inside a directory.
func (f *FS) ReadDir(path string) ([]platform.Info, error) {
	info, ok := f.entries[fold(path)]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: path, Err: fs.ErrNotExist}
	}
	if !info.IsDir {
		return nil, &fs.PathError{Op: "readdir", Path: path, Err: fs.ErrInvalid}
	}

	dir := fold(path)
	var entries []platform.Info
	for key, entry := range f.entries {
		if key != dir && fold(parentOf(entry.Path)) == dir {
			entries = append(entries, entry)
		}
	}
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

// LongPath gives the long spelling of a path.
func (f *FS) LongPath(path string) (string, error) {
	if long, ok := f.long[fold(path)]; ok {
		return long, nil
	}
	return path, nil
}

// DriveType answers what kind of drive holds the path.
func (f *FS) DriveType(path string) (platform.DriveKind, error) {
	if kind, ok := f.drives[fold(rootOf(path))]; ok {
		return kind, nil
	}
	return platform.DriveFixed, nil
}

// Remove deletes an entry.
func (f *FS) Remove(path string) error { return f.delete(path, &f.removed) }

// Trash moves an entry to the recycle bin.
func (f *FS) Trash(path string) error { return f.delete(path, &f.trashed) }

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
	for other := range f.entries {
		if other != key && fold(parentOf(f.entries[other].Path)) == key {
			return fmt.Errorf("remove %s: the directory is not empty", path)
		}
	}
	delete(f.entries, key)
	*log = append(*log, path)
	return nil
}

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

// rootOf is the drive root of a path.
func rootOf(path string) string {
	cleaned := strings.ReplaceAll(path, "/", `\`)
	if len(cleaned) >= 2 && cleaned[1] == ':' {
		return cleaned[:2] + `\`
	}
	return cleaned
}
