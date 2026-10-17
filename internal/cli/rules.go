package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/rules"
)

// newRulesCmd returns `sweep rules`, the group that works with the rule files
// themselves rather than with what they would clean.
func newRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "Check and inspect the rule files",
		Args:  countArgs(cobra.NoArgs),
	}
	cmd.AddCommand(newRulesValidateCmd(), newRulesShowCmd())
	return cmd
}

// newRulesValidateCmd returns `sweep rules validate`.
func newRulesValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [path...]",
		Short: "Check rule files and report every finding with its place",
		Long: "Check rule files and report every finding as file:line:column.\n\n" +
			"With no path it checks everything sweep loads: the built-in rules, the\n" +
			"rules of the user and every directory named as --rules-dir. It prints\n" +
			"nothing when there is nothing to report, and leaves with code 2 when\n" +
			"there is.",
		Args: countArgs(cobra.ArbitraryArgs),
		RunE: func(cmd *cobra.Command, paths []string) error {
			if len(paths) == 0 {
				return validateEverything(cmd)
			}
			return validatePaths(cmd, paths)
		},
	}
}

// validateEverything checks every source the loader reads. What the loader
// reported goes to stdout, where the answer of the command belongs.
func validateEverything(cmd *cobra.Command) error {
	set, err := loadSet(cmd)
	if err != nil {
		w := &writer{out: cmd.OutOrStdout()}
		w.printf("%v\n", err)
		return usageError(errors.New("the rule files have findings"))
	}
	printWarnings(cmd, set.Warnings)
	return nil
}

// validatePaths checks the files named on the command line.
func validatePaths(cmd *cobra.Command, paths []string) error {
	var (
		findings []error
		warnings []string
	)

	env := platform.OSEnv{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			findings = append(findings, fmt.Errorf("read %s: %w", path, err))
			continue
		}
		file, err := rules.LoadFile(path, data)
		if err != nil {
			findings = append(findings, err)
			continue
		}

		found := rules.ValidateSchema(file)
		semantic, warns := rules.ValidateSemantics(file, env)
		for _, finding := range append(found, semantic...) {
			findings = append(findings, finding)
		}
		warnings = append(warnings, warns...)
	}

	printWarnings(cmd, warnings)
	w := &writer{out: cmd.OutOrStdout()}
	for _, finding := range findings {
		w.printf("%v\n", finding)
	}
	if len(findings) != 0 {
		return usageError(errors.New("the rule files have findings"))
	}
	return w.err
}

// newRulesShowCmd returns `sweep rules show`.
func newRulesShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Print one rule as the sources left it, with its variables expanded",
		Long: "Print the rule the sources were merged into, so that its paths can be\n" +
			"checked without running a scan: the id, what it cleans, and every\n" +
			"target with its %VARIABLES% expanded to what this machine holds.",
		Args: countArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadSet(cmd)
			if err != nil {
				return err
			}
			printWarnings(cmd, set.Warnings)

			rule := findRule(set, args[0])
			if rule == nil {
				return usageError(fmt.Errorf("unknown rule '%s'", args[0]))
			}
			return printRule(cmd.OutOrStdout(), rule, platform.OSEnv{})
		},
	}
}

// findRule returns the rule of a set by id, or nil when no source defines it.
func findRule(set *rules.Set, id string) *model.Rule {
	for _, rule := range set.Rules {
		if rule.ID == id {
			return rule
		}
	}
	return nil
}

// printRule writes one merged rule the way `sweep rules show` reads it.
func printRule(out io.Writer, rule *model.Rule, env platform.Env) error {
	w := &writer{out: out}
	w.printf("%s — %s\n", rule.ID, rule.Name)
	if rule.Description != "" {
		w.printf("  %s\n", rule.Description)
	}
	w.printf("  category: %s\n  risk: %s\n  source: %s\n",
		rule.Category, rule.Risk, rule.Source)
	if rule.DefaultEnabled {
		w.printf("  in the default profile\n")
	}
	if len(rule.Tags) > 0 {
		w.printf("  tags: %s\n", strings.Join(rule.Tags, ", "))
	}
	if rule.RequiresAdmin {
		w.printf("  needs administrator rights\n")
	}
	if len(rule.RequiresClosed) > 0 {
		w.printf("  must be closed: %s\n", strings.Join(rule.RequiresClosed, ", "))
	}
	if rule.OSMinBuild > 0 {
		w.printf("  oldest build: %d\n", rule.OSMinBuild)
	}
	if rule.Trash {
		w.printf("  goes to the recycle bin\n")
	}

	w.printf("  targets:\n")
	for _, target := range rule.Targets {
		w.printf("    %s\n", targetLine(target, env))
	}
	return w.err
}

// targetLine renders one target of a rule as a single line.
func targetLine(target model.Target, env platform.Env) string {
	if target.Kind == model.KindCommand {
		command := strings.TrimSpace(strings.Join(append([]string{target.Cmd}, target.Args...), " "))
		return command + " (command)"
	}

	line := fmt.Sprintf("%s (mode: %s", platform.Expand(target.Path, env), target.Mode)
	if target.MinAgeDays > 0 {
		line += fmt.Sprintf(", older than %d day(s)", target.MinAgeDays)
	}
	if target.MinSize > 0 {
		line += fmt.Sprintf(", at least %d bytes", target.MinSize)
	}
	if len(target.Include) > 0 {
		line += ", only: " + strings.Join(target.Include, ", ")
	}
	if len(target.Exclude) > 0 {
		line += ", except: " + strings.Join(target.Exclude, ", ")
	}
	return line + ")"
}
