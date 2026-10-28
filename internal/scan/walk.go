package scan

import (
	"context"
	"errors"
	"io/fs"
	"sort"
	"strings"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
)

// collect turns one target into items. Every taken entry is told through add,
// every path the walk cannot serve through skip; an error that is neither the
// path's fault nor the walk's is returned for the caller to report.
func (s *Scanner) collect(ctx context.Context, t model.Target, add func(model.Item), skip func(model.Skip)) error {
	expanded := platform.Expand(t.Path, s.env)
	if unset := platform.EnvNames(expanded); len(unset) > 0 {
		skip(model.Skip{Path: t.Path, Reason: model.ReasonNotFound,
			Detail: i18n.T("scan.skip.var", unset[0])})
		return nil
	}

	var candidates []string
	if strings.ContainsAny(expanded, "*?") {
		names, err := s.fs.Glob(expanded)
		if err != nil {
			return err
		}
		sort.Strings(names)
		if len(names) == 0 {
			// A mask that names no path is a path that is not there.
			skip(model.Skip{Path: t.Path, Reason: model.ReasonNotFound,
				Detail: i18n.T("scan.skip.mask")})
			return nil
		}
		candidates = names
	} else {
		if _, err := s.fs.Lstat(expanded); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				skip(model.Skip{Path: t.Path, Reason: model.ReasonNotFound,
					Detail: i18n.T("scan.skip.notfound")})
				return nil
			}
			return err
		}
		candidates = []string{expanded}
	}

	keep := compileFilters(t)
	for _, path := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.candidate(ctx, path, t.Mode, keep, add, skip); err != nil {
			return err
		}
	}
	return nil
}

// candidate walks one of the places the target takes.
func (s *Scanner) candidate(ctx context.Context, path string, mode model.Mode,
	keep filters, add func(model.Item), skip func(model.Skip)) error {

	info, err := s.fs.Lstat(path)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			skip(model.Skip{Path: path, Reason: model.ReasonNotFound, Detail: "the path does not exist"})
			return nil
		case errors.Is(err, fs.ErrPermission):
			skip(model.Skip{Path: path, Reason: model.ReasonAccessDenied, Detail: err.Error()})
			return nil
		}
		return err
	}

	switch mode {
	case model.ModeSelf:
		return s.entry(ctx, info, keep, add, skip)

	case model.ModeContents:
		if info.Reparse || !info.IsDir {
			if info.IsDir { // a junction is not walked into: the link itself is the item
				return s.entry(ctx, info, keep, add, skip)
			}
			// The rule asked for the contents of directories, and a file
			// holds none. Removing it here would be a guess.
			skip(model.Skip{Path: path, Reason: model.ReasonNotFound,
				Detail: i18n.T("scan.skip.contents")})
			return nil
		}
		entries, err := s.children(ctx, path, skip)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := s.entry(ctx, entry, keep, add, skip); err != nil {
				return err
			}
		}
		return nil

	default: // model.ModeFiles
		if info.Reparse {
			return nil
		}
		return s.walk(ctx, path, info, keep, add, skip)
	}
}

// walk takes the files below a directory the filters keep. Directories are
// not items, a junction is not entered: the rule asked for files, and a link
// is not a file.
func (s *Scanner) walk(ctx context.Context, root string, info platform.Info,
	keep filters, add func(model.Item), skip func(model.Skip)) error {

	if info.Reparse {
		// A junction or a link is not walked into: what is below belongs to
		// the place it points at, not here.
		return nil
	}
	if info.IsDir {
		entries, err := s.children(ctx, info.Path, skip)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := s.walk(ctx, root, entry, keep, add, skip); err != nil {
				return err
			}
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if keep.allow(relPath(root, info.Path), info) {
		add(model.Item{Path: info.Path, Size: info.Size, Files: 1,
			ModTime: info.ModTime, Reparse: info.Reparse})
	}
	return nil
}

// children lists what is inside a directory, sorted, with the per path errors
// the specification names given to skip and the rest passed on.
func (s *Scanner) children(ctx context.Context, path string, skip func(model.Skip)) ([]platform.Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := s.fs.ReadDir(path)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			skip(model.Skip{Path: path, Reason: model.ReasonNotFound, Detail: i18n.T("scan.skip.disappeared")})
		case errors.Is(err, fs.ErrPermission):
			skip(model.Skip{Path: path, Reason: model.ReasonAccessDenied, Detail: err.Error()})
		default:
			return nil, err
		}
		return nil, nil
	}
	return entries, nil
}

// entry turns one entry into an item, accumulating what the walk found under
// it. A junction or a link counts as the entry itself and is not walked into:
// removing the link is what the rule asked for. An entry the filters take out
// is left out entirely: the filters are the intent of the rule, not a path
// that went wrong.
func (s *Scanner) entry(ctx context.Context, info platform.Info,
	keep filters, add func(model.Item), skip func(model.Skip)) error {

	item := model.Item{Path: info.Path, IsDir: info.IsDir, ModTime: info.ModTime, Reparse: info.Reparse}
	switch {
	case info.Reparse:
		item.Size = info.Size
	case info.IsDir:
		size, files, err := s.aggregate(ctx, info)
		if err != nil {
			// The whole walk below stops here. What the path can answer
			// for, the walk says through skip; the rest is not its fault
			// and is passed on.
			if pass := s.skipIO(info.Path, err, skip); pass != nil {
				return pass
			}
			return nil
		}
		item.Size, item.Files = size, files
	default:
		item.Size, item.Files = info.Size, 1
	}
	seen := info
	seen.Size = item.Size // the Size and age filters read what was accumulated
	if !keep.allow(lastElement(info.Path), seen) {
		return nil
	}
	add(item)
	return nil
}

// aggregate walks everything below one directory and sums the size and the
// number of files. A junction or a link counts as one entry of its own size
// and is not walked into.
func (s *Scanner) aggregate(ctx context.Context, dir platform.Info) (int64, int, error) {
	entries, err := s.fs.ReadDir(dir.Path)
	if err != nil {
		return 0, 0, err
	}
	var size int64
	files := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
		if entry.Reparse {
			size += entry.Size
			continue
		}
		if entry.IsDir {
			belowSize, belowFiles, err := s.aggregate(ctx, entry)
			if err != nil {
				return 0, 0, err
			}
			size += belowSize
			files += belowFiles
			continue
		}
		size += entry.Size
		files++
	}
	return size, files, nil
}

// skipIO repeats the error a ReadDir gave in the words of SkipReason, and
// hands back the errors no path can answer for, for the caller to pass on
// instead of blaming the path with them.
func (s *Scanner) skipIO(path string, err error, skip func(model.Skip)) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		skip(model.Skip{Path: path, Reason: model.ReasonNotFound, Detail: i18n.T("scan.skip.disappeared")})
	case errors.Is(err, fs.ErrPermission):
		skip(model.Skip{Path: path, Reason: model.ReasonAccessDenied, Detail: err.Error()})
	default:
		return err
	}
	return nil
}

// relPath is the path of a file relative to the root of a target, the form
// the include and exclude masks are matched against.
func relPath(root, path string) string {
	if len(path) > len(root) && strings.EqualFold(path[:len(root)], root) && path[len(root)] == '\\' {
		return path[len(root)+1:]
	}
	return lastElement(path)
}

// lastElement is the name of the last segment of a path.
func lastElement(path string) string {
	trimmed := strings.TrimRight(strings.ReplaceAll(path, "/", `\`), `\`)
	if index := strings.LastIndex(trimmed, `\`); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}
