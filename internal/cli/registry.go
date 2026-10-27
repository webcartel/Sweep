package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/registry"
)

// registryFinding is one finding as the JSON document spells it.
type registryFinding struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Key        string `json:"key"`
	Value      string `json:"value"`
	Path       string `json:"path"`
	Reason     string `json:"reason"`
	Admin      bool   `json:"admin"`
	HasSubkeys bool   `json:"has_subkeys"`
}

// registryScanAnswer is the whole `sweep registry scan --json` document.
type registryScanAnswer struct {
	Schema   int               `json:"schema"`
	Findings []registryFinding `json:"findings"`
}

// newRegistryCmd is the branch for the registry: a narrow, curated set of
// dead entries rather than rules over the whole hive.
func newRegistryCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Find and remove dead registry entries",
		Long: "The registry branch handles the three places Windows leaves dead\n" +
			"entries: uninstall commands, Run values and App Paths. It never runs\n" +
			"without a choice, and it exports a .reg backup before every delete.",
	}
	cmd.AddCommand(newRegistryScanCmd(d), newRegistryCleanCmd(d))
	return cmd
}

// newRegistryScanCmd only reads; it changes nothing.
func newRegistryScanCmd(d deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "List the dead registry entries without changing anything",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if d.Registry == nil {
				return usageError(fmt.Errorf("the registry is not available on this system"))
			}
			findings, notes, err := registry.Scan(d.Registry, d.FS, d.Env)
			if err != nil {
				return err
			}
			for _, note := range notes {
				_, _ = fmt.Fprintln(c.ErrOrStderr(), note)
			}
			if asJSON {
				answer := registryScanAnswer{Schema: 1, Findings: make([]registryFinding, 0, len(findings))}
				for _, finding := range findings {
					answer.Findings = append(answer.Findings, jsonOfFinding(finding))
				}
				encoder := json.NewEncoder(c.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(answer)
			}
			tw := tabwriter.NewWriter(c.OutOrStdout(), 2, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(tw, "KIND\tNAME\tKEY\tADMIN\tREASON"); err != nil {
				return err
			}
			for _, finding := range findings {
				admin := "no"
				if finding.Admin {
					admin = "yes"
				}
				if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					finding.Kind, finding.Name, keyOf(finding), admin, finding.Reason); err != nil {
					return err
				}
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print one JSON document instead of a table")
	return cmd
}

// keyOf names the key (and value, when there is one) a finding lives in.
func keyOf(finding registry.Finding) string {
	if finding.Value == "" {
		return finding.Key
	}
	return finding.Key + `\` + finding.Value
}

// jsonOfFinding copies a finding into its JSON shape.
func jsonOfFinding(finding registry.Finding) registryFinding {
	return registryFinding{
		Kind: string(finding.Kind), ID: finding.ID, Name: finding.Name,
		Key: finding.Key, Value: finding.Value, Path: finding.Path,
		Reason: finding.Reason, Admin: finding.Admin, HasSubkeys: finding.HasSubkeys,
	}
}

// newRegistryCleanCmd is filled in with the cleaner; the plan keeps the
// command declared so the help text is whole.
func newRegistryCleanCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove the chosen dead entries, after a .reg backup",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return usageError(fmt.Errorf("registry clean is not available yet"))
		},
	}
	return cmd
}
