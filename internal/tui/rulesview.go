package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/rules"
)

// The screens of the rules: what the run knows, what the config adds, the
// user's own files and the editor.
const (
	screenRules = iota + 8
	screenRuleList
	screenRuleDetail
	screenConfig
	screenUserRules
	screenEditor
	screenPreview
)

// rulesState is what the rules screens remember between frames.
type rulesState struct {
	set          *rules.Set
	rulesDir     string
	configPath   string
	settingsPath string
	reload       func() (*rules.Set, error)
	env          platform.Env

	// langNotice is what the last save of the language said, and
	// langFailed is whether it had to say it the hard way.
	langNotice string
	langFailed bool
	// langEntry is the language the picker opened with, and langCursor
	// the row the arrow keys stand on.
	langEntry  i18n.Lang
	langCursor int

	ruleCursor   int
	ruleOffset   int
	userCursor   int
	userOffset   int
	configOffset int

	detail *model.Rule
	// pendingDelete is the file the delete is asked about.
	pendingDelete string
	// userFiles are the rule files of the user, in name order.
	userFiles []string
	message   string
	editor    editorState
}

// rulesOf answers the rules of the set, or nothing before the set arrives.
func (m *Model) rulesOf() []*model.Rule {
	if m.set == nil {
		return nil
	}
	return m.set.Rules
}

// sourceOf names where a rule came from, in the words of the screen.
func sourceOf(rule *model.Rule) string {
	if rule.Source == rules.SourceBuiltin || rule.Source == "" {
		return i18n.T("tui.rules.source.builtin")
	}
	return filepath.Base(rule.Source)
}

// refreshRules reads the sources again after an edit and takes the new set
// into the model.
func (m *Model) refreshRules() error {
	if m.reload == nil {
		return nil
	}
	set, err := m.reload()
	if err != nil {
		return err
	}
	m.set = set
	m.rebuildProfiles()
	return nil
}

// rebuildProfiles recounts the profile rows after the set changed.
func (m *Model) rebuildProfiles() {
	if m.set == nil {
		return
	}
	var names []string
	for name := range m.set.Profiles {
		if name != "default" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	rows := make([]ProfileRow, 0, len(names)+1)
	count := func(name string) int {
		chosen, _, err := rules.Select(m.set, name)
		if err != nil {
			return 0
		}
		return len(chosen)
	}
	if profile, ok := m.set.Profiles["default"]; ok {
		rows = append(rows, ProfileRow{Name: "default", Rules: count("default"), MaxRisk: profile.MaxRisk})
	}
	for _, name := range names {
		profile := m.set.Profiles[name]
		rows = append(rows, ProfileRow{Name: name, Rules: count(name), MaxRisk: profile.MaxRisk})
	}
	if len(rows) != 0 {
		m.profiles = rows
		if m.cursor >= len(rows) {
			m.cursor = len(rows) - 1
		}
	}
}

// openRules takes the model to the settings screen.
func (m *Model) openRules() {
	m.message = ""
	m.screen = screenRules
}

// ---- the settings screen ----

func (m *Model) updateRules(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "esc", "q":
		m.screen = screenProfile
	case "b":
		m.screen = screenRuleList
		m.ruleCursor, m.ruleOffset = 0, 0
	case "c":
		m.screen = screenConfig
		m.configOffset = 0
	case "u":
		m.screen = screenUserRules
		m.refreshUserFiles()
	case "l":
		m.openLanguage()
	case "?":
		m.openHelp()
	}
	return nil, true
}

