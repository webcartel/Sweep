package rules

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/webcartel/sweep/internal/model"
)

// File is one parsed rule file. The YAML nodes are kept next to the rules so
// that every later check can name the line and column of what it reports.
type File struct {
	Path  string
	Rules []ParsedRule
	Doc   *yaml.Node
}

// ParsedRule is a rule together with the node it was read from.
type ParsedRule struct {
	Rule *model.Rule
	Node *yaml.Node
}

// LoadFile parses one rule file. The path is kept in the file and in every
// rule's Source, so anything reported later can name where it came from.
func LoadFile(path string, data []byte) (*File, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	file := &File{Path: path, Doc: &doc}
	root := docRoot(&doc)
	if root == nil {
		return file, nil
	}

	for _, node := range items(field(root, "rules")) {
		rule, err := parseRule(node, path)
		if err != nil {
			return nil, err
		}
		file.Rules = append(file.Rules, ParsedRule{Rule: rule, Node: node})
	}
	return file, nil
}

// parseRule reads one rule from its mapping node.
func parseRule(node *yaml.Node, source string) (*model.Rule, error) {
	rule := &model.Rule{Source: source}

	rule.ID = scalar(field(node, "id"))
	rule.Name = scalar(field(node, "name"))
	rule.Description = scalar(field(node, "description"))
	rule.Category = scalar(field(node, "category"))
	rule.Risk = model.Risk(scalar(field(node, "risk")))
	rule.DefaultEnabled = boolOf(field(node, "default_enabled"))
	rule.RequiresAdmin = boolOf(field(node, "requires_admin"))
	rule.OSMinBuild = intOf(field(node, "os_min_build"))
	rule.RequiresClosed = stringList(field(node, "requires_closed"))
	rule.Tags = stringList(field(node, "tags"))

	// trash has no default of its own to read: it is off unless the rule says
	// otherwise, except for dangerous rules, whose data goes to the recycle bin.
	if trash := field(node, "trash"); trash != nil {
		rule.Trash = boolOf(trash)
	} else {
		rule.Trash = rule.Risk == model.RiskDangerous
	}

	for _, target := range items(field(node, "targets")) {
		parsed, err := parseTarget(target, source)
		if err != nil {
			return nil, err
		}
		rule.Targets = append(rule.Targets, parsed)
	}
	return rule, nil
}

// parseTarget reads one target from its mapping node.
func parseTarget(node *yaml.Node, source string) (model.Target, error) {
	target := model.Target{
		Kind: model.KindPath,     // the default
		Mode: model.ModeContents, // the default: clear what the path matched
	}

	if kind := scalar(field(node, "kind")); kind != "" {
		target.Kind = model.Kind(kind)
	}
	if mode := scalar(field(node, "mode")); mode != "" {
		target.Mode = model.Mode(mode)
	}
	target.Path = scalar(field(node, "path"))
	target.Cmd = scalar(field(node, "cmd"))
	target.Args = stringList(field(node, "args"))
	target.Include = stringList(field(node, "include"))
	target.Exclude = stringList(field(node, "exclude"))
	target.MinAgeDays = intOf(field(node, "min_age_days"))

	if size := field(node, "min_size"); size != nil {
		bytes, err := parseSize(scalar(size))
		if err != nil {
			return model.Target{}, errorAt(source, size, "%v", err)
		}
		target.MinSize = bytes
	}
	return target, nil
}

// parseSize reads a size such as "50MB" as a number of bytes. Suffixes are
// powers of 1024, the way Windows reports the size of a file.
func parseSize(text string) (int64, error) {
	digits := strings.TrimSpace(strings.ToUpper(text))
	multiplier := int64(1)

	for _, unit := range []struct {
		suffix string
		factor int64
	}{
		{"KB", 1024},
		{"MB", 1024 * 1024},
		{"GB", 1024 * 1024 * 1024},
		{"TB", 1024 * 1024 * 1024 * 1024},
		{"B", 1},
	} {
		if strings.HasSuffix(digits, unit.suffix) {
			multiplier = unit.factor
			digits = strings.TrimSpace(strings.TrimSuffix(digits, unit.suffix))
			break
		}
	}

	value, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("size %q is not a number with an optional B/KB/MB/GB/TB suffix", text)
	}
	if value < 0 {
		return 0, fmt.Errorf("size %q is negative", text)
	}
	if value > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("size %q is too large", text)
	}
	return value * multiplier, nil
}

// docRoot returns the mapping a document holds, or nil for an empty one.
func docRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	return doc.Content[0]
}

// field returns the value node of a key of a mapping, or nil when the mapping
// is not a mapping or holds no such key.
func field(mapping *yaml.Node, name string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// items returns the elements of a sequence, or nil for any other node.
func items(node *yaml.Node) []*yaml.Node {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	return node.Content
}

// scalar returns the text of a scalar, or "" for any other node.
func scalar(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

// boolOf decodes a scalar as a bool. The schema pass rejects wrong types, so a
// value that does not fit reads as false here.
func boolOf(node *yaml.Node) bool {
	var value bool
	if node == nil {
		return false
	}
	_ = node.Decode(&value)
	return value
}

// intOf decodes a scalar as an int. The schema pass rejects wrong types, so a
// value that does not fit reads as 0 here.
func intOf(node *yaml.Node) int {
	var value int
	if node == nil {
		return 0
	}
	_ = node.Decode(&value)
	return value
}

// stringList decodes a sequence of scalars. The schema pass rejects wrong
// types, so a value that does not fit reads as empty here.
func stringList(node *yaml.Node) []string {
	if node == nil {
		return nil
	}
	var value []string
	_ = node.Decode(&value)
	return value
}
