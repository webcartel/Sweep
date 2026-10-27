package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/model"
)

// screenList lands after the plan; the screen of the scan.
const (
	screenList = iota + 2
)

// The state of the run ahead of the screens: what a scan comes out with,
// and what the user agreed to take away.

// startScan reads the rules of the profile under the cursor and lets the
// scanner loose on them. The command it answers with waits for the first
// event of the stream.
func (m *Model) startScan() tea.Cmd {
	if m.scanner == nil || m.rulesFor == nil {
		return nil
	}
	m.pickedProfile = m.profiles[m.cursor].Name
	rules, err := m.rulesFor(m.profiles[m.cursor].Name)
	if err != nil {
		m.scanError = err
		return nil
	}
	m.scanRules = rules
	var ctx context.Context
	ctx, m.cancelScan = context.WithCancel(context.Background())
	m.scanEvents = m.scanner.Scan(ctx, rules, model.ScanOptions{Throttle: 100 * time.Millisecond})
	return waitScanEvent(m.scanEvents)
}

// waitScanEvent turns the next event of the stream into a message. The
// spec asks exactly this: every event the channel holds is one message,
// and the stream draws them out of one command at a time.
func waitScanEvent(events <-chan model.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return scanStreamEnded{}
		}
		return event
	}
}

// scanStreamEnded says the scan is done talking.
type scanStreamEnded struct{}

// noteScanEvent keeps what the scan tells while it walks, so the screen of
// the scan names the work: the rule in progress, the notes it had.
func (m *Model) noteScanEvent(event model.Event) {
	switch one := event.(type) {
	case model.RuleStarted:
		if m.progress == nil {
			m.progress = map[string]model.RulePlan{}
		}
		m.progress[one.RuleID] = model.RulePlan{Rule: &model.Rule{ID: one.RuleID}}
	case model.RuleProgress:
		plan := m.progress[one.RuleID]
		plan.Files = int(one.Files)
		plan.Bytes = one.Bytes
		m.progress[one.RuleID] = plan
	case model.RuleScanned:
		if m.progress == nil {
			m.progress = map[string]model.RulePlan{}
		}
		m.progress[one.Plan.Rule.ID] = one.Plan
	case model.Warning:
		m.notes = append(m.notes, one.Message)
	case model.ItemSkipped:
		m.notes = append(m.notes, "skipped "+one.Skip.Path+": "+string(one.Skip.Reason))
	}
}

// updateScanning, the part of the screen of the scan.
func (m *Model) updateScanning(msg tea.Msg) (tea.Cmd, bool) {
	switch one := msg.(type) {
	case tea.KeyMsg:
		if one.String() == "esc" || one.String() == "ctrl+c" || one.String() == "q" {
			if m.cancelScan != nil {
				m.cancelScan()
			}
			return tea.Quit, true
		}
	case model.ScanDone:
		m.plan = one.Plan
		m.adoptPlan()
		m.screen = screenList
		return nil, true
	case scanStreamEnded:
		if m.plan == nil {
			m.screen = screenProfile
			return nil, true
		}
	case model.Event:
		// A start, a progress line, a skip or a warning: the next wait
		// keeps the stream alive, and what came in is drawn from what
		// the screen remembers.
		m.noteScanEvent(one)
		return waitScanEvent(m.scanEvents), true
	}
	return nil, true
}

// adoptPlan builds the choice the profile starts with: every rule except
// the dangerous ones.
func (m *Model) adoptPlan() {
	m.selection = model.Selection{Rules: map[string]bool{}, Exclude: map[string]bool{}}
	picked := map[string]bool{}
	for _, id := range m.preselect {
		picked[id] = true
	}
	for _, rp := range m.plan.Rules {
		if rp.Rule == nil {
			m.selection.Rules[""] = true
			continue
		}
		id := rp.Rule.ID
		switch {
		case picked[id]:
			m.selection.Rules[id] = true
		default:
			m.selection.Rules[id] = rp.Rule.Risk != model.RiskDangerous
		}
	}
}
