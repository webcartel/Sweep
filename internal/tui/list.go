package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/report"
)

// The colors the run spends: the ones of the risk, and the rest is
// monochrome with a dim for the keys at the bottom.
var (
	safeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	moderateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	dangerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// The screens confirm opens.
const (
	screenConfirm = iota + 3
)

// listState is what the main screen remembers: where the cursor stands over
// the rows the filter leaves, the letters of the search, the order of the
// rows, whether the run sends to the bin, and the first row the screen
// shows.
type listState struct {
	row       int
	offset    int
	filter    string
	filtering bool
	sort      string // "size" or "name"; it starts named after the size
	trash     bool
	showEmpty bool   // show the rules that found nothing
	notice    string // what the last choice of many rules did
}

// visibleRows answers the rows the filter leaves, in the order of the sort.
func (m *Model) visibleRows() []model.RulePlan {
	var rows []model.RulePlan
	for _, rp := range m.plan.Rules {
		if !m.showEmpty && rp.Files == 0 && rp.Bytes == 0 {
			continue
		}
		if m.filter != "" {
			name, id := "", ""
			if rp.Rule != nil {
				name, id = rp.Rule.Name, rp.Rule.ID
			}
			if !strings.Contains(strings.ToLower(id+" "+name), strings.ToLower(m.filter)) {
				continue
			}
		}
		rows = append(rows, rp)
	}
	switch m.sort {
	case "name":
		sort.SliceStable(rows, func(i, j int) bool {
			return ruleNameOf(rows[i]) < ruleNameOf(rows[j])
		})
	default: // by size, the larger first
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Bytes > rows[j].Bytes })
	}
	return rows
}

// ruleNameOf names a rule of a plan line.
func ruleNameOf(rp model.RulePlan) string {
	if rp.Rule == nil {
		return ""
	}
	return rp.Rule.Name + rp.Rule.ID
}

// totals answers what the choice keeps: the rules, the files and the bytes
// of the plan lines the selection did not turn out, minus the items the
// detail view excluded.
func (m *Model) totals() (rules int, files int64, bytes int64) {
	for _, rp := range m.plan.Rules {
		id := ""
		if rp.Rule != nil {
			id = rp.Rule.ID
		}
		if !m.selection.Rules[id] {
			continue
		}
		rules++
		files += int64(rp.Files)
		bytes += rp.Bytes
		for _, item := range rp.Items {
			if m.selection.Exclude[item.Path] {
				files -= int64(item.Files)
				bytes -= item.Size
			}
		}
	}
	return rules, files, bytes
}

// chooseSafeOnly clears the choice and ticks every safe rule: the button
// says "only", so a dangerous rule ticked before must go out.
func (m *Model) chooseSafeOnly() {
	safe := 0
	for _, rp := range m.plan.Rules {
		if rp.Rule == nil {
			continue
		}
		on := rp.Rule.Risk == model.RiskSafe
		m.selection.Rules[rp.Rule.ID] = on
		if on {
			safe++
		}
	}
	m.notice = fmt.Sprintf(i18n.T("tui.list.safenotice"), ruleCount(safe))
}

// chooseNone takes every rule out of the choice.
func (m *Model) chooseNone() {
	for _, rp := range m.plan.Rules {
		if rp.Rule != nil {
			m.selection.Rules[rp.Rule.ID] = false
		}
	}
	m.notice = "Every rule is out of the choice."
}

