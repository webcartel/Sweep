package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// errorProcs is a process list that cannot be read: the machine did not
// give its answer.
type errorProcs struct{ err error }

func (e errorProcs) Running([]string) ([]string, error) { return nil, e.err }

// osErrPermission names the refusal Windows books under access denied.
func osErrPermission() error { return os.ErrPermission }

// drainTake reads the stream till it closes, with a timeout that says when
// the executor forgot to close it.
func drainTake(t *testing.T, stream <-chan model.Event) []model.Event {
	t.Helper()
	var seen []model.Event
	gone := time.After(2 * time.Second)
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				return seen
			}
			seen = append(seen, event)
		case <-gone:
			t.Fatalf("the stream never closed after %d events", len(seen))
		}
	}
}

// execOver matches cleanExec and gives an executor over a heavier tree.
func execOver(t *testing.T, build func(fsys *fake.FS)) (*Executor, *fake.FS) {
	t.Helper()
	x, fsys := cleanExec(t)
	build(fsys)
	return x, fsys
}

func rule(id string) *model.Rule {
	return &model.Rule{ID: id, Risk: model.RiskSafe}
}

func TestExecuteRunsTheRulesAndTellsTheTally(t *testing.T) {
	x, _ := execOver(t, func(fsys *fake.FS) {
		addFile(t, fsys, `C:\Users\probe\AppData\Local\a.cas`, 100)
		addFile(t, fsys, `C:\Users\probe\AppData\Local\b.cas`, 250)
	})
	first := rule("one")
	second := rule("two")
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: first, Items: []model.Item{
			{Path: `C:\Users\probe\AppData\Local\a.cas`, Size: 100, Mode: model.ModeContents},
		}},
		{Rule: second, Items: []model.Item{
			{Path: `C:\Users\probe\AppData\Local\b.cas`, Size: 250, Mode: model.ModeContents},
		}},
	}}
	now := func() time.Time { return time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC) }

	stream := x.Execute(t.Context(), plan, model.Selection{},
		model.ExecOptions{Workers: 2, Now: now})
	seen := drainTake(t, stream)

	var (
		removed []model.ItemRemoved
		done    model.ExecDone
	)
	for _, event := range seen {
		switch one := event.(type) {
		case model.ItemRemoved:
			removed = append(removed, one)
		case model.ExecDone:
			done = one
		}
	}
	if len(removed) != 2 {
		t.Fatalf("removed %d events, want two", len(removed))
	}
	if done.Result == nil {
		t.Fatal("no ExecDone in the stream")
	}
	if done.Result.StartedAt.IsZero() || done.Result.FinishedAt.IsZero() {
		t.Error("the result does not name its hours")
	}
	if done.Result.FreedBytes != 350 || done.Result.Removed != 2 {
		t.Errorf("result = %+v, want 350 freed over two", done.Result)
	}
	if done.Result.Skipped != 0 || done.Result.Failed != 0 {
		t.Errorf("result = %+v, want no skips nor failures", done.Result)
	}
	if len(done.Result.PerRule) != 2 ||
		done.Result.PerRule[0].Status != model.StatusDone ||
		done.Result.PerRule[0].Rule != first {
		t.Errorf("PerRule = %+v, want one per rule in the order of the plan", done.Result.PerRule)
	}
	if got := done.Result.PerRule[1].Bytes; got != 250 {
		t.Errorf("Bytes of the second = %d, want 250", got)
	}
}

func TestExecuteLeavesAnExcludedRuleAndAnExcludedPathAlone(t *testing.T) {
	x, fsys := execOver(t, func(fsys *fake.FS) {
		addFile(t, fsys, `C:\Users\probe\AppData\Local\a.cas`, 100)
		addFile(t, fsys, `C:\Users\probe\AppData\Local\b.cas`, 250)
	})
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: rule("one"), Items: []model.Item{
			{Path: `C:\Users\probe\AppData\Local\a.cas`, Size: 100, Mode: model.ModeContents},
			{Path: `C:\Users\probe\AppData\Local\b.cas`, Size: 250, Mode: model.ModeContents},
		}},
	}}
	excluded := model.Item{Path: `C:\Users\probe\AppData\Local\b.cas`}

	stream := x.Execute(t.Context(), plan, model.Selection{
		Rules: map[string]bool{"one": true},
		Exclude: map[string]bool{
			`C:\Users\probe\AppData\Local\b.cas`: true,
		},
	}, model.ExecOptions{Workers: 1})
	seen := drainTake(t, stream)

	if len(fsys.Removed()) != 1 {
		t.Fatalf("Removed() = %v, want only the kept path", fsys.Removed())
	}
	for _, event := range seen {
		switch one := event.(type) {
		case model.ItemRemoved:
			if one.Path == excluded.Path {
				t.Errorf("the deselected path was removed and told: %+v", one)
			}
		case model.ExecDone:
			if one.Result.Removed != 1 || one.Result.Skipped != 0 {
				t.Errorf("result = %+v, want one removed and nothing said", one.Result)
			}
		}
	}
}

