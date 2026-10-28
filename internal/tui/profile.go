package tui

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/i18n"
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
	case "g":
		m.openRules()
		return nil, true
	case "r":
		return m.openRegistry(), true
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
	b.add(titleStyle.Render(i18n.T("tui.profile.headline")))
	b.add(mutedStyle.Render(i18n.T("tui.profile.intro")))
	b.add("")
	b.add(titleStyle.Render(i18n.T("tui.profile.choose")))
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
		line += m.skin.sep() + m.rulesCount(p.Rules)
		if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		index := b.add(line)
		b.add("    " + mutedStyle.Render(about))
		b.hit(index, 0, 78, actPickProfile, strconv.Itoa(i))
		b.hit(index+1, 0, 78, actPickProfile, strconv.Itoa(i))
		b.add("")
	}

	b.add(mutedStyle.Render(i18n.T("tui.profile.promise")))

	hint := m.skin.arrows() + i18n.T("tui.profile.hint.choose") + m.skin.sep() +
		i18n.T("tui.profile.hint.start") + m.skin.sep() +
		i18n.T("tui.profile.hint.rules") + m.skin.sep() +
		i18n.T("tui.profile.hint.registry") + m.skin.sep() +
		i18n.T("tui.profile.hint.help") + m.skin.sep() +
		i18n.T("tui.profile.hint.quit")
	startLabel := i18n.T("tui.profile.start")
	if m.cursor < len(m.profiles) {
		if title, _ := humanProfile(m.profiles[m.cursor].Name); title != "" {
			startLabel = i18n.T("tui.profile.startwith", title)
		}
	}
	return m.renderFrame(i18n.T("tui.frame.step", 1, 4), *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: startLabel, primary: true, action: actStart},
		buttonSpec{label: i18n.T("tui.profile.rules"), action: actRules},
		buttonSpec{label: i18n.T("tui.profile.registry"), action: actRegOpen},
		buttonSpec{label: i18n.T("tui.profile.help"), action: actHelp},
		buttonSpec{label: i18n.T("tui.profile.quit"), action: actQuit},
	))
}

// viewScanning draws the scan at work: how far the walk came, what it found
// so far, and the button that stops it.
func (m *Model) viewScanning() string {
	b := &body{}
	b.add(titleStyle.Render(i18n.T("tui.scan.headline")))
	total := len(m.scanRules)
	done := len(m.progress)
	if total != 0 {
		b.add(mutedStyle.Render(i18n.T("tui.scan.ruleof", done, total) + m.skin.sep() + i18n.T("tui.scan.keepopen")))
		b.add(m.skin.bar(48, float64(done)/float64(total)))
	} else {
		b.add(mutedStyle.Render(i18n.T("tui.scan.reading")))
	}
	b.add("")

	var files int64
	var bytes int64
	for _, plan := range m.progress {
		files += int64(plan.Files)
		bytes += plan.Bytes
	}
	b.add(fmt.Sprintf(i18n.T("tui.scan.found"), report.FormatBytes(bytes), m.filesCount(files)))

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
		b.add(dangerStyle.Render(i18n.T("tui.scan.scanerror") + m.scanError.Error()))
	} else if len(m.notes) != 0 {
		b.add(mutedStyle.Render(i18n.T("tui.scan.notes", m.notesCount(len(m.notes)))))
	}
	b.add("")
	b.add(mutedStyle.Render(i18n.T("tui.scan.absent")))

	return m.renderFrame(i18n.T("tui.frame.step", 2, 4), *b, mutedStyle.Render(i18n.T("tui.scan.cancelhint")), makeButtons(
		buttonSpec{label: i18n.T("tui.scan.cancel"), action: actCancelScan},
	))
}
