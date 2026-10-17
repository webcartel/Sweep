package cli

// Exit codes of the spec's table. The codes a later stage owns — 3 for a run
// that needs a confirmation and has no terminal, 4 for a run that finished in
// part — are not declared until a command returns them.
const (
	// ExitInternal is returned for an error the program did not expect.
	ExitInternal = 1
	// ExitUsage is returned for a mistake the user can fix by changing a flag,
	// the config or a rule file.
	ExitUsage = 2
)

// ExitError carries the code the process should leave with. main reads it; a
// command returns it for anything the user can fix.
type ExitError struct {
	Code int
	Err  error
}

// Error renders the error underneath.
func (e *ExitError) Error() string { return e.Err.Error() }

// Unwrap lets errors.As look through to the error underneath.
func (e *ExitError) Unwrap() error { return e.Err }

// usageError marks an error as something the user can fix by changing a flag,
// the config or a rule file.
func usageError(err error) error {
	return &ExitError{Code: ExitUsage, Err: err}
}
