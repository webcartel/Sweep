// Package schema publishes the JSON Schema of the rule format. The file is the
// source of truth for rule authors, editors and the validator in internal/rules.
package schema

import _ "embed"

// JSON is schema/rule.schema.json exactly as the repository publishes it.
//
//go:embed rule.schema.json
var JSON []byte
