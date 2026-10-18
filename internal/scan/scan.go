package scan

import (
	"context"
	"errors"
	"fmt"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
)

// Scanner reads the disk and builds the plan of what the removal can take.
// Nothing it does changes the disk.
type Scanner struct {
	fs    platform.FS
	env   platform.Env
	guard *guard.Guard
	sys   platform.System
}

// planFor builds the plan of one rule: what its targets hold, why a path was
// left out, and what is worth saying before the plan is shown. It is the one
// place the guard is asked about a path: nothing enters the plan it has not
// been asked about.
func (s *Scanner) planFor(ctx context.Context, rule *model.Rule) (model.RulePlan, []string, error) {
	if s.sys != nil {
		if build, err := s.sys.OSBuild(); err == nil && rule.OSMinBuild > build {
			var skipped []model.Skip
			for _, target := range rule.Targets {
				skipped = append(skipped, model.Skip{Path: target.Path, Reason: model.ReasonOSTooOld,
					Detail: fmt.Sprintf("the rule names build %d, and this Windows is build %d",
						rule.OSMinBuild, build)})
			}
			return model.RulePlan{Rule: rule, Skipped: skipped}, nil, nil
		}
	}

	plan := model.RulePlan{Rule: rule}
	var warnings []string

	for _, target := range rule.Targets {
		if target.Kind == model.KindCommand {
			// A rule holds one command and its arguments; a second one in the
			// same rule goes nowhere and is said so.
			if plan.Command == nil {
				plan.Command = &model.CommandPlan{
					Command: target.Cmd,
					Args:    append([]string(nil), target.Args...),
				}
			} else {
				warnings = append(warnings, fmt.Sprintf(
					"the rule %s names a second command; the plan runs the first one", rule.ID))
			}
			continue
		}
		if err := ctx.Err(); err != nil {
			return model.RulePlan{}, warnings, err
		}
		if err := s.target(ctx, target,
			func(item model.Item) { s.takeItem(&plan, target, item) },
			func(reason model.Skip) { plan.Skipped = append(plan.Skipped, reason) },
			&warnings); err != nil {
			return model.RulePlan{Rule: rule}, warnings, err
		}
	}
	return plan, warnings, nil
}

// target does the guard's work for one target: the target path is asked about
// once, every item below it on its own.
func (s *Scanner) target(ctx context.Context, t model.Target,
	add func(model.Item), skip func(model.Skip), warnings *[]string) error {

	asked, err := s.guard.Check(t.Path, t.Mode, t.Include, t.MinAgeDays)
	switch {
	case err != nil:
		*warnings = append(*warnings,
			fmt.Sprintf("the rule cannot read the target %s: %v", t.Path, err))
		// One unreadable target does not stop the scan.
		return nil
	case !asked.Allowed:
		skip(model.Skip{Path: asked.Path, Reason: asked.Reason, Detail: asked.Message})
		return nil
	}

	if err := s.collect(ctx, t, add, skip); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		// A walk that went wrong along the way does not stop the scan: what
		// was found before is still the plan.
		*warnings = append(*warnings,
			fmt.Sprintf("the walk of the target %s stopped itself: %v", t.Path, err))
	}
	return nil
}

// takeItem adopts one item into the plan, but only after the guard has been
// asked about the item itself: a target that is allowed can still hold a path
// the guard refuses, a junction for example.
func (s *Scanner) takeItem(plan *model.RulePlan, t model.Target, item model.Item) {
	asked, err := s.guard.Check(item.Path, t.Mode, t.Include, t.MinAgeDays)
	switch {
	case err != nil:
		plan.Skipped = append(plan.Skipped, model.Skip{Path: item.Path,
			Reason: model.ReasonAccessDenied, Detail: err.Error()})
	case !asked.Allowed:
		plan.Skipped = append(plan.Skipped, model.Skip{Path: item.Path,
			Reason: model.ReasonProtected, Detail: asked.Message})
	default:
		plan.Items = append(plan.Items, item)
		plan.Bytes += item.Size
		plan.Files += item.Files
	}
}
