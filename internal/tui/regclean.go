package tui

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbletea"

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
		m.errText = "The registry is not available on this system."
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
	b.add(titleStyle.Render("Reading the registry..."))
	b.add(mutedStyle.Render("Looking for uninstall commands, autostart values and App Paths that point nowhere."))
	b.add("")
	b.add(mutedStyle.Render("This only reads. Nothing is removed on this screen."))
	if m.errText != "" {
		b.add("")
		b.add(dangerStyle.Render(m.errText))
	}
	return m.renderFrame("Registry 1 of 4", *b, mutedStyle.Render("Esc back"), makeButtons(
		buttonSpec{label: "Cancel", action: actRegBack},
	))
}

// viewRegReview draws the findings with a tick each.
func (m *Model) viewRegReview() string {
	b := &body{}
	count := m.selectedCount()
	b.add(titleStyle.Render(fmt.Sprintf("Dead registry entries (%d found)", len(m.findings))))
	b.add(mutedStyle.Render("Nothing is ticked. Read the reason, then tick only what you understand."))
	b.add("")
	if len(m.findings) == 0 {
		b.add("Nothing dead was found.")
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
		b.add(mutedStyle.Render("Key: " + keyLineOf(m.findings[m.regCursor])))
	}
	if len(m.regNotes) != 0 {
		b.add(mutedStyle.Render(fmt.Sprintf("%d notes; the report names them", len(m.regNotes))))
	}
	hint := "Space tick" + m.skin.sep() + "C continue" + m.skin.sep() + "Esc back"
	return m.renderFrame("Registry 2 of 4", *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: fmt.Sprintf("Continue (%d)", count), primary: true, action: actRegContinue},
		buttonSpec{label: "Back", action: actRegBack},
	))
}

// viewRegConfirm restates what the cleaner will do.
func (m *Model) viewRegConfirm() string {
	selected := m.selectedFindings()
	b := &body{}
	b.add(titleStyle.Render(fmt.Sprintf("Remove %d registry entries?", len(selected))))
	b.add(mutedStyle.Render("Every key is exported to a .reg file before the first deletion."))
	b.add("")
	admin := 0
	for _, finding := range selected {
		if finding.Admin {
			admin++
		}
	}
	b.add(fmt.Sprintf("Entries: %d", len(selected)))
	if admin != 0 {
		if m.regElevated {
			b.add(fmt.Sprintf("Needs administrator: %d (the process has it)", admin))
		} else {
			b.add(warningStyle().Render(fmt.Sprintf("Needs administrator: %d", admin)))
		}
	}
	b.add("Backups: " + m.backup)
	b.add("")
	b.add(mutedStyle.Render("Restore is a double-click on the .reg file of that run."))
	buttons := []buttonSpec{{label: "Yes, remove", primary: true, action: actRegClean}}
	if m.adminSelected() {
		buttons = append(buttons, buttonSpec{label: "Restart as administrator", action: actRegRestart})
	}
	buttons = append(buttons, buttonSpec{label: "Back", action: actRegBack})
	return m.renderFrame("Registry 3 of 4", *b, mutedStyle.Render("Enter remove"+m.skin.sep()+"Esc back"), makeButtons(buttons...))
}

// viewRegRunning says the cleaner works.
func (m *Model) viewRegRunning() string {
	b := &body{}
	b.add(titleStyle.Render("Removing the chosen entries..."))
	b.add(mutedStyle.Render("The backup runs before the first deletion."))
	return m.renderFrame("Registry 4 of 4", *b, "", footerButtons{})
}

// viewRegDone answers what the cleaner came to.
func (m *Model) viewRegDone() string {
	result := m.regResult
	b := &body{}
	b.add(titleStyle.Render(fmt.Sprintf("Removed %d entries", result.Removed)))
	if result.Skipped != 0 {
		b.add(warningStyle().Render(fmt.Sprintf("Skipped: %d", result.Skipped)))
	}
	if result.Failed != 0 {
		b.add(dangerStyle.Render(fmt.Sprintf("Failed: %d", result.Failed)))
	}
	if result.BackupDir != "" {
		b.add("")
		b.add("Backups: " + result.BackupDir)
		b.add(mutedStyle.Render("A double-click on a .reg file puts the entry back."))
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
	return m.renderFrame("Registry done", *b, mutedStyle.Render("R scan again"+m.skin.sep()+"Esc back"), makeButtons(
		buttonSpec{label: "Back to the start", primary: true, action: actRegBack},
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
		m.errText = "The elevation was refused. The entries that need it were taken out."
		return nil
	}
	return tea.Quit
}
