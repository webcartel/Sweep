package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform/fake"
	"github.com/webcartel/sweep/internal/scan"
)

// The rules --preselect names are in the choice once the plan lands, even
// the dangerous ones: the word of the run that was already agreed to.
func TestPreselectSelectsRules(t *testing.T) {
	scanner := testMachine(t)
	rules := twoScanRules(`%LOCALAPPDATA%\big`)
	m := New(Deps{Profiles: testProfiles, Scanner: scanner,
		Preselect: []string{"danger.rule"},
		RulesFor:  func(string) ([]*model.Rule, error) { return rules, nil }})
	cur, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for cmd != nil {
		msg := cmd()
		cur, cmd = cur.Update(msg)
	}
	m = cur.(*Model)
	if !m.selection.Rules["danger.rule"] {
		t.Fatalf("the preselect rule stayed out: %+v", m.selection.Rules)
	}
}

// buildAdminMachine puts a machine where the safe rule wants an elevated
// process and the process is none: the modal offers the restart.
func buildAdminMachine(t *testing.T) *Model {
	t.Helper()
	fsys, env, _ := machineParts(t)
	guarded, err := guard.New(fsys, env, `C:\Users\probe`, nil)
	if err != nil {
		t.Fatalf("the guard failed to build: %v", err)
	}
	rules := []*model.Rule{
		{ID: "admin.rule", Risk: model.RiskSafe, RequiresAdmin: true,
			Targets: []model.Target{{Kind: model.KindPath, Path: `%LOCALAPPDATA%\big`, Mode: model.ModeContents}}},
	}
	strong := fake.NewSystem()
	strong.SetElevated(false)
	m := New(Deps{
		Profiles: testProfiles,
		Scanner:  scan.New(scan.Deps{FS: fsys, Env: env, System: strong, Guard: guarded}),
		System:   strong,
		RulesFor: func(string) ([]*model.Rule, error) { return rules, nil },
	})
	cur, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for cmd != nil {
		msg := cmd()
		cur, cmd = cur.Update(msg)
	}
	return cur.(*Model)
}

// The modal offers the restart while a rule wants the elevation the
// process does not have, and Enter does not start the run.
func TestConfirmOffersRestart(t *testing.T) {
	m := buildAdminMachine(t)
	m = stepIn(m, 0, 'c')
	if !m.adminNeeded() {
		t.Fatalf("the run does not ask for the elevation")
	}
	if !strings.Contains(m.View(), "Restart as administrator") {
		t.Fatalf("the modal hides the restart in %q", m.View())
	}
	cur, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = cur.(*Model)
	if m.screen != screenConfirm {
		t.Fatalf("the run started instead of the offer, screen = %d", m.screen)
	}
}

// The restart names the rules the choice holds, under the preselect.
func TestRestartPreselects(t *testing.T) {
	m := buildAdminMachine(t)
	m = stepIn(m, 0, 'c')
	var taken []string
	m.restart = func(preselect []string) error {
		taken = preselect
		return nil
	}
	stepIn(m, tea.KeyEnter)
	if strings.Join(taken, ",") != "admin.rule" {
		t.Fatalf("the restart chose %v, want the admin rule alone", taken)
	}
}

// The user's no to the elevation takes the admin rules out of the choice.
func TestUACRefusalDeselects(t *testing.T) {
	m := buildAdminMachine(t)
	m = stepIn(m, 0, 'c')
	m.restart = func([]string) error { return errRefused }
	cur, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = cur.(*Model)
	if m.screen != screenConfirm {
		t.Fatalf("the refusal does not stand at the modal, screen = %d", m.screen)
	}
	if m.selection.Rules["admin.rule"] {
		t.Fatalf("the rule stayed in the choice after the no")
	}
}

var errRefused = errorString{"the user said no"}

type errorString struct{ s string }

func (e errorString) Error() string { return e.s }