func TestExecuteLeavesAnUnselectedRuleOutOfEverything(t *testing.T) {
	x, _ := execOver(t, func(fsys *fake.FS) {
		addFile(t, fsys, `C:\Users\probe\AppData\Local\a.cas`, 100)
	})
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: rule("one"), Items: []model.Item{
			{Path: `C:\Users\probe\AppData\Local\a.cas`, Size: 100, Mode: model.ModeContents},
		}},
		{Rule: rule("two"), Items: []model.Item{
			{Path: `C:\Users\probe\AppData\Local\a.cas`, Size: 100, Mode: model.ModeContents},
		}},
	}}

	stream := x.Execute(t.Context(), plan, model.Selection{
		Rules: map[string]bool{"one": true, "two": false},
	}, model.ExecOptions{Workers: 2})
	seen := drainTake(t, stream)

	done := seen[len(seen)-1].(model.ExecDone)
	if len(done.Result.PerRule) != 1 || done.Result.PerRule[0].Rule.ID != "one" {
		t.Errorf("PerRule = %+v, want only the rule that stayed", done.Result.PerRule)
	}
	if done.Result.Removed != 1 {
		t.Errorf("Removed = %d, want one", done.Result.Removed)
	}
}

func TestExecuteSkipsARuleWhoseProgramsCannotBeAsked(t *testing.T) {
	x, fsys := execOver(t, func(fsys *fake.FS) {
		addFile(t, fsys, `C:\Users\probe\AppData\Local\a.cas`, 100)
	})
	held := errorProcs{err: errors.New("the snapshot did not come through")}
	x.procs = held
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: &model.Rule{ID: "one", Risk: model.RiskSafe, RequiresClosed: []string{"studio"}},
			Items: []model.Item{
				{Path: `C:\Users\probe\AppData\Local\a.cas`, Size: 100, Mode: model.ModeContents},
			}},
	}}

	stream := x.Execute(t.Context(), plan, model.Selection{}, model.ExecOptions{Workers: 1})
	seen := drainTake(t, stream)

	if len(fsys.Removed()) != 0 {
		t.Errorf("Removed() = %v, want nothing", fsys.Removed())
	}
	var warned model.Event
	done := seen[len(seen)-1].(model.ExecDone)
	for _, event := range seen {
		if _, ok := event.(model.Warning); ok {
			warned = event
		}
	}
	if warned == nil {
		t.Error("the unreadable snapshot said nothing")
	}
	if got := done.Result.PerRule[0].Skipped[0].Reason; got != model.ReasonSystemUnknown {
		t.Errorf("reason = %s, want system_unknown", got)
	}
	if status := done.Result.PerRule[0].Status; status != model.StatusSkipped {
		t.Errorf("status = %s, want skipped", status)
	}
}

func TestExecuteLeavesACommandToItsSkips(t *testing.T) {
	x, _ := cleanExec(t)
	procs := fake.NewProcs()
	procs.Set("studio")
	x.procs = procs
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: &model.Rule{ID: "one", Risk: model.RiskSafe, RequiresClosed: []string{"studio"}},
			Command: &model.CommandPlan{Command: "cmd", Args: []string{"/c", "exit", "0"}}},
	}}

	stream := x.Execute(t.Context(), plan, model.Selection{}, model.ExecOptions{Workers: 1})
	seen := drainTake(t, stream)

	for _, event := range seen {
		switch one := event.(type) {
		case model.CommandRan:
			t.Errorf("the command ran while its program was up: %+v", one)
		case model.ExecDone:
			if status := one.Result.PerRule[0].Status; status != model.StatusSkipped {
				t.Errorf("status = %s, want skipped", status)
			}
		}
	}
}

func TestExecuteTellsHowEachRuleFaredWhileTheOthersRunOn(t *testing.T) {
	x, _ := execOver(t, func(fsys *fake.FS) {
		addFile(t, fsys, `C:\Users\probe\AppData\Local\a.cas`, 100)
		addFile(t, fsys, `C:\Users\probe\AppData\Local\b.cas`, 250)
	})
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: rule("one"), Items: []model.Item{
			{Path: `C:\Users\probe\AppData\Local\a.cas`, Size: 100, Mode: model.ModeContents},
		}},
		{Rule: rule("two"), Items: []model.Item{
			{Path: `C:\Users\probe\AppData\Local\b.cas`, Size: 250, Mode: model.ModeContents},
		}},
	}}

	stream := x.Execute(t.Context(), plan, model.Selection{}, model.ExecOptions{Workers: 2})
	seen := drainTake(t, stream)

	finished := 0
	for _, event := range seen {
		if per, ok := event.(model.RuleFinished); ok {
			finished++
			if per.Result.Rule == nil {
				t.Error("a RuleFinished with no rule in it")
			}
		}
	}
	if finished != 2 {
		t.Errorf("RuleFinished = %d events, want one per rule", finished)
	}
}

