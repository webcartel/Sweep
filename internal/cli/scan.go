package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/i18n"
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
		Short: i18n.T("cli.scan.short"),
		Long:  i18n.T("cli.scan.long"),
		Args:  countArgs(cobra.NoArgs),
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
				// What the machine's own config asked the guard to use is
				// the user's business to fix: it goes with the code of the
				// flags, not the code of an error.
				return usageError(err)
			}
			scanner := scan.New(scan.Deps{
				FS: d.FS, Env: d.Env, System: d.System, Guard: guarded,
			})

			// The rules that finished before the interruption stay in the
			// answer: the spec asks for the partial result, not silence.
			var finished []model.RulePlan
			var plan *model.Plan
			for event := range scanner.Scan(cmd.Context(), rulesList, model.ScanOptions{}) {
				switch one := event.(type) {
				case model.Warning:
					// The spec keeps what is not the answer on stderr, even
					// when the answer is one JSON document.
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), one.Message)
				case model.ItemSkipped:
					// In the table answer the skips sit in the SKIPPED
					// column only by their number; what path was left out
					// and why goes on the stream of remarks. The JSON
					// document holds them and says nothing twice.
					if !asJSON {
						_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
							i18n.T("cli.scan.skipped", one.Skip.Path, string(one.Skip.Reason)))
					}
				case model.RuleScanned:
					finished = append(finished, one.Plan)
				case model.ScanDone:
					plan = one.Plan
				}
			}

			if cmd.Context().Err() != nil {
				// What stood completed when Ctrl+C came is printed, then the
				// word of the interruption.
				if len(finished) != 0 {
					partial := &model.Plan{CreatedAt: time.Now(), Rules: finished}
					if asJSON {
						_ = report.PlanJSON(cmd.OutOrStdout(), partial)
					} else {
						_ = printPlanTable(cmd.OutOrStdout(), partial)
					}
				}
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("cli.scan.interrupted"))
				return nil
			}
			if plan == nil {
				return errorText("cli.scan.noplan")
			}

			if asJSON {
				return report.PlanJSON(cmd.OutOrStdout(), plan)
			}
			return printPlanTable(cmd.OutOrStdout(), plan)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "default", i18n.T("cli.scan.flag.profile"))
	cmd.Flags().StringArrayVar(&ruleFlags, "rule", nil,
		i18n.T("cli.scan.flag.rule"))
	cmd.Flags().BoolVar(&asJSON, "json", false, i18n.T("cli.common.json"))

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
		return nil, nil, usageError(errorText("cli.scan.norule",
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
	_, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
		i18n.T("cli.scan.header.id"), i18n.T("cli.scan.header.items"),
		i18n.T("cli.scan.header.files"), i18n.T("cli.scan.header.size"),
		i18n.T("cli.scan.header.skipped"))
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
	if _, err := fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%d\n",
		i18n.T("cli.scan.total"),
		total.items, total.files, report.FormatBytes(total.size), total.skipped); err != nil {
		return err
	}
	return tw.Flush()
}
