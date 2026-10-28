package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/i18n"
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
		Short: i18n.T("cli.registry.short"),
		Long:  i18n.T("cli.registry.long"),
	}
	cmd.AddCommand(newRegistryScanCmd(d), newRegistryCleanCmd(d))
	return cmd
}

// newRegistryScanCmd only reads; it changes nothing.
func newRegistryScanCmd(d deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "scan",
		Short: i18n.T("cli.registry.scan.short"),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if d.Registry == nil {
				return usageError(errorText("cli.registry.unavailable"))
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
			if err := printRegistryTable(c.OutOrStdout(), findings); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, i18n.T("cli.registry.scan.flag.json"))
	return cmd
}

// printRegistryTable draws the findings: kind, name, key, admin and why.
func printRegistryTable(out io.Writer, findings []registry.Finding) error {
	tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
	if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
		i18n.T("cli.registry.header.kind"), i18n.T("cli.registry.header.name"),
		i18n.T("cli.registry.header.key"), i18n.T("cli.registry.header.admin"),
		i18n.T("cli.registry.header.reason")); err != nil {
		return err
	}
	for _, finding := range findings {
		admin := i18n.T("cli.common.no")
		if finding.Admin {
			admin = i18n.T("cli.common.yes")
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			finding.Kind, finding.Name, keyOf(finding), admin, finding.Reason); err != nil {
			return err
		}
	}
	return tw.Flush()
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

// keyOf names the key (and value, when there is one) a finding lives in.
