package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"sync"
	"time"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/i18n"
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

// Deps collects what a scanner reaches the machine through. Every part of it
// is what a frontend builds once and hands down.
type Deps struct {
	FS     platform.FS
	Env    platform.Env
	System platform.System
	Guard  *guard.Guard
}

// New puts the parts together.
func New(d Deps) *Scanner {
	return &Scanner{fs: d.FS, env: d.Env, guard: d.Guard, sys: d.System}
}

// Scan turns the rules into one plan, telling about its work as it goes. The
// stream is buffered, closed by the scanner itself, and the plan it hands over
// in ScanDone stays the same from then on. A scan of a context that is gone
// closes the stream without a plan.
func (s *Scanner) Scan(ctx context.Context, rules []*model.Rule, opt model.ScanOptions) <-chan model.Event {
	if opt.Throttle <= 0 {
		opt.Throttle = 100 * time.Millisecond
	}
	if opt.Workers <= 0 {
		opt.Workers = runtime.NumCPU()
	}
	now := opt.Now
	if now == nil {
		now = time.Now
	}
	if workers := opt.Workers; workers > len(rules) {
		opt.Workers = len(rules)
	}

	out := make(chan model.Event, 256)
	go s.scan(ctx, rules, opt, now, out)
	return out
}

// scan runs the worker pool behind Scan and owns the stream: every send and
// the final close happen here and nowhere else.
func (s *Scanner) scan(ctx context.Context, rules []*model.Rule, opt model.ScanOptions, now func() time.Time, out chan<- model.Event) {
	defer close(out)

	var (
		wg    sync.WaitGroup
		slots = make(chan struct{}, opt.Workers)
		plans = make([]model.RulePlan, len(rules))
	)
	done := ctx.Done()

	for i, rule := range rules {
		select {
		case <-done:
			wg.Wait()
			return
		case slots <- struct{}{}:
		}
		wg.Add(1)
		go func(place int, rule *model.Rule) {
			defer wg.Done()
			defer func() { <-slots }()

			// The rule is spoken about before the disk is touched: a frontend
			// knows it is being read from the first event on.
			select {
			case out <- model.RuleStarted{RuleID: rule.ID}:
			case <-done:
				return
			}

			// Progress goes out no more often than the throttle allows, and a
			// stream nobody reads does not hold the rule back.
			last := now()
			report := func(files int, bytes int64) {
				if point := now(); point.Sub(last) < opt.Throttle {
					return
				}
				last = now()
				select {
				case out <- model.RuleProgress{RuleID: rule.ID, Files: files, Bytes: bytes}:
				case <-done:
				}
			}

			plan, warnings, err := s.planFor(ctx, rule, report, func(reason model.Skip) {
				select {
				case out <- model.ItemSkipped{RuleID: rule.ID, Skip: reason}:
				case <-done:
				}
			})
			plans[place] = plan
			if err != nil && ctx.Err() != nil {
				return
			}
			for _, message := range warnings {
				select {
				case out <- model.Warning{RuleID: rule.ID, Message: message}:
				case <-done:
					return
				}
			}
			if err != nil {
				// A glob or a walk that ended in something the path is not
				// to blame for, and that is not the context's doing.
				select {
				case out <- model.Warning{RuleID: rule.ID,
					Message: i18n.T("scan.warn.walk", err)}:
				case <-done:
				}
				return
			}
			select {
			case out <- model.RuleScanned{Plan: plan}:
			case <-done:
			}
		}(i, rule)
	}
	wg.Wait()

	if ctx.Err() != nil {
		return
	}
	plan := &model.Plan{CreatedAt: now()}
	plan.Rules = append(plan.Rules, plans...)
	select {
	case out <- model.ScanDone{Plan: plan}:
	case <-done:
	}
}

// planFor builds the plan of one rule: what its targets hold, why a path was
// left out, and what is worth saying before the plan is shown. It is the one
// place the guard is asked about a path: nothing enters the plan it has not
// been asked about.
func (s *Scanner) planFor(ctx context.Context, rule *model.Rule, report func(files int, bytes int64), onSkip func(model.Skip)) (model.RulePlan, []string, error) {
	if s.sys != nil {
		if build, err := s.sys.OSBuild(); err == nil && rule.OSMinBuild > build {
			var skipped []model.Skip
			for _, target := range rule.Targets {
				skipped = append(skipped, model.Skip{Path: target.Path, Reason: model.ReasonOSTooOld,
					Detail: i18n.T("scan.skip.build", rule.OSMinBuild, build)})
			}
			return model.RulePlan{Rule: rule, Skipped: skipped}, nil, nil
		}
	}

	plan := model.RulePlan{Rule: rule}
	// Every path the plan leaves out is said twice: once in the plan to
	// stay, once on the stream, so that a frontend presents the skip while
	// the scan is still walking.
	note := func(reason model.Skip) {
		if onSkip != nil {
			onSkip(reason)
		}
	}
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
			func(item model.Item) {
				files := plan.Files
				s.takeItem(&plan, target, item, note)
				if report != nil && plan.Files != files {
					report(plan.Files, plan.Bytes)
				}
			},
			func(reason model.Skip) {
				plan.Skipped = append(plan.Skipped, reason)
				note(reason)
			},
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

	// A target that names a variable the machine does not set is a path that
	// is not there, said before the guard is asked, which would only refuse
	// a path it cannot make sense of.
	if unset := platform.EnvNames(platform.Expand(t.Path, s.env)); len(unset) > 0 {
		skip(model.Skip{Path: t.Path, Reason: model.ReasonNotFound,
			Detail: i18n.T("scan.skip.var", unset[0])})
		return nil
	}

	asked, err := s.guard.Check(t.Path, t.Mode, t.Include, t.MinAgeDays)
	switch {
	case err != nil:
		if errors.Is(err, fs.ErrPermission) {
			// What the guard found in the way of reading is the path's own
			// business: it is said as a skip, the way a walk of it would.
			skip(model.Skip{Path: t.Path, Reason: model.ReasonAccessDenied, Detail: err.Error()})
			return nil
		}
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
func (s *Scanner) takeItem(plan *model.RulePlan, t model.Target, item model.Item,
	note func(model.Skip)) {
	tell := func(reason model.Skip) {
		plan.Skipped = append(plan.Skipped, reason)
		if note != nil {
			note(reason)
		}
	}
	asked, err := s.guard.Check(item.Path, t.Mode, t.Include, t.MinAgeDays)
	switch {
	case err != nil:
		tell(model.Skip{Path: item.Path,
			Reason: model.ReasonAccessDenied, Detail: err.Error()})
	case !asked.Allowed:
		tell(model.Skip{Path: item.Path,
			Reason: model.ReasonProtected, Detail: asked.Message})
	default:
		item.Mode = t.Mode
		item.Include = t.Include
		item.MinAgeDays = t.MinAgeDays
		plan.Items = append(plan.Items, item)
		plan.Bytes += item.Size
		plan.Files += item.Files
	}
}
