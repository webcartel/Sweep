package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/exec"
	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/i18n"
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
	// Registry is the part of the registry the cleanup reads and changes.
	Registry platform.Registry
	Cwd      string
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
	reg, err := platform.NewRegistry()
	if err != nil {
		return deps{}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return deps{}
	}
	return deps{FS: pfs, Env: platform.OSEnv{}, System: system, Registry: reg, Cwd: cwd}
}

// NewRootCmd returns the root command of the CLI. The frontend only parses
// flags, assembles the rules and the profile and prints events; the work
// itself belongs to the core packages.
//
// Running `sweep` with no arguments in a terminal opens the TUI.
func NewRootCmd() *cobra.Command { return newRootCmd(realDeps(), os.Args[1:]) }

func newRootCmd(d deps, args []string) *cobra.Command {
	// The language is set before a single command is built: cobra prints a
	// command's help from the text that command was built with.
	lang, settingsPath, langErr := langEarly(d, args)
	i18n.Use(lang)

	root := &cobra.Command{
		Use:   "sweep",
		Short: i18n.T("cli.root.short"),
		Long:  i18n.T("cli.root.long"),
		// A command that fails prints the error alone: the usage text of a
		// command is long, and the error is the part worth reading.
		SilenceUsage: true,
	}

	// A flag the command does not have is a mistake on the command line, which
	// the spec gives the same code as a mistake in the config or a rule.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err) })

	root.PersistentFlags().String("config", "",
		i18n.T("cli.root.flag.config"))
	root.PersistentFlags().StringArray("rules-dir", nil,
		i18n.T("cli.root.flag.rulesdir"))
	root.PersistentFlags().Bool("ascii", false,
		i18n.T("cli.root.flag.ascii"))
	root.PersistentFlags().Bool("no-ascii", false,
		i18n.T("cli.root.flag.noascii"))
	root.PersistentFlags().String("preselect", "",
		i18n.T("cli.root.flag.preselect"))
	root.PersistentFlags().String("preselect-registry", "",
		i18n.T("cli.root.flag.preselectregistry"))
	_ = root.PersistentFlags().MarkHidden("preselect-registry")
	root.PersistentFlags().String("lang", "",
		i18n.T("cli.root.flag.lang"))

	// A language sweep does not speak is a mistake to say, and it is said
	// before any command runs.
	if langErr != nil {
		root.PersistentPreRunE = func(*cobra.Command, []string) error { return langErr }
	}

	root.RunE = func(c *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageError(errorText("cli.root.unknownarg", args[0]))
		}
		return openTUI(d, c, settingsPath)
	}

	root.AddCommand(newVersionCmd(), newListCmd(d), newRulesCmd(d), newScanCmd(d), newCleanCmd(d), newRegistryCmd(d))

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

// preselectFlag reads the rules a run was already agreed to: one flag,
// the ids set apart by commas.
func preselectFlag(c *cobra.Command) []string {
	return idsOfFlag(c, "preselect")
}

// preselectRegistryFlag reads the registry findings a restart brought back.
func preselectRegistryFlag(c *cobra.Command) []string {
	return idsOfFlag(c, "preselect-registry")
}

// idsOfFlag reads one comma or semicolon separated flag.
func idsOfFlag(c *cobra.Command, name string) []string {
	preselect, err := c.Flags().GetString(name)
	if err != nil || preselect == "" {
		return nil
	}
	var ids []string
	for _, id := range strings.FieldsFunc(preselect, func(r rune) bool { return r == ',' || r == ';' }) {
		if strings.TrimSpace(id) != "" {
			ids = append(ids, strings.TrimSpace(id))
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
// openTUI is the root command's body: it loads the rules, builds the
// screens and hands the run to them. settingsPath is where the language
// this run picked may be saved again from the interface.
func openTUI(d deps, c *cobra.Command, settingsPath string) error {
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
	countOf := func(name string) int {
		chosen, _, err := chooseRules(set, name, nil)
		if err != nil {
			return 0
		}
		return len(chosen)
	}
	if _, ok := set.Profiles["default"]; ok {
		rows = append(rows, tui.ProfileRow{Name: "default", Rules: countOf("default"), MaxRisk: set.Profiles["default"].MaxRisk})
	}
	for _, name := range names {
		profile := set.Profiles[name]
		rows = append(rows, tui.ProfileRow{Name: name, Rules: countOf(name), MaxRisk: profile.MaxRisk})
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
		LogDir:            tuiLogDir(d),
		Preselect:         preselectFlag(c),
		Set:               set,
		Registry:          d.Registry,
		RegistryPreselect: preselectRegistryFlag(c),
	}
	dir, err := dataDir(d)
	if err != nil {
		return usageError(err)
	}
	cfgPath, err := c.Flags().GetString("config")
	if err != nil {
		return usageError(err)
	}
	if cfgPath == "" {
		cfgPath = filepath.Join(dir, "config.yaml")
	}
	run.RulesDir = filepath.Join(dir, "rules")
	run.ConfigPath = cfgPath
	run.SettingsPath = settingsPath
	run.RulesFor = func(profile string) ([]*model.Rule, error) {
		chosen, _, err := chooseRules(set, profile, nil)
		if err != nil {
			return nil, err
		}
		return chosen, nil
	}
	run.Reload = func() (*rules.Set, error) {
		fresh, err := loadSet(d, c)
		if err != nil {
			return nil, err
		}
		set = fresh
		return fresh, nil
	}
	run.Restart = func(preselect []string) error { return restartAsAdmin(c, preselect) }
	run.RestartRegistry = func(ids []string) error { return restartAsAdminRegistry(c, ids) }

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
		return "", fmt.Errorf("%s: %w", i18n.T("cli.root.datadir"), err)
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

// errorText makes an error out of the text of a key. A translated message is
// not a format of its own, so it never goes to fmt.Errorf unformatted.
func errorText(key string, args ...any) error {
	return errors.New(i18n.T(key, args...))
}
