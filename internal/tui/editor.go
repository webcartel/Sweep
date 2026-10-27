package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"go.yaml.in/yaml/v3"

	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/rules"
)

// editorField is one line of the form. Most fields hold text; a toggle holds
// "true"/"false" and a choice cycles through its list.
type editorField struct {
	label   string
	value   string
	choices []string
}

func (f editorField) toggle() bool { return len(f.choices) != 0 }

// editorState is the rule form: its fields, where the cursor is and the file
// the save goes to.
type editorState struct {
	fields    []editorField
	cursor    int
	offset    int
	editing   bool
	errText   string
	filePath  string
	preview   string
	previewAt int
}

// fieldNames answers the labels of the form, in order.
func editorLabels() []string {
	return []string{
		"Rule id", "Name", "Description", "Category", "Risk",
		"Everyday cleanup", "Needs administrator", "Tags",
		"Path", "Mode", "Older than (days)", "Only these masks", "Never these masks",
	}
}

// blankEditor builds the form of a new rule.
func blankEditor() editorState {
	labels := editorLabels()
	fields := make([]editorField, len(labels))
	for i, label := range labels {
		fields[i] = editorField{label: label}
	}
	fields[0].value = "user.my-rule"
	fields[3].value = "user"
	fields[4] = editorField{label: labels[4], value: "safe", choices: []string{"safe", "moderate", "dangerous"}}
	fields[5] = editorField{label: labels[5], value: "true", choices: []string{"true", "false"}}
	fields[6] = editorField{label: labels[6], value: "false", choices: []string{"false", "true"}}
	fields[9] = editorField{label: labels[9], value: "contents", choices: []string{"contents", "files", "self"}}
	fields[10].value = "0"
	return editorState{fields: fields}
}

// editorOfRule fills the form from a rule.
func editorOfRule(rule *model.Rule, path string) editorState {
	ed := blankEditor()
	ed.filePath = path
	target := model.Target{}
	for _, one := range rule.Targets {
		if one.Kind == model.KindPath {
			target = one
			break
		}
	}
	set := func(index int, value string) { ed.fields[index].value = value }
	set(0, rule.ID)
	set(1, rule.Name)
	set(2, rule.Description)
	set(3, rule.Category)
	set(4, string(rule.Risk))
	set(5, boolWord(rule.DefaultEnabled))
	set(6, boolWord(rule.RequiresAdmin))
	set(7, strings.Join(rule.Tags, ", "))
	set(8, target.Path)
	set(9, string(target.Mode))
	set(10, strconv.Itoa(target.MinAgeDays))
	set(11, strings.Join(target.Include, ", "))
	set(12, strings.Join(target.Exclude, ", "))
	return ed
}

