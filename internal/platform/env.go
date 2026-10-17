package platform

import "os"

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
