package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/exec"
	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/report"
	"github.com/webcartel/sweep/internal/scan"
)

// Exit codes the clean command answers with on top of those every command
// shares.
const (
	// ExitNoTerminal is returned for a confirm run whose confirmation no
	// one can give: there is no terminal to ask, and no --yes was given.
	ExitNoTerminal = 3
	// ExitPartial is returned for a run that finished with paths left
	// behind, be it skipped or failed.
	ExitPartial = 4
)

// newCleanCmd returns `sweep clean`: it scans, shows the plan, asks, and
// then takes what was agreed to.
func newCleanCmd(d deps) *cobra.Command {
	var (
		profile          string
		ruleFlags        []string
		dryRun           bool
		yes              bool
		trashFlag        bool
		noTrashFlag      bool
		includeDangerous bool
		onReboot         bool
		reportPath       string
		asJSON           bool
	)
	cmd := &cobra.Command{
		Use:   "clean",
		Short: i18n.T("cli.clean.short"),
		Long:  i18n.T("cli.clean.long"),
		Args:  countArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cleanRun(d, cmd, cleanFlags{
				profile: profile, ruleFlags: ruleFlags, dryRun: dryRun, yes: yes,
				trash: &trashFlag, noTrash: &noTrashFlag,
				includeDangerous: includeDangerous, onReboot: onReboot,
				report: reportPath, json: asJSON,
			})
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "default", i18n.T("cli.clean.flag.profile"))
	cmd.Flags().StringArrayVar(&ruleFlags, "rule", nil,
		i18n.T("cli.clean.flag.rule"))
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, i18n.T("cli.clean.flag.dryrun"))
	cmd.Flags().BoolVar(&yes, "yes", false, i18n.T("cli.clean.flag.yes"))
	cmd.Flags().BoolVar(&trashFlag, "trash", false, i18n.T("cli.clean.flag.trash"))
	cmd.Flags().BoolVar(&noTrashFlag, "no-trash", false, i18n.T("cli.clean.flag.notrash"))
	cmd.Flags().BoolVar(&includeDangerous, "include-dangerous", false,
		i18n.T("cli.clean.flag.dangerous"))
	cmd.Flags().BoolVar(&onReboot, "on-reboot", false,
		i18n.T("cli.clean.flag.onreboot"))
	cmd.Flags().StringVar(&reportPath, "report", "", i18n.T("cli.clean.flag.report"))
	cmd.Flags().BoolVar(&asJSON, "json", false, i18n.T("cli.common.json"))

	return cmd
}

// cleanFlags is what the flags of clean worked out to, taken out of the
// command so that the walk of a run can be tested on its own.
type cleanFlags struct {
	profile          string
	ruleFlags        []string
	dryRun, yes      bool
	trash, noTrash   *bool
	includeDangerous bool
	onReboot         bool
	report           string
	json             bool
}

