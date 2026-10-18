package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/rules"
)

// newRulesCmd returns `sweep rules`, the group that works with the rule files
// themselves rather than with what they would clean.
func newRulesCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "Check and inspect the rule files",
		Args:  countArgs(cobra.NoArgs),
	}
	cmd.AddCommand(newRulesValidateCmd(d), newRulesShowCmd(d))
	return cmd
}

// newRulesValidateCmd returns `sweep rules validate`.
func newRulesValidateCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "validate [path...]",
		Short: "Check rule files and report every finding with its place",
		Long: "Check rule files and report every finding as file:line:column on\n" +
			"the error stream.\n\n" +
			"With no path it checks everything sweep loads: the built-in rules, the\n" +
			"rules of the user and every directory named as --rules-dir. A path that\n" +
			"is a directory stands for the rule files it holds. It prints nothing\n" +
			"when there is nothing to report, and leaves with code 2 when there is.",
		Args: countArgs(cobra.ArbitraryArgs),
		RunE: func(cmd *cobra.Command, paths []string) error {
			if len(paths) == 0 {
				return validateEverything(d, cmd)
			}
			return validatePaths(d, cmd, paths)
		},
	}
}

// validateEverything checks every source the loader reads. The findings come
// back as the error of the command, which cobra puts on stderr once, where the
// findings of a checker belong.
func validateEverything(d deps, cmd *cobra.Command) error {
	set, err := loadSet(d, cmd)
	if err != nil {
		return usageError(err)
	}
	printWarnings(cmd, set.Warnings)
	return nil
}

// validatePaths checks the files named on the command line. A directory stands
// for the rule files it holds, so that pointing the command at a folder does
// what the reader expects.
func validatePaths(d deps, cmd *cobra.Command, paths []string) error {
	files, err := ruleFiles(paths)
	if err != nil {
		return usageError(err)
	}

	var (
		findings []error
		warnings []string
	)
	env := d.Env
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			findings = append(findings, err)
			continue
		}
		file, err := rules.LoadFile(path, data)
		if err != nil {
			findings = append(findings, err)
			continue
		}

		found, warns := rules.Validate(file, env)
		for _, finding := range found {
			findings = append(findings, finding)
		}
		warnings = append(warnings, warns...)
	}

	printWarnings(cmd, warnings)
	if len(findings) != 0 {
		return usageError(errors.Join(findings...))
	}
	return nil
}

// ruleFiles turns the paths of the command line into the rule files to check: a
// file stands for itself, a directory for the rule files it holds, in name
// order. A path that cannot be read is an error of its own, named as the
// system named it.
func ruleFiles(paths []string) ([]string, error) {
	var files []string

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, path)
			continue
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
				files = append(files, filepath.Join(path, entry.Name()))
			}
		}
	}
	return files, nil
}

// newRulesShowCmd returns `sweep rules show`.
func newRulesShowCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Print one rule as the sources left it, with its variables expanded",
		Long: "Print the rule the sources were merged into, so that its paths can be\n" +
			"checked without running a scan: the id, what it cleans, and every\n" +
			"target with its %VARIABLES% expanded to what this machine holds.",
		Args: countArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadSet(d, cmd)
			if err != nil {
				return err
			}
			printWarnings(cmd, set.Warnings)

			rule := findRule(set, args[0])
			if rule == nil {
				return usageError(fmt.Errorf("unknown rule '%s'", args[0]))
			}
			return printRule(cmd.OutOrStdout(), rule, d.Env)
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
	w.printf("%s - %s\n", rule.ID, rule.Name)
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
