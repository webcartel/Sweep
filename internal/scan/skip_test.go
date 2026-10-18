package scan

import (
	"context"
	"reflect"
	"testing"

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

	plan, warnings, err := s.planFor(context.Background(), rule, nil)
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

	plan, warnings, err := s.planFor(context.Background(), rule, nil)
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

		plan, warnings, err := s.planFor(context.Background(), rule, nil)
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
		plan, warnings, err := s.planFor(context.Background(), rule, nil)
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
