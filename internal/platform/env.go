package platform

import (
	"os"
	"regexp"
)

// envName matches a %VARIABLE% reference inside a path. Parentheses are allowed
// in the name, since Windows itself sets %ProgramFiles(x86)%.
var envName = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_()]*)%`)

// Env is the environment of the machine, as the core needs to see it. It is an
// interface so that the checks that read it can run against an environment the
// test chooses, instead of the one the test machine happens to have.
type Env interface {
	// Get returns the value of a variable and whether the variable is set.
	Get(name string) (string, bool)
}

// OSEnv reads the environment of the running process.
//
// It holds no WinAPI and therefore carries no build tag: os.LookupEnv answers
// the same question on every platform the tests run on.
type OSEnv struct{}

// Get reads one variable with os.LookupEnv.
func (OSEnv) Get(name string) (string, bool) { return os.LookupEnv(name) }

// EnvNames lists the variables a path refers to, in the order they appear and
// with repeats, so that a caller can report every place it has to look at.
func EnvNames(text string) []string {
	var names []string
	for _, reference := range envName.FindAllStringSubmatch(text, -1) {
		names = append(names, reference[1])
	}
	return names
}

// Expand replaces every %VARIABLE% reference of a path with what the environment
// holds. A variable that is not set stays as written, so that a caller showing
// the result can point at what it did not find.
func Expand(text string, env Env) string {
	return envName.ReplaceAllStringFunc(text, func(reference string) string {
		name := envName.FindStringSubmatch(reference)[1]
		if value, ok := env.Get(name); ok {
			return value
		}
		return reference
	})
}
