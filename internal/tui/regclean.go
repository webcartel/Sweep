package tui

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbletea"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/registry"
	"github.com/webcartel/sweep/internal/report"
)

// The screens of the registry cleanup.
const (
	screenRegScan = iota + 15
	screenRegReview
	screenRegConfirm
	screenRegRunning
	screenRegDone
)

// regState is what the registry screens remember.
type regState struct {
	reg             platform.Registry
	fsys            platform.FS
	regEnv          platform.Env
	backup          string
	regElevated     bool
	registryRestart func(ids []string) error

	findings  []registry.Finding
	selected  map[string]bool
	regCursor int
	regOffset int
	regNotes  []string
	errText   string
	regResult registry.Result
}

// registryScanDone carries what the scan found.
type registryScanDone struct {
	findings []registry.Finding
	notes    []string
	err      error
}

// registryCleanDone carries what the cleaner came to.
type registryCleanDone struct {
	result registry.Result
	err    error
}

// openRegistry starts the registry flow.
func (m *Model) openRegistry() tea.Cmd {
	m.screen = screenRegScan
	m.selected = map[string]bool{}
	m.errText = ""
	m.findings = nil
	m.regNotes = nil
	m.regResult = registry.Result{}
	return m.scanRegistry()
}

// scanRegistry reads the registry in one command.
func (m *Model) scanRegistry() tea.Cmd {
	if m.reg == nil {
		m.errText = i18n.T("tui.reg.unavailable")
		return nil
	}
	reg, fsys, env := m.reg, m.fsys, m.regEnv
	return func() tea.Msg {
		findings, notes, err := registry.Scan(reg, fsys, env)
		return registryScanDone{findings: findings, notes: notes, err: err}
	}
}

// updateRegScan takes the answer of the scan.
func (m *Model) updateRegScan(msg tea.Msg) (tea.Cmd, bool) {
	switch one := msg.(type) {
	case registryScanDone:
		if one.err != nil {
			m.errText = one.err.Error()
			return nil, true
		}
		m.findings = one.findings
		m.regNotes = one.notes
		if m.selected == nil {
			m.selected = map[string]bool{}
		}
		for _, id := range m.registryPreselect {
			m.selected[id] = true
		}
		m.screen = screenRegReview
		return nil, true
	case tea.KeyMsg:
		if one.String() == "esc" || one.String() == "q" {
			m.screen = screenProfile
		}
	}
	return nil, true
}

// updateRegReview moves and ticks the findings.
func (m *Model) updateRegReview(msg tea.Msg) (tea.Cmd, bool) {
	if wheel, ok := msg.(tea.MouseMsg); ok {
		switch wheel.Button {
		case tea.MouseButtonWheelUp:
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case tea.MouseButtonWheelDown:
			msg = tea.KeyMsg{Type: tea.KeyDown}
		}
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "esc", "q":
		m.screen = screenProfile
	case "up", "k":
		if m.regCursor > 0 {
			m.regCursor--
		}
	case "down", "j":
		if m.regCursor < len(m.findings)-1 {
			m.regCursor++
		}
	case " ":
		m.toggleRegistryFinding(m.regCursor)
	case "c":
		if m.selectedCount() != 0 {
			m.screen = screenRegConfirm
		}
	case "?":
		m.openHelp()
	}
	return nil, true
}

// toggleRegistryFinding ticks or unticks one finding.
func (m *Model) toggleRegistryFinding(index int) {
	if index < 0 || index >= len(m.findings) {
		return
	}
	id := m.findings[index].ID
	m.selected[id] = !m.selected[id]
}

// selectedCount answers how many findings are ticked.
func (m *Model) selectedCount() int {
	count := 0
	for _, finding := range m.findings {
		if m.selected[finding.ID] {
			count++
		}
	}
	return count
}

// selectedFindings answers the ticked findings.
func (m *Model) selectedFindings() []registry.Finding {
	var out []registry.Finding
	for _, finding := range m.findings {
		if m.selected[finding.ID] {
			out = append(out, finding)
		}
	}
	return out
}

// adminSelected says whether a ticked finding needs the elevated process.
func (m *Model) adminSelected() bool {
	if m.regElevated {
		return false
	}
	for _, finding := range m.selectedFindings() {
		if finding.Admin {
			return true
		}
	}
	return false
}

// updateRegConfirm runs the cleaner on the word.
func (m *Model) updateRegConfirm(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "esc", "q":
		m.screen = screenRegReview
	case "enter":
		return m.startRegistryClean(), true
	}
	return nil, true
}

