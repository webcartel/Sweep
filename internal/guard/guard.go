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

	filtered := mode == model.ModeFiles && len(include) > 0 && minAgeDays > 0
	for _, candidate := range []string{target.Given, target.Final} {
		if r, ok := g.match(candidate); ok && !r.permits(filtered) {
			return Verdict{
				Path:    target.Final,
				Reason:  model.ReasonProtected,
				Message: fmt.Sprintf("the path %s is protected by %s", quote(candidate), quote(r.path)),
			}, nil
		}
	}
	return Verdict{Path: target.Final, Allowed: true}, nil
}

// Normalize turns a path written in a rule into the path the engine works
// with, and refuses the ones that must not be touched at all.
func (g *Guard) Normalize(path string) (Path, error) {
	if path == "" {
		return Path{}, refuse("the path is empty")
	}

	expanded := platform.Expand(path, g.env)
	if names := platform.EnvNames(expanded); len(names) > 0 {
		return Path{}, refuse(fmt.Sprintf("the path %s names %%%s%%, which this machine does not set", quote(path), names[0]))
	}

	given := cleanPath(stripDevicePrefix(expanded))
	if given == "" {
		return Path{}, refuse(fmt.Sprintf("the path %s names no place at all", quote(path)))
	}
	if !isAbsPath(given) {
		given = cleanPath(joinPath(g.cwd, given))
	}

	if long, err := g.fs.LongPath(given); err == nil {
		given = long
	}

	final, chain, err := g.follow(given)
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

// follow walks the links the path itself is, and answers where it ends up,
// with every target on the way. A reparse point that points nowhere — a cloud
// placeholder, a deduplicated file — ends where it stands.
func (g *Guard) follow(path string) (string, []string, error) {
	const limit = 8

	seen := map[string]bool{}
	var chain []string
	current := path

	for hop := 0; hop <= limit; hop++ {
		key := pathKey(current)
		if seen[key] {
			return "", nil, refuse(fmt.Sprintf("the links of %s lead back to themselves", quote(path)))
		}
		seen[key] = true

		info, err := g.fs.Lstat(current)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return current, chain, nil
			}
			return "", nil, err
		}
		if !info.Reparse || info.Link == "" {
			return current, chain, nil
		}

		target := strings.TrimPrefix(info.Link, `\??\`)
		if !isAbsPath(target) {
			target = joinPath(dirPath(current), target)
		}
		current = cleanPath(target)
		chain = append(chain, current)
	}

	return "", nil, refuse(fmt.Sprintf("the links of %s are more than %d deep", quote(path), limit))
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
		return refuse(fmt.Sprintf("the path %s is not on a fixed local disk", quote(path)))
	}
	return nil
}

// refuse is the refusal of a path that must not be touched.
func refuse(message string) *Refusal {
	return &Refusal{Reason: model.ReasonProtected, Message: message}
}
