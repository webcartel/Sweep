package tui

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/report"
)

// Additional screens of the run.
const (
	screenScanning = iota + 1
)

// ProfileRow is one line of the screen of the profile: what a profile of
// the set is named and what watching it promises.
type ProfileRow struct {
	Name    string
	Rules   int
	MaxRisk model.Risk
}

// Model additions: the profiles of the machine and where the cursor of the
// profile screen stands.
type profileState struct {
	profiles []ProfileRow
	cursor   int
	// pickedProfile is the profile the scan answers with; the screens of
	// the list and of the run name it in the header.
	pickedProfile string
}

// updateProfile, the part of the start screen: arrows move the cursor, Enter
// starts the scan, q leaves the run.
func (m *Model) updateProfile(msg tea.Msg) (tea.Cmd, bool) {
	if wheel, ok := msg.(tea.MouseMsg); ok {
		switch wheel.Button {
		case tea.MouseButtonWheelUp:
			if m.cursor > 0 {
				m.cursor--
			}
			return nil, true
		case tea.MouseButtonWheelDown:
			if m.cursor < len(m.profiles)-1 {
				m.cursor++
			}
			return nil, true
		}
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "q", "esc", "ctrl+c":
		return tea.Quit, true
	case "?":
		m.openHelp()
		return nil, true
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return nil, true
	case "down", "j":
		if m.cursor < len(m.profiles)-1 {
			m.cursor++
		}
		return nil, true
	case "enter":
		m.screen = screenScanning
		return m.startScan(), true
	}
	return nil, false
}

// viewProfile draws the start screen: what sweep does, the three ways to
// clean, and the one button that starts the scan.
func (m *Model) viewProfile() string {
	b := &body{}
	b.add(titleStyle.Render("Free space safely."))
	b.add(mutedStyle.Render("sweep finds files you no longer need. Nothing goes without your approval."))
	b.add("")
	b.add(titleStyle.Render("Choose how deep to clean:"))
	b.add("")

	for i, p := range m.profiles {
		title, about := humanProfile(p.Name)
		mark := "  "
		if i == m.cursor {
			mark = m.skin.pointer() + " "
		}
		name := p.Name
		line := fmt.Sprintf("%s%s", mark, title)
		if title != p.Name {
			line += m.skin.sep() + name
		}
		line += m.skin.sep() + fmt.Sprintf("%d rules", p.Rules)
		if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		index := b.add(line)
		b.add("    " + mutedStyle.Render(about))
		b.hit(index, 0, 78, actPickProfile, strconv.Itoa(i))
		b.hit(index+1, 0, 78, actPickProfile, strconv.Itoa(i))
		b.add("")
	}

	b.add(mutedStyle.Render("You will see exactly what will be removed before anything is deleted."))

	hint := m.skin.arrows() + " choose" + m.skin.sep() + "Enter start" + m.skin.sep() + "? help" + m.skin.sep() + "Q quit"
	startLabel := "Start scan"
	if m.cursor < len(m.profiles) {
		if title, _ := humanProfile(m.profiles[m.cursor].Name); title != "" {
			startLabel = "Start: " + title
		}
	}
	return m.renderFrame("Step 1 of 4", *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: startLabel, primary: true, action: actStart},
		buttonSpec{label: "Help", action: actHelp},
		buttonSpec{label: "Quit", action: actQuit},
	))
}

// viewScanning draws the scan at work: how far the walk came, what it found
// so far, and the button that stops it.
func (m *Model) viewScanning() string {
	b := &body{}
	b.add(titleStyle.Render("Looking for files to clean..."))
	total := len(m.rules)
	done := len(m.progress)
	if total != 0 {
		b.add(mutedStyle.Render(fmt.Sprintf("Rule %d of %d%skeep this window open", done, total, m.skin.sep())))
		b.add(m.skin.bar(48, float64(done)/float64(total)))
	} else {
		b.add(mutedStyle.Render("Reading the rules of the profile..."))
	}
	b.add("")

	var files int64
	var bytes int64
	for _, plan := range m.progress {
		files += int64(plan.Files)
		bytes += plan.Bytes
	}
	b.add(fmt.Sprintf("Found so far: %s in %s files", report.FormatBytes(bytes), humanCount(files)))

	// The rules already walked, the largest first: the screen says what is
	// being found while the walk goes on.
	type walked struct {
		name  string
		files int
		bytes int64
	}
	var found []walked
	for _, rp := range m.progress {
		name := ""
		if rp.Rule != nil {
			name = rp.Rule.Name
			if name == "" {
				name = rp.Rule.ID
			}
		}
		found = append(found, walked{name: name, files: rp.Files, bytes: rp.Bytes})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].bytes > found[j].bytes })
	if len(found) > 6 {
		found = found[:6]
	}
	if len(found) != 0 {
		b.add("")
		for _, one := range found {
			b.add(fmt.Sprintf("  %-40s %8s %8s", mutedLine(one.name, 40),
				humanCount(int64(one.files)), report.FormatBytes(one.bytes)))
		}
	}
	if m.scanError != nil {
		b.add("")
		b.add(dangerStyle.Render("The scan stopped: " + m.scanError.Error()))
	} else if len(m.notes) != 0 {
		b.add(mutedStyle.Render(fmt.Sprintf("%d notes - details are in the final report", len(m.notes))))
	}
	b.add("")
	b.add(mutedStyle.Render("Folders that do not exist are normal: the app may not be installed."))

	return m.renderFrame("Step 2 of 4", *b, mutedStyle.Render("Esc cancel"), makeButtons(
		buttonSpec{label: "Cancel", action: actCancelScan},
	))
}
