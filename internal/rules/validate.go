package rules

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/schema"
)

// schemaName is the name the rule schema is compiled under.
const schemaName = "rule.schema.json"

// schemaPrinter renders the schema's findings in the language this run
// speaks. x/text carries its own translations for the standard kinds the
// schema library reports, and falls back to English for any it has not
// got, so a missing translation never blanks the message.
func schemaPrinter() *message.Printer {
	tag := language.English
	if i18n.Current() == i18n.LangRU {
		tag = language.Russian
	}
	return message.NewPrinter(tag)
}

// ruleSchema holds the schema of the rule format, read once from the published
// file the binary embeds.
var ruleSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.JSON))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("rules.err.schema.read"), err)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaName, document); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("rules.err.schema.add"), err)
	}
	return compiler.Compile(schemaName)
})

// Validate runs both validation passes over one parsed file: the JSON Schema
// that fixes the shape of the format, then the semantic checks it cannot
// express. The warnings of the second pass come back too.
//
// This is the pair `rules validate` and the loader both need, in one place, so
// that what the command reports and what the loader refuses cannot disagree.
func Validate(file *File, env platform.Env) ([]Error, []string) {
	findings := ValidateSchema(file)
	semantic, warnings := ValidateSemantics(file, env)
	return append(findings, semantic...), warnings
}

// ValidateSchema checks a parsed file against the rule format's JSON Schema and
// reports every place where the file does not fit it.
func ValidateSchema(file *File) []Error {
	compiled, err := ruleSchema()
	if err != nil {
		return []Error{errorTextAt(file.Path, docRoot(file.Doc), i18n.T("rules.err.schema.use", err))}
	}

	var instance any
	if root := docRoot(file.Doc); root != nil {
		if err := root.Decode(&instance); err != nil {
			return []Error{errorTextAt(file.Path, root, i18n.T("rules.err.schema.decode", err))}
		}
	}

	if err := compiled.Validate(instance); err != nil {
		var invalid *jsonschema.ValidationError
		if !errors.As(err, &invalid) {
			return []Error{errorTextAt(file.Path, docRoot(file.Doc), err.Error())}
		}
		return sorted(schemaFindings(file.Path, docRoot(file.Doc), invalid))
	}
	return nil
}

// sorted orders findings the way the file reads, top to bottom. The schema
// reports the unexpected keys of one mapping in an order of its own, which
// would otherwise reach the user as a shuffled list.
func sorted(findings []Error) []Error {
	slices.SortFunc(findings, func(a, b Error) int {
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		if a.Col != b.Col {
			return a.Col - b.Col
		}
		return strings.Compare(a.Msg, b.Msg)
	})
	return findings
}

// schemaFindings turns the schema's error tree into findings that name a place
// in the file.
func schemaFindings(path string, root *yaml.Node, invalid *jsonschema.ValidationError) []Error {
	var findings []Error

	for _, leaf := range reasons(invalid) {
		place := nodeAt(root, leaf.InstanceLocation)

		// A mapping with unexpected keys comes back as one error naming all of
		// them. Report each key where it stands instead, so a file with two
		// typos names two places.
		if unexpected, ok := leaf.ErrorKind.(*kind.AdditionalProperties); ok {
			for _, name := range unexpected.Properties {
				named := &kind.AdditionalProperties{Properties: []string{name}}
				findings = append(findings,
					errorTextAt(path, keyOf(place, name), named.LocalizedString(schemaPrinter())))
			}
			continue
		}

		findings = append(findings, errorTextAt(path, place, leaf.ErrorKind.LocalizedString(schemaPrinter())))
	}
	return findings
}

// reasons returns the errors that carry a reason of their own, skipping the
// wrappers the schema's combinator keywords put around them.
func reasons(invalid *jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(invalid.Causes) == 0 {
		return []*jsonschema.ValidationError{invalid}
	}

	var found []*jsonschema.ValidationError
	for _, cause := range invalid.Causes {
		found = append(found, reasons(cause)...)
	}
	return found
}

// nodeAt walks the document to the place an instance location names, or nil
// when the document has nothing there.
func nodeAt(root *yaml.Node, location []string) *yaml.Node {
	node := root
	for _, step := range location {
		if node == nil {
			return nil
		}
		switch node.Kind {
		case yaml.SequenceNode:
			index, err := strconv.Atoi(step)
			if err != nil || index < 0 || index >= len(node.Content) {
				return nil
			}
			node = node.Content[index]
		case yaml.MappingNode:
			node = field(node, step)
		default:
			return nil
		}
	}
	return node
}

// keyOf returns the key node a mapping holds under a name, or the mapping
// itself when it holds none.
func keyOf(mapping *yaml.Node, name string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return mapping
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			return mapping.Content[i]
		}
	}
	return mapping
}