// cleanRun reads the rules, plans once, and runs what the plan holds through
// the executor, telling the log, the stream of remarks and the answer.
func cleanRun(d deps, cmd *cobra.Command, flags cleanFlags) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	set, err := loadSet(d, cmd)
	if err != nil {
		return err
	}
	printWarnings(cmd, set.Warnings)

	selectList := set
	if flags.includeDangerous {
		// The agreement to see the dangerous rules needs the ceiling of
		// the profile lifted: without it the rules stay out before the
		// plan even knows them.
		if _, ok := selectList.Profiles[flags.profile]; ok {
			raised := map[string]model.Profile{}
			for name, profile := range selectList.Profiles {
				profile.MaxRisk = model.RiskDangerous
				raised[name] = profile
			}
			raisedSet := *selectList
			raisedSet.Profiles = raised
			selectList = &raisedSet
		}
	}
	rulesList, warnings, err := chooseRules(selectList, flags.profile, flags.ruleFlags)
	if err != nil {
		return err
	}
	printWarnings(cmd, warnings)

	guarded, err := guard.New(d.FS, d.Env, d.Cwd, set.Config.ExcludePaths)
	if err != nil {
		return usageError(err)
	}
	scanner := scan.New(scan.Deps{FS: d.FS, Env: d.Env, System: d.System, Guard: guarded})

	var plan *model.Plan
	for event := range scanner.Scan(cmd.Context(), rulesList, model.ScanOptions{}) {
		switch one := event.(type) {
		case model.Warning:
			_, _ = fmt.Fprintln(errOut, one.Message)
		case model.ItemSkipped:
			if !flags.json {
				_, _ = fmt.Fprintln(errOut,
					i18n.T("cli.clean.skipped", one.Skip.Path, string(one.Skip.Reason)))
			}
		case model.ScanDone:
			plan = one.Plan
		}
	}
	if cmd.Context().Err() != nil {
		// What stood completed when Ctrl+C came is printed, then the
		// word of the interruption; the ask for a run that never got to
		// take anything away is not asked.
		_, _ = fmt.Fprintln(errOut, i18n.T("cli.scan.interrupted"))
		return nil
	}
	if plan == nil {
		return errorText("cli.scan.noplan")
	}

	// A rule whose data the user may still want stays out of the run
	// unless the command line said otherwise; the flags can still name
	// one with --rule, which is their own agreement to give.
	selection := model.Selection{Rules: map[string]bool{}, Exclude: map[string]bool{}}
	for _, rp := range plan.Rules {
		if rp.Rule == nil {
			// The executor keeps a rule with no id under an empty name.
			selection.Rules[""] = true
			continue
		}
		if rp.Rule.Risk == model.RiskDangerous && !flags.includeDangerous {
			continue
		}
		selection.Rules[rp.Rule.ID] = true
	}

	// The trash of a dangerous rule is the only say the run has over what
	// the user may still want: deleting it for good is not on offer.
	if flags.noTrash != nil && *flags.noTrash {
		var dangerous []string
		for _, rp := range plan.Rules {
			if rp.Rule != nil && rp.Rule.Risk == model.RiskDangerous {
				dangerous = append(dangerous, rp.Rule.ID)
			}
		}
		if len(dangerous) != 0 {
			return usageError(errorText("cli.clean.dangerous",
				strings.Join(dangerous, ", ")))
		}
	}

	// A run whose work cannot be told afterwards does not start at all:
	// the log is set up before anything is taken away.
	sink, closeLog, err := openLog(d)
	if err != nil {
		return &ExitError{Code: ExitInternal, Err: err}
	}
	defer closeLog()

	options := model.ExecOptions{
		DryRun:   flags.dryRun,
		OnReboot: flags.onReboot,
	}
	if flags.trash != nil && *flags.trash {
		over := true
		options.Trash = &over
	} else if flags.noTrash != nil && *flags.noTrash {
		over := false
		options.Trash = &over
	}

	// A clean that cannot ask is not a clean that may run: without a
	// terminal the agreement has to be said on the command line.
	if !flags.dryRun && !flags.yes && !isTerminal(cmd.OutOrStdout()) {
		return &ExitError{Code: ExitNoTerminal, Err: errorText("cli.clean.noterminal")}
	}
	if !flags.dryRun && !flags.yes {
		if !askContinue(cmd.InOrStdin(), out) {
			return nil
		}
	}

	// The answer of the run goes to --report's file when one is named,
	// and to the output of the command otherwise. The plan is printed
	// before the question regardless, the way `sweep scan` answers, so
	// that what the agreement asks about is on the screen.
	if flags.json {
		_ = report.PlanJSON(cmd.OutOrStdout(), plan)
	} else {
		_ = printPlanTable(cmd.OutOrStdout(), plan)
	}
	var answer = out
	if flags.report != "" {
		file, err := createFile(flags.report)
		if err != nil {
			return &ExitError{Code: ExitInternal, Err: err}
		}
		defer func() { _ = file.Close() }()
		answer = file
	}

	var (
		result  *model.Result
		skipped int
		failed  int
	)
	for event := range exec.New(exec.Deps{
		FS: d.FS, Guard: guarded, System: d.System,
	}).Execute(cmd.Context(), plan, selection, options) {
		switch one := event.(type) {
		case model.ItemSkipped:
			skipped++
			if !flags.json {
				_, _ = fmt.Fprintln(errOut,
					"skipped "+one.Skip.Path+": "+string(one.Skip.Reason))
			}
			_ = sink.Append(report.Entry{
				Time: time.Now(), RuleID: one.RuleID, Path: one.Skip.Path,
				Action: "skipped", Result: string(one.Skip.Reason),
			})
		case model.ItemRemoved:
			action := "removed"
			if one.ToTrash {
				action = "trashed"
			}
			_ = sink.Append(report.Entry{
				Time: time.Now(), RuleID: one.RuleID, Path: one.Path,
				Action: action, Size: one.Bytes, Result: "ok",
			})
		case model.ItemPending:
			_ = sink.Append(report.Entry{
				Time: time.Now(), RuleID: one.RuleID, Path: one.Path,
				Action: "pending", Result: "ok",
			})
		case model.ItemFailed:
			failed++
			if !flags.json {
				_, _ = fmt.Fprintln(errOut, i18n.T("cli.clean.failed", one.Path, one.Err.Error()))
			}
			_ = sink.Append(report.Entry{
				Time: time.Now(), RuleID: one.RuleID, Path: one.Path,
				Action: "failed", Result: one.Err.Error(),
			})
		case model.CommandRan:
			_ = sink.Append(report.Entry{
				Time: time.Now(), RuleID: one.RuleID, Path: one.Command,
				Action: "ran", Result: "ok",
			})
		case model.ExecDone:
			result = one.Result
			result.DryRun = flags.dryRun
			// What was on offer is what the run's own selection held:
			// the scan counted it before the question was asked.
			for _, rp := range plan.Rules {
				if rp.Rule != nil && !selection.Rules[rp.Rule.ID] {
					continue
				}
				result.Items += len(rp.Items)
				result.Files += int64(rp.Files)
				result.BytesFound += rp.Bytes
			}
			if err := report.ExecJSON(answer, result); err != nil {
				return err
			}
		}
	}
	if result == nil && cmd.Context().Err() != nil {
		// The stream closed because the run was cancelled. The paths the
		// rules made it through are on the disk in the order the events
		// named them; the run says so instead of an error.
		_, _ = fmt.Fprintln(errOut, i18n.T("cli.clean.interrupted"))
		return nil
	}
	if result == nil {
		return errorText("cli.clean.noanswer")
	}

	if flags.report == "" && !flags.json {
		printRunSummary(out, result)
	}
	// A dry run took nothing away, so nothing can be left behind; the
	// skips and the failures are named, but they are not debts of a run.
	if flags.dryRun {
		return nil
	}
	if skipped+failed != 0 {
		return &ExitError{Code: ExitPartial, Err: errorText("cli.clean.leftbehind",
			skipped+failed)}
	}
	return nil
}

