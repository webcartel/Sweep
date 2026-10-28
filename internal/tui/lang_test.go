package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/platform/fake"
	"github.com/webcartel/sweep/internal/rules"
	"github.com/webcartel/sweep/internal/settings"
)

// langMachine is the settings screens over the real built-in set, with a
// settings file and an elevated system of the test's own.
func langMachine(t *testing.T) (*Model, string) {
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
	settingsPath := filepath.Join(dir, "settings.yaml")
	m := New(Deps{
		Profiles: testProfiles, AsciiFlag: true, NoASCIIFlag: false,
		RulesDir: dir, ConfigPath: filepath.Join(dir, "config.yaml"),
		Set: set, Env: env, System: fake.NewSystem(),
		Reload:       func() (*rules.Set, error) { return loader.Load() },
		SettingsPath: settingsPath,
	})
	return m, settingsPath
}

// english puts the language back after a test that switched it.
func english(t *testing.T) {
	t.Helper()
	old := i18n.Current()
	t.Cleanup(func() { i18n.Use(old) })
}

// The settings screen is the way to the picker.
func TestLanguageMenuOpensThePicker(t *testing.T) {
	m, _ := langMachine(t)

	m = keyIn(m, "g", 0)
	if m.screen != screenRules {
		t.Fatalf("g left the screen at %d, want the settings", m.screen)
	}
	m = keyIn(m, "l", 0)
	if m.screen != screenLang {
		t.Fatalf("l left the screen at %d, want the picker", m.screen)
	}
}

// Choosing Russian changes the language at once: the very screen the user
// is on redraws in the language they picked.
func TestLanguageChoiceRendersInRussian(t *testing.T) {
	english(t)
	m, _ := langMachine(t)
	m = keyIn(m, "g", 0)
	m = keyIn(m, "l", 0)

	m = clickAction(t, m, actLangRU)

	if got := i18n.Current(); got != i18n.LangRU {
		t.Fatalf("the language is %q, want Russian", got)
	}
	if view := m.View(); !strings.Contains(view, "Язык") {
		t.Fatalf("the picker did not redraw in Russian: %q", view)
	}
}

// The choice is written where the next run reads it.
func TestLanguageChoiceWritesTheFile(t *testing.T) {
	english(t)
	m, path := langMachine(t)
	m = keyIn(m, "g", 0)
	m = keyIn(m, "l", 0)

	// The click mutates the model in place and writes the file; the
	// answer is not read here.
	clickAction(t, m, actLangRU)

	name, found, err := settings.Read(path)
	if err != nil {
		t.Fatalf("read the setting back: %v", err)
	}
	if !found || name != "ru" {
		t.Fatalf("the setting is %q, %v; want ru, true", name, found)
	}
}

// Language names always read in their own language, so a user who cannot
// read the screen can still find the way back.
func TestLanguageNamesStayInTheirOwnLanguage(t *testing.T) {
	english(t)
	m, _ := langMachine(t)
	m = keyIn(m, "g", 0)
	m = keyIn(m, "l", 0)

	view := m.View()
	for _, want := range []string{"English", "Русский"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the picker does not offer %q: %q", want, view)
		}
	}

	m = clickAction(t, m, actLangRU)
	view = m.View()
	for _, want := range []string{"English", "Русский"} {
		if !strings.Contains(view, want) {
			t.Fatalf("after switching, the picker hides %q: %q", want, view)
		}
	}
}

func TestBackLeavesThePicker(t *testing.T) {
	m, _ := langMachine(t)
	m = keyIn(m, "g", 0)
	m = keyIn(m, "l", 0)

	m = keyIn(m, "", tea.KeyEsc)

	if m.screen != screenRules {
		t.Fatalf("Esc left the screen at %d, want the settings", m.screen)
	}
}

// The picker is reachable by mouse as well as by the keyboard.
func TestPickerIsClickable(t *testing.T) {
	english(t)
	m, _ := langMachine(t)
	m = keyIn(m, "g", 0)

	m = clickAction(t, m, actLang)
	if m.screen != screenLang {
		t.Fatalf("the click left the screen at %d, want the picker", m.screen)
	}
}