// updateList, the part of the main screen.
func (m *Model) updateList(msg tea.Msg) (tea.Cmd, bool) {
	if wheel, ok := msg.(tea.MouseMsg); ok {
		switch wheel.Button {
		case tea.MouseButtonWheelUp:
			return m.updateListKey("up")
		case tea.MouseButtonWheelDown:
			return m.updateListKey("down")
		}
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	if m.filtering {
		switch key.String() {
		case "esc":
			m.filtering = false
			m.filter = ""
			return nil, true
		case "enter":
			m.filtering = false
			return nil, true
		case "backspace":
			if runes := []rune(m.filter); len(runes) > 0 {
				m.filter = string(runes[:len(runes)-1])
			}
			m.row = 0
			return nil, true
		}
		runes := key.Runes
		if len(runes) != 0 {
			m.filter += string(runes)
			m.row = 0
		}
		return nil, true
	}
	rows := m.visibleRows()
	switch key.String() {
	case "up", "k":
		if m.row > 0 {
			m.row--
		}
	case "down", "j":
		if m.row < len(rows)-1 {
			m.row++
		}
	case " ":
		if m.row < len(rows) {
			id := ruleIDOf(rows[m.row])
			m.selection.Rules[id] = !m.selection.Rules[id]
			m.notice = ""
		}
	case "a":
		m.chooseSafeOnly()
	case "n":
		m.chooseNone()
	case "o":
		if m.sort == "size" {
			m.sort = "name"
		} else {
			m.sort = "size"
		}
		m.row = 0
	case "/":
		m.filtering = true
		m.filter = ""
		m.row = 0
	case "?":
		m.openHelp()
		return nil, true
	case "t":
		m.trash = !m.trash
	case "e":
		m.showEmpty = !m.showEmpty
		m.row = 0
		m.offset = 0
	case "c":
		m.screen = screenConfirm
		m.confirmText = ""
		return nil, true
	case "d", "enter":
		return m.openDetail()
	case "r":
		m.upkeepAfterScan()
		m.screen = screenScanning
		return m.startScan(), true
	case "q", "ctrl+c":
		return tea.Quit, true
	}
	return nil, true
}

// updateListKey feeds one key name into the list, so the wheel and the
// clicks answer the same way the keys do.
func (m *Model) updateListKey(name string) (tea.Cmd, bool) {
	return m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)})
}

// upkeepAfterScan wipes what the run held of the scan before, so a rescan
// starts clean.
func (m *Model) upkeepAfterScan() {
	m.plan = nil
	m.progress = map[string]model.RulePlan{}
	m.notes = nil
	m.row = 0
	m.offset = 0
}

// ruleIDOf names a rule of a plan line.
func ruleIDOf(rp model.RulePlan) string {
	if rp.Rule == nil {
		return ""
	}
	return rp.Rule.ID
}

// riskMark is what the cell of the risk says, in the only color the run
// spends: the one of the risk itself.
func riskMark(rp model.RulePlan) string {
	return riskStyleOf(riskOf(rp)).Render(riskWord(riskOf(rp)))
}

// riskOf names the risk of a rule of a plan line.
func riskOf(rp model.RulePlan) model.Risk {
	if rp.Rule == nil {
		return ""
	}
	return rp.Rule.Risk
}

