// Package guard decides what the engine may touch. It normalizes a path and
// compares it against the protected list, and no rule can turn it off: a rule
// from the community that names a system directory has to find nothing rather
// than take something with it.
package guard

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
)

// Guard reads the file system and the environment it was built with. It never
// asks the operating system anything on its own, so what it decides is the
// same on every machine that runs it — including the Linux job of CI, which
// runs the same cases against a file system in memory.
type Guard struct {
	fs  platform.FS
	env platform.Env
	cwd string
	// extra holds the paths the config adds to the protected list, already
	// read into the form the comparison uses.
	extra []string
	// ranges is the protected list of this machine, built once, when the
	// guard is built.
	ranges []protectedRange
}

// New returns a guard over a file system and an environment. cwd is what a
// relative path is taken from. The paths in excludePaths are added to the
// protected list, and a path the guard cannot read there stops it: a
// misspelled exclusion must not pass unnoticed.
func New(fs platform.FS, env platform.Env, cwd string, excludePaths []string) (*Guard, error) {
	g := &Guard{fs: fs, env: env, cwd: cwd}

	for _, entry := range excludePaths {
		path, err := g.exclusion(entry)
		if err != nil {
			return nil, err
		}
		g.extra = append(g.extra, path)
	}
	g.ranges = protectedRanges(env, g.extra)
	return g, nil
}

// Refusal says why a path may not be used, in the terms of the report.
type Refusal struct {
	Reason  model.SkipReason
	Message string
}

// Error makes a refusal an error a caller can carry up.
func (r *Refusal) Error() string { return r.Message }

// Path is a target path after normalization.
type Path struct {
	// Given is the path after the variables, absoluteness, cleaning, the
	// names Windows ignores at the end, and long names.
	Given string
	// Final is where the path ends after the links it is itself are followed.
	Final string
	// Info describes Final, or Given when Final is not there.
	Info platform.Info
	// Exists says whether the target is on the disk at all. A path that is
	// not there is not a refusal: the scan reports it as not found.
	Exists bool
	// Chain is every link target on the way, in order.
	Chain []string
}

// Verdict is what the guard says about one path.
type Verdict struct {
	// Path is the path the verdict is about: the normalized one when
	// normalization worked, and the one it was given otherwise.
	Path string
	// Allowed says whether the engine may touch it.
	Allowed bool
	// Reason is empty when Allowed, and the reason of the refusal otherwise.
	Reason model.SkipReason
	// Message says which part of the protected list refused the path.
	Message string
}

// Check says whether the engine may touch a path, in the mode of the rule that
// wants it. include and minAgeDays are the rule's filters: they are the one
// thing that lets `mode: files` reach into a root or a user directory.
func (g *Guard) Check(path string, mode model.Mode, include []string, minAgeDays int) (Verdict, error) {
	target, err := g.Normalize(path)
	if err != nil {
		var refusal *Refusal
		if errors.As(err, &refusal) {
			return Verdict{Path: path, Reason: refusal.Reason, Message: refusal.Message}, nil
		}
		return Verdict{}, err
	}
	return g.CheckNormalized(target, mode, include, minAgeDays)
}

// CheckNormalized says whether the engine may touch a path that Normalize has
// already answered for. It holds the part of Check that compares against the
// protected list, and the verdict names the path the rule wrote, the way
// Check's does.
func (g *Guard) CheckNormalized(target Path, mode model.Mode, include []string, minAgeDays int) (Verdict, error) {
	filtered := mode == model.ModeFiles && len(include) > 0 && minAgeDays > 0
	for _, candidate := range []string{target.Given, target.Final} {
		if r, ok := g.match(candidate); ok && !r.permits(filtered) {
			return Verdict{
				// The verdict names the path the rule wrote: that is the one
				// the message is about, and where it leads is refused on its
				// own account.
				Path:    target.Given,
				Reason:  model.ReasonProtected,
				Message: fmt.Sprintf("the path %s is protected by %s", quote(candidate), quote(r.path)),
			}, nil
		}
	}
	return Verdict{Path: target.Given, Allowed: true}, nil
}