func (m *Model) viewRules() string {
	b := &body{}
	b.add(titleStyle.Render(i18n.T("tui.rules.title")))
	b.add(mutedStyle.Render(i18n.T("tui.rules.intro")))
	b.add("")

	builtin, own := 0, 0
	for _, rule := range m.rulesOf() {
		if rule.Source == rules.SourceBuiltin || rule.Source == "" {
			builtin++
			continue
		}
		own++
	}
	b.add(fmt.Sprintf(i18n.T("tui.rules.counts"), builtin, own))
	b.add(fmt.Sprintf(i18n.T("tui.rules.profiles"), len(m.profiles)))
	config := m.configPath
	state := i18n.T("tui.rules.confignotfound")
	if _, err := os.Stat(config); err == nil {
		state = i18n.T("tui.rules.configfound")
	}
	b.add(fmt.Sprintf(i18n.T("tui.rules.config"), config, state))
	if m.rulesDir != "" {
		b.add(fmt.Sprintf(i18n.T("tui.rules.yourrules"), m.rulesDir))
	}
	if m.langNotice != "" {
		b.add("")
		b.add(m.langNotice)
	}
	if m.message != "" {
		b.add("")
		b.add(warningStyle().Render(m.message))
	}
	b.add("")
	b.add(mutedStyle.Render(i18n.T("tui.rules.holds")))
	b.add("  " + i18n.T("tui.rules.holds.browse"))
	b.add("  " + i18n.T("tui.rules.holds.config"))
	b.add("  " + i18n.T("tui.rules.holds.lang"))
	b.add("  " + i18n.T("tui.rules.holds.mine"))
	b.add("")
	b.add(mutedStyle.Render(i18n.T("tui.rules.undo")))

	hint := i18n.T("tui.rules.hint.browse") + m.skin.sep() +
		i18n.T("tui.rules.hint.config") + m.skin.sep() +
		i18n.T("tui.rules.hint.lang") + m.skin.sep() +
		i18n.T("tui.rules.hint.mine") + m.skin.sep() +
		i18n.T("tui.hint.back")
	return m.renderFrame(i18n.T("tui.rules.title"), *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: i18n.T("tui.rules.browse"), primary: true, action: actBrowseRules},
		buttonSpec{label: i18n.T("tui.rules.viewconfig"), action: actViewConfig},
		buttonSpec{label: i18n.T("tui.rules.language"), action: actLang},
		buttonSpec{label: i18n.T("tui.rules.mine"), action: actUserRules},
		buttonSpec{label: i18n.T("tui.button.back"), action: actBack},
	))
}

// ---- the browser of every rule ----

func (m *Model) updateRuleList(msg tea.Msg) (tea.Cmd, bool) {
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
	all := m.sortedRules()
	switch key.String() {
	case "esc", "q":
		m.screen = screenRules
	case "up", "k":
		if m.ruleCursor > 0 {
			m.ruleCursor--
		}
	case "down", "j":
		if m.ruleCursor < len(all)-1 {
			m.ruleCursor++
		}
	case "enter", "d":
		if m.ruleCursor < len(all) {
			m.detail = all[m.ruleCursor]
			m.pendingDelete = ""
			m.screen = screenRuleDetail
		}
	case "?":
		m.openHelp()
	}
	return nil, true
}

// sortedRules answers every rule of the set, by id.
func (m *Model) sortedRules() []*model.Rule {
	all := append([]*model.Rule(nil), m.rulesOf()...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all
}

func (m *Model) viewRuleList() string {
	all := m.sortedRules()
	b := &body{}
	b.add(titleStyle.Render(fmt.Sprintf(i18n.T("tui.rules.all"), len(all))))
	b.add(mutedStyle.Render(i18n.T("tui.rules.readonly")))
	b.add("")

	width := m.screenWidth()
	nameWidth := 34
	if nameWidth > width-56 {
		nameWidth = width - 56
	}
	if nameWidth < 18 {
		nameWidth = 18
	}
	room := m.screenHeight() - 12
	if room < 1 {
		room = 1
	}
	if m.ruleCursor < m.ruleOffset {
		m.ruleOffset = m.ruleCursor
	}
	if m.ruleCursor >= m.ruleOffset+room {
		m.ruleOffset = m.ruleCursor - room + 1
	}
	shown := all
	if m.ruleOffset < len(shown) {
		shown = shown[m.ruleOffset:]
	} else {
		shown = nil
	}
	if len(shown) > room {
		shown = shown[:room]
	}
	b.add(mutedStyle.Render(fmt.Sprintf("    %-*s %-28s %-7s %s", nameWidth,
		i18n.T("tui.rules.col.name"), i18n.T("tui.rules.col.profile"),
		i18n.T("tui.rules.col.risk"), i18n.T("tui.rules.col.source"))))
	for i, rule := range shown {
		index := m.ruleOffset + i
		pick := " "
		if index == m.ruleCursor {
			pick = m.skin.pointer()
		}
		name := rule.Name
		if name == "" {
			name = rule.ID
		}
		line := fmt.Sprintf("%s   %-*s %-28s %-7s %s",
			pick, nameWidth, fit(name, nameWidth), fit(rule.ID, 28),
			riskWord(rule.Risk), sourceOf(rule))
		if index == m.ruleCursor {
			line = selectedStyle.Render(line)
		}
		lineIndex := b.add(line)
		b.hit(lineIndex, 0, m.screenWidth()-8, actRuleOpen, fmt.Sprint(index))
	}
	if len(all) == 0 {
		b.add(mutedStyle.Render(i18n.T("tui.rules.empty")))
	}
	return m.renderFrame(i18n.T("tui.rules.browsestep"), *b, mutedStyle.Render(i18n.T("tui.rules.browsehint")+m.skin.sep()+i18n.T("tui.list.hint.details")+m.skin.sep()+i18n.T("tui.hint.back")), makeButtons(
		buttonSpec{label: i18n.T("tui.button.back"), action: actBack},
		buttonSpec{label: i18n.T("tui.profile.help"), action: actHelp},
	))
}

// ---- one rule, read-only ----

func (m *Model) updateRuleDetail(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "esc", "q":
		m.screen = screenRuleList
		m.pendingDelete = ""
	case "e":
		return m.openInNotepad(), true
	case "x":
		if m.detail != nil && !isBuiltinSource(m.detail.Source) {
			m.pendingDelete = m.detail.Source
		}
	case "?":
		m.openHelp()
	}
	return nil, true
}

