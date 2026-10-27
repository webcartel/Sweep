package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbletea"
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
	set        *rules.Set
	rulesDir   string
	configPath string
	reload     func() (*rules.Set, error)
	env        platform.Env

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
		return "built-in"
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
	case "?":
		m.openHelp()
	}
	return nil, true
}

func (m *Model) viewRules() string {
	b := &body{}
	b.add(titleStyle.Render("Rules & settings"))
	b.add(mutedStyle.Render("Rules decide what sweep looks for. Built-in rules stay read-only."))
	b.add("")

	builtin, own := 0, 0
	for _, rule := range m.rulesOf() {
		if rule.Source == rules.SourceBuiltin || rule.Source == "" {
			builtin++
			continue
		}
		own++
	}
	b.add(fmt.Sprintf("Rules:     %d built-in, %d of yours", builtin, own))
	b.add(fmt.Sprintf("Profiles:  %d available", len(m.profiles)))
	config := m.configPath
	state := "not found - defaults are used"
	if _, err := os.Stat(config); err == nil {
		state = "found"
	}
	b.add(fmt.Sprintf("Config:    %s (%s)", config, state))
	if m.rulesDir != "" {
		b.add(fmt.Sprintf("Your rules: %s", m.rulesDir))
	}
	if m.message != "" {
		b.add("")
		b.add(warningStyle().Render(m.message))
	}
	b.add("")
	b.add(mutedStyle.Render("What each screen holds:"))
	b.add("  Browse rules   - every rule, its paths and where it came from")
	b.add("  View config    - profiles, switches and excluded paths")
	b.add("  My rules       - your own files: new, edit, delete")
	b.add("")
	b.add(mutedStyle.Render("You can undo an edit: every save leaves a .bak next to the file."))

	hint := "B browse" + m.skin.sep() + "C config" + m.skin.sep() + "U my rules" + m.skin.sep() + "Esc back"
	return m.renderFrame("Rules", *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: "Browse rules", primary: true, action: actBrowseRules},
		buttonSpec{label: "View config", action: actViewConfig},
		buttonSpec{label: "My rules", action: actUserRules},
		buttonSpec{label: "Back", action: actBack},
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
	b.add(titleStyle.Render(fmt.Sprintf("All rules (%d)", len(all))))
	b.add(mutedStyle.Render("Built-in rules are read-only. Your files win over a built-in rule with the same id."))
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
	b.add(mutedStyle.Render(fmt.Sprintf("    %-*s %-28s %-7s %s", nameWidth, "NAME", "PROFILE ID", "RISK", "SOURCE")))
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
		b.add(mutedStyle.Render("No rules loaded."))
	}
	return m.renderFrame("Rules - browse", *b, mutedStyle.Render("Up/Down move"+m.skin.sep()+"Enter details"+m.skin.sep()+"Esc back"), makeButtons(
		buttonSpec{label: "Back", action: actBack},
		buttonSpec{label: "Help", action: actHelp},
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
		return "No rule is open."
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
	b.add(fmt.Sprintf("Risk: %s%s%s", riskWord(rule.Risk),
		m.skin.sep(), map[bool]string{true: "in Everyday cleanup", false: "off by default"}[rule.DefaultEnabled]))
	if rule.RequiresAdmin {
		b.add(warningStyle().Render("Needs administrator rights."))
	}
	if len(rule.RequiresClosed) != 0 {
		b.add(warningStyle().Render("Skipped while running: " + strings.Join(rule.RequiresClosed, ", ")))
	}
	if len(rule.Tags) != 0 {
		b.add(mutedStyle.Render("Tags: " + strings.Join(rule.Tags, ", ")))
	}
	b.add("")
	b.add(titleStyle.Render("What it removes"))
	for _, target := range rule.Targets {
		if target.Kind == model.KindCommand {
			b.add("  $ " + target.Cmd + " " + strings.Join(target.Args, " "))
			continue
		}
		line := fmt.Sprintf("  %s (%s", target.Path, modeWord(target.Mode))
		if target.MinAgeDays != 0 {
			line += fmt.Sprintf(", older than %d days", target.MinAgeDays)
		}
		if len(target.Include) != 0 {
			line += ", only " + strings.Join(target.Include, " ")
		}
		if len(target.Exclude) != 0 {
			line += ", except " + strings.Join(target.Exclude, " ")
		}
		b.add(line + ")")
	}

	if m.pendingDelete != "" {
		b.add("")
		b.add(dangerStyle.Render("Delete this file? A .bak copy stays next to it."))
		return m.renderFrame("Delete a rule", *b, mutedStyle.Render("This cannot be undone from the screen"), makeButtons(
			buttonSpec{label: "Yes, delete", primary: true, action: actDeleteYes},
			buttonSpec{label: "Cancel", action: actBack},
		))
	}

	buttons := []buttonSpec{{label: "Back", primary: true, action: actBack}, {label: "Help", action: actHelp}}
	if !isBuiltinSource(rule.Source) {
		buttons = append([]buttonSpec{
			{label: "Edit in Notepad", action: actNotepad},
			{label: "Delete", action: actDeleteRule},
		}, buttons...)
	}
	return m.renderFrame("Rule", *b, mutedStyle.Render("Read-only for built-in rules"), makeButtons(buttons...))
}

// modeWord names a mode in plain words.
func modeWord(mode model.Mode) string {
	switch mode {
	case model.ModeSelf:
		return "the item itself"
	case model.ModeFiles:
		return "matching files"
	default:
		return "the contents"
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
		"Config file: " + m.configPath,
	}
	if _, err := os.Stat(m.configPath); err != nil {
		lines = append(lines, "The file is not there: sweep uses its defaults.")
	}
	if m.set == nil {
		return lines
	}
	config := m.set.Config
	allow := "no"
	if config.AllowUserCommands {
		allow = "yes"
	}
	lines = append(lines, "Commands in user rules: "+allow)
	if len(config.ExcludePaths) == 0 {
		lines = append(lines, "Excluded paths: none")
	} else {
		lines = append(lines, "Excluded paths:")
		for _, path := range config.ExcludePaths {
			lines = append(lines, "  "+path)
		}
	}
	if len(config.Rules) == 0 {
		lines = append(lines, "Rule switches: none")
	} else {
		lines = append(lines, "Rule switches:")
		var ids []string
		for id := range config.Rules {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			state := "off"
			if config.Rules[id].Enabled {
				state = "on"
			}
			lines = append(lines, fmt.Sprintf("  %s: %s", id, state))
		}
	}
	lines = append(lines, "", "Profiles:")
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
		lines = append(lines, fmt.Sprintf("  %s: %d rules", name, count))
		if profile.DefaultOnly {
			lines = append(lines, "    takes every rule of Everyday cleanup")
		}
		if profile.MaxRisk != "" {
			lines = append(lines, "    highest risk: "+string(profile.MaxRisk))
		}
		if len(profile.Include) != 0 {
			lines = append(lines, "    include: "+strings.Join(profile.Include, ", "))
		}
		if len(profile.IncludeTags) != 0 {
			lines = append(lines, "    tags: "+strings.Join(profile.IncludeTags, ", "))
		}
		if len(profile.IncludeCategories) != 0 {
			lines = append(lines, "    categories: "+strings.Join(profile.IncludeCategories, ", "))
		}
		if len(profile.Exclude) != 0 {
			lines = append(lines, "    exclude: "+strings.Join(profile.Exclude, ", "))
		}
	}
	return lines
}

func (m *Model) viewConfig() string {
	b := &body{}
	b.add(titleStyle.Render("Effective config"))
	b.add(mutedStyle.Render("Read-only: sweep reads this file at start. Edit it in a text editor."))
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
	return m.renderFrame("Rules - config", *b, mutedStyle.Render("Up/Down scroll"+m.skin.sep()+"Esc back"), makeButtons(
		buttonSpec{label: "Back", action: actBack},
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
	b.add(titleStyle.Render("My rules"))
	b.add(mutedStyle.Render(m.rulesDir))
	b.add("")
	if len(m.userFiles) == 0 {
		b.add("You have no rules of your own yet.")
		b.add(mutedStyle.Render("Press N or the button below to make the first one."))
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
		extra := "not loaded: the file has an error"
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
	hint := "N new" + m.skin.sep() + "Enter edit" + m.skin.sep() + "E notepad" + m.skin.sep() + "X delete" + m.skin.sep() + "Esc back"
	return m.renderFrame("Rules - mine", *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: "New rule", primary: true, action: actNewRule},
		buttonSpec{label: "Notepad", action: actNotepad},
		buttonSpec{label: "Back", action: actBack},
	))
}
