// Command sweep is the entry point of the cleaner: with no arguments in a
// terminal it opens the TUI, and it runs the CLI otherwise.
package main

import (
	"errors"
	"os"

	"github.com/webcartel/sweep/internal/cli"
)

func main() {
	err := cli.NewRootCmd().Execute()
	if err == nil {
		return
	}

	// A command that knows what the user has to fix says which code to leave
	// with; anything else is an error the program did not expect.
	code := cli.ExitInternal
	var exit *cli.ExitError
	if errors.As(err, &exit) {
		code = exit.Code
	}
	os.Exit(code)
}
