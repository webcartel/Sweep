// Command sweep is the entry point of the cleaner: with no arguments in a
// terminal it opens the TUI, and it runs the CLI otherwise.
package main

import (
	"os"

	"github.com/webcartel/sweep/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
