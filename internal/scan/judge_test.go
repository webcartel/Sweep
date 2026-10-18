package scan

import (
	"context"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// judgeEnv stands for the machine the judgement reads: the built-in style of
// variables, nothing else.
func judgeEnv() platform.Env {
	env := fake.NewEnv()
	env.Set(`USERPROFILE`, `C:\Users\probe`)
	env.Set(`SystemRoot`, `C:\Windows`)
	env.Set(`LOCALAPPDATA`, `C:\Users\probe\AppData\Local`)
	env.Set(`TEMP`, `C:\Users\probe\AppData\Local\Temp`)
	return env
}

// judgeScanner puts the guard over what the test gives, the way the CLI does.
func judgeScanner(t *testing.T, fsys platform.FS) *Scanner {
	t.Helper()
	env := judgeEnv()
	g, err := guard.New(fsys, env, `C:\Users\probe`, nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	return &Scanner{fs: fsys, env: env, guard: g}
}

// aTreeOf builds a fake with many files under one directory, spread over two
// branches. With files = 10000 the tree holds twenty thousand files: the size
// the judgement of the walk is about.
func aTreeOf(files int) *fake.FS {
	fsys := fake.NewFS()
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\big`, IsDir: true})
	for _, branch := range []string{`wide`, `deep`} {
		dir := `C:\Users\probe\AppData\Local\big\` + branch
		fsys.Add(platform.Info{Path: dir, IsDir: true})
		for i := 0; i < files/2; i++ {
			fsys.Add(platform.Info{Path: dir + `\f` + label(i), Size: 1})
			fsys.Add(platform.Info{Path: dir + `\d` + label(i), IsDir: true})
			fsys.Add(platform.Info{Path: dir + `\d` + label(i) + `\g` + label(i), Size: 1})
		}
	}
	return fsys
}

// bigRule is the one rule of the judgement: everything in the big directory,
// counted as a whole.
func bigRule() *model.Rule {
	return &model.Rule{ID: "judge.big", Targets: []model.Target{
		{Kind: model.KindPath, Path: `%LOCALAPPDATA%\big`, Mode: model.ModeSelf}}}
}

// TestMemoryDoesNotGrowWithFiles: twenty thousand files are one entry with the
// counts of the walk below it, not twenty thousand entries.
func TestMemoryDoesNotGrowWithFiles(t *testing.T) {
	fsys := aTreeOf(10000)
	s := judgeScanner(t, fsys)

	plan, warnings, err := s.planFor(context.Background(), bigRule(), nil, nil)
	if err != nil {
		t.Fatalf("planFor: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want nothing", warnings)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("Items = %d, want the one top level entry", len(plan.Items))
	}
	item := plan.Items[0]
	if item.Files != 20000 {
		t.Errorf("Files = %d, want the twenty thousand files counted once", item.Files)
	}
	if item.Size != 20000 {
		t.Errorf("Size = %d, want one file of one byte, twenty thousand times", item.Size)
	}
}

// TestScanOfABigTreeIsCancellableInASecond: the walk of a tree the size of a
// judgement case stops when the context says so, and the stream closes well
// inside the second the specification allows.
func TestScanOfABigTreeIsCancellableInASecond(t *testing.T) {
	fsys := aTreeOf(10000)
	s := judgeScanner(t, fsys)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	events := s.Scan(ctx, []*model.Rule{bigRule()}, model.ScanOptions{Workers: 1, Throttle: time.Hour})
	gone := make(chan []model.Event, 1)
	go func() { gone <- drain(events) }()

	select {
	case got := <-gone:
		if time.Since(start) > time.Second {
			t.Fatalf("the stream closed after %v, want inside a second", time.Since(start))
		}
		for _, event := range got {
			if isDone(event) {
				t.Fatalf("the events held %+v, want no ScanDone for an interrupted scan", got)
			}
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("the stream did not close inside the second")
	}
}
