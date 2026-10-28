package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/report"
)

// updateDone, the part of the screen of the result. The s saves what the
// run came to next to the log; q and Esc leave the run on the way out.
func (m *Model) updateDone(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "s":
		m.saveNoteReport()
		return nil, true
	case "q", "esc", "ctrl+c":
		return tea.Quit, true
	}
	return nil, false
}

// saveNoteReport saves the answer and keeps a note of what happened for the
// screen: a silent save read as a button that does nothing.
func (m *Model) saveNoteReport() {
	path, err := m.saveReport()
	switch {
	case err != nil:
		m.run.saveNote = "The report was not saved: " + err.Error()
	case path == "":
		m.run.saveNote = "There is no log directory to save the report in."
	default:
		m.run.saveNote = "Report saved: " + path
	}
}

// saveReport writes the answer of the run into one file of the log
// directory, named after the hour it was saved in, and answers the path.
func (m *Model) saveReport() (string, error) {
	if m.run.result == nil || m.logDir == "" {
		return "", nil
	}
	name := "sweep-report-" + time.Now().Format("20060102-150405") + ".json"
	// Next to the day's log, the way the ruling of the run keeps it.
	next := filepath.Join(filepath.Dir(report.LogFile(m.logDir, time.Now())), name)
	file, err := os.Create(next)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	if err := report.ExecJSON(file, m.run.result); err != nil {
		return "", err
	}
	return next, nil
}

// viewDone draws the answer of the run the way a person reads it: the space
// back, what happened to the files, and the one next step.
func (m *Model) viewDone() string {
	r := m.run.result
	if r == nil {
		b := &body{}
		b.add(titleStyle.Render(i18n.T("tui.done.noresult")))
		b.add(mutedStyle.Render(i18n.T("tui.done.nothingremoved")))
		return m.renderFrame(i18n.T("tui.done.step"), *b, mutedStyle.Render(i18n.T("tui.done.quithint")), makeButtons(
			buttonSpec{label: i18n.T("tui.profile.quit"), action: actQuit},
		))
	}
	b := &body{}
	b.add(bigStyle.Render(i18n.T("tui.done.freed") + report.FormatBytes(r.FreedBytes)))
	b.add(mutedStyle.Render(fmt.Sprintf(i18n.T("tui.done.summary"),
		m.filesCount(r.Files), m.skin.sep(), len(r.PerRule))))
	b.add("")
	b.add(fmt.Sprintf("%-19s%s", i18n.T("tui.done.removed"), humanCount(int64(r.Removed))))
	if r.TrashBytes != 0 {
		b.add(fmt.Sprintf("%-19s%s", i18n.T("tui.done.bin"), report.FormatBytes(r.TrashBytes)))
	}
	if r.Skipped != 0 {
		b.add(fmt.Sprintf("%-19s%s", i18n.T("tui.done.skipped"), humanCount(int64(r.Skipped))))
		b.add(mutedStyle.Render(i18n.T("tui.done.skippedwhy")))
	}
	if r.Failed != 0 {
		b.add(fmt.Sprintf("%-19s%s", i18n.T("tui.done.failed"), humanCount(int64(r.Failed))))
		b.add(mutedStyle.Render(i18n.T("tui.done.failedwhy")))
		var failed []string
		for _, one := range r.PerRule {
			if one.Status == model.StatusFailed {
				name := one.Rule.Name
				if name == "" && one.Rule != nil {
					name = one.Rule.ID
				}
				failed = append(failed, "  "+name)
			}
		}
		if len(failed) > 5 {
			failed = failed[:5]
		}
		for _, line := range failed {
			b.add(mutedStyle.Render(line))
		}
	}
	if len(r.PendingReboot) != 0 {
		b.add(fmt.Sprintf("%-19s%s", i18n.T("tui.done.reboot"), m.filesCount(int64(len(r.PendingReboot)))))
	}
	b.add("")
	b.add(mutedStyle.Render(i18n.T("tui.done.loghint")))
	if m.run.saveNote != "" {
		b.add(successStyle.Render(m.run.saveNote))
	}

	hint := i18n.T("tui.done.hint.save") + m.skin.sep() + i18n.T("tui.done.quithint")
	return m.renderFrame(i18n.T("tui.done.step"), *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: i18n.T("tui.done.savereport"), primary: true, action: actSaveReport},
		buttonSpec{label: i18n.T("tui.profile.quit"), action: actQuit},
	))
}
