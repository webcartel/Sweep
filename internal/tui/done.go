package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbletea"
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
		b.add(titleStyle.Render("The run ended without a result."))
		b.add(mutedStyle.Render("Nothing was removed."))
		return m.renderFrame("Done", *b, mutedStyle.Render("Q quit"), makeButtons(
			buttonSpec{label: "Quit", action: actQuit},
		))
	}
	b := &body{}
	b.add(bigStyle.Render("Freed " + report.FormatBytes(r.FreedBytes)))
	b.add(mutedStyle.Render(fmt.Sprintf("%s files removed%s%d rules", humanCount(r.Files), m.skin.sep(), len(r.PerRule))))
	b.add("")
	b.add(fmt.Sprintf("Removed:            %s", humanCount(int64(r.Removed))))
	if r.TrashBytes != 0 {
		b.add(fmt.Sprintf("Sent to the bin:    %s", report.FormatBytes(r.TrashBytes)))
	}
	if r.Skipped != 0 {
		b.add(fmt.Sprintf("Skipped:            %s", humanCount(int64(r.Skipped))))
		b.add(mutedStyle.Render("Skipped files were in use. Close the app and run Sweep again."))
	}
	if r.Failed != 0 {
		b.add(fmt.Sprintf("Failed:             %s", humanCount(int64(r.Failed))))
		b.add(mutedStyle.Render("Close the apps that hold these files and try again."))
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
		b.add(fmt.Sprintf("After a restart:    %s files", humanCount(int64(len(r.PendingReboot)))))
	}
	b.add("")
	b.add(mutedStyle.Render("Every action is in the log. Press S to save a JSON report."))
	if m.run.saveNote != "" {
		b.add(successStyle.Render(m.run.saveNote))
	}

	hint := "S save report" + m.skin.sep() + "Q quit"
	return m.renderFrame("Done", *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: "Save report", primary: true, action: actSaveReport},
		buttonSpec{label: "Quit", action: actQuit},
	))
}
