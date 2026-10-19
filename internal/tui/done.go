package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbletea"
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
		_ = m.saveReport()
		return nil, true
	case "q", "esc", "ctrl+c":
		return tea.Quit, true
	}
	return nil, false
}

// saveReport writes the answer of the run into one file of the log
// directory, named after the hour it was saved in.
func (m *Model) saveReport() error {
	if m.run.result == nil || m.logDir == "" {
		return nil
	}
	name := "sweep-report-" + time.Now().Format("20060102-150405") + ".json"
	// Next to the day's log, the way the ruling of the run keeps it.
	next := filepath.Join(filepath.Dir(report.LogFile(m.logDir, time.Now())), name)
	file, err := os.Create(next)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return report.ExecJSON(file, m.run.result)
}

// viewDone draws what the run came to: the bytes it drove off the disk,
// what went to the bin, what stayed behind.
func (m *Model) viewDone() string {
	r := m.run.result
	if r == nil {
		return "the run ended without the result"
	}
	lines := []string{fmt.Sprintf("cleaned: %d removed, %s freed", r.Removed, report.FormatBytes(r.FreedBytes))}
	if r.TrashBytes != 0 {
		lines = append(lines, fmt.Sprintf("%s to the bin", report.FormatBytes(r.TrashBytes)))
	}
	if r.Skipped != 0 {
		lines = append(lines, fmt.Sprintf("%d skipped", r.Skipped))
	}
	if r.Failed != 0 {
		lines = append(lines, fmt.Sprintf("%d failed", r.Failed))
	}
	lines = append(lines, "s save the answer  q leave")
	joined := ""
	for _, line := range lines {
		joined += line + "\n"
	}
	return joined
}

//
