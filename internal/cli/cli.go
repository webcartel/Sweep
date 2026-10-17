package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd returns the root command of the CLI. The frontend only parses
// flags, assembles the rules and the profile and prints events; the work
// itself belongs to the core packages.
//
// Running `sweep` with no arguments in a terminal opens the TUI. Until the TUI
// lands (stage 5) cobra prints this help text instead.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "sweep",
		Short: "Clean Windows according to YAML rules",
		Long: "sweep cleans a Windows machine according to a set of YAML rules.\n\n" +
			"Run it with no arguments to open the TUI, `sweep scan` to see what\n" +
			"would be cleaned without changing anything, and `sweep clean` to\n" +
			"remove it after confirmation.",
	}

	root.AddCommand(newVersionCmd())

	return root
}
