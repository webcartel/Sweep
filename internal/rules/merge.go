package rules

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
)

// Set is every rule the program knows once the sources are merged, together
// with the profiles, the switches the config sets and what the merge wants to
// say out loud.
type Set struct {
	Rules    []*model.Rule
	Profiles map[string]model.Profile
	Toggles  map[string]model.RuleToggle
	Warnings []string
}

// Loader reads the rule sources in the order they outrank each other and merges
// them by id: the built-in rules first, then the user's own, then the
// directories given as --rules-dir, and finally the config.
//
// A rule from a later source replaces the one an earlier source defined under
// the same id, keeps the place the id first appeared at, and gives a warning.
// An id defined twice inside one source is an error instead: neither file
// outranks the other.
type Loader struct {
	// Env is the environment the checks read. A nil Env means the environment
	// of the running process.
	Env platform.Env
	// Builtin is the file system holding the built-in rules. A nil Builtin
	// means the rules embedded in the binary.
	Builtin fs.FS
	// UserDir holds the user's own rule files. A directory that is not there
	// simply holds no rules.
	UserDir string
	// ExtraDirs hold the directories named as --rules-dir, in order. One that is
	// not there is an error, since the caller asked for it by name.
	ExtraDirs []string
	// Config switches rules on or off and adds profiles.
	Config model.Config
}

// source is one place rules come from.
type source struct {
	builtin bool
	files   []*File
}

// Load reads every source, merges the rules and returns the set.
//
// A file that fails either validation pass is reported and left out of the set,
// so that one broken file does not hide the rules of the others.
func (l Loader) Load() (*Set, error) {
	sources, warnings, findings := l.read()

	set := &Set{
		Profiles: BuiltinProfiles(),
		Toggles:  map[string]model.RuleToggle{},
		Warnings: warnings,
	}

	placed := map[string]int{}   // id -> its place in set.Rules
	owner := map[string]string{} // id -> the file that defines it now

	for _, src := range sources {
		defined := map[string]string{} // id -> "file:line" inside this source

		for _, file := range src.files {
			for _, parsed := range file.Rules {
				id := parsed.Rule.ID

				if where, ok := defined[id]; ok {
					findings = append(findings, errorAt(file.Path, keyOf(parsed.Node, "id"),
						"id '%s' is already defined in %s", id, where))
					continue
				}
				defined[id] = fmt.Sprintf("%s:%d", file.Path, keyOf(parsed.Node, "id").Line)

				if !src.builtin && !l.Config.AllowUserCommands {
					if kind := commandKind(&parsed); kind != nil {
						findings = append(findings, errorAt(file.Path, kind,
							"rule '%s' runs a command; set allow_user_commands: true to allow it", id))
					}
				}

				if place, seen := placed[id]; seen {
					set.Warnings = append(set.Warnings, errorAt(file.Path, keyOf(parsed.Node, "id"),
						"rule '%s' replaces the one from %s", id, owner[id]).Error())
					set.Rules[place] = parsed.Rule
				} else {
					placed[id] = len(set.Rules)
					set.Rules = append(set.Rules, parsed.Rule)
				}
				owner[id] = file.Path
			}
		}
	}

	for _, id := range slices.Sorted(maps.Keys(l.Config.Rules)) {
		set.Toggles[id] = l.Config.Rules[id]
		if _, known := placed[id]; !known {
			set.Warnings = append(set.Warnings,
				fmt.Sprintf("the config switches rule '%s', which no source defines", id))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(l.Config.Profiles)) {
		if _, builtin := set.Profiles[name]; builtin {
			set.Warnings = append(set.Warnings,
				fmt.Sprintf("the config profile '%s' replaces the built-in one", name))
		}
		set.Profiles[name] = l.Config.Profiles[name]
	}

	if len(findings) != 0 {
		return nil, errors.Join(findings...)
	}
	return set, nil
}

// read reads every source in the order they outrank each other, checking each
// file as it goes.
func (l Loader) read() ([]source, []string, []error) {
	var (
		sources  []source
		warnings []string
		findings []error
	)

	files, warns, found := l.readBuiltin()
	sources = append(sources, source{builtin: true, files: files})
	warnings = append(warnings, warns...)
	findings = append(findings, found...)

	for i, dir := range append([]string{l.UserDir}, l.ExtraDirs...) {
		if dir == "" {
			continue
		}
		files, warns, found := l.readDir(dir, i > 0)
		sources = append(sources, source{files: files})
		warnings = append(warnings, warns...)
		findings = append(findings, found...)
	}
	return sources, warnings, findings
}

// readBuiltin reads the built-in rules.
func (l Loader) readBuiltin() ([]*File, []string, []error) {
	fsys := l.Builtin
	if fsys == nil {
		fsys = Builtin()
	}

	files, err := readRuleFS(fsys)
	if err != nil {
		return nil, nil, []error{err}
	}
	return l.check(files)
}

// readDir reads the rule files of one directory, in name order. A required
// directory has to be there; the user's own may be missing, since a machine that
// was never given rules of its own has none.
func (l Loader) readDir(dir string, required bool) ([]*File, []string, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && !required {
			return nil, nil, nil
		}
		return nil, nil, []error{fmt.Errorf("read the rules directory %s: %w", dir, err)}
	}

	var (
		files    []*File
		findings []error
	)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			findings = append(findings, fmt.Errorf("read %s: %w", path, err))
			continue
		}
		file, err := LoadFile(path, data)
		if err != nil {
			findings = append(findings, err)
			continue
		}
		files = append(files, file)
	}

	checked, warnings, found := l.check(files)
	return checked, warnings, append(findings, found...)
}

// check runs the two validation passes over the files of one source and keeps
// the ones that pass.
func (l Loader) check(files []*File) ([]*File, []string, []error) {
	var (
		kept     []*File
		warnings []string
		findings []error
	)

	env := l.env()
	for _, file := range files {
		found, warns := Validate(file, env)
		warnings = append(warnings, warns...)

		if len(found) != 0 {
			for _, finding := range found {
				findings = append(findings, finding)
			}
			continue
		}
		kept = append(kept, file)
	}
	return kept, warnings, findings
}

// env is the environment the checks read: the process's own unless the caller
// named another.
func (l Loader) env() platform.Env {
	if l.Env == nil {
		return platform.OSEnv{}
	}
	return l.Env
}

// commandKind returns the node of the target that makes a rule run a command, or
// nil when the rule runs none.
func commandKind(parsed *ParsedRule) *yaml.Node {
	targets := items(field(parsed.Node, "targets"))
	for i, target := range parsed.Rule.Targets {
		if target.Kind == model.KindCommand && i < len(targets) {
			return keyOf(targets[i], "kind")
		}
	}
	return nil
}