// startRegistryClean hands the choice to the cleaner.
func (m *Model) startRegistryClean() tea.Cmd {
	m.screen = screenRegRunning
	reg := m.reg
	selected := m.selectedFindings()
	options := registry.Options{
		Elevated: m.regElevated, BackupRoot: m.backup, Now: time.Now,
		Log: func(action, path, note string) {
			m.logEntry(report.Entry{Time: time.Now(), Path: path, Action: action, Result: note})
		},
	}
	return func() tea.Msg {
		result, err := registry.Clean(reg, selected, options)
		return registryCleanDone{result: result, err: err}
	}
}

// updateRegRunning takes the answer of the cleaner.
func (m *Model) updateRegRunning(msg tea.Msg) (tea.Cmd, bool) {
	if one, ok := msg.(registryCleanDone); ok {
		m.regResult = one.result
		if one.err != nil {
			m.errText = one.err.Error()
		}
		m.screen = screenRegDone
	}
	return nil, true
}

// updateRegDone leaves or starts again.
func (m *Model) updateRegDone(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "r":
		return m.openRegistry(), true
	case "esc", "q", "enter":
		m.screen = screenProfile
	}
	return nil, true
}

// viewRegScan draws the read.
func (m *Model) viewRegScan() string {
	b := &body{}
	b.add(titleStyle.Render(i18n.T("tui.reg.scan")))
	b.add(mutedStyle.Render(i18n.T("tui.reg.scanintro")))
	b.add("")
	b.add(mutedStyle.Render(i18n.T("tui.reg.readonly")))
	if m.errText != "" {
		b.add("")
		b.add(dangerStyle.Render(m.errText))
	}
	return m.renderFrame(i18n.T("tui.reg.scanstep"), *b, mutedStyle.Render(i18n.T("tui.hint.back")), makeButtons(
		buttonSpec{label: i18n.T("tui.reg.cancel"), action: actRegBack},
	))
}

// viewRegReview draws the findings with a tick each.
func (m *Model) viewRegReview() string {
	b := &body{}
	count := m.selectedCount()
	b.add(titleStyle.Render(fmt.Sprintf(i18n.T("tui.reg.found"), len(m.findings))))
	b.add(mutedStyle.Render(i18n.T("tui.reg.reviewintro")))
	b.add("")
	if len(m.findings) == 0 {
		b.add(i18n.T("tui.reg.nothing"))
	}
	room := m.screenHeight() - 14
	if room < 2 {
		room = 2
	}
	if m.regCursor < m.regOffset {
		m.regOffset = m.regCursor
	}
	if m.regCursor >= m.regOffset+room {
		m.regOffset = m.regCursor - room + 1
	}
	shown := m.findings
	if m.regOffset < len(shown) {
		shown = shown[m.regOffset:]
	} else {
		shown = nil
	}
	if len(shown) > room {
		shown = shown[:room]
	}
	for i, finding := range shown {
		index := m.regOffset + i
		pick := " "
		if index == m.regCursor {
			pick = m.skin.pointer()
		}
		mark := m.skin.checkbox(m.selected[finding.ID])
		admin := ""
		if finding.Admin {
			admin = " [A]"
		}
		name := finding.Name
		if name == "" {
			name = finding.Key
		}
		line := fmt.Sprintf("%s %s %-10s %-28s %s%s", pick, mark, finding.Kind, fit(name, 28), finding.Reason, admin)
		if index == m.regCursor {
			line = selectedStyle.Render(line)
		}
		lineIndex := b.add(line)
		b.hit(lineIndex, 0, m.screenWidth()-8, actRegToggle, fmt.Sprint(index))
	}
	if m.regCursor < len(m.findings) {
		b.add("")
		b.add(mutedStyle.Render(i18n.T("tui.reg.key") + keyLineOf(m.findings[m.regCursor])))
	}
	if len(m.regNotes) != 0 {
		b.add(mutedStyle.Render(fmt.Sprintf(i18n.T("tui.reg.notes"), len(m.regNotes))))
	}
	hint := i18n.T("tui.reg.hint.tick") + m.skin.sep() + i18n.T("tui.reg.hint.continue") + m.skin.sep() + i18n.T("tui.hint.back")
	return m.renderFrame(i18n.T("tui.reg.reviewstep"), *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: fmt.Sprintf(i18n.T("tui.reg.continue"), count), primary: true, action: actRegContinue},
		buttonSpec{label: i18n.T("tui.reg.back"), action: actRegBack},
	))
}

