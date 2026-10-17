package rules

import (
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Error is a finding in a rule file, together with the place it was found.
type Error struct {
	File      string
	Line, Col int
	Msg       string
}

// Error renders a finding the way editors and the CLI expect to read it.
func (e Error) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg)
}

// errorAt builds a finding anchored to a node of a file. A nil node leaves the
// position at 0:0, which reads as "somewhere in the file".
func errorAt(file string, node *yaml.Node, format string, args ...any) Error {
	finding := Error{File: file, Msg: fmt.Sprintf(format, args...)}
	if node != nil {
		finding.Line, finding.Col = node.Line, node.Column
	}
	return finding
}

// syntaxError turns what the YAML parser refused into a finding with a place.
// The parser names the line inside its message and gives nothing else, so the
// column is the beginning of that line; a message without a line keeps 0:0.
func syntaxError(file string, err error) Error {
	message := err.Error()

	if _, after, found := strings.Cut(message, "line "); found {
		digits, rest, colon := strings.Cut(after, ":")
		if line, convErr := strconv.Atoi(digits); colon && convErr == nil {
			return Error{File: file, Line: line, Col: 1, Msg: strings.TrimSpace(rest)}
		}
	}
	return Error{File: file, Msg: message}
}