// isBuiltinSource says whether a rule came with the binary.
func isBuiltinSource(source string) bool {
	return source == rules.SourceBuiltin || source == ""
}

func (m *Model) viewRuleDetail() string {
	rule := m.detail
	if rule == nil {
		return i18n.T("tui.rules.norule")
	}
	b := &body{}
	name := rule.Name
	if name == "" {
		name = rule.ID
	}
	b.add(titleStyle.Render(name))
	b.add(mutedStyle.Render(rule.ID + m.skin.sep() + sourceOf(rule)))
	if rule.Description != "" {
		for _, line := range wrapText(rule.Description, m.screenWidth()-8) {
			b.add(line)
		}
	}
	b.add("")
	enabled := i18n.T("tui.rules.disabled")
	if rule.DefaultEnabled {
		enabled = i18n.T("tui.rules.enabled")
	}
	b.add(fmt.Sprintf(i18n.T("tui.rules.risk"), riskWord(rule.Risk)) + m.skin.sep() + enabled)
	if rule.RequiresAdmin {
		b.add(warningStyle().Render(i18n.T("tui.detail.admin")))
	}
	if len(rule.RequiresClosed) != 0 {
		b.add(warningStyle().Render(i18n.T("tui.detail.closed") + strings.Join(rule.RequiresClosed, ", ")))
	}
	if len(rule.Tags) != 0 {
		b.add(mutedStyle.Render(i18n.T("tui.rules.tags") + strings.Join(rule.Tags, ", ")))
	}
	b.add("")
	b.add(titleStyle.Render(i18n.T("tui.rules.whatremoves")))
	for _, target := range rule.Targets {
		if target.Kind == model.KindCommand {
			b.add("  $ " + target.Cmd + " " + strings.Join(target.Args, " "))
			continue
		}
		line := fmt.Sprintf("  %s (%s", target.Path, modeWord(target.Mode))
		if target.MinAgeDays != 0 {
			line += fmt.Sprintf(i18n.T("tui.rules.olderthan"), target.MinAgeDays)
		}
		if len(target.Include) != 0 {
			line += i18n.T("tui.rules.only") + strings.Join(target.Include, " ")
		}
		if len(target.Exclude) != 0 {
			line += i18n.T("tui.rules.except") + strings.Join(target.Exclude, " ")
		}
		b.add(line + ")")
	}

	if m.pendingDelete != "" {
		b.add("")
		b.add(dangerStyle.Render(i18n.T("tui.rules.deletequestion")))
		return m.renderFrame(i18n.T("tui.rules.deletestep"), *b, mutedStyle.Render(i18n.T("tui.rules.deletehint")), makeButtons(
			buttonSpec{label: i18n.T("tui.rules.deleteyes"), primary: true, action: actDeleteYes},
			buttonSpec{label: i18n.T("tui.scan.cancel"), action: actBack},
		))
	}

	buttons := []buttonSpec{{label: i18n.T("tui.button.back"), primary: true, action: actBack}, {label: i18n.T("tui.profile.help"), action: actHelp}}
	if !isBuiltinSource(rule.Source) {
		buttons = append([]buttonSpec{
			{label: i18n.T("tui.rules.editnotepad"), action: actNotepad},
			{label: i18n.T("tui.rules.delete"), action: actDeleteRule},
		}, buttons...)
	}
	return m.renderFrame(i18n.T("tui.rules.step"), *b, mutedStyle.Render(i18n.T("tui.rules.readonlyhint")), makeButtons(buttons...))
}

