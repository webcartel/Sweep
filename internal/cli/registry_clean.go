package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/registry"
	"github.com/webcartel/sweep/internal/report"
)

// registryCleanEntry is one finding as the clean JSON document spells it.
type registryCleanEntry struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Key    string `json:"key"`
	Value  string `json:"value"`
	Path   string `json:"path"`
	Action string `json:"action"`
	Note   string `json:"note"`
}

// registryCleanAnswer is the one document `registry clean --json` prints.
type registryCleanAnswer struct {
	Schema    int                  `json:"schema"`
	DryRun    bool                 `json:"dry_run"`
	BackupDir string               `json:"backup_dir"`
	Removed   int                  `json:"removed"`
	Skipped   int                  `json:"skipped"`
	Failed    int                  `json:"failed"`
	Entries   []registryCleanEntry `json:"entries"`
}

// registryCleanFlags is the command line of the cleaner.
type registryCleanFlags struct {
	selectors []string
	dryRun    bool
	yes       bool
	asJSON    bool
}

// newRegistryCleanCmd removes the chosen findings after a .reg backup.
func newRegistryCleanCmd(d deps) *cobra.Command {
	var flags registryCleanFlags
	cmd := &cobra.Command{
		Use:   "clean",
		Short: i18n.T("cli.registry.clean.short"),
		Long:  i18n.T("cli.registry.clean.long"),
		Args:  cobra.NoArgs,
		RunE:  func(c *cobra.Command, _ []string) error { return runRegistryClean(c, d, flags) },
	}
	cmd.Flags().StringArrayVar(&flags.selectors, "select", nil,
		i18n.T("cli.registry.clean.flag.select"))
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, i18n.T("cli.registry.clean.flag.dryrun"))
	cmd.Flags().BoolVar(&flags.yes, "yes", false, i18n.T("cli.registry.clean.flag.yes"))
	cmd.Flags().BoolVar(&flags.asJSON, "json", false, i18n.T("cli.common.json"))
	return cmd
}

// runRegistryClean is the body of the command.
func runRegistryClean(cmd *cobra.Command, d deps, flags registryCleanFlags) error {
	if d.Registry == nil {
		return usageError(errorText("cli.registry.unavailable"))
	}
	findings, notes, err := registry.Scan(d.Registry, d.FS, d.Env)
	if err != nil {
		return err
	}
	for _, note := range notes {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), note)
	}
	selected, missing := registry.Select(findings, flags.selectors)
	if len(missing) != 0 {
		return usageError(errorText("cli.registry.clean.nomatch", missing[0]))
	}
	if flags.dryRun && len(selected) == 0 && len(flags.selectors) == 0 {
		selected = findings
	}
	if !flags.dryRun && len(selected) == 0 {
		if len(findings) == 0 {
			return printRegistryNone(cmd, flags.asJSON)
		}
		return usageError(errorText("cli.registry.clean.noselect"))
	}

	elevated := false
	if d.System != nil {
		if strong, err := d.System.Elevated(); err == nil {
			elevated = strong
		}
	}
	root := ""
	if !flags.dryRun {
		local, _ := d.Env.Get("LOCALAPPDATA")
		if local == "" {
			return &ExitError{Code: ExitInternal, Err: errorText("cli.registry.clean.localappdata")}
		}
		root = filepath.Join(local, "sweep", "registry-backups")
	}

	if !flags.dryRun && !flags.yes && !isTerminal(cmd.OutOrStdout()) {
		return &ExitError{Code: ExitNoTerminal, Err: errorText("cli.registry.clean.noterminal")}
	}
	if !flags.asJSON {
		if err := printRegistryTable(cmd.OutOrStdout(), selected); err != nil {
			return err
		}
	}
	if !flags.dryRun && !flags.yes {
		if !askContinue(cmd.InOrStdin(), cmd.OutOrStdout()) {
			return nil
		}
	}

	sink, closeLog, err := openLog(d)
	if err != nil && !flags.dryRun {
		return &ExitError{Code: ExitInternal, Err: err}
	}
	if closeLog != nil {
		defer closeLog()
	}
	options := registry.Options{
		DryRun: flags.dryRun, Elevated: elevated, BackupRoot: root,
		Now: time.Now,
		Log: func(action, path, note string) {
			if sink != nil {
				_ = sink.Append(report.Entry{Time: time.Now(), Path: path, Action: action, Result: note})
			}
		},
	}
	result, err := registry.Clean(d.Registry, selected, options)
	if err != nil {
		return &ExitError{Code: ExitInternal, Err: err}
	}

	if flags.asJSON {
		return printRegistryCleanJSON(cmd, flags.dryRun, result)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), i18n.T("cli.registry.clean.summary"),
		result.Removed, result.Skipped, result.Failed)
	if result.BackupDir != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), i18n.T("cli.registry.clean.backup"), result.BackupDir)
	}
	if result.Skipped+result.Failed != 0 {
		return &ExitError{Code: ExitPartial, Err: errorText("cli.registry.clean.leftbehind",
			result.Skipped+result.Failed)}
	}
	return nil
}

// printRegistryNone answers a run that found nothing.
func printRegistryNone(cmd *cobra.Command, asJSON bool) error {
	if asJSON {
		answer := registryCleanAnswer{Schema: 1, Entries: []registryCleanEntry{}}
		return writeRegistryCleanJSON(cmd, answer)
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), i18n.T("cli.registry.clean.none"))
	return err
}

// printRegistryCleanJSON writes the one document of the run.
func printRegistryCleanJSON(cmd *cobra.Command, dryRun bool, result registry.Result) error {
	answer := registryCleanAnswer{
		Schema: 1, DryRun: dryRun, BackupDir: result.BackupDir,
		Removed: result.Removed, Skipped: result.Skipped, Failed: result.Failed,
		Entries: make([]registryCleanEntry, 0, len(result.Entries)),
	}
	for _, entry := range result.Entries {
		answer.Entries = append(answer.Entries, registryCleanEntry{
			ID: entry.Finding.ID, Kind: string(entry.Finding.Kind), Name: entry.Finding.Name,
			Key: entry.Finding.Key, Value: entry.Finding.Value, Path: entry.Finding.Path,
			Action: entry.Action, Note: entry.Note,
		})
	}
	return writeRegistryCleanJSON(cmd, answer)
}

// writeRegistryCleanJSON encodes the document with an indent that reads.
func writeRegistryCleanJSON(cmd *cobra.Command, answer registryCleanAnswer) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(answer)
}
