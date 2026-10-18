// Command sweep is the entry point of the cleaner: with no arguments in a
// terminal it opens the TUI, and it runs the CLI otherwise. Ctrl+C cancels the
// command it was running; the command itself says where it stopped.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"

	"github.com/webcartel/sweep/internal/cli"
)

func main() {
	// Ctrl+C turns into a cancelled context, so that the command the user
	// interrupted can give its partial answer and leave the stream the way
	// the specification asks of it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := cli.NewRootCmd().ExecuteContext(ctx)
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
