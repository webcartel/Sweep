package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/exec"
	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/rules"
	"github.com/webcartel/sweep/internal/scan"
	"github.com/webcartel/sweep/internal/tui"
)

// deps collects what the CLI reaches the machine through. The root command is
// built from this once; the tests build it from the fakes of a machine of
// their own.
type deps struct {
	FS     platform.FS
	Env    platform.Env
	System platform.System
	Cwd    string
	// LogDir is where the log of a run goes. The machine reads it out of
	// %LOCALAPPDATA% when the builder gives nothing.
	LogDir string
	// TUI is how the run opens its screens. The machine runs the terminal
	// program of bubble tea when the builder gives nothing.
	TUI func(tui.Deps) error
}

// realDeps reads the machine the process runs on; it is all NewRootCmd does.
func realDeps() deps {
	pfs, err := platform.NewFS()
	if err != nil {
		return deps{}
	}
	system, err := platform.NewSystem()
	if err != nil {
		return deps{}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return deps{}
	}
	return deps{FS: pfs, Env: platform.OSEnv{}, System: system, Cwd: cwd}
}

// NewRootCmd returns the root command of the CLI. The frontend only parses
// flags, assembles the rules and the profile and prints events; the work
// itself belongs to the core packages.
//
// Running `sweep` with no arguments in a terminal opens the TUI. Until the TUI
// lands (stage 5) cobra prints this help text instead.
func NewRootCmd() *cobra.Command { return newRootCmd(realDeps()) }

func newRootCmd(d deps) *cobra.Command {
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
	root.PersistentFlags().Bool("ascii", false,
		"draw the screens with the plain characters of ASCII")
	root.PersistentFlags().Bool("no-ascii", false,
		"draw the screens with the frames a terminal draws")
	root.PersistentFlags().String("preselect", "",
		"the rules a run was already agreed to; the way the restart comes back")

	root.RunE = func(c *cobra.Command, args []string) error {
		if len(args) != 0 {
			_ = c.Help()
			return nil
		}
		return openTUI(d, c)
	}

	root.AddCommand(newVersionCmd(), newListCmd(d), newRulesCmd(d), newScanCmd(d), newCleanCmd(d))

	return root
}

// asciiFlags reads the say of the two flags of the drawing.
func asciiFlags(c *cobra.Command) (ascii bool, noASCII bool, err error) {
	ascii, err = c.Flags().GetBool("ascii")
	if err != nil {
		return false, false, err
	}
	noASCII, err = c.Flags().GetBool("no-ascii")
	return ascii, noASCII, err
}

// preselectFlag reads the rules a run was already agreed to.
func preselectFlag(c *cobra.Command) []string {
	preselect, err := c.Flags().GetString("preselect")
	if err != nil || preselect == "" {
		return nil
	}
	var ids []string
	for _, id := range filepath.SplitList(preselect) {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// tuiLogDir is where the log of the screens' run goes: the say of the
// builder first, then the %LOCALAPPDATA% of the machine.
func tuiLogDir(d deps) string {
	if d.LogDir != "" {
		return d.LogDir
	}
	local, _ := d.Env.Get("LOCALAPPDATA")
	return local
}

// openTUI assembles what the run reaches the machine through and lets the
// screens of bubble tea take over.
func openTUI(d deps, c *cobra.Command) error {
	set, err := loadSet(d, c)
	if err != nil {
		return err
	}
	printWarnings(c, set.Warnings)

	names := make([]string, 0, len(set.Profiles))
	for name := range set.Profiles {
		if name != "default" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	rows := make([]tui.ProfileRow, 0, len(set.Profiles)+1)
	if _, ok := set.Profiles["default"]; ok {
		rows = append(rows, tui.ProfileRow{Name: "default", Rules: len(set.Rules), MaxRisk: set.Profiles["default"].MaxRisk})
	}
	for _, name := range names {
		profile := set.Profiles[name]
		chosen, _, err := chooseRules(set, name, nil)
		if err != nil {
			return usageError(err)
		}
		rows = append(rows, tui.ProfileRow{Name: name, Rules: len(chosen), MaxRisk: profile.MaxRisk})
	}

	guarded, err := guard.New(d.FS, d.Env, d.Cwd, set.Config.ExcludePaths)
	if err != nil {
		return usageError(err)
	}
	scanner := scan.New(scan.Deps{FS: d.FS, Env: d.Env, System: d.System, Guard: guarded})
	executor := exec.New(exec.Deps{FS: d.FS, Guard: guarded, System: d.System})

	ascii, noASCII, err := asciiFlags(c)
	if err != nil {
		return usageError(err)
	}
	run := tui.Deps{
		FS: d.FS, Env: d.Env, System: d.System,
		Profiles: rows, Scanner: scanner, Executor: executor,
		AsciiFlag: ascii, NoASCIIFlag: noASCII,
		LogDir:    tuiLogDir(d),
		Preselect: preselectFlag(c),
	}
	run.RulesFor = func(profile string) ([]*model.Rule, error) {
		chosen, _, err := chooseRules(set, profile, nil)
		if err != nil {
			return nil, err
		}
		return chosen, nil
	}
	run.Restart = func(preselect []string) error { return restartAsAdmin(c, preselect) }

	if d.TUI != nil {
		return d.TUI(run)
	}
	return tui.Run(run)
}

// dataDir is where sweep keeps the config and the rules of the user:
// %APPDATA%\sweep on Windows, which is what the spec names. It is the
// environment the frontend was built with that answers, so that a command run
// against the fakes of a test reads a data directory of the test's own.
func dataDir(d deps) (string, error) {
	if place, ok := d.Env.Get("APPDATA"); ok && place != "" {
		return filepath.Join(place, "sweep"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the directory of the config: %w", err)
	}
	return filepath.Join(dir, "sweep"), nil
}

// loadSet reads every rule source the machine and the flags name: the built-in
// rules, the rules of the user, the directories of --rules-dir and the config.
func loadSet(d deps, cmd *cobra.Command) (*rules.Set, error) {
	dir, err := dataDir(d)
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
		Env:       d.Env,
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
