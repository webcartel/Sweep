package rules

import (
	"io/fs"
	"strings"

	builtinrules "github.com/webcartel/sweep/rules"
)

// SourceBuiltin marks a rule that came with the binary rather than from a file
// on disk.
const SourceBuiltin = "builtin"

// builtinPrefix is how a built-in rule file is named in a finding, so it reads
// "rules/browsers/chrome.yaml:3:5: ...".
const builtinPrefix = "rules/"

// Builtin returns the rule files that ship with the binary, at their path below
// the repository's rules directory.
func Builtin() fs.FS { return builtinrules.FS }

// ReadBuiltin reads every built-in rule file, in the order the file system
// walks them. The rules carry Source SourceBuiltin instead of a path on disk.
func ReadBuiltin() ([]*File, error) { return readRuleFS(Builtin()) }

// readRuleFS reads every rule file of a file system whose root is the rules
// directory, which is how the built-in rules are laid out both in the binary and
// in the repository.
func readRuleFS(fsys fs.FS) ([]*File, error) {
	var files []*File

	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".yaml") {
			return nil
		}

		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		file, err := LoadFile(builtinPrefix+name, data)
		if err != nil {
			return err
		}
		for _, parsed := range file.Rules {
			parsed.Rule.Source = SourceBuiltin
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
