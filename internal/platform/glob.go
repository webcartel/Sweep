package platform

import (
	"errors"
	"io/fs"
	"sort"
	"strings"
)

// ErrMaskHasNoRoot is returned for a mask that has no literal directory to
// walk from: without one there is nothing to look at.
var ErrMaskHasNoRoot = errors.New("the mask has no literal prefix to walk from")

// GlobPaths turns a mask into the paths that exist and that it takes.
//
// The mask is expected to be absolute and already free of %VARS%. `**` is
// turned into directories here, which is what CompileMask leaves to its
// caller: the walk follows every level below the point where it stands, and
// the compiled mask then decides which of the paths found are really taken.
// The walk never enters a reparse point, so a junction is a match of its own
// but not a way through.
func GlobPaths(fsys FS, mask string) ([]string, error) {
	compiled, err := CompileMask(mask)
	if err != nil {
		return nil, err
	}

	// A mask with no mask character names one path: there is nothing to walk.
	if !strings.ContainsAny(mask, "*?") {
		taken, err := exists(fsys, mask)
		if err != nil {
			return nil, err
		}
		if !taken {
			return nil, nil
		}
		return []string{mask}, nil
	}

	start, segments, err := maskRoot(mask)
	if err != nil {
		return nil, err
	}

	// The walk does not go through a junction, and a mask that starts inside
	// one is no exception: what stands there is a link, and following it would
	// take the walk somewhere the guard never agreed to look. A mask that names
	// the link itself has no mask character and was answered above.
	if info, ok, err := readInfo(fsys, start); err != nil {
		return nil, err
	} else if ok && info.Reparse {
		return nil, nil
	}

	w := &walker{fsys: fsys, mask: compiled, taken: map[string]string{}}
	if err := w.walk([]string{start}, segments); err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(w.taken))
	for _, path := range w.taken {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		a, b := strings.ToLower(paths[i]), strings.ToLower(paths[j])
		if a == b {
			return paths[i] < paths[j]
		}
		return a < b
	})
	return paths, nil
}

// maskRoot splits a mask into the literal directory the walk starts from and
// the segments that are still to be matched.
func maskRoot(mask string) (string, []string, error) {
	segments := strings.Split(strings.TrimRight(mask, `\`), `\`)

	first := 0
	for first < len(segments) && !strings.ContainsAny(segments[first], "*?") {
		first++
	}
	if first == 0 {
		return "", nil, ErrMaskHasNoRoot
	}

	literal := strings.Join(segments[:first], `\`)
	if strings.HasSuffix(literal, ":") {
		// "C:" is the root of a drive, and it needs its separator to be a
		// path a directory can be read from.
		literal += `\`
	}
	return literal, segments[first:], nil
}

// walker carries one walk of the file system for one mask.
type walker struct {
	fsys  FS
	mask  *Mask
	taken map[string]string // folded path -> the path as it was found
}

// walk takes the directories that stand at the current boundary and the
// segments of the mask that are still to be matched.
func (w *walker) walk(dirs []string, segments []string) error {
	head, rest := segments[0], segments[1:]

	for _, dir := range dirs {
		switch {
		case strings.Contains(head, "**"):
			// `**` takes one or more levels, so every entry below the
			// boundary is a candidate, and every directory below it is a
			// boundary for what comes next.
			entries, err := w.below(dir)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if len(rest) == 0 {
					if err := w.add(entry.Path); err != nil {
						return err
					}
					continue
				}
				if entry.IsDir && !entry.Reparse {
					if err := w.walk([]string{entry.Path}, rest); err != nil {
						return err
					}
				}
			}

		case strings.ContainsAny(head, "*?"):
			entries, err := w.inside(dir)
			if err != nil {
				return err
			}
			segment, err := CompileMask(head)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if !segment.Match(entry.Name) {
					continue
				}
				if len(rest) == 0 {
					if err := w.add(entry.Path); err != nil {
						return err
					}
					continue
				}
				if entry.IsDir && !entry.Reparse {
					if err := w.walk([]string{entry.Path}, rest); err != nil {
						return err
					}
				}
			}

		default:
			path := joinPath(dir, head)
			if len(rest) == 0 {
				if err := w.add(path); err != nil {
					return err
				}
				continue
			}
			info, ok, err := readInfo(w.fsys, path)
			if err != nil {
				return err
			}
			if ok && info.IsDir && !info.Reparse {
				if err := w.walk([]string{path}, rest); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// add keeps a path when the whole mask takes it and it exists.
func (w *walker) add(path string) error {
	if !w.mask.Match(path) {
		return nil
	}
	taken, err := exists(w.fsys, path)
	if err != nil {
		return err
	}
	if taken {
		w.taken[foldPath(path)] = path
	}
	return nil
}

// below lists every entry under a directory, at any depth, without entering a
// reparse point.
func (w *walker) below(dir string) ([]Info, error) {
	entries, err := w.inside(dir)
	if err != nil {
		return nil, err
	}
	all := entries
	for _, entry := range entries {
		if entry.IsDir && !entry.Reparse {
			deeper, err := w.below(entry.Path)
			if err != nil {
				return nil, err
			}
			all = append(all, deeper...)
		}
	}
	return all, nil
}

// inside lists what a directory holds. A directory that is not there, or that
// cannot be read, holds nothing rather than being an error: a mask over a tree
// that is missing simply finds nothing.
func (w *walker) inside(dir string) ([]Info, error) {
	entries, err := w.fsys.ReadDir(dir)
	if err != nil {
		if softError(err) {
			return nil, nil
		}
		return nil, err
	}
	return entries, nil
}

// exists says whether a path is there.
func exists(fsys FS, path string) (bool, error) {
	_, ok, err := readInfo(fsys, path)
	return ok, err
}

// readInfo describes a path, and answers "not there" instead of failing for
// the errors that only mean the path cannot be a match.
func readInfo(fsys FS, path string) (Info, bool, error) {
	info, err := fsys.Lstat(path)
	if err != nil {
		if softError(err) {
			return Info{}, false, nil
		}
		return Info{}, false, err
	}
	return info, true, nil
}

// softError says whether an error only means "nothing to look at here".
func softError(err error) bool {
	return errors.Is(err, fs.ErrNotExist) ||
		errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, fs.ErrInvalid)
}

// joinPath puts a name inside a directory.
func joinPath(dir, name string) string {
	if strings.HasSuffix(dir, `\`) {
		return dir + name
	}
	return dir + `\` + name
}

// foldPath is the key two spellings of one path share.
func foldPath(path string) string { return strings.ToLower(strings.TrimRight(path, `\`)) }