// boolWord writes a flag the way the form reads it.
func boolWord(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// startNewRule opens the form on a blank rule.
func (m *Model) startNewRule() {
	if m.rulesDir == "" {
		m.message = "No rule directory is known, so there is nowhere to save."
		return
	}
	m.editor = blankEditor()
	m.message = ""
	m.screen = screenEditor
}

// openUserFile opens one of the user's files: the form when it holds one
// plain path rule, Notepad otherwise.
func (m *Model) openUserFile(path string) tea.Cmd {
	data, err := os.ReadFile(path)
	if err != nil {
		m.message = "Cannot read " + path + ": " + err.Error()
		return nil
	}
	file, err := rules.LoadFile(path, data)
	if err != nil {
		m.message = "Cannot read " + filepath.Base(path) + ": " + err.Error()
		return m.openNotepadAt(path)
	}
	if len(file.Rules) != 1 || !plainPathRule(file.Rules[0].Rule) {
		// Several rules or a command in one file: the form cannot hold it,
		// so the file opens as it is.
		return m.openNotepadAt(path)
	}
	m.editor = editorOfRule(file.Rules[0].Rule, path)
	m.message = ""
	m.screen = screenEditor
	return nil
}

// plainPathRule says whether the form can hold the rule whole.
func plainPathRule(rule *model.Rule) bool {
	if rule == nil {
		return false
	}
	for _, target := range rule.Targets {
		if target.Kind != model.KindPath {
			return false
		}
	}
	return true
}

// updateEditor, the part of the form.
func (m *Model) updateEditor(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	ed := &m.editor
	if ed.editing {
		switch key.String() {
		case "esc", "enter":
			ed.editing = false
		case "backspace":
			runes := []rune(ed.fields[ed.cursor].value)
			if len(runes) > 0 {
				ed.fields[ed.cursor].value = string(runes[:len(runes)-1])
			}
		default:
			if len(key.Runes) != 0 {
				ed.fields[ed.cursor].value += string(key.Runes)
			}
		}
		return nil, true
	}
	switch key.String() {
	case "esc", "q":
		m.screen = screenUserRules
	case "up", "k":
		if ed.cursor > 0 {
			ed.cursor--
		}
	case "down", "j":
		if ed.cursor < len(ed.fields)-1 {
			ed.cursor++
		}
	case " ":
		field := &ed.fields[ed.cursor]
		if field.toggle() {
			cycleField(field)
		}
	case "enter":
		field := &ed.fields[ed.cursor]
		if field.toggle() {
			cycleField(field)
			return nil, true
		}
		ed.editing = true
	case "ctrl+s":
		return m.saveEditor(), true
	case "p":
		return m.previewEditor(), true
	case "?":
		m.openHelp()
	}
	return nil, true
}

// cycleField moves a toggle or a choice to its next value.
func cycleField(field *editorField) {
	for i, choice := range field.choices {
		if choice == field.value && len(field.choices) > 0 {
			field.value = field.choices[(i+1)%len(field.choices)]
			return
		}
	}
	if len(field.choices) != 0 {
		field.value = field.choices[0]
	}
}

// field answers one field of the form by its label.
func (ed editorState) field(label string) string {
	for _, one := range ed.fields {
		if one.label == label {
			return one.value
		}
	}
	return ""
}

// viewEditor draws the form.
func (m *Model) viewEditor() string {
	ed := &m.editor
	b := &body{}
	title := "New rule"
	if ed.filePath != "" {
		title = "Edit " + filepath.Base(ed.filePath)
	}
	b.add(titleStyle.Render(title))
	b.add(mutedStyle.Render("Enter edits a value, Space flips a choice, P shows the YAML."))
	b.add("")

	room := m.screenHeight() - 13
	if room < 6 {
		room = 6
	}
	if ed.cursor < ed.offset {
		ed.offset = ed.cursor
	}
	if ed.cursor >= ed.offset+room {
		ed.offset = ed.cursor - room + 1
	}
	shown := ed.fields
	if ed.offset < len(shown) {
		shown = shown[ed.offset:]
	} else {
		shown = nil
	}
	if len(shown) > room {
		shown = shown[:room]
	}
	for i, field := range shown {
		index := ed.offset + i
		pick := " "
		if index == ed.cursor {
			pick = m.skin.pointer()
		}
		value := field.value
		if value == "" {
			value = mutedStyle.Render("(not set)")
		}
		if index == ed.cursor && ed.editing {
			value += m.skin.caret()
		}
		line := fmt.Sprintf("%s %-22s %s", pick, field.label, value)
		if index == ed.cursor {
			line = selectedStyle.Render(line)
		}
		lineIndex := b.add(line)
		b.hit(lineIndex, 0, m.screenWidth()-8, actField, strconv.Itoa(index))
	}
	if ed.errText != "" {
		b.add("")
		b.add(dangerStyle.Render(ed.errText))
	}
	hint := "Enter edit" + m.skin.sep() + "Space flip" + m.skin.sep() + "P preview" + m.skin.sep() + "Ctrl+S save" + m.skin.sep() + "Esc back"
	return m.renderFrame("Rules - form", *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: "Save", primary: true, action: actSaveRule},
		buttonSpec{label: "Preview YAML", action: actPreviewRule},
		buttonSpec{label: "Notepad", action: actNotepad},
		buttonSpec{label: "Cancel", action: actBack},
	))
}

// updatePreview, the part of the YAML preview.
func (m *Model) updatePreview(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	ed := &m.editor
	switch key.String() {
	case "esc", "q":
		m.screen = screenEditor
	case "up", "k":
		if ed.previewAt > 0 {
			ed.previewAt--
		}
	case "down", "j":
		ed.previewAt++
	case "enter", "ctrl+s":
		return m.saveEditor(), true
	}
	return nil, true
}