func TestExecuteSkipsAWholeRuleWhileItsAppIsRunning(t *testing.T) {
	x, fsys := execOver(t, func(fsys *fake.FS) {
		addFile(t, fsys, `C:\Users\probe\AppData\Local\a.cas`, 100)
	})
	procs := fake.NewProcs()
	procs.Set("studio")
	x.procs = procs
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: &model.Rule{ID: "one", RequiresClosed: []string{"studio"}},
			Items: []model.Item{
				{Path: `C:\Users\probe\AppData\Local\a.cas`, Size: 100, Mode: model.ModeContents},
			}},
	}}

	stream := x.Execute(t.Context(), plan, model.Selection{}, model.ExecOptions{Workers: 1})
	seen := drainTake(t, stream)

	if len(fsys.Removed()) != 0 {
		t.Errorf("Removed() = %v, want nothing", fsys.Removed())
	}
	done := seen[len(seen)-1].(model.ExecDone)
	if done.Result.PerRule[0].Status != model.StatusSkipped {
		t.Errorf("status = %s, want skipped", done.Result.PerRule[0].Status)
	}
	if got := done.Result.PerRule[0].Skipped[0].Reason; got != model.ReasonAppRunning {
		t.Errorf("reason = %s, want app_running", got)
	}
}

func TestExecuteSkipsAWholeRuleThatNeedsAnAdmin(t *testing.T) {
	x, fsys := execOver(t, func(fsys *fake.FS) {
		addFile(t, fsys, `C:\Users\probe\AppData\Local\a.cas`, 100)
	})
	sys := fake.NewSystem()
	sys.SetElevated(false)
	x.system = sys
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: &model.Rule{ID: "one", RequiresAdmin: true}, Items: []model.Item{
			{Path: `C:\Users\probe\AppData\Local\a.cas`, Size: 100, Mode: model.ModeContents},
		}},
	}}

	stream := x.Execute(t.Context(), plan, model.Selection{}, model.ExecOptions{Workers: 1})
	seen := drainTake(t, stream)

	if len(fsys.Removed()) != 0 {
		t.Errorf("Removed() = %v, want nothing", fsys.Removed())
	}
	done := seen[len(seen)-1].(model.ExecDone)
	if got := done.Result.PerRule[0].Skipped[0].Reason; got != model.ReasonNeedsAdmin {
		t.Errorf("reason = %s, want needs_admin", got)
	}
	if status := done.Result.PerRule[0].Status; status != model.StatusSkipped {
		t.Errorf("status = %s, want skipped", status)
	}
}

func TestExecuteRunsACommandRuleAndTellsItFailedWhenItDid(t *testing.T) {
	x, _ := cleanExec(t)
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: rule("one"), Command: &model.CommandPlan{
			Command: "cmd", Args: []string{"/c", "exit", "10"},
		}},
		{Rule: rule("two"), Command: &model.CommandPlan{
			Command: "cmd", Args: []string{"/c", "exit", "0"},
		}},
	}}

	stream := x.Execute(t.Context(), plan, model.Selection{}, model.ExecOptions{Workers: 2})
	seen := drainTake(t, stream)

	var failed int
	var done model.ExecDone
	for _, event := range seen {
		switch one := event.(type) {
		case model.ItemFailed:
			failed++
		case model.ExecDone:
			done = one
		}
	}
	if failed != 1 {
		t.Errorf("failed = %d events, want one", failed)
	}
	doneResult := done.Result
	if doneResult == nil {
		t.Fatal("no ExecDone in the stream")
	}
	if got := doneResult.PerRule[0].Status; got != model.StatusFailed {
		t.Errorf("status of the first = %s, want failed", got)
	}
	if got := doneResult.PerRule[1].Status; got != model.StatusDone {
		t.Errorf("status of the second = %s, want done", got)
	}
	if doneResult.Failed != 1 {
		t.Errorf("Failed = %d, want one", doneResult.Failed)
	}
}

