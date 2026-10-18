//go:build windows

package cli

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// isTerminal says whether the output of the command goes to a console, which
// is where the question of a clean can be asked.
func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(file.Fd()), &mode) == nil
}