// askContinue puts the question of the spec and takes the answer: only the
// word of an agreement starts the run.
func askContinue(in io.Reader, out io.Writer) bool {
	if _, err := fmt.Fprint(out, i18n.T("cli.clean.continue")); err != nil {
		return false
	}
	said := bufio.NewScanner(in)
	if !said.Scan() {
		return false
	}
	answer := strings.TrimSpace(said.Text())
	if len(answer) != 1 || (answer[0] != 'y' && answer[0] != 'Y') {
		return false
	}
	return true
}

// printRunSummary says in a few short lines what a run came to.
func printRunSummary(out io.Writer, result *model.Result) {
	if _, err := fmt.Fprintf(out, i18n.T("cli.clean.summary"), result.Removed,
		report.FormatBytes(result.FreedBytes)); err != nil {
		return
	}
	if result.TrashBytes != 0 {
		_, _ = fmt.Fprintf(out, i18n.T("cli.clean.summary.trash"), report.FormatBytes(result.TrashBytes))
	}
	if result.DryRun {
		_, _ = fmt.Fprint(out, i18n.T("cli.clean.summary.dryrun"))
	}
	_, _ = fmt.Fprintln(out)
	if result.PendingReboot != nil {
		_, _ = fmt.Fprintf(out, i18n.T("cli.clean.summary.reboot"), len(result.PendingReboot))
	}
}

// openLog opens today's log under the data folder of the machine, and hands
// the sink it writes through back together with the closing of the file.
func openLog(d deps) (report.LogSink, func(), error) {
	local := d.LogDir
	if local == "" {
		local, _ = d.Env.Get("LOCALAPPDATA")
	}
	if local == "" {
		return nil, nil, errorText("cli.clean.log.localappdata")
	}
	at := time.Now()
	log := report.LogFile(local, at)
	if err := os.MkdirAll(filepath.Dir(log), 0o700); err != nil {
		return nil, nil, errorText("cli.clean.log.open", err)
	}
	file, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", i18n.T("cli.clean.log.file", log), err)
	}
	return report.LogTo(file), func() { _ = file.Close() }, nil
}

// createFile makes the file at the path given, with the directories it
// needs on the way, for the run answer of --report and nothing else.
func createFile(path string) (*os.File, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	return os.Create(path)
}
