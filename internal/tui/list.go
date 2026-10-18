package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/report"
)

// The screens confirm opens.
const (
	screenConfirm = iota + 3
)

// listState is what the main screen remembers: where the cursor stands over
// the rows the filter leaves, the letters of the search, the order of the
// rows, and whether the run sends to the bin.
type listState struct {
	row       int
	filter    string
	filtering bool
	sort      string // "size" or "name"; it starts named after the size
	trash     bool
}

// visibleRows answers the rows the filter leaves, in the order of the sort.
func (m *Model) visibleRows() []model.RulePlan {
	var rows []model.RulePlan
	for _, rp := range m.plan.Rules {
		if m.filter != "" {
			name := ""
			if rp.Rule != nil {
				name = rp.Rule.Name
			}
			id := ""
			if rp.Rule != nil {
				id = rp.Rule.ID
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

// updateList, the part of the main screen.
func (m *Model) updateList(msg tea.Msg) (tea.Cmd, bool) {
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
		}
	case "a":
		for _, rp := range m.plan.Rules {
			if rp.Rule == nil {
				continue
			}
			if rp.Rule.Risk != model.RiskDangerous {
				m.selection.Rules[rp.Rule.ID] = true
			}
		}
	case "n":
		for _, rp := range m.plan.Rules {
			if rp.Rule == nil {
				continue
			}
			m.selection.Rules[rp.Rule.ID] = false
		}
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
	case "t":
		m.trash = !m.trash
	case "d":
		m.screen = screenConfirm
		return nil, true
	case "r":
		m.upkeepAfterScan()
		m.screen = screenScanning
		return m.startScan(), true
	case "q", "ctrl+c":
		return tea.Quit, true
	}
	return nil, true
}

// upkeepAfterScan wipes what the run held of the scan before, so a rescan
// starts clean.
func (m *Model) upkeepAfterScan() {
	m.plan = nil
	m.progress = map[string]model.RulePlan{}
	m.notes = nil
	m.row = 0
}

// ruleIDOf names a rule of a plan line.
func ruleIDOf(rp model.RulePlan) string {
	if rp.Rule == nil {
		return ""
	}
	return rp.Rule.ID
}

// riskMark is what the cell of the risk says.
func riskMark(rp model.RulePlan) string {
	switch riskOf(rp) {
	case model.RiskDangerous:
		return "DANGER"
	case model.RiskModerate:
		return "MOD"
	default:
		return "SAFE"
	}
}

// riskOf names the risk of a rule of a plan line.
func riskOf(rp model.RulePlan) model.Risk {
	if rp.Rule == nil {
		return ""
	}
	return rp.Rule.Risk
}

// viewList draws the main screen: the header of the run, the rows of the
// rules and the line of the keys at the bottom.
func (m *Model) viewList(width int) string {
	rows := m.visibleRows()
	var rules int
	var files int64
	var bytes int64
	var lines []string
	if m.filtering {
		lines = append(lines, "search: "+m.filter)
	}
	for i, rp := range rows {
		mark := m.skin.checkbox(m.selection.Rules[ruleIDOf(rp)])
		pick := " "
		if i == m.row {
			pick = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %s %-14s %-6s files %-6d %8s",
			pick, mark, ruleIDOf(rp), riskMark(rp), rp.Files, report.FormatBytes(rp.Bytes)))
	}
	_, files, bytes = m.totals()
	rules, _, _ = m.totals()
	lines = append(lines, fmt.Sprintf("profile %s: %d rules, %d files, %s", m.profiles[m.row].Name, rules, files, report.FormatBytes(bytes)))
	lines = append(lines, "space choice  a safe  n none  / filter  o sort  d confirm  t bin  r rescan  q quit")
	_ = width
	return strings.Join(lines, "\n")
}
