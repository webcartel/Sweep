package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
	"github.com/webcartel/sweep/internal/rules"
)

// rulesMachine builds a TUI over the real built-in set and a user rule
// directory of the test's own, with a reload that rereads both.
func rulesMachine(t *testing.T) (*Model, string, platform.Env) {
	t.Helper()
	dir := t.TempDir()
	env := fake.NewEnv()
	env.Set("APPDATA", `C:\Users\probe\AppData\Roaming`)
	env.Set("LOCALAPPDATA", `C:\Users\probe\AppData\Local`)
	env.Set("TEMP", `C:\Users\probe\AppData\Local\Temp`)
	env.Set("USERPROFILE", `C:\Users\probe`)
	env.Set("SystemRoot", `C:\Windows`)
	env.Set("ProgramData", `C:\ProgramData`)
	loader := rules.Loader{Env: env, UserDir: dir}
	set, err := loader.Load()
	if err != nil {
		t.Fatalf("load the built-in set: %v", err)
	}
	m := New(Deps{
		Profiles: testProfiles, AsciiFlag: true, NoASCIIFlag: false,
		RulesDir: dir, ConfigPath: filepath.Join(dir, "config.yaml"),
		Set: set, Env: env,
		Reload: func() (*rules.Set, error) { return loader.Load() },
	})
	return m, dir, env
}

// keyIn feeds one named key into the model.
func keyIn(m *Model, name string, keyType tea.KeyType) *Model {
	if keyType != 0 {
		next, _ := m.Update(tea.KeyMsg{Type: keyType})
		return next.(*Model)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)})
	return next.(*Model)
}

// The start screen opens the settings, and the settings name the sources.
func TestRulesScreensOpen(t *testing.T) {
	m, _, _ := rulesMachine(t)
	m = keyIn(m, "g", 0)
	if m.screen != screenRules {
		t.Fatalf("g left the screen at %d, want the settings", m.screen)
	}
	view := m.View()
	for _, want := range []string{"Rules & settings", "built-in", "Config:"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the settings screen does not say %q", want)
		}
	}
	m = keyIn(m, "b", 0)
	if m.screen != screenRuleList || !strings.Contains(m.View(), "browsers.chrome.cache") {
		t.Fatalf("the browser is not on the rules: %q", m.View())
	}
	m = keyIn(m, "", tea.KeyEsc)
	m = keyIn(m, "c", 0)
	if m.screen != screenConfig || !strings.Contains(m.View(), "Everyday") && !strings.Contains(m.View(), "default") {
		t.Fatalf("the config screen does not name the profiles: %q", m.View())
	}
}

// A new rule written through the form lands in the user directory, passes
// the same checks the CLI runs, and the set is reloaded.
func TestEditorSavesAUserRule(t *testing.T) {
	m, dir, _ := rulesMachine(t)
	m = keyIn(m, "g", 0)
	m = keyIn(m, "u", 0)
	m = keyIn(m, "n", 0)
	if m.screen != screenEditor {
		t.Fatalf("N left the screen at %d, want the form", m.screen)
	}
	ed := &m.editor
	ed.fields[0].value = "user.my-cache"
	ed.fields[1].value = "My app cache"
	ed.fields[8].value = `%LOCALAPPDATA%\MyApp\Cache`
	ed.fields[9].value = "contents"

	if cmd := m.previewEditor(); cmd != nil {
		t.Fatalf("the preview asked for work")
	}
	if m.screen != screenPreview {
		t.Fatalf("the preview left the screen at %d: %s", m.screen, ed.errText)
	}
	if !strings.Contains(m.editor.preview, "id: user.my-cache") {
		t.Fatalf("the preview holds no id: %q", m.editor.preview)
	}
	m.saveEditor()
	if ed.errText != "" {
		t.Fatalf("the save refused: %s", ed.errText)
	}
	path := filepath.Join(dir, "user.my-cache.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the rule file is not there: %v", err)
	}
	found := false
	for _, rule := range m.rulesOf() {
		if rule.ID == "user.my-cache" {
			found = true
			if rule.Name != "My app cache" {
				t.Errorf("the reloaded rule is named %q", rule.Name)
			}
		}
	}
	if !found {
		t.Fatal("the reloaded set does not hold the new rule")
	}
	if m.screen != screenUserRules {
		t.Fatalf("after the save the screen is %d, want my rules", m.screen)
	}
}

// A form the checks refuse writes no file.
func TestEditorRefusesAnInvalidRule(t *testing.T) {
	m, dir, _ := rulesMachine(t)
	m = keyIn(m, "g", 0)
	m = keyIn(m, "u", 0)
	m = keyIn(m, "n", 0)
	ed := &m.editor
	ed.fields[1].value = "" // no name
	ed.fields[8].value = `%LOCALAPPDATA%\MyApp\Cache`
	m.saveEditor()
	if ed.errText == "" {
		t.Fatal("the form saved a rule without a name")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("the refused save wrote %d files", len(entries))
	}
}

// Editing a user file keeps a .bak of the previous version.
func TestEditorEditsAndBacksUp(t *testing.T) {
	m, dir, _ := rulesMachine(t)
	source := "schema: 1\nrules:\n  - id: user.old\n    name: Old name\n    category: user\n    risk: safe\n    targets:\n      - path: '%LOCALAPPDATA%\\Old'\n"
	path := filepath.Join(dir, "user.old.yaml")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write the rule: %v", err)
	}
	if err := m.refreshRules(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	m.openUserFile(path)
	if m.screen != screenEditor {
		t.Fatalf("the file did not open in the form, screen %d, message %q", m.screen, m.message)
	}
	m.editor.fields[1].value = "New name"
	m.saveEditor()
	if m.editor.errText != "" {
		t.Fatalf("the save refused: %s", m.editor.errText)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || !strings.Contains(string(backup), "Old name") {
		t.Fatalf("the backup holds no old name: %v %q", err, backup)
	}
	saved, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(saved), "New name") {
		t.Fatalf("the saved file holds no new name: %v %q", err, saved)
	}
}

// Deleting a user rule leaves its .bak and takes the rule out of the set.
func TestDeleteUserRuleBacksUp(t *testing.T) {
	m, dir, _ := rulesMachine(t)
	path := filepath.Join(dir, "user.gone.yaml")
	source := "schema: 1\nrules:\n  - id: user.gone\n    name: Gone\n    category: user\n    risk: safe\n    targets:\n      - path: '%LOCALAPPDATA%\\Gone'\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := m.refreshRules(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	m.pendingDelete = path
	m.deleteUserRule()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the rule file survived: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("no backup stayed: %v", err)
	}
	for _, rule := range m.rulesOf() {
		if rule.ID == "user.gone" {
			t.Fatal("the deleted rule is still in the set")
		}
	}
}

// A built-in rule opens read-only: the detail screen offers no editing.
func TestBuiltinRuleIsReadOnly(t *testing.T) {
	m, _, _ := rulesMachine(t)
	m.screen = screenRuleList
	m.detail = m.sortedRules()[0]
	m.screen = screenRuleDetail
	view := m.View()
	if strings.Contains(view, "Edit in Notepad") || strings.Contains(view, "Delete") {
		t.Fatalf("a built-in rule offers editing: %q", view)
	}
	if !strings.Contains(view, "Read-only") {
		t.Fatalf("the detail does not say read-only: %q", view)
	}
}