// modeWord names a mode in plain words.
func modeWord(mode model.Mode) string {
	switch mode {
	case model.ModeSelf:
		return i18n.T("tui.rules.mode.item")
	case model.ModeFiles:
		return i18n.T("tui.rules.mode.files")
	default:
		return i18n.T("tui.rules.mode.contents")
	}
}

// ---- the effective config ----

func (m *Model) updateConfig(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "esc", "q":
		m.screen = screenRules
	case "up", "k":
		if m.configOffset > 0 {
			m.configOffset--
		}
	case "down", "j":
		m.configOffset++
	}
	return nil, true
}

// configLines answers the config as lines a person reads.
func (m *Model) configLines() []string {
	lines := []string{
		i18n.T("tui.rules.configfile") + m.configPath,
	}
	if _, err := os.Stat(m.configPath); err != nil {
		lines = append(lines, i18n.T("tui.rules.confignofile"))
	}
	if m.set == nil {
		return lines
	}
	config := m.set.Config
	allow := i18n.T("tui.rules.no")
	if config.AllowUserCommands {
		allow = i18n.T("tui.rules.yes")
	}
	lines = append(lines, i18n.T("tui.rules.allowcommands")+allow)
	if len(config.ExcludePaths) == 0 {
		lines = append(lines, i18n.T("tui.rules.excludenone"))
	} else {
		lines = append(lines, i18n.T("tui.rules.excludes"))
		for _, path := range config.ExcludePaths {
			lines = append(lines, "  "+path)
		}
	}
	if len(config.Rules) == 0 {
		lines = append(lines, i18n.T("tui.rules.switchnone"))
	} else {
		lines = append(lines, i18n.T("tui.rules.switches"))
		var ids []string
		for id := range config.Rules {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			state := i18n.T("tui.rules.off")
			if config.Rules[id].Enabled {
				state = i18n.T("tui.rules.on")
			}
			lines = append(lines, fmt.Sprintf("  %s: %s", id, state))
		}
	}
	lines = append(lines, "", i18n.T("tui.rules.profileshead"))
	var names []string
	for name := range m.set.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		profile := m.set.Profiles[name]
		chosen, _, err := rules.Select(m.set, name)
		count := 0
		if err == nil {
			count = len(chosen)
		}
		lines = append(lines, fmt.Sprintf(i18n.T("tui.rules.profilecount"), name, m.rulesCount(count)))
		if profile.DefaultOnly {
			lines = append(lines, i18n.T("tui.rules.takesevery"))
		}
		if profile.MaxRisk != "" {
			lines = append(lines, i18n.T("tui.rules.highestrisk")+string(profile.MaxRisk))
		}
		if len(profile.Include) != 0 {
			lines = append(lines, i18n.T("tui.rules.include")+strings.Join(profile.Include, ", "))
		}
		if len(profile.IncludeTags) != 0 {
			lines = append(lines, i18n.T("tui.rules.tagline")+strings.Join(profile.IncludeTags, ", "))
		}
		if len(profile.IncludeCategories) != 0 {
			lines = append(lines, i18n.T("tui.rules.cats")+strings.Join(profile.IncludeCategories, ", "))
		}
		if len(profile.Exclude) != 0 {
			lines = append(lines, i18n.T("tui.rules.exclude")+strings.Join(profile.Exclude, ", "))
		}
	}
	return lines
}

func (m *Model) viewConfig() string {
	b := &body{}
	b.add(titleStyle.Render(i18n.T("tui.rules.effective")))
	b.add(mutedStyle.Render(i18n.T("tui.rules.configreadonly")))
	b.add("")
	lines := m.configLines()
	room := m.screenHeight() - 10
	if room < 1 {
		room = 1
	}
	if m.configOffset > len(lines)-room {
		m.configOffset = len(lines) - room
	}
	if m.configOffset < 0 {
		m.configOffset = 0
	}
	shown := lines
	if m.configOffset < len(shown) {
		shown = shown[m.configOffset:]
	} else {
		shown = nil
	}
	if len(shown) > room {
		shown = shown[:room]
	}
	for _, line := range shown {
		b.add(line)
	}
	return m.renderFrame(i18n.T("tui.rules.configstep"), *b, mutedStyle.Render(i18n.T("tui.editor.scrollhint")+m.skin.sep()+i18n.T("tui.hint.back")), makeButtons(
		buttonSpec{label: i18n.T("tui.button.back"), action: actBack},
	))
}