// viewRegConfirm restates what the cleaner will do.
func (m *Model) viewRegConfirm() string {
	selected := m.selectedFindings()
	b := &body{}
	b.add(titleStyle.Render(fmt.Sprintf(i18n.T("tui.reg.confirm"), len(selected))))
	b.add(mutedStyle.Render(i18n.T("tui.reg.exportnote")))
	b.add("")
	admin := 0
	for _, finding := range selected {
		if finding.Admin {
			admin++
		}
	}
	b.add(fmt.Sprintf(i18n.T("tui.reg.entries"), len(selected)))
	if admin != 0 {
		if m.regElevated {
			b.add(fmt.Sprintf(i18n.T("tui.reg.adminhas"), admin))
		} else {
			b.add(warningStyle().Render(fmt.Sprintf(i18n.T("tui.reg.adminneed"), admin)))
		}
	}
	b.add(i18n.T("tui.reg.backups") + m.backup)
	b.add("")
	b.add(mutedStyle.Render(i18n.T("tui.reg.restore")))
	buttons := []buttonSpec{{label: i18n.T("tui.reg.yesremove"), primary: true, action: actRegClean}}
	if m.adminSelected() {
		buttons = append(buttons, buttonSpec{label: i18n.T("tui.reg.restart"), action: actRegRestart})
	}
	buttons = append(buttons, buttonSpec{label: i18n.T("tui.reg.back"), action: actRegBack})
	return m.renderFrame(i18n.T("tui.reg.confirmstep"), *b, mutedStyle.Render(i18n.T("tui.reg.hint.remove")+m.skin.sep()+i18n.T("tui.hint.back")), makeButtons(buttons...))
}

// viewRegRunning says the cleaner works.
func (m *Model) viewRegRunning() string {
	b := &body{}
	b.add(titleStyle.Render(i18n.T("tui.reg.running")))
	b.add(mutedStyle.Render(i18n.T("tui.reg.runningnote")))
	return m.renderFrame(i18n.T("tui.reg.runningstep"), *b, "", footerButtons{})
}

// viewRegDone answers what the cleaner came to.
func (m *Model) viewRegDone() string {
	result := m.regResult
	b := &body{}
	b.add(titleStyle.Render(fmt.Sprintf(i18n.T("tui.reg.done"), result.Removed)))
	if result.Skipped != 0 {
		b.add(warningStyle().Render(fmt.Sprintf(i18n.T("tui.reg.skipped"), result.Skipped)))
	}
	if result.Failed != 0 {
		b.add(dangerStyle.Render(fmt.Sprintf(i18n.T("tui.reg.failed"), result.Failed)))
	}
	if result.BackupDir != "" {
		b.add("")
		b.add(i18n.T("tui.reg.backups") + result.BackupDir)
		b.add(mutedStyle.Render(i18n.T("tui.reg.donerestore")))
	}
	if m.errText != "" {
		b.add("")
		b.add(dangerStyle.Render(m.errText))
	}
	for _, entry := range result.Entries {
		if entry.Action == "deleted" {
			continue
		}
		b.add(mutedStyle.Render(fmt.Sprintf("%s: %s (%s)", entry.Finding.Name, entry.Action, entry.Note)))
	}
	return m.renderFrame(i18n.T("tui.reg.donestep"), *b, mutedStyle.Render(i18n.T("tui.reg.donehint")+m.skin.sep()+i18n.T("tui.hint.back")), makeButtons(
		buttonSpec{label: i18n.T("tui.reg.backto"), primary: true, action: actRegBack},
	))
}

// keyLineOf names the key and value of a finding.
func keyLineOf(finding registry.Finding) string {
	if finding.Value == "" {
		return finding.Key
	}
	return finding.Key + `\` + finding.Value
}

// registryBackupDir answers where the .reg copies go on this machine.
func registryBackupDir(env platform.Env) string {
	local, _ := env.Get("LOCALAPPDATA")
	if local == "" {
		return ""
	}
	return filepath.Join(local, "sweep", "registry-backups")
}

// elevatedOf says whether the screens may change HKLM. An unreadable
// elevation counts as none, so the user is asked rather than refused late.
func elevatedOf(system platform.System) bool {
	if system == nil {
		return false
	}
	strong, err := system.Elevated()
	if err != nil {
		return false
	}
	return strong
}

// restartRegistry hands the choice to an elevated run.
func (m *Model) restartRegistry() tea.Cmd {
	if m.registryRestart == nil || !m.adminSelected() {
		return nil
	}
	var ids []string
	for _, finding := range m.selectedFindings() {
		ids = append(ids, finding.ID)
	}
	if err := m.registryRestart(ids); err != nil {
		// The elevation is the user's say: what they turned down leaves
		// the choice, and the question stands open again.
		for _, finding := range m.findings {
			if finding.Admin {
				m.selected[finding.ID] = false
			}
		}
		m.errText = i18n.T("tui.reg.elevationrefused")
		return nil
	}
	return tea.Quit
}
