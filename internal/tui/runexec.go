package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/exec"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/report"
)

const (
	screenDone = iota + 6 // the result of the run
)

// runState is what the screen of the run keeps: the counts the events
// brought in, the last lines of the feed, the runner and its stream.
type runState struct {
	freed      int64
	trashed    int64
	removed    int
	skipped    int
	failed     int
	rulesDone  int
	feed       []string
	logSink    report.LogSink
	logClose   func()
	result     *model.Result
	cancelExec context.CancelFunc
	execEvents <-chan model.Event
	execCmd    tea.Cmd
	executor   *exec.Executor
}

// startRun, the run the modal agrees to: the executor takes the plan with
// the choice of the user, one stream for the events, one context to stop
// what Esc asks to stop.
func (m *Model) startRun() tea.Cmd {
	if m.executor == nil {
		return nil
	}
	m.run = runState{executor: m.executor}
	if m.logDir != "" {
		// The log is on before the first path goes, the way the clean of
		// the CLI keeps its promise.
		sink, closeLog, err := openRunLog(m.logDir)
		if err == nil {
			m.run.logSink, m.run.logClose = sink, closeLog
		}
	}
	var ctx context.Context
	ctx, m.run.cancelExec = context.WithCancel(context.Background())
	options := model.ExecOptions{}
	if m.trash {
		over := true
		options.Trash = &over
	}
	m.run.execEvents = m.run.executor.Execute(ctx, m.plan, m.selection, options)
	m.run.execCmd = waitExecEvent(m.run.execEvents)
	return m.run.execCmd
}

// openRunLog opens today's JSONL log of the directory the machine keeps
// them in, and hands the sink and its closing back.
func openRunLog(dir string) (report.LogSink, func(), error) {
	at := time.Now()
	log := report.LogFile(dir, at)
	if err := os.MkdirAll(filepath.Dir(log), 0o700); err != nil {
		return nil, nil, fmt.Errorf("open the log: %v", err)
	}
	file, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open the log %s: %w", log, err)
	}
	return report.LogTo(file), func() { _ = file.Close() }, nil
}

// logEntry appends what one event did to the log, when a log is kept.
func (m *Model) logEntry(entry report.Entry) {
	if m.run.logSink != nil {
		_ = m.run.logSink.Append(entry)
	}
}

// closeLog puts the log down, once.
func (m *Model) closeLog() {
	if m.run.logClose != nil {
		m.run.logClose()
		m.run.logClose = nil
		m.run.logSink = nil
	}
}

// waitExecEvent turns the next event of the run into a message.
func waitExecEvent(events <-chan model.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return execStreamEnded{}
		}
		return event
	}
}

// execStreamEnded says the run stopped talking.
type execStreamEnded struct{}

// updateRunning, the part of the screen of the run.
func (m *Model) updateRunning(msg tea.Msg) (tea.Cmd, bool) {
	switch one := msg.(type) {
	case tea.KeyMsg:
		if one.String() == "esc" || one.String() == "q" || one.String() == "ctrl+c" {
			// q while the work is going does not leave the run: it stands
			// back to the word, the way the spec names it.
			if m.run.cancelExec != nil {
				m.run.cancelExec()
				m.run.cancelExec = nil
			}
			m.run.execEvents = nil
			m.run.execCmd = nil
			m.closeLog()
			m.screen = screenConfirm
			return nil, true
		}
	case model.ItemRemoved:
		m.run.freed += one.Bytes
		if one.ToTrash {
			m.run.trashed += one.Bytes
			m.run.feed = append(m.run.feed, "to the bin: "+one.Path)
		} else {
			m.run.removed++
			m.run.feed = append(m.run.feed, "removed: "+one.Path)
		}
		action := "removed"
		if one.ToTrash {
			action = "trashed"
		}
		m.logEntry(report.Entry{Time: time.Now(), RuleID: one.RuleID,
			Path: one.Path, Action: action, Size: one.Bytes, Result: "ok"})
		m.run.execCmd = waitExecEvent(m.run.execEvents)
		return m.run.execCmd, true
	case model.ItemFailed:
		m.run.failed++
		m.run.feed = append(m.run.feed, "failed: "+one.Path)
		m.logEntry(report.Entry{Time: time.Now(), RuleID: one.RuleID,
			Path: one.Path, Action: "failed", Result: one.Err.Error()})
		m.run.execCmd = waitExecEvent(m.run.execEvents)
		return m.run.execCmd, true
	case model.ItemSkipped:
		m.run.skipped++
		m.run.feed = append(m.run.feed, "skipped: "+one.Skip.Path+" ("+string(one.Skip.Reason)+")")
		m.logEntry(report.Entry{Time: time.Now(), RuleID: one.RuleID,
			Path: one.Skip.Path, Action: "skipped", Result: string(one.Skip.Reason)})
		m.run.execCmd = waitExecEvent(m.run.execEvents)
		return m.run.execCmd, true
	case model.ItemPending:
		m.logEntry(report.Entry{Time: time.Now(), RuleID: one.RuleID,
			Path: one.Path, Action: "pending", Result: "ok"})
		m.run.execCmd = waitExecEvent(m.run.execEvents)
		return m.run.execCmd, true
	case model.CommandRan:
		m.run.feed = append(m.run.feed, "ran: "+one.Command)
		m.logEntry(report.Entry{Time: time.Now(), RuleID: one.RuleID,
			Path: one.Command, Action: "ran", Result: "ok"})
		m.run.execCmd = waitExecEvent(m.run.execEvents)
		return m.run.execCmd, true
	case model.ExecDone:
		m.run.result = one.Result
		m.closeLog()
		m.screen = screenDone
		return nil, true
	case execStreamEnded:
		if m.run.result == nil {
			// The stream fell short of the result: the context stopped it.
			m.screen = screenConfirm
		}
		return nil, true
	case model.RuleFinished:
		m.run.rulesDone++
		m.run.feed = append(m.run.feed, fmt.Sprintf("rule %s: %s",
			rpRuleID(one.Result), one.Result.Status))
		m.run.execCmd = waitExecEvent(m.run.execEvents)
		return m.run.execCmd, true
	}
	return nil, true
}

// viewRunning draws the run at work: what the rules closed so far, the
// counts, the last lines of the events and what the keys still stand for.
func (m *Model) viewRunning() string {
	total := 0
	if m.plan != nil {
		total = len(m.plan.Rules)
	}
	lines := []string{"cleaning...", fmt.Sprintf("rules %d of %d", m.run.rulesDone, total)}
	if m.run.trashed != 0 {
		lines = append(lines, report.FormatBytes(m.run.trashed)+" to the bin")
	}
	lines = append(lines, "freed "+report.FormatBytes(m.run.freed))
	lines = append(lines, m.run.feed...)
	lines = append(lines, "esc stand back to the word")
	return lines[0] + "\n" + joinFeed(lines[1:])
}

// rpRuleID names the rule a result answers with.
func rpRuleID(r model.RuleResult) string {
	if r.Rule == nil {
		return ""
	}
	return r.Rule.ID
}

// joinFeed keeps the last lines the screen holds.
func joinFeed(lines []string) string {
	if len(lines) > 10 {
		lines = lines[len(lines)-10:]
	}
	out := ""
	for _, line := range lines {
		out += line + "\n"
	}
	return out
}