// Normalize turns a path written in a rule into the path the engine works
// with, and refuses the ones that must not be touched at all.
func (g *Guard) Normalize(path string) (Path, error) {
	if path == "" {
		return Path{}, refuse(i18n.T("guard.err.empty"))
	}

	expanded := platform.Expand(path, g.env)
	if names := platform.EnvNames(expanded); len(names) > 0 {
		return Path{}, refuse(i18n.T("guard.err.var", quote(path), names[0]))
	}

	text := deviceFree(expanded)
	given := cleanPath(text)
	if given == "" {
		return Path{}, refuse(i18n.T("guard.err.nopath", quote(path)))
	}
	if !isAbsPath(text) {
		given = g.absolute(text, given)
	}

	if long, err := g.fs.LongPath(given); err == nil {
		given = long
	}

	final, chain, err := g.resolve(given)
	if err != nil {
		return Path{}, err
	}

	for _, candidate := range []string{given, final} {
		if err := g.checkLocalFixed(candidate); err != nil {
			return Path{}, err
		}
	}

	target := Path{Given: given, Final: final, Chain: chain}
	for _, candidate := range []string{final, given} {
		info, err := g.fs.Lstat(candidate)
		if err == nil {
			target.Info = info
			target.Exists = true
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return Path{}, err
		}
	}
	return target, nil
}

// absolute places a path that does not start at a drive or a share. A path that
// starts with a single separator starts at the root of the drive the working
// directory is on, the way Windows reads it — `\Windows\System32` is
// `C:\Windows\System32` — and anything else is taken from the working directory
// itself.
func (g *Guard) absolute(text, path string) string {
	base := g.cwd
	if strings.HasPrefix(text, `\`) || strings.HasPrefix(text, `/`) {
		if root, ok := driveRootOf(g.cwd); ok {
			base = root
		}
	}
	return cleanPath(joinPath(base, path))
}

// resolve walks the links on the way to a path, component by component, and
// answers where the path ends up and every target it followed. A link in the
// middle of a path is resolved as well as the path itself: a junction in a
// directory would otherwise carry a rule into a place the guard never looked
// at, and the file at the end of it need not be a link at all.
func (g *Guard) resolve(path string) (string, []string, error) {
	const limit = 8

	seen := map[string]bool{}
	var chain []string

	current := path
	for hop := 0; hop <= limit; hop++ {
		key := pathKey(current)
		if seen[key] {
			return "", nil, refuse(i18n.T("guard.err.loop", quote(path)))
		}
		seen[key] = true

		next, target, ok, err := g.linkOn(current)
		if err != nil {
			return "", nil, err
		}
		if !ok {
			return current, chain, nil
		}
		chain = append(chain, target)
		current = next
	}

	return "", nil, refuse(i18n.T("guard.err.depth", quote(path), limit))
}

// linkOn finds the first link on the way to a path and answers the path with
// that link replaced by what it points at, together with the target. A path
// that stops at something that is not there has no link on the way: the rest of
// it cannot be a reparse point, so the walk stops there and lets the refusal
// list decide.
func (g *Guard) linkOn(path string) (string, string, bool, error) {
	volume, rest := splitVolume(path)
	if volume == "" {
		return path, "", false, nil
	}

	segments := strings.Split(rest, `\`)
	current := strings.TrimSuffix(volume, `\`)

	for i, segment := range segments {
		if segment == "" {
			continue
		}
		candidate := joinPath(current, segment)
		info, err := g.fs.Lstat(candidate)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrInvalid) {
				return path, "", false, nil
			}
			return "", "", false, err
		}
		if !info.Reparse || info.Link == "" {
			current = candidate
			continue
		}

		target := deviceFree(info.Link)
		if !isAbsPath(target) {
			target = joinPath(current, target)
		}
		target = cleanPath(target)
		// The target is a place Windows may have written with a short name,
		// and the refusal list knows only the long one.
		if long, err := g.fs.LongPath(target); err == nil {
			target = long
		}

		composed := target
		for _, name := range segments[i+1:] {
			composed = joinPath(composed, name)
		}
		return cleanPath(composed), target, true, nil
	}

	return path, "", false, nil
}

// checkLocalFixed refuses everything but a fixed local disk: a stick belongs
// to somebody, a share is on another machine, and a path with no drive at all
// is a UNC path or something Windows does not know.
func (g *Guard) checkLocalFixed(path string) error {
	kind, err := g.fs.DriveType(path)
	if err != nil {
		return err
	}
	if kind != platform.DriveFixed {
		return refuse(i18n.T("guard.err.disk", quote(path)))
	}
	return nil
}

// refuse is the refusal of a path that must not be touched.
func refuse(message string) *Refusal {
	return &Refusal{Reason: model.ReasonProtected, Message: message}
}
