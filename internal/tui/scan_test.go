package tui

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
	"github.com/webcartel/sweep/internal/scan"
)

// testMachine builds the scanner of the fake of a machine with one
// directory the rules aim at.
func testMachine(t *testing.T) *scan.Scanner {
	_, _, scanner := machineParts(t)
	return scanner
}

// machineParts builds the fake of a machine the scanner reads: the disk,
// the environment and the scanner that reads them.
func machineParts(t *testing.T) (*fake.FS, *fake.Env, *scan.Scanner) {
	t.Helper()
	fsys := fake.NewFS()
	env := fake.NewEnv()
	env.Set(`USERPROFILE`, `C:\Users\probe`)
	env.Set(`SystemRoot`, `C:\Windows`)
	env.Set(`ProgramData`, `C:\ProgramData`)
	env.Set(`ProgramFiles`, `C:\Program Files`)
	env.Set(`ProgramFiles(x86)`, `C:\Program Files (x86)`)
	env.Set(`APPDATA`, `C:\Users\probe\AppData\Roaming`)
	env.Set(`LOCALAPPDATA`, `C:\Users\probe\AppData\Local`)
	env.Set(`TEMP`, `C:\Temp`)
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\big`, IsDir: true})
	fsys.Add(platform.Info{Path: `C:\Users\probe\AppData\Local\big\f0`, Size: 1})
	guarded, err := guard.New(fsys, env, `C:\Users\probe`, nil)
	if err != nil {
		t.Fatalf("the guard failed to build: %v", err)
	}
	return fsys, env, scan.New(scan.Deps{
		FS: fsys, Env: env, System: fake.NewSystem(), Guard: guarded,
	})
}

// twoScanRules is what a scan of the fake scans: one rule of each risk.
func twoScanRules(target string) []*model.Rule {
	return []*model.Rule{
		{ID: "safe.rule", Risk: model.RiskSafe,
			Targets: []model.Target{{Kind: model.KindPath, Path: target, Mode: model.ModeContents}}},
		{ID: "danger.rule", Risk: model.RiskDangerous,
			Targets: []model.Target{{Kind: model.KindPath, Path: target, Mode: model.ModeContents}}},
	}
}

// The scan of the screen: the events reach the model, the plan lands, and
// the run moves on to the screen of the list with the choice the profile
// asks for - dangerous out.
func TestScanCollectsPlan(t *testing.T) {
	scanner := testMachine(t)
	rules := twoScanRules(`%LOCALAPPDATA%\big`)
	m := New(Deps{
		Profiles: testProfiles,
		Scanner:  scanner,
		RulesFor: func(string) ([]*model.Rule, error) { return rules, nil },
	})

	// The run of the stream is the loop bubble tea drives: one message of
	// one command at a time. The scan screen answers a message with the
	// command that waits for the next event, until the plan lands.
	cur, wait := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	cmd := wait
	for cmd != nil {
		msg := cmd()
		cur, cmd = cur.Update(msg)
	}
	m = cur.(*Model)

	if m.plan == nil {
		t.Fatalf("the scan ended without the plan")
	}
	if len(m.plan.Rules) != 2 {
		t.Fatalf("the plan holds %d rules, want 2", len(m.plan.Rules))
	}
	var files int64
	for _, rp := range m.plan.Rules {
		files += int64(rp.Files)
	}
	// Both rules aim at the tree, so both count the one file: the plan
	// counts what is on offer per rule, not per path.
	if files != 2 {
		t.Fatalf("the plan counted %d files, want 2", files)
	}
	if m.screen != screenList {
		t.Fatalf("screen = %d, want screenList (%d)", m.screen, screenList)
	}
	if !m.selection.Rules["safe.rule"] || m.selection.Rules["danger.rule"] {
		t.Fatalf("the default choice kept dangerous in: %+v", m.selection.Rules)
	}
}

// A scan cut off - Esc pressed while it works - leaves a context the model
// can stop.
func TestScanQuitCancels(t *testing.T) {
	scanner := testMachine(t)
	m := New(Deps{Profiles: testProfiles, Scanner: scanner,
		RulesFor: func(string) ([]*model.Rule, error) {
			return []*model.Rule{{ID: "r"}}, nil
		}})
	after, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = after.(*Model)
	if m.cancelScan == nil {
		t.Fatalf("the scan went without a context of its own")
	}
	after, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = after.(*Model)
	if m.cancelScan == nil {
		t.Fatalf("the context lost its cancel")
	}
	m.cancelScan()
}
