package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/webcartel/sweep/internal/model"
)

// aPaperModel puts a model on the screen of the list with the plan of two
// rules, without a scan: what the screens of paper show.
func aPaperModel(ascii bool) *Model {
	m := New(Deps{Profiles: testProfiles, AsciiFlag: ascii, NoASCIIFlag: !ascii})
	m.screen = screenList
	m.pickedProfile = "default"
	plan := &model.Plan{}
	for _, rule := range twoScanRules(`%LOCALAPPDATA%\big`) {
		item := model.Item{Path: `C:\Users\probe\AppData\Local\big\f0`, Size: 1, Files: 1}
		plan.Rules = append(plan.Rules, model.RulePlan{Rule: rule, Items: []model.Item{item}, Bytes: 1, Files: 1})
	}
	m.plan = plan
	m.adoptPlan()
	return m
}

// The screens of the ASCII run: the list, the question, the answer.
func TestPaperListToConfirmAndBack(t *testing.T) {
	tm := teatest.NewTestModel(t, aPaperModel(true), teatest.WithInitialTermSize(100, 30))
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "safe.rule")
	}, teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(50*time.Millisecond))

	tm.Type("c")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Ready to clean 1 B")
	}, teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(50*time.Millisecond))

	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Space toggle")
	}, teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(50*time.Millisecond))

	if err := tm.Quit(); err != nil {
		t.Fatalf("the screens declined to close: %v", err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// A run of a risky rule stands at the word until it is said.
func TestPaperConfirmDangerousAsksTheWord(t *testing.T) {
	m := aPaperModel(true)
	m.selection.Rules["safe.rule"] = false
	m.selection.Rules["danger.rule"] = true
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 30))

	tm.Type("c")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "danger.rule")
	}, teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(50*time.Millisecond))

	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	time.Sleep(100 * time.Millisecond) // the word stands between the letters
	tm.Type("yes")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc}) // back out of the word

	out := func(bts []byte) bool { return strings.Contains(string(bts), "Space toggle") }
	teatest.WaitFor(t, tm.Output(), out, teatest.WithDuration(5*time.Second),
		teatest.WithCheckInterval(50*time.Millisecond))
	if err := tm.Quit(); err != nil {
		t.Fatalf("the screens declined to close: %v", err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// clickAction presses the first box of the screen drawn last that carries
// the action, the way a mouse would.
func clickAction(t *testing.T, m *Model, action string) *Model {
	t.Helper()
	_ = m.View()
	for _, box := range m.hits {
		if box.action != action {
			continue
		}
		x := (box.x1 + box.x2) / 2
		y := (box.y1 + box.y2) / 2
		next, _ := m.Update(tea.MouseMsg{
			X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		})
		return next.(*Model)
	}
	t.Fatalf("no %q box on the screen", action)
	return m
}

// A press on the row of a rule takes the rule in or out, without the
// keyboard.
func TestListMouseTogglesRow(t *testing.T) {
	m := clickAction(t, aPaperModel(true), actToggleRule)
	// The first row is the safe rule, chosen by the profile.
	if m.selection.Rules["safe.rule"] {
		t.Fatalf("the press did not turn the safe rule out: %+v", m.selection.Rules)
	}
	m = clickAction(t, m, actToggleRule)
	if !m.selection.Rules["safe.rule"] {
		t.Fatalf("the second press did not turn it back in")
	}
}

// The buttons of the list open the question and the bin toggle.
func TestListMouseButtons(t *testing.T) {
	m := clickAction(t, aPaperModel(true), actToggleBin)
	if !m.trash {
		t.Fatalf("the bin button did not turn the bin on")
	}
	m = clickAction(t, m, actConfirm)
	if m.screen != screenConfirm {
		t.Fatalf("the clean button left the screen at %d", m.screen)
	}
	m = clickAction(t, m, actBack)
	if m.screen != screenList {
		t.Fatalf("the back button left the screen at %d", m.screen)
	}
}

// The start screen answers a press on a profile card and the start button.
func TestProfileMouse(t *testing.T) {
	m := New(Deps{Profiles: testProfiles, AsciiFlag: true, NoASCIIFlag: false})
	m = clickAction(t, m, actPickProfile)
	if m.cursor != 0 {
		t.Fatalf("the press moved the cursor to %d", m.cursor)
	}
	m = clickAction(t, m, actStart)
	if m.screen != screenScanning {
		t.Fatalf("the start button left the screen at %d", m.screen)
	}
}