// ---- the user's own rule files ----

// refreshUserFiles rereads the names in the user rule directory.
func (m *Model) refreshUserFiles() {
	m.userCursor = 0
	m.userOffset = 0
	entries, err := os.ReadDir(m.rulesDir)
	if err != nil {
		return
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".yaml") {
			continue
		}
		files = append(files, filepath.Join(m.rulesDir, entry.Name()))
	}
	sort.Strings(files)
	m.userFiles = files
}

func (m *Model) updateUserRules(msg tea.Msg) (tea.Cmd, bool) {
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
		m.screen = screenRules
	case "n":
		m.startNewRule()
	case "up", "k":
		if m.userCursor > 0 {
			m.userCursor--
		}
	case "down", "j":
		if m.userCursor < len(m.userFiles)-1 {
			m.userCursor++
		}
	case "enter", "e":
		if m.userCursor < len(m.userFiles) {
			return m.openUserFile(m.userFiles[m.userCursor]), true
		}
	case "x":
		if m.userCursor < len(m.userFiles) {
			m.pendingDelete = m.userFiles[m.userCursor]
			m.detail = m.ruleOfFile(m.userFiles[m.userCursor])
			m.screen = screenRuleDetail
		}
	case "?":
		m.openHelp()
	}
	return nil, true
}

// ruleOfFile answers the rule a file defines, when the set holds it.
func (m *Model) ruleOfFile(path string) *model.Rule {
	for _, rule := range m.rulesOf() {
		if rule.Source == path {
			return rule
		}
	}
	return nil
}

func (m *Model) viewUserRules() string {
	b := &body{}
	b.add(titleStyle.Render(i18n.T("tui.rules.mine")))
	b.add(mutedStyle.Render(m.rulesDir))
	b.add("")
	if len(m.userFiles) == 0 {
		b.add(i18n.T("tui.rules.userempty"))
		b.add(mutedStyle.Render(i18n.T("tui.rules.userhint")))
	}
	room := m.screenHeight() - 12
	if room < 1 {
		room = 1
	}
	if m.userCursor < m.userOffset {
		m.userOffset = m.userCursor
	}
	if m.userCursor >= m.userOffset+room {
		m.userOffset = m.userCursor - room + 1
	}
	shown := m.userFiles
	if m.userOffset < len(shown) {
		shown = shown[m.userOffset:]
	} else {
		shown = nil
	}
	if len(shown) > room {
		shown = shown[:room]
	}
	for i, path := range shown {
		index := m.userOffset + i
		pick := " "
		if index == m.userCursor {
			pick = m.skin.pointer()
		}
		rule := m.ruleOfFile(path)
		label := filepath.Base(path)
		extra := i18n.T("tui.rules.notloaded")
		if rule != nil {
			label = rule.Name
			if label == "" {
				label = rule.ID
			}
			extra = rule.ID + m.skin.sep() + riskWord(rule.Risk)
		}
		line := fmt.Sprintf("%s %-40s %s", pick, fit(label, 40), extra)
		if index == m.userCursor {
			line = selectedStyle.Render(line)
		}
		lineIndex := b.add(line)
		b.hit(lineIndex, 0, m.screenWidth()-8, actOpenUser, fmt.Sprint(index))
	}
	if m.message != "" {
		b.add("")
		b.add(successStyle.Render(m.message))
	}
	hint := i18n.T("tui.rules.hint.new") + m.skin.sep() + i18n.T("tui.editor.hint") + m.skin.sep() +
		i18n.T("tui.rules.hint.notepad") + m.skin.sep() + i18n.T("tui.rules.hint.delete") + m.skin.sep() +
		i18n.T("tui.hint.back")
	return m.renderFrame(i18n.T("tui.rules.minestep"), *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: i18n.T("tui.rules.newrule"), primary: true, action: actNewRule},
		buttonSpec{label: i18n.T("tui.editor.notepad"), action: actNotepad},
		buttonSpec{label: i18n.T("tui.button.back"), action: actBack},
	))
}