// viewPreview shows the YAML the save will write.
func (m *Model) viewPreview() string {
	ed := &m.editor
	b := &body{}
	b.add(titleStyle.Render("The rule as YAML"))
	b.add(mutedStyle.Render("This is the whole file sweep will read."))
	b.add("")
	lines := strings.Split(strings.TrimRight(ed.preview, "\n"), "\n")
	room := m.screenHeight() - 11
	if room < 1 {
		room = 1
	}
	if ed.previewAt > len(lines)-room {
		ed.previewAt = len(lines) - room
	}
	if ed.previewAt < 0 {
		ed.previewAt = 0
	}
	shown := lines
	if ed.previewAt < len(shown) {
		shown = shown[ed.previewAt:]
	} else {
		shown = nil
	}
	if len(shown) > room {
		shown = shown[:room]
	}
	for _, line := range shown {
		b.add(line)
	}
	if ed.errText != "" {
		b.add("")
		b.add(dangerStyle.Render(ed.errText))
	}
	return m.renderFrame("Rules - preview", *b, mutedStyle.Render("Enter save"+m.skin.sep()+"Up/Down scroll"+m.skin.sep()+"Esc back to the form"), makeButtons(
		buttonSpec{label: "Save", primary: true, action: actSaveRule},
		buttonSpec{label: "Back to the form", action: actPreviewBack},
	))
}

// previewEditor builds the YAML and opens the preview.
func (m *Model) previewEditor() tea.Cmd {
	ed := &m.editor
	data, err := m.editorYAML()
	if err != nil {
		ed.errText = err.Error()
		return nil
	}
	ed.preview = string(data)
	ed.errText = ""
	ed.previewAt = 0
	m.screen = screenPreview
	return nil
}

// editorYAML turns the form into the bytes of a rule file.
func (m *Model) editorYAML() ([]byte, error) {
	ed := &m.editor
	id := strings.TrimSpace(ed.field("Rule id"))
	if id == "" {
		return nil, fmt.Errorf("a rule needs an id")
	}
	if strings.ContainsAny(id, ` \/:*?"<>|`) {
		return nil, fmt.Errorf("the id cannot hold spaces or %s", `\/:*?"<>|`)
	}
	if strings.TrimSpace(ed.field("Name")) == "" {
		return nil, fmt.Errorf("a rule needs a name")
	}
	path := strings.TrimSpace(ed.field("Path"))
	if path == "" {
		return nil, fmt.Errorf("a rule needs a path")
	}
	age, err := strconv.Atoi(strings.TrimSpace(ed.field("Older than (days)")))
	if err != nil || age < 0 {
		return nil, fmt.Errorf("older than must be a number of days, zero or more")
	}
	target := yamlTarget{
		Path:       path,
		Mode:       ed.field("Mode"),
		Include:    splitMasks(ed.field("Only these masks")),
		Exclude:    splitMasks(ed.field("Never these masks")),
		MinAgeDays: age,
	}
	rule := yamlRule{
		ID:             id,
		Name:           strings.TrimSpace(ed.field("Name")),
		Description:    strings.TrimSpace(ed.field("Description")),
		Category:       strings.TrimSpace(ed.field("Category")),
		Risk:           ed.field("Risk"),
		DefaultEnabled: ed.field("Everyday cleanup") == "true",
		RequiresAdmin:  ed.field("Needs administrator") == "true",
		Tags:           splitMasks(ed.field("Tags")),
		Targets:        []yamlTarget{target},
	}
	return yaml.Marshal(yamlFile{Schema: 1, Rules: []yamlRule{rule}})
}

// splitMasks reads a comma-separated list of masks or tags.
func splitMasks(text string) []string {
	var out []string
	for _, piece := range strings.Split(text, ",") {
		if value := strings.TrimSpace(piece); value != "" {
			out = append(out, value)
		}
	}
	return out
}

// yamlFile, yamlRule and yamlTarget are the shape the editor writes.
type yamlFile struct {
	Schema int        `yaml:"schema"`
	Rules  []yamlRule `yaml:"rules"`
}

type yamlRule struct {
	ID             string       `yaml:"id"`
	Name           string       `yaml:"name"`
	Description    string       `yaml:"description,omitempty"`
	Category       string       `yaml:"category"`
	Risk           string       `yaml:"risk"`
	DefaultEnabled bool         `yaml:"default_enabled"`
	RequiresAdmin  bool         `yaml:"requires_admin,omitempty"`
	Tags           []string     `yaml:"tags,omitempty"`
	Targets        []yamlTarget `yaml:"targets"`
}

type yamlTarget struct {
	Path       string   `yaml:"path"`
	Mode       string   `yaml:"mode"`
	Include    []string `yaml:"include,omitempty"`
	Exclude    []string `yaml:"exclude,omitempty"`
	MinAgeDays int      `yaml:"min_age_days,omitempty"`
}

