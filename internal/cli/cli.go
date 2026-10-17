package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/rules"
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
		// A command that fails prints the error alone: the usage text of a
		// command is long, and the error is the part worth reading.
		SilenceUsage: true,
	}

	// A flag the command does not have is a mistake on the command line, which
	// the spec gives the same code as a mistake in the config or a rule.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err) })

	root.PersistentFlags().String("config", "",
		"path to config.yaml (default %APPDATA%\\sweep\\config.yaml)")
	root.PersistentFlags().StringArray("rules-dir", nil,
		"extra directory of rule files; repeat it to read several")

	root.AddCommand(newVersionCmd(), newListCmd(), newRulesCmd())

	return root
}

// dataDir is where sweep keeps the config and the rules of the user:
// %APPDATA%\sweep on Windows, which is what the spec names. os.UserConfigDir
// answers it, and says so instead of quietly reading a relative path when the
// machine has no such directory.
func dataDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the directory of the config: %w", err)
	}
	return filepath.Join(dir, "sweep"), nil
}

// loadSet reads every rule source the machine and the flags name: the built-in
// rules, the rules of the user, the directories of --rules-dir and the config.
func loadSet(cmd *cobra.Command) (*rules.Set, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, usageError(err)
	}

	path, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, usageError(err)
	}
	if path == "" {
		path = filepath.Join(dir, "config.yaml")
	}
	config, err := rules.LoadConfig(path)
	if err != nil {
		return nil, usageError(err)
	}

	dirs, err := cmd.Flags().GetStringArray("rules-dir")
	if err != nil {
		return nil, usageError(err)
	}

	set, err := rules.Loader{
		Env:       platform.OSEnv{},
		UserDir:   filepath.Join(dir, "rules"),
		ExtraDirs: dirs,
		Config:    config,
	}.Load()
	if err != nil {
		return nil, usageError(err)
	}
	return set, nil
}

// printWarnings writes what the merge and the profile had to say where the spec
// keeps everything that is not the answer.
func printWarnings(cmd *cobra.Command, warnings []string) {
	for _, warning := range warnings {
		// A warning that cannot be written is not worth failing a command over:
		// the answer the caller asked for is already on its way to stdout.
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), warning)
	}
}

// writer writes a series of lines and remembers the first error. A printer that
// stopped at the first failed write would leave the user with half a line; this
// one finishes and hands the error back at the end.
type writer struct {
	out io.Writer
	err error
}

// printf writes one line, unless an earlier write already failed.
func (w *writer) printf(format string, args ...any) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintf(w.out, format, args...)
}

// countArgs turns a cobra argument check into a usage error, so that a call with
// the wrong arguments leaves the process with the code of the spec.
func countArgs(check cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := check(cmd, args); err != nil {
			return usageError(err)
		}
		return nil
	}
}