func TestExecuteDoesNotRunTheCommandOfADryRun(t *testing.T) {
	x, _ := cleanExec(t)
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: rule("one"), Command: &model.CommandPlan{
			Command: "cmd", Args: []string{"/c", "echo", "cleaned"},
		}},
	}}

	stream := x.Execute(t.Context(), plan, model.Selection{},
		model.ExecOptions{Workers: 1, DryRun: true, Now: time.Now})
	seen := drainTake(t, stream)

	for _, event := range seen {
		switch one := event.(type) {
		case model.ItemFailed:
			t.Errorf("the command ran anyway: %+v", one)
		case model.ExecDone:
			if status := one.Result.PerRule[0].Status; status != model.StatusDryRun {
				t.Errorf("status = %s, want dry_run", status)
			}
		}
	}
}

func TestExecuteKeepsTheDiskButTalliesInADryRun(t *testing.T) {
	x, fsys := execOver(t, func(fsys *fake.FS) {
		addFile(t, fsys, `C:\Users\probe\AppData\Local\a.cas`, 100)
		addFile(t, fsys, `C:\Users\probe\AppData\Local\b.cas`, 250)
	})
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: rule("one"), Items: []model.Item{
			{Path: `C:\Users\probe\AppData\Local\a.cas`, Size: 100, Mode: model.ModeContents},
			{Path: `C:\Users\probe\AppData\Local\b.cas`, Size: 250, Mode: model.ModeContents},
		}},
	}}

	stream := x.Execute(t.Context(), plan, model.Selection{},
		model.ExecOptions{Workers: 1, DryRun: true, Now: time.Now})
	seen := drainTake(t, stream)

	if len(fsys.Removed())+len(fsys.Trashed()) != 0 {
		t.Errorf("the disk was touched: removed %v trashed %v", fsys.Removed(), fsys.Trashed())
	}
	var removed int
	var done model.ExecDone
	for _, event := range seen {
		switch one := event.(type) {
		case model.ItemRemoved:
			removed++
		case model.ExecDone:
			done = one
		}
	}
	if removed != 2 || done.Result.Removed != 2 || done.Result.FreedBytes != 350 {
		t.Errorf("%d events, result = %+v, want the tally of two at 350", removed, done.Result)
	}
	if status := done.Result.PerRule[0].Status; status != model.StatusDryRun {
		t.Errorf("status = %s, want dry_run", status)
	}
}

func TestExecuteSaysThePathsPutOffTillTheReboot(t *testing.T) {
	x, fsys := execOver(t, func(fsys *fake.FS) {
		addFile(t, fsys, `C:\Users\probe\AppData\Local\busy.log`, 700)
	})
	fsys.DenyDel(`C:\Users\probe\AppData\Local\busy.log`, osErrPermission())
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: &model.Rule{ID: "one", Risk: model.RiskModerate}, Items: []model.Item{
			{Path: `C:\Users\probe\AppData\Local\busy.log`, Size: 700, Mode: model.ModeContents},
		}},
	}}

	stream := x.Execute(t.Context(), plan, model.Selection{},
		model.ExecOptions{Workers: 1, OnReboot: true, Now: time.Now})
	seen := drainTake(t, stream)

	if pend := fsys.Pending(); len(pend) != 1 {
		t.Errorf("Pending() = %v, want the busy path", pend)
	}
	done := seen[len(seen)-1].(model.ExecDone)
	if got := done.Result.PendingReboot; len(got) != 1 {
		t.Errorf("PendingReboot = %v, want the busy path", got)
	}
	if done.Result.Removed != 0 || done.Result.Skipped != 0 {
		t.Errorf("result = %+v, want the path neither removed nor skipped", done.Result)
	}
}

func TestExecuteClosesTheStreamOfACanceledRunSoon(t *testing.T) {
	x, _ := execOver(t, func(fsys *fake.FS) {
		for i := 1; i <= 800; i++ {
			addFile(t, fsys, `C:\Users\probe\AppData\Local\f`+fmt.Sprint(i)+`.tmp`, 1)
		}
	})
	items := make([]model.Item, 0, 800)
	for i := 1; i <= 800; i++ {
		items = append(items, model.Item{
			Path: `C:\Users\probe\AppData\Local\f` + fmt.Sprint(i) + `.tmp`,
			Size: 1, Mode: model.ModeContents,
		})
	}
	plan := &model.Plan{Rules: []model.RulePlan{
		{Rule: rule("one"), Items: items},
	}}
	ctx, stop := context.WithCancel(context.Background())
	stream := x.Execute(ctx, plan, model.Selection{}, model.ExecOptions{Workers: 1})

	// The cancellation lands in the middle of the work and with no one
	// reading: the blocked sends give up and the stream closes without
	// a result.
	before := time.Now()
	stop()
	saw := false
	for event := range stream {
		if _, ok := event.(model.ExecDone); ok {
			saw = true
		}
	}
	if since := time.Since(before); since > time.Second {
		t.Errorf("the stream closed after %s", since)
	}
	if saw {
		t.Error("a canceled run sent ExecDone")
	}
}