// saveEditor validates the form, writes the file and reloads the set.
func (m *Model) saveEditor() tea.Cmd {
	ed := &m.editor
	ed.errText = ""
	data, err := m.editorYAML()
	if err != nil {
		ed.errText = err.Error()
		return nil
	}
	if m.rulesDir == "" {
		ed.errText = "No rule directory is known, so there is nowhere to save."
		return nil
	}
	target := filepath.Join(m.rulesDir, strings.TrimSpace(ed.field("Rule id"))+".yaml")
	if other := m.otherFileWithID(target, strings.TrimSpace(ed.field("Rule id"))); other != "" {
		ed.errText = "Another file already defines this id: " + filepath.Base(other)
		return nil
	}
	file, err := rules.LoadFile(target, data)
	if err != nil {
		ed.errText = err.Error()
		return nil
	}
	findings := rules.ValidateSchema(file)
	semantic, _ := rules.ValidateSemantics(file, m.rulesEnv())
	findings = append(findings, semantic...)
	if len(findings) != 0 {
		ed.errText = findings[0].Error()
		return nil
	}
	if err := os.MkdirAll(m.rulesDir, 0o700); err != nil {
		ed.errText = "Cannot make the rule directory: " + err.Error()
		return nil
	}
	if _, err := os.Stat(target); err == nil {
		if old, readErr := os.ReadFile(target); readErr == nil {
			_ = os.WriteFile(target+".bak", old, 0o600)
		}
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		ed.errText = "Cannot write " + filepath.Base(target) + ": " + err.Error()
		return nil
	}
	if err := m.refreshRules(); err != nil {
		ed.errText = "The rule is saved, but the reload failed: " + err.Error()
		return nil
	}
	m.refreshUserFiles()
	m.message = "Saved " + filepath.Base(target) + "."
	m.screen = screenUserRules
	return nil
}

// otherFileWithID answers a user file other than target that defines id.
func (m *Model) otherFileWithID(target, id string) string {
	entries, err := os.ReadDir(m.rulesDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".yaml") {
			continue
		}
		path := filepath.Join(m.rulesDir, entry.Name())
		if path == target {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		file, err := rules.LoadFile(path, data)
		if err != nil {
			continue
		}
		for _, parsed := range file.Rules {
			if parsed.Rule.ID == id {
				return path
			}
		}
	}
	return ""
}

// rulesEnv answers the environment the semantic check reads.
func (m *Model) rulesEnv() platform.Env {
	if m.env == nil {
		return envMap{}
	}
	return m.env
}

// deleteUserRule backs the file up and removes it, then reloads.
func (m *Model) deleteUserRule() tea.Cmd {
	path := m.pendingDelete
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err == nil {
		_ = os.WriteFile(path+".bak", data, 0o600)
	}
	if err := os.Remove(path); err != nil {
		m.message = "Cannot delete " + filepath.Base(path) + ": " + err.Error()
		return nil
	}
	if err := m.refreshRules(); err != nil {
		m.message = "Deleted, but the reload failed: " + err.Error()
	} else {
		m.message = "Deleted " + filepath.Base(path) + ". A .bak copy stays."
	}
	m.pendingDelete = ""
	m.detail = nil
	m.refreshUserFiles()
	m.screen = screenUserRules
	return nil
}

// openInNotepad hands a user's file to Notepad and reloads when it closes.
func (m *Model) openInNotepad() tea.Cmd {
	path := ""
	switch {
	case m.detail != nil && !isBuiltinSource(m.detail.Source):
		path = m.detail.Source
	case m.screen == screenUserRules && m.userCursor < len(m.userFiles):
		path = m.userFiles[m.userCursor]
	case m.editor.filePath != "":
		path = m.editor.filePath
	}
	if path == "" {
		m.message = "Save the rule first: Notepad needs a file to open."
		return nil
	}
	return m.openNotepadAt(path)
}

// openNotepadAt runs Notepad on a file and comes back when it closes.
func (m *Model) openNotepadAt(path string) tea.Cmd {
	if runtime.GOOS != "windows" {
		m.message = "Notepad is only on Windows; the file is " + path
		return nil
	}
	cmd := exec.Command("notepad.exe", path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return notepadDone{err: err}
	})
}

// notepadDone says the editor of the file closed.
type notepadDone struct{ err error }

// afterNotepad reloads the set once Notepad closes.
func (m *Model) afterNotepad(msg notepadDone) tea.Cmd {
	if msg.err != nil {
		m.message = "Notepad: " + msg.err.Error()
		return nil
	}
	if err := m.refreshRules(); err != nil {
		m.message = "The reload failed: " + err.Error()
		return nil
	}
	m.refreshUserFiles()
	m.message = "Reloaded after the editor."
	return nil
}
