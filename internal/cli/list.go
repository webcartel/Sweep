package cli

import (
	"encoding/json"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/rules"
)

// listRow is one rule of `sweep list`, in the table and in the JSON document.
type listRow struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Category      string `json:"category"`
	Risk          string `json:"risk"`
	InProfile     bool   `json:"in_profile"`
	RequiresAdmin bool   `json:"requires_admin"`
	Source        string `json:"source"`
}

// listReport is the whole `sweep list --json` answer.
type listReport struct {
	Schema  int       `json:"schema"`
	Profile string    `json:"profile"`
	Rules   []listRow `json:"rules"`
}

// newListCmd returns `sweep list`.
func newListCmd() *cobra.Command {
	var (
		category string
		profile  string
		asJSON   bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the rules and say which of them the profile takes",
		Long: "List every rule sweep loaded, where it came from and whether the\n" +
			"profile takes it, in the order the sources give them: the built-in\n" +
			"rules first, then the ones of the user.",
		Args: countArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			set, err := loadSet(cmd)
			if err != nil {
				return err
			}
			printWarnings(cmd, set.Warnings)

			report, warnings, err := buildList(set, profile, category)
			if err != nil {
				return err
			}
			printWarnings(cmd, warnings)

			if asJSON {
				return printListJSON(cmd.OutOrStdout(), report)
			}
			return printListTable(cmd.OutOrStdout(), report)
		},
	}

	cmd.Flags().StringVar(&category, "category", "", "only rules of this category")
	cmd.Flags().StringVar(&profile, "profile", "default", "profile of rules to check against")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print one JSON document")

	return cmd
}

// buildList collects the rows of `sweep list`: every rule of the set, in the
// order the set keeps them, each with whether the profile takes it.
func buildList(set *rules.Set, profile, category string) (listReport, []string, error) {
	selected, warnings, err := rules.Select(set, profile)
	if err != nil {
		return listReport{}, nil, usageError(err)
	}

	taken := map[string]bool{}
	for _, rule := range selected {
		taken[rule.ID] = true
	}

	report := listReport{Schema: 1, Profile: profile, Rules: []listRow{}}
	for _, rule := range set.Rules {
		if category != "" && rule.Category != category {
			continue
		}
		report.Rules = append(report.Rules, listRow{
			ID:            rule.ID,
			Name:          rule.Name,
			Category:      rule.Category,
			Risk:          string(rule.Risk),
			InProfile:     taken[rule.ID],
			RequiresAdmin: rule.RequiresAdmin,
			Source:        rule.Source,
		})
	}
	return report, warnings, nil
}

// printListJSON writes the document a script reads.
func printListJSON(out io.Writer, report listReport) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// printListTable writes the same thing for a person to read.
func printListTable(out io.Writer, report listReport) error {
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	w := &writer{out: table}
	w.printf("ID\tRISK\tPROFILE\tADMIN\n")
	for _, row := range report.Rules {
		w.printf("%s\t%s\t%s\t%s\n",
			row.ID, row.Risk, yesNo(row.InProfile), yesNo(row.RequiresAdmin))
	}
	if w.err != nil {
		return w.err
	}
	return table.Flush()
}

// yesNo renders a flag the way the table reads.
func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
