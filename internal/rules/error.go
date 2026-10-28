package rules

import (
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/webcartel/sweep/internal/i18n"
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

// errorAt builds a finding anchored to a node of a file, worded in the
// language the interface speaks: the third argument is a catalog key. A nil
// node leaves the position at 0:0, which reads as "somewhere in the file".
func errorAt(file string, node *yaml.Node, key string, args ...any) Error {
	return errorTextAt(file, node, i18n.T(key, args...))
}

// errorTextAt builds a finding whose message is already worded: the text
// of a parser, or an error that came from somewhere else in the program.
func errorTextAt(file string, node *yaml.Node, message string) Error {
	finding := Error{File: file, Msg: message}
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
