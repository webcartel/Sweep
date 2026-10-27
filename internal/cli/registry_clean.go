package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

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
		Short: "Remove the chosen dead entries, after a .reg backup",
		Long: "Only the findings named with --select are removed, and every key is\n" +
			"exported to a .reg file before the first deletion.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return runRegistryClean(c, d, flags) },
	}
	cmd.Flags().StringArrayVar(&flags.selectors, "select", nil,
		"a finding to remove: kind:key[:value]; repeat it. A key holding a backslash is written as it is")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "show what would be removed, change nothing")
	cmd.Flags().BoolVar(&flags.yes, "yes", false, "do not ask for a confirmation")
	cmd.Flags().BoolVar(&flags.asJSON, "json", false, "print one JSON document")
	return cmd
}

// runRegistryClean is the body of the command.
func runRegistryClean(cmd *cobra.Command, d deps, flags registryCleanFlags) error {
	if d.Registry == nil {
		return usageError(fmt.Errorf("the registry is not available on this system"))
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
		return usageError(fmt.Errorf("no finding matches %q", missing[0]))
	}
	if flags.dryRun && len(selected) == 0 && len(flags.selectors) == 0 {
		selected = findings
	}
	if !flags.dryRun && len(selected) == 0 {
		if len(findings) == 0 {
			return printRegistryNone(cmd, flags.asJSON)
		}
		return usageError(fmt.Errorf("nothing is chosen: name at least one finding with --select"))
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
			return &ExitError{Code: ExitInternal, Err: fmt.Errorf("%%LOCALAPPDATA%% is not set, so the backup has no home")}
		}
		root = filepath.Join(local, "sweep", "registry-backups")
	}

	if !flags.dryRun && !flags.yes && !isTerminal(cmd.OutOrStdout()) {
		return &ExitError{Code: ExitNoTerminal, Err: fmt.Errorf(
			"there is no terminal to ask; say --yes to remove without the question, or --dry-run to only see the plan")}
	}
	if !flags.asJSON {
		printRegistryTable(cmd.OutOrStdout(), selected)
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
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "removed %d, skipped %d, failed %d\n",
		result.Removed, result.Skipped, result.Failed)
	if result.BackupDir != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "backup %s\n", result.BackupDir)
	}
	if result.Skipped+result.Failed != 0 {
		return &ExitError{Code: ExitPartial, Err: fmt.Errorf(
			"%d findings were left behind; the report names why", result.Skipped+result.Failed)}
	}
	return nil
}

// printRegistryNone answers a run that found nothing.
func printRegistryNone(cmd *cobra.Command, asJSON bool) error {
	if asJSON {
		answer := registryCleanAnswer{Schema: 1, Entries: []registryCleanEntry{}}
		return writeRegistryCleanJSON(cmd, answer)
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), "no dead entries were found")
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
