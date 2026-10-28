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

// openPicker walks the settings screen to the picker.
func openPicker(t *testing.T) *Model {
	t.Helper()
	m, _ := langMachine(t)
	m = keyIn(m, "g", 0)
	m = keyIn(m, "l", 0)
	if m.screen != screenLang {
		t.Fatalf("the picker did not open: screen %d", m.screen)
	}
	return m
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

// The languages are rows of the page, not buttons of the footer: the
// footer holds the two answers, confirm and cancel.
func TestLanguagesAreRowsOfThePage(t *testing.T) {
	english(t)
	m := openPicker(t)

	view := m.View()
	for _, want := range []string{"English", "Русский", i18n.T("tui.lang.confirm"), i18n.T("tui.lang.cancel")} {
		if !strings.Contains(view, want) {
			t.Fatalf("the picker does not show %q: %q", want, view)
		}
	}
	// The language names are on their own lines, above the buttons.
	lines := strings.Split(view, "\n")
	englishLine, buttonLine := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "English") && englishLine < 0 {
			englishLine = i
		}
		if strings.Contains(line, i18n.T("tui.lang.confirm")) && buttonLine < 0 {
			buttonLine = i
		}
	}
	if englishLine < 0 || buttonLine < 0 || englishLine >= buttonLine {
		t.Fatalf("a language is not above the buttons (name %d, buttons %d)", englishLine, buttonLine)
	}
}

// Picking a language shows it at once, so the choice is judged by its
// result rather than by its name.
func TestPickingALanguagePreviewsIt(t *testing.T) {
	english(t)
	m := openPicker(t)

	m = clickAction(t, m, actLangRU)

	if got := i18n.Current(); got != i18n.LangRU {
		t.Fatalf("the language is %q, want the preview to apply it", got)
	}
	if view := m.View(); !strings.Contains(view, "Язык") {
		t.Fatalf("the picker did not redraw in Russian: %q", view)
	}
}

// Nothing is saved until the user confirms.
func TestConfirmSavesTheLanguage(t *testing.T) {
	english(t)
	m, path := langMachine(t)
	m = keyIn(m, "g", 0)
	m = keyIn(m, "l", 0)

	m = clickAction(t, m, actLangRU)
	if _, found, _ := settings.Read(path); found {
		t.Fatal("picking a language wrote the setting before the confirmation")
	}

	m = clickAction(t, m, actLangSave)

	if m.screen != screenRules {
		t.Fatalf("the confirmation left the screen at %d", m.screen)
	}
	name, found, err := settings.Read(path)
	if err != nil {
		t.Fatalf("read the setting back: %v", err)
	}
	if !found || name != "ru" {
		t.Fatalf("the setting is %q, %v; want ru, true", name, found)
	}
	if got := i18n.Current(); got != i18n.LangRU {
		t.Fatalf("the language is %q after the confirmation", got)
	}
}

// Cancel puts back the language the screen opened with and saves nothing.
func TestCancelRevertsTheLanguage(t *testing.T) {
	english(t)
	m, path := langMachine(t)
	m = keyIn(m, "g", 0)
	m = keyIn(m, "l", 0)

	m = clickAction(t, m, actLangRU)
	m = clickAction(t, m, actLangCancel)

	if got := i18n.Current(); got != i18n.LangEN {
		t.Fatalf("cancel left the language at %q", got)
	}
	if m.screen != screenRules {
		t.Fatalf("cancel left the screen at %d", m.screen)
	}
	if _, found, _ := settings.Read(path); found {
		t.Fatal("cancel wrote the setting")
	}
}

// The way back responds to the mouse: the picker's footer draws Cancel,
// and the generic Back action is the safety net behind it.
func TestBackRespondsToTheMouseOnThePicker(t *testing.T) {
	english(t)
	m := openPicker(t)

	m = clickAction(t, m, actLangCancel)
	if m.screen != screenRules {
		t.Fatalf("the cancel button left the screen at %d", m.screen)
	}

	// The same screen, reached again, answers the generic back action too:
	// the handler used to have no case for this screen at all.
	m = openPicker(t)
	_ = m.View()
	m.hits = append(m.hits, clickBox{x1: 1, x2: 4, y1: 1, y2: 1, action: actBack})
	next, _ := m.Update(tea.MouseMsg{
		X: 2, Y: 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	m = next.(*Model)
	if m.screen != screenRules {
		t.Fatalf("the generic back action left the screen at %d", m.screen)
	}
}

// Esc closes the picker without saving.
func TestEscapeLeavesThePicker(t *testing.T) {
	english(t)
	m := openPicker(t)

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

// The list of what will be cleaned has a way back to the start screen.
func TestListHasABackButton(t *testing.T) {
	m := chomped(t)

	m = clickAction(t, m, actBack)

	if m.screen != screenProfile {
		t.Fatalf("the back button left the screen at %d, want the start", m.screen)
	}
}

func TestListEscapeReturnsToTheStart(t *testing.T) {
	m := chomped(t)

	m = keyIn(m, "", tea.KeyEsc)

	if m.screen != screenProfile {
		t.Fatalf("Esc left the screen at %d, want the start", m.screen)
	}
}
