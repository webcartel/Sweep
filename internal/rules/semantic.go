package rules

import (
	"go.yaml.in/yaml/v3"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
)

// maskFields are the fields of a target that hold glob masks.
var maskFields = []string{"path", "include", "exclude"}

// ValidateSemantics runs the checks the JSON Schema cannot express: an id used
// twice in one file, a mask that does not compile, and a target whose kind and
// fields disagree.
//
// An environment variable that is not set is only a warning: a rule file
// written for another machine is still worth reading.
func ValidateSemantics(file *File, env platform.Env) ([]Error, []string) {
	var findings []Error
	var warnings []string

	findings = append(findings, duplicateIDs(file)...)
	for _, parsed := range file.Rules {
		for _, target := range items(field(parsed.Node, "targets")) {
			findings = append(findings, checkTarget(file.Path, target, env, &warnings)...)
		}
	}
	return sorted(findings), warnings
}

// duplicateIDs reports every id that a second rule of the same file repeats.
// Rules without an id are left to the schema, which requires one.
func duplicateIDs(file *File) []Error {
	var findings []Error

	used := map[string]int{}
	for _, parsed := range file.Rules {
		id := parsed.Rule.ID
		if id == "" {
			continue
		}
		if line, seen := used[id]; seen {
			findings = append(findings,
				errorAt(file.Path, keyOf(parsed.Node, "id"), "id '%s' is already used at line %d", id, line))
			continue
		}
		used[id] = parsed.Node.Line
	}
	return findings
}

// checkTarget runs the checks of one target and appends its warnings.
func checkTarget(file string, node *yaml.Node, env platform.Env, warnings *[]string) []Error {
	if node.Kind != yaml.MappingNode {
		// The schema reports the shape of what is not a target; telling the
		// author what a path target needs on top of that repeats one mistake
		// as two findings.
		return nil
	}

	var findings []Error

	targetPath := scalar(field(node, "path"))
	kind := scalar(field(node, "kind"))
	if kind == "" {
		kind = string(model.KindPath)
	}

	switch model.Kind(kind) {
	case model.KindCommand:
		if scalar(field(node, "cmd")) == "" {
			findings = append(findings, errorAt(file, node, "a command target takes cmd"))
		}
		if targetPath != "" {
			findings = append(findings,
				errorAt(file, keyOf(node, "path"), "a command target takes cmd, not path"))
		}
	default:
		if targetPath == "" {
			findings = append(findings, errorAt(file, node, "a path target takes path"))
		}
		if field(node, "cmd") != nil {
			findings = append(findings,
				errorAt(file, keyOf(node, "cmd"), "a path target takes path, not cmd"))
		}
		if field(node, "args") != nil {
			findings = append(findings,
				errorAt(file, keyOf(node, "args"), "a path target takes path, not args"))
		}
	}

	for _, name := range maskFields {
		value := field(node, name)
		if value == nil {
			continue
		}
		if value.Kind == yaml.SequenceNode {
			for _, mask := range value.Content {
				findings = append(findings, checkMask(file, name, mask)...)
			}
			continue
		}
		findings = append(findings, checkMask(file, name, value)...)
	}

	if targetPath != "" {
		for _, name := range platform.EnvNames(targetPath) {
			if _, set := env.Get(name); !set {
				*warnings = append(*warnings,
					errorAt(file, field(node, "path"), "environment variable %%%s%% is not set", name).Error())
			}
		}
	}
	return findings
}

// checkMask reports a mask the rule language cannot read. The mask language
// belongs to the platform layer, which also matches with it, so that the check
// and the walking of the file system cannot drift apart.
//
// An empty path is left to the schema, which requires a path to hold
// something; an empty name mask is a mistake only this pass can see, since a
// mask that is empty takes nothing and does it in silence.
func checkMask(file, name string, node *yaml.Node) []Error {
	if node.Kind != yaml.ScalarNode {
		return nil // the schema reports the shape of what is not a mask
	}
	mask := scalar(node)
	if mask == "" && name == "path" {
		return nil
	}
	if _, err := platform.CompileMask(mask); err != nil {
		return []Error{errorAt(file, node, "%s '%s': %v", name, mask, err)}
	}
	return nil
}
