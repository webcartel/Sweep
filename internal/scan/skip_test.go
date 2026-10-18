package scan

import (
	"context"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// probeScanner builds a scanner with the guard over the same fake the tests
// fill with entries, an environment that stands for a real machine and a
// system that reports the given build.
func probeScanner(t *testing.T, sys platform.System) (*Scanner, *fake.FS) {
	t.Helper()
	fsys := fake.NewFS()
	for _, path := range []struct {
		path string
		is   bool
	}{
		{`C:\Windows`, true}, {`C:\Windows\System32`, true}, {`C:\Users`, true},
		{`C:\Users\probe`, true}, {`C:\Users\probe\AppData`, true},
		{`C:\Users\probe\AppData\Local`, true}, {`C:\Users\probe\AppData\Roaming`, true},
		{`C:\Program Files`, true}, {`C:\ProgramData`, true},
		{`C:\Temp`, true},
		{`C:\Users\probe\AppData\Local\a`, true}, {`C:\Users\probe\AppData\Local\b`, true},
	} {
		fsys.Add(platform.Info{Path: path.path, IsDir: path.is})
	}
	probe := fake.NewEnv()
	probe.Set(`USERPROFILE`, `C:\Users\probe`)
	probe.Set(`SystemRoot`, `C:\Windows`)
	probe.Set(`ProgramData`, `C:\ProgramData`)
	probe.Set(`ProgramFiles`, `C:\Program Files`)
	probe.Set(`ProgramFiles(x86)`, `C:\Program Files (x86)`)
	probe.Set(`APPDATA`, `C:\Users\probe\AppData\Roaming`)
	probe.Set(`LOCALAPPDATA`, `C:\Users\probe\AppData\Local`)
	probe.Set(`TEMP`, `C:\Temp`)

	guarded, err := guard.New(fsys, probe, `C:\Users\probe`, nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	s := &Scanner{fs: fsys, env: probe, guard: guarded, sys: sys}
	return s, fsys
}

func TestPlanForCommand(t *testing.T) {
	s, _ := probeScanner(t, fake.NewSystem())
	rule := &model.Rule{
		ID:      "dev.npm-cache",
		Targets: []model.Target{{Kind: model.KindCommand, Cmd: "npm", Args: []string{"cache", "clean"}}},
	}

	plan, warnings, err := s.planFor(context.Background(), rule, nil, nil)
	if err != nil {
		t.Fatalf("planFor: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none", warnings)
	}
	if plan.Command == nil || plan.Command.Command != "npm" || !reflect.DeepEqual(plan.Command.Args, []string{"cache", "clean"}) {
		t.Errorf("Command = %+v, want the command and its arguments", plan.Command)
	}
	if len(plan.Items) != 0 || plan.Bytes != 0 || plan.Files != 0 {
		t.Errorf("plan = %+v, want no items at all: the size of a command is not known", plan)
	}
}

func TestPlanSkipsProtected(t *testing.T) {
	s, fsys := probeScanner(t, fake.NewSystem())
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\app`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\app\cache`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\app\cache\fine`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\app\cache\fine\d.txt`, Size: 8})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\app\cache\bad`, IsDir: true, Reparse: true,
		Link: `C:\Windows\System32`})

	rule := &model.Rule{ID: "apps.probe.cache", Targets: []model.Target{
		{Kind: model.KindPath, Path: `%LOCALAPPDATA%\app\cache`, Mode: model.ModeContents},
	}}

	plan, warnings, err := s.planFor(context.Background(), rule, nil, nil)
	if err != nil {
		t.Fatalf("planFor: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none", warnings)
	}

	if want := []string{`C:\Users\probe\AppData\Local\app\cache\fine`}; !reflect.DeepEqual(planItems(plan), want) {
		t.Errorf("items = %q, want the child the guard allowed", planItems(plan))
	}
	if plan.Bytes != 8 || plan.Files != 1 {
		t.Errorf("Bytes/Files = %d/%d, want 8/1", plan.Bytes, plan.Files)
	}
	if plan.Skipped[0].Path != `C:\Users\probe\AppData\Local\app\cache\bad` || plan.Skipped[0].Reason != model.ReasonProtected {
		t.Errorf("Skipped = %+v, want the link refused for the place it leads to", plan.Skipped)
	}
}

func TestPlanSkipsOSMinBuild(t *testing.T) {
	newer := fake.NewSystem()
	newer.SetBuild(22000)

	rule := &model.Rule{ID: "browsers.new-chrome", OSMinBuild: 22000, Targets: []model.Target{
		{Kind: model.KindPath, Path: `%LOCALAPPDATA%\a`, Mode: model.ModeContents},
		{Kind: model.KindPath, Path: `%LOCALAPPDATA%\b`, Mode: model.ModeContents},
	}}

	t.Run("an older build skips every target", func(t *testing.T) {
		s, _ := probeScanner(t, fake.NewSystem())
		s.sys.(*fake.System).SetBuild(19045)

		plan, warnings, err := s.planFor(context.Background(), rule, nil, nil)
		if err != nil {
			t.Fatalf("planFor: %v", err)
		}
		if len(warnings) != 0 {
			t.Errorf("warnings = %q, want none", warnings)
		}
		if len(plan.Items) != 0 || plan.Bytes != 0 {
			t.Errorf("plan = %+v, want nothing: the disk was not touched", plan)
		}
		if len(plan.Skipped) != 2 {
			t.Fatalf("Skipped = %+v, want one skip per target", plan.Skipped)
		}
		for _, skip := range plan.Skipped {
			if skip.Reason != model.ReasonOSTooOld {
				t.Errorf("Skip = %+v, want os_too_old", skip)
			}
		}
	})

	t.Run("the rule of this build is scanned", func(t *testing.T) {
		s, _ := probeScanner(t, newer)
		plan, warnings, err := s.planFor(context.Background(), rule, nil, nil)
		if err != nil {
			t.Fatalf("planFor: %v", err)
		}
		if len(warnings) != 0 {
			t.Errorf("warnings = %q, want none", warnings)
		}
		if len(plan.Skipped) != 0 {
			t.Errorf("Skipped = %+v, want none: the build is new enough", plan.Skipped)
		}
	})
}

// planItems lists the paths of a plan, for the assertions above.
func planItems(plan model.RulePlan) []string {
	var got []string
	for _, item := range plan.Items {
		got = append(got, item.Path)
	}
	return got
}

func TestSkipUnsetVariableIsNotFound(t *testing.T) {
	s, fsys := probeScanner(t, fake.NewSystem())
	_ = fsys

	rule := &model.Rule{ID: "test.unset", Targets: []model.Target{
		{Kind: model.KindPath, Path: `%NOT_SET_AT_ALL%\gone`, Mode: model.ModeSelf}}}
	plan, warnings, err := s.planFor(context.Background(), rule, nil, nil)
	if err != nil {
		t.Fatalf("planFor: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none: the variable says what is missing", warnings)
	}
	if len(plan.Items) != 0 {
		t.Errorf("Items = %q, want none", planItems(plan))
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Reason != model.ReasonNotFound ||
		!strings.Contains(plan.Skipped[0].Detail, "NOT_SET_AT_ALL") {
		t.Errorf("Skipped = %+v, want not_found naming the variable", plan.Skipped)
	}
}

func TestSkipAccessDeniedIsThePathsFaultAndNoMore(t *testing.T) {
	// The directory a: the walk cannot open it. The rule aims at it in the
	// mode of self, so the walk stops with it alone.
	s, fsys := probeScanner(t, fake.NewSystem())
	fsys.Deny(`C:\Users\probe\AppData\Local\a`, fs.ErrPermission)

	rule := &model.Rule{ID: "test.denied", Targets: []model.Target{
		{Kind: model.KindPath, Path: `%LOCALAPPDATA%\a`, Mode: model.ModeSelf}}}
	plan, warnings, err := s.planFor(context.Background(), rule, nil, nil)
	if err != nil {
		t.Fatalf("planFor: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none: the path spoke for itself", warnings)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Reason != model.ReasonAccessDenied {
		t.Errorf("Skipped = %+v, want one access_denied for the directory", plan.Skipped)
	}

	// A directory the walk can read that holds the denied one: what was
	// found before stands in the plan, the denied one is skipped.
	fsys.Deny(`C:\Users\probe\AppData\Local\b\locked`, fs.ErrPermission)
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\b\locked`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\b\free`, Size: 5,
		ModTime: time.Now().Add(-24 * time.Hour)})
	older := &model.Rule{ID: "test.free", Targets: []model.Target{
		{Kind: model.KindPath, Path: `%LOCALAPPDATA%\b`, Mode: model.ModeFiles,
			MinAgeDays: 1}}}
	plan, warnings, err = s.planFor(context.Background(), older, nil, nil)
	if err != nil {
		t.Fatalf("planFor the files mode: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none", warnings)
	}
	if len(plan.Items) != 1 || plan.Items[0].Path != `C:\Users\probe\AppData\Local\b\free` {
		t.Errorf("Items = %q, want the one file the walk could read", planItems(plan))
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Reason != model.ReasonAccessDenied {
		t.Errorf("Skipped = %+v, want access_denied for locked", plan.Skipped)
	}
}
