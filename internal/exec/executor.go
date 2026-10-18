package exec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
)

// NullProcs stands for a process list that always answers nothing, for the
// builders that have nothing to give.
type NullProcs struct{}

func (NullProcs) Running([]string) ([]string, error) { return nil, nil }

// commandTimeout is how long a rule's command may run. A cleanup script
// that hangs must not hold the whole run hostage.
const commandTimeout = 10 * time.Minute

// Executor takes the plan of a scan, asks one last time for every path, and
// puts each of them away the way the rule and the options say.
type Executor struct {
	fs     platform.FS
	guard  *guard.Guard
	procs  platform.Procs
	system platform.System
}

// Deps collects what an executor reaches the machine through.
type Deps struct {
	FS     platform.FS
	Guard  *guard.Guard
	Procs  platform.Procs
	System platform.System
}

// New puts the parts together. A builder that has nothing to give for the
// process list stands for one that always answers nothing; a system of nil
// stands for a machine the process does not run elevated on.
func New(d Deps) *Executor {
	if d.Procs == nil {
		d.Procs = NullProcs{}
	}
	return &Executor{fs: d.FS, guard: d.Guard, procs: d.Procs, system: d.System}
}

// Execute turns the plan into work and tells about every path as it goes.
// The stream is buffered and closed by the executor itself; a run of a
// context that is gone closes the stream without a result.
func (x *Executor) Execute(ctx context.Context, plan *model.Plan, sel model.Selection, opt model.ExecOptions) <-chan model.Event {
	if opt.Workers <= 0 {
		opt.Workers = runtime.NumCPU()
	}
	if opt.Workers > len(plan.Rules) {
		opt.Workers = len(plan.Rules)
	}
	out := make(chan model.Event, 256)
	go x.execute(ctx, plan, sel, opt, out)
	return out
}

// execute runs the worker pool behind Execute and owns the stream: every
// send and the final close happen here and nowhere else.
func (x *Executor) execute(ctx context.Context, plan *model.Plan, sel model.Selection, opt model.ExecOptions, out chan<- model.Event) {
	defer close(out)

	now := opt.Now
	if now == nil {
		now = time.Now
	}
	result := &model.Result{StartedAt: now()}
	var (
		wg      sync.WaitGroup
		slots   = make(chan struct{}, opt.Workers)
		results = make([]ruleOutcome, len(plan.Rules))
	)
	done := ctx.Done()

	for i := range plan.Rules {
		rulePlan := &plan.Rules[i]
		results[i].result.Rule = rulePlan.Rule
		if sel.Rules != nil && !sel.Rules[ruleIDOf(rulePlan.Rule)] {
			// A rule the user took away is not spoken about: it was
			// deselected on purpose, and the report stays silent about it.
			continue
		}
		select {
		case <-done:
			wg.Wait()
			return
		case slots <- struct{}{}:
		}
		wg.Add(1)
		go func(place int, rp *model.RulePlan) {
			defer wg.Done()
			defer func() { <-slots }()
			kept := x.runRule(ctx, rp, sel, opt, out, done)
			results[place] = kept
			tell(done, out, model.RuleFinished{Result: kept.result})
		}(i, rulePlan)
	}
	wg.Wait()

	if ctx.Err() != nil {
		return
	}
	for _, one := range results {
		if one.result.Rule == nil || (sel.Rules != nil && !sel.Rules[ruleIDOf(one.result.Rule)]) {
			continue
		}
		result.PerRule = append(result.PerRule, one.result)
		result.FreedBytes += one.result.Bytes
		result.TrashBytes += one.trash
		result.PendingReboot = append(result.PendingReboot, one.pending...)
		result.Removed += one.removed
		result.Skipped += len(one.result.Skipped)
		result.Failed += one.failed
	}
	result.FinishedAt = now()

	select {
	case out <- model.ExecDone{Result: result}:
	case <-done:
	}
}

// ruleOutcome is what one rule came to, with the counters the run totals
// cannot compute from the result on their own.
type ruleOutcome struct {
	result  model.RuleResult
	removed int
	failed  int
	trash   int64
	pending []string
}

// ruleIDOf names the rule; a rule the loader made with no id at all stands
// under an empty name.
func ruleIDOf(rule *model.Rule) string {
	if rule == nil {
		return ""
	}
	return rule.ID
}

// tell puts one event on the stream unless the context of the run is gone;
// the many goroutines of a run reach no further than this one door.
func tell(done <-chan struct{}, out chan<- model.Event, event model.Event) bool {
	select {
	case out <- event:
		return true
	case <-done:
		return false
	}
}

