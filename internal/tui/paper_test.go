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
	plan := &model.Plan{}
	for _, rule := range twoScanRules(`%LOCALAPPDATA%\big`) {
		item := model.Item{Path: `C:\Users\probe\AppData\Local\big\f0`, Size: 1, Files: 1}
		plan.Rules = append(plan.Rules, model.RulePlan{Rule: rule, Items: []model.Item{item}, Bytes: 1, Files: 1})
	}
	m.plan = plan
	m.adoptPlan()
	return m
}

// The screens of the ASCII run: a list, the modality of the word, the
// answer of the run.
func TestPaperListToConfirmAndBack(t *testing.T) {
	tm := teatest.NewTestModel(t, aPaperModel(true), teatest.WithInitialTermSize(100, 30))
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "safe.rule")
	}, teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(50*time.Millisecond))

	tm.Type("d")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "clean 1 rules, 1 B")
	}, teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(50*time.Millisecond))

	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "space choice")
	}, teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(50*time.Millisecond))

	if err := tm.Quit(); err != nil {
		t.Fatalf("the screens declined to close: %v", err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// A run of a dangerous rule stands at the word until it is said.
func TestPaperConfirmDangerousAsksTheWord(t *testing.T) {
	m := aPaperModel(true)
	m.selection.Rules["safe.rule"] = false
	m.selection.Rules["danger.rule"] = true
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 30))

	tm.Type("d")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "dangerous: danger.rule")
	}, teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(50*time.Millisecond))

	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	time.Sleep(100 * time.Millisecond) // the word stands between the letters
	tm.Type("yes")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc}) // back out of the word

	out := func(bts []byte) bool { return strings.Contains(string(bts), "space choice") }
	teatest.WaitFor(t, tm.Output(), out, teatest.WithDuration(5*time.Second),
		teatest.WithCheckInterval(50*time.Millisecond))
	if err := tm.Quit(); err != nil {
		t.Fatalf("the screens declined to close: %v", err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}
