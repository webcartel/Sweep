//go:build !windows

package cli

import "io"

// isTerminal says whether the output of the command goes to something a
// question could be asked in. The builds that are not Windows exist for the
// tests and the earlier CI signal, and never clean.
func isTerminal(w io.Writer) bool { return false }
