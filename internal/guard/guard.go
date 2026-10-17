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
	// extra holds the paths the config adds to the protected list. They are
	// read when a path is checked, not when the guard is built.
	extra []string
}

// New returns a guard over a file system and an environment. cwd is what a
// relative path is taken from.
func New(fs platform.FS, env platform.Env, cwd string, excludePaths []string) (*Guard, error) {
	return &Guard{
		fs:    fs,
		env:   env,
		cwd:   cwd,
		extra: append([]string(nil), excludePaths...),
	}, nil
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

// Normalize turns a path written in a rule into the path the engine works
// with, and refuses the ones that must not be touched at all.
func (g *Guard) Normalize(path string) (Path, error) {
	if path == "" {
		return Path{}, refuse("the path is empty")
	}

	expanded := platform.Expand(path, g.env)
	if names := platform.EnvNames(expanded); len(names) > 0 {
		return Path{}, refuse(fmt.Sprintf("the path %q names %%%s%%, which this machine does not set", path, names[0]))
	}

	given := cleanPath(stripDevicePrefix(expanded))
	if given == "" {
		return Path{}, refuse(fmt.Sprintf("the path %q names no place at all", path))
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
			return "", nil, refuse(fmt.Sprintf("the links of %q lead back to themselves", path))
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

	return "", nil, refuse(fmt.Sprintf("the links of %q are more than %d deep", path, limit))
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
		return refuse(fmt.Sprintf("the path %q is not on a fixed local disk", path))
	}
	return nil
}

// refuse is the refusal of a path that must not be touched.
func refuse(message string) *Refusal {
	return &Refusal{Reason: model.ReasonProtected, Message: message}
}