// runRule takes one rule to the end of its work, telling the stream as it
// goes.
func (x *Executor) runRule(ctx context.Context, rp *model.RulePlan, sel model.Selection, opt model.ExecOptions, out chan<- model.Event, done <-chan struct{}) ruleOutcome {
	id := ruleIDOf(rp.Rule)
	kept := ruleOutcome{result: model.RuleResult{Rule: rp.Rule, Status: model.StatusDone}}
	if opt.DryRun {
		kept.result.Status = model.StatusDryRun
	}
	put := func(event model.Event) { tell(done, out, event) }
	skipAll := func(reason model.SkipReason, detail string) {
		for _, item := range rp.Items {
			skip := model.Skip{Path: item.Path, Reason: reason, Detail: detail}
			kept.result.Skipped = append(kept.result.Skipped, skip)
			put(model.ItemSkipped{RuleID: id, Skip: skip})
		}
		if rp.Command != nil {
			skip := model.Skip{Path: rp.Command.Command, Reason: reason, Detail: detail}
			kept.result.Skipped = append(kept.result.Skipped, skip)
			put(model.ItemSkipped{RuleID: id, Skip: skip})
		}
		kept.result.Status = model.StatusSkipped
	}

	running, asked := x.busyWith(rp.Rule)
	if asked != nil {
		// The snapshot of the machine did not come through, and the safe
		// side of an unreadable machine is not to run: a short list would
		// have read as "nothing is running".
		put(model.Warning{RuleID: id,
			Message: fmt.Sprintf("the running programs could not be asked: %v", asked)})
		skipAll(model.ReasonSystemUnknown,
			fmt.Sprintf("the running programs could not be asked: %v", asked))
		return kept
	}
	if len(running) > 0 {
		skipAll(model.ReasonAppRunning,
			fmt.Sprintf("%s is running", strings.Join(running, ", ")))
		return kept
	}
	if rp.Rule.RequiresAdmin && !x.elevated() {
		skipAll(model.ReasonNeedsAdmin, "the rule asks for an administrator")
		return kept
	}

	if rp.Command != nil {
		if !opt.DryRun {
			if err := x.runCommand(id, ctx, rp, put); err != nil {
				kept.result.Status = model.StatusFailed
				kept.failed++
			}
		}
		return kept
	}

	for i := range rp.Items {
		item := &rp.Items[i]
		if sel.Exclude != nil && sel.Exclude[item.Path] {
			// A path the user deselected is not taken away and not
			// counted anywhere: the plan stays what it was.
			continue
		}
		got := x.runItem(rp.Rule, *item, opt, put)
		if got.skip != nil {
			kept.result.Skipped = append(kept.result.Skipped, *got.skip)
			continue
		}
		if got.deadly != nil {
			kept.result.Status = model.StatusFailed
			kept.failed++
			continue
		}
		if got.pending {
			kept.pending = append(kept.pending, item.Path)
			continue
		}
		kept.result.Bytes += got.freed
		kept.result.Files++
		kept.removed++
		if got.trashed {
			kept.trash += got.freed
		}
	}
	return kept
}

// busyWith names the programs of the rule that are running now, and says how
// the asking went: a list that could not be read is not an empty one.
func (x *Executor) busyWith(rule *model.Rule) ([]string, error) {
	if rule == nil || len(rule.RequiresClosed) == 0 {
		return nil, nil
	}
	return x.procs.Running(rule.RequiresClosed)
}

// elevated says whether the process runs with administrator rights.
func (x *Executor) elevated() bool {
	if x.system == nil {
		return false
	}
	elevated, err := x.system.Elevated()
	return err == nil && elevated
}

// runCommand runs the command of one rule: no shell in between, no more
// than ten minutes, and what it says ends up in the telling — its output
// under each word of a failure, and the run itself under its success.
func (x *Executor) runCommand(id string, ctx context.Context, rp *model.RulePlan, put func(model.Event)) error {
	run, stop := context.WithTimeout(ctx, commandTimeout)
	defer stop()
	command := exec.CommandContext(run, rp.Command.Command, rp.Command.Args...)
	var said bytes.Buffer
	command.Stdout = &said
	command.Stderr = &said
	if err := command.Run(); err != nil {
		message := said.String()
		if message != "" {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(message))
		}
		put(model.ItemFailed{RuleID: id, Path: rp.Command.Command, Err: err})
		return err
	}
	put(model.CommandRan{RuleID: id, Command: rp.Command.Command})
	return nil
}