// viewList draws the main screen: what the scan found, one line per rule,
// the explanation of the row under the cursor, and the buttons of the run.
func (m *Model) viewList() string {
	rows := m.visibleRows()
	rules, files, bytes := m.totals()

	inner := m.screenWidth() - 6
	nameWidth := 48
	if nameWidth > inner-40 {
		nameWidth = inner - 40
	}
	if nameWidth < 20 {
		nameWidth = 20
	}

	b := &body{}
	if m.filtering {
		b.add(i18n.T("tui.list.find") + " " + m.filter + m.skin.caret())
	} else {
		b.add(titleStyle.Render(fmt.Sprintf(i18n.T("tui.list.found"), report.FormatBytes(bytes))))
		_, about := humanProfile(m.profileOfHeader())
		b.add(mutedStyle.Render(fmt.Sprintf(i18n.T("tui.list.chosen"),
			rules, ruleCount(len(m.plan.Rules)), m.skin.sep(), humanCount(files), m.skin.sep(), about)))
	}
	b.add("")

	order := ""
	if m.sort == "name" {
		order = i18n.T("tui.list.sorted")
	}
	header := fmt.Sprintf("    %-*s  %-7s %8s %9s   %s", nameWidth,
		i18n.T("tui.list.item"), i18n.T("tui.list.risk"), i18n.T("tui.list.files"),
		i18n.T("tui.list.size"), order)
	b.add(mutedStyle.Render(mutedLine(riskLegend(), inner)))
	b.add(mutedStyle.Render(header))

	// Rows that fit between the header and the explanation lines.
	room := m.screenHeight() - 16
	if room < 1 {
		room = 1
	}
	if m.row < m.offset {
		m.offset = m.row
	}
	if m.row >= m.offset+room {
		m.offset = m.row - room + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
	shown := rows
	if m.offset < len(shown) {
		shown = shown[m.offset:]
	} else {
		shown = nil
	}
	if len(shown) > room {
		shown = shown[:room]
	}

	for i, rp := range shown {
		index := m.offset + i
		mark := m.skin.checkbox(m.selection.Rules[ruleIDOf(rp)])
		pick := " "
		if index == m.row {
			pick = m.skin.pointer()
		}
		name := ""
		if rp.Rule != nil {
			name = rp.Rule.Name
		}
		if name == "" {
			name = ruleIDOf(rp)
		}
		sizeText := report.FormatBytes(rp.Bytes)
		if rp.Files == 0 && rp.Bytes == 0 {
			sizeText = i18n.T("tui.list.nothing")
		}
		line := fmt.Sprintf("%s %s %-*s  %-7s %8s %9s   %s",
			pick, mark, nameWidth, fit(name, nameWidth),
			riskMark(rp), humanCount(int64(rp.Files)), sizeText, m.skin.chevron())
		if index == m.row {
			line = selectedStyle.Render(line)
		}
		lineIndex := b.add(line)
		b.hit(lineIndex, 0, inner-4, actToggleRule, ruleIDOf(rp))
		b.boxes = append(b.boxes, clickBox{
			y1: lineIndex, y2: lineIndex,
			x1: lipgloss.Width(line) - 1, x2: lipgloss.Width(line),
			action: actOpenDetail, arg: fmt.Sprint(index),
		})
	}

	b.add("")
	b.add(m.mutedRowDescription(rows))

	hidden := 0
	for _, rp := range m.plan.Rules {
		if rp.Files == 0 && rp.Bytes == 0 {
			hidden++
		}
	}
	if hidden != 0 {
		label := i18n.T("tui.list.hidden.show", hidden)
		if m.showEmpty {
			label = i18n.T("tui.list.hidden.hide", hidden)
		}
		index := b.add(mutedStyle.Render(label))
		b.hit(index, 0, len(label)+2, actShowEmpty, "")
	}

	binState := i18n.T("tui.list.bin.off")
	binButton := i18n.T("tui.list.bin.buttonoff")
	if m.trash {
		binState = i18n.T("tui.list.bin.on")
		binButton = i18n.T("tui.list.bin.buttonon")
	}
	b.add(mutedStyle.Render(i18n.T("tui.list.bintitle") + " " + binState))
	b.add(mutedStyle.Render(i18n.T("tui.list.nothingyet")))
	if m.notice != "" {
		b.add(successStyle.Render(m.notice))
	}

	cleanLabel := fmt.Sprintf(i18n.T("tui.list.cleanup"), report.FormatBytes(bytes))
	if bytes == 0 {
		cleanLabel = i18n.T("tui.list.nothingchosen")
	}
	hint := i18n.T("tui.list.hint.toggle") + m.skin.sep() +
		i18n.T("tui.list.hint.safe") + m.skin.sep() +
		i18n.T("tui.list.hint.none") + m.skin.sep() +
		i18n.T("tui.list.hint.find") + m.skin.sep() +
		i18n.T("tui.list.hint.sort") + m.skin.sep() +
		i18n.T("tui.list.hint.details") + m.skin.sep() +
		i18n.T("tui.list.hint.help")
	return m.renderFrame(i18n.T("tui.frame.step", 3, 4), *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: cleanLabel, primary: true, action: actConfirm},
		buttonSpec{label: i18n.T("tui.list.safely"), action: actSelectSafe},
		buttonSpec{label: binButton, action: actToggleBin},
		buttonSpec{label: i18n.T("tui.profile.help"), action: actHelp},
		buttonSpec{label: i18n.T("tui.profile.quit"), action: actQuit},
	))
}

// mutedRowDescription answers the one line under the rows: what the rule
// under the cursor takes away, in the words of the rule itself.
func (m *Model) mutedRowDescription(rows []model.RulePlan) string {
	if m.filtering || len(rows) == 0 || m.row >= len(rows) || rows[m.row].Rule == nil {
		return ""
	}
	rule := rows[m.row].Rule
	about := rule.Description
	if about == "" {
		about = rule.Name
	}
	limit := m.screenWidth() - 12
	if limit < 20 {
		limit = 20
	}
	if len(about) > limit {
		about = about[:limit-1] + m.skin.ellipsis()
	}
	return mutedStyle.Render(m.skin.pointer() + " " + about)
}

// profileOfHeader names the profile the run came out of; a scan that has
// not picked yet asks for the profile under the cursor.
func (m *Model) profileOfHeader() string {
	if m.pickedProfile != "" {
		return m.pickedProfile
	}
	if m.cursor < len(m.profiles) {
		return m.profiles[m.cursor].Name
	}
	return ""
}
