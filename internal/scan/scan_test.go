package scan

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// labFS counts how many calls to Lstat stood at the same time: it tells how
// far the scan really ran side by side.
type labFS struct {
	platform.FS
	standing atomic.Int64
	peak     atomic.Int64
}

func (f *labFS) Lstat(path string) (platform.Info, error) {
	live := f.standing.Add(1)
	for {
		highest := f.peak.Load()
		if live <= highest || f.peak.CompareAndSwap(highest, live) {
			break
		}
	}
	time.Sleep(2 * time.Millisecond)
	info, err := f.FS.Lstat(path)
	f.standing.Add(-1)
	return info, err
}

// buildGuard builds a guard over what the test gives, as the CLI does.
func buildGuard(t *testing.T, env platform.Env, fsys platform.FS) *guard.Guard {
	t.Helper()
	g, err := guard.New(fsys, env, `C:\Users\probe`, nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	return g
}

// theEnv stands for the environment of a machine: the guard refuses the whole
// of C:\Users unless the profile is named, and the rules of the tests name
// their targets through LOCALAPPDATA.
func theEnv() *fake.Env {
	env := fake.NewEnv()
	env.Set(`USERPROFILE`, `C:\Users\probe`)
	env.Set(`SystemRoot`, `C:\Windows`)
	env.Set(`ProgramData`, `C:\ProgramData`)
	env.Set(`ProgramFiles`, `C:\Program Files`)
	env.Set(`ProgramFiles(x86)`, `C:\Program Files (x86)`)
	env.Set(`APPDATA`, `C:\Users\probe\AppData\Roaming`)
	env.Set(`LOCALAPPDATA`, `C:\Users\probe\AppData\Local`)
	env.Set(`TEMP`, `C:\Temp`)
	return env
}

// bigTree builds the fake the runtime tests read: one directory the rules aim
// at, with several files under it.
func bigTree(files int) *fake.FS {
	fsys := fake.NewFS()
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\big`, IsDir: true})
	for i := 0; i < files; i++ {
		fsys.Add(platform.Info{Path: naming(i), Size: 1})
	}
	return fsys
}

func naming(i int) string { return `C:\Users\probe\AppData\Local\big\f` + label(i) }

// label gives the number its place in the name.
func label(i int) string {
	var digits []byte
	if i == 0 {
		return "0"
	}
	for ; i > 0; i /= 10 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
	}
	return string(digits)
}

func TestScanEventOrder(t *testing.T) {
	fsys := bigTree(3)
	env := theEnv()
	s := New(Deps{FS: fsys, Env: env, System: fake.NewSystem(), Guard: buildGuard(t, env, fsys)})

	rules := []*model.Rule{
		{ID: "one.rule", Targets: []model.Target{{Kind: model.KindPath, Path: `%LOCALAPPDATA%\big`, Mode: model.ModeContents}}},
		{ID: "two.rule", Targets: []model.Target{{Kind: model.KindPath, Path: `%LOCALAPPDATA%\big`, Mode: model.ModeContents}}},
	}
	events := drain(s.Scan(context.Background(), rules, model.ScanOptions{Workers: 1, Throttle: time.Hour}))

	if len(events) == 0 || !isDone(events[len(events)-1]) {
		t.Fatalf("the list of events ended with %+v, want ScanDone", events[len(events)-1])
	}

	// Every rule is scanned after it started, at most once, and the plan keeps
	// the rules in the order they came in.
	waiting := map[string]bool{}
	for _, event := range events[:len(events)-1] {
		switch e := event.(type) {
		case model.RuleStarted:
			if waiting[e.RuleID] {
				t.Errorf("rule %s started twice", e.RuleID)
			}
			waiting[e.RuleID] = true
		case model.RuleScanned:
			if !waiting[e.Plan.Rule.ID] {
				t.Errorf("rule %s scanned before it started", e.Plan.Rule.ID)
			}
			waiting[e.Plan.Rule.ID] = false
		}
	}
	for _, rule := range rules {
		if waiting[rule.ID] {
			t.Errorf("rule %s never scanned", rule.ID)
		}
	}
	done := events[len(events)-1].(model.ScanDone)
	var ids []string
	for _, plan := range done.Plan.Rules {
		ids = append(ids, plan.Rule.ID)
	}
	if !reflect.DeepEqual(ids, []string{"one.rule", "two.rule"}) {
		t.Errorf("plan rules = %q, want the order the rules came in", ids)
	}
	if plan := done.Plan.Rules[0]; plan.Bytes != 3 || plan.Files != 3 || len(plan.Items) != 3 {
		t.Errorf("RulePlan = %+v, want each file of the target", plan)
	}
}

// drain reads every event the scan gives, bounded in time so that a scanner
// that never closes cannot take the test run down with it.
func drain(events <-chan model.Event) []model.Event {
	var got []model.Event
	timeout := time.After(30 * time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return got
			}
			got = append(got, event)
		case <-timeout:
			return got
		}
	}
}

func isDone(event model.Event) bool {
	_, ok := event.(model.ScanDone)
	return ok
}

func TestScanParallelism(t *testing.T) {
	fsys := bigTree(6)
	env := theEnv()

	lab := &labFS{FS: fsys}
	s := New(Deps{FS: lab, Env: env, System: fake.NewSystem(), Guard: buildGuard(t, env, lab)})

	rules := rulesOf(6, model.ModeSelf)
	drain(s.Scan(context.Background(), rules, model.ScanOptions{Workers: 2, Throttle: time.Hour}))

	if got := lab.peak.Load(); got < 2 {
		t.Errorf("at most %d calls to the disk stood together, want the two workers", got)
	}
}

// rulesOf gives the big directory to n rules with distinct ids.
func rulesOf(n int, mode model.Mode) []*model.Rule {
	var rules []*model.Rule
	for i := 0; i < n; i++ {
		rules = append(rules, &model.Rule{ID: "r." + label(i), Targets: []model.Target{
			{Kind: model.KindPath, Path: `%LOCALAPPDATA%\big`, Mode: mode}}})
	}
	return rules
}

func TestScanThrottle(t *testing.T) {
	fsys := bigTree(20)
	env := theEnv()

	s := New(Deps{FS: fsys, Env: env, System: fake.NewSystem(), Guard: buildGuard(t, env, fsys)})

	// A clock that stands still: with the throttle at 100 ms and time that
	// never moves, one rule may send at most one RuleProgress.
	still := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

	var progresses []model.RuleProgress
	for event := range s.Scan(context.Background(), rulesOf(1, model.ModeSelf),
		model.ScanOptions{Workers: 1, Throttle: 100 * time.Millisecond, Now: still}) {
		if progress, ok := event.(model.RuleProgress); ok {
			progresses = append(progresses, progress)
		}
	}
	if len(progresses) > 1 {
		t.Errorf("progress = %d events, want at most one while the clock did not move", len(progresses))
	}
	if len(progresses) == 1 && (progresses[0].Files != 20 || progresses[0].Bytes != 20) {
		t.Errorf("progress = %+v, want the counts of the plan", progresses[0])
	}
}

func TestScanCancel(t *testing.T) {
	fsys := bigTree(50)
	env := theEnv()

	s := New(Deps{FS: fsys, Env: env, System: fake.NewSystem(), Guard: buildGuard(t, env, fsys)})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	events := drain(s.Scan(ctx, rulesOf(8, model.ModeSelf),
		model.ScanOptions{Workers: 4, Throttle: time.Hour}))
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Fatalf("the scan closed after %v, want nothing longer than a second", elapsed)
	}
	for _, event := range events {
		if isDone(event) {
			t.Fatalf("events held %+v, want no ScanDone for an interrupted scan", events)
		}
	}
}

func TestScanClosesWithoutReader(t *testing.T) {
	fsys := bigTree(4)
	env := theEnv()
	s := New(Deps{FS: fsys, Env: env, System: fake.NewSystem(), Guard: buildGuard(t, env, fsys)})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// The scanner must not hang on a stream nobody reads and must close
	// within the second, without a plan.
	events := s.Scan(ctx, rulesOf(200, model.ModeContents),
		model.ScanOptions{Workers: 4, Throttle: time.Hour})

	// A reader that comes late: the buffer had filled, the sends stood their
	// ground and did not hold the scan back.
	came := make(chan []model.Event, 1)
	go func() {
		select {
		case came <- drain(events):
		case <-time.After(2 * time.Second):
			close(came)
		}
	}()
	select {
	case got := <-came:
		for _, event := range got {
			if isDone(event) {
				t.Fatalf("events held %+v, want no ScanDone for an interrupted scan", got)
			}
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("the scan did not close within the second")
	}
}

func TestScanEmptySetOfRules(t *testing.T) {
	fsys := fake.NewFS()
	env := theEnv()
	s := New(Deps{FS: fsys, Env: env, System: fake.NewSystem(), Guard: buildGuard(t, env, fsys)})

	events := drain(s.Scan(context.Background(), nil, model.ScanOptions{}))
	if len(events) != 1 || !isDone(events[0]) {
		t.Fatalf("events = %+v, want exactly one ScanDone", events)
	}
	plan := events[0].(model.ScanDone).Plan
	if plan.CreatedAt.IsZero() {
		t.Error("CreatedAt = zero, want the moment the scan made the plan")
	}
	if len(plan.Rules) != 0 {
		t.Errorf("Rules = %+v, want none", plan.Rules)
	}
}
