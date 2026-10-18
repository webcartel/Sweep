package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/report"
	"github.com/webcartel/sweep/internal/rules"
	"github.com/webcartel/sweep/internal/scan"
)

// newScanCmd returns `sweep scan`: it reads the disk and says what would be
// cleaned, without changing anything.
func newScanCmd(d deps) *cobra.Command {
	var (
		profile   string
		ruleFlags []string
		asJSON    bool
	)
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Say what would be cleaned, without cleaning it",
		Long: "Read the disk and print the plan: what every rule of the profile\n" +
			"holds, how much it would free and what it could not take. Nothing is\n" +
			"written or removed by this command.",
		Args: countArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			set, err := loadSet(d, cmd)
			if err != nil {
				return err
			}
			printWarnings(cmd, set.Warnings)

			rulesList, warnings, err := chooseRules(set, profile, ruleFlags)
			if err != nil {
				return err
			}
			printWarnings(cmd, warnings)

			guarded, err := guard.New(d.FS, d.Env, d.Cwd, set.Config.ExcludePaths)
			if err != nil {
				return err
			}
			scanner := scan.New(scan.Deps{
				FS: d.FS, Env: d.Env, System: d.System, Guard: guarded,
			})

			var plan *model.Plan
			for event := range scanner.Scan(cmd.Context(), rulesList, model.ScanOptions{}) {
				switch one := event.(type) {
				case model.Warning:
					// The spec keeps what is not the answer on stderr, even
					// when the answer is one JSON document.
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), one.Message)
				case model.ScanDone:
					plan = one.Plan
				}
			}

			if cmd.Context().Err() != nil {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "scan interrupted")
				return nil
			}
			if plan == nil {
				return fmt.Errorf("the scan ended without a plan")
			}

			if asJSON {
				return report.PlanJSON(cmd.OutOrStdout(), plan)
			}
			return printPlanTable(cmd.OutOrStdout(), plan)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "default", "profile of rules to scan for")
	cmd.Flags().StringArrayVar(&ruleFlags, "rule", nil,
		"id or mask of the rules to scan for; repeat it and it overrides the profile")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print one JSON document")

	return cmd
}

// chooseRules works out the rules a scan runs: those the profile takes, or the
// ones the --rule flags name, by id or by mask, wherever they came from.
func chooseRules(set *rules.Set, profile string, ruleFlags []string) ([]*model.Rule, []string, error) {
	if len(ruleFlags) == 0 {
		selected, warnings, err := rules.Select(set, profile)
		return selected, warnings, usageToUsage(err)
	}

	var masks []*masksOf
	for _, flag := range ruleFlags {
		compiled, err := platform.CompileMask(flag)
		if err != nil {
			// A mask that does not compile is still an id to compare by.
			compiled = nil
		}
		masks = append(masks, &masksOf{given: flag, mask: compiled})
	}

	var (
		found   []*model.Rule
		warning []string
	)
	for _, rule := range set.Rules {
		for _, one := range masks {
			if one.matches(rule.ID) {
				found = append(found, rule)
				break
			}
		}
	}
	if len(found) == 0 {
		return nil, nil, usageError(fmt.Errorf(
			"no rule matches --rule %s; run `sweep list` to see what there is",
			strings.Join(ruleFlags, ", ")))
	}
	return found, warning, nil
}

// usageToUsage keeps an error of the rule sources at the code of the flags; it
// is here so that chooseRules reads straight through.
func usageToUsage(err error) error {
	if err != nil {
		return usageError(err)
	}
	return nil
}

// masksOf is one --rule flag: an id or a mask, and the mask compiled when it
// was one at all.
type masksOf struct {
	given string
	mask  *platform.Mask
}

// matches says whether the flag answers for a rule: an exact id is matched
// first, and a mask runs on the id the way Windows compares paths.
func (m *masksOf) matches(id string) bool {
	if strings.EqualFold(m.given, id) {
		return true
	}
	return m.mask != nil && m.mask.Match(id)
}

// printPlanTable prints the plan as the table of the spec: one row per rule,
// one row for the whole scan, with the sizes in the short form.
func printPlanTable(out interface{ Write([]byte) (int, error) }, p *model.Plan) error {
	tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
	_, err := fmt.Fprintln(tw, "ID\tITEMS\tFILES\tSIZE\tSKIPPED")
	if err != nil {
		return err
	}

	var total struct {
		items, files, skipped int
		size                  int64
	}
	for _, rule := range p.Rules {
		name := ""
		if rule.Rule != nil {
			name = rule.Rule.ID
		}
		if _, err := fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%d\n",
			name, len(rule.Items), rule.Files, report.FormatBytes(rule.Bytes), len(rule.Skipped)); err != nil {
			return err
		}
		total.items += len(rule.Items)
		total.files += rule.Files
		total.skipped += len(rule.Skipped)
		total.size += rule.Bytes
	}
	if _, err := fmt.Fprintf(tw, "TOTAL\t%d\t%d\t%s\t%d\n",
		total.items, total.files, report.FormatBytes(total.size), total.skipped); err != nil {
		return err
	}
	return tw.Flush()
}
