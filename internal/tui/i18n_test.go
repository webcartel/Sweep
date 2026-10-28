package tui

import (
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/i18n"
)

// useLang puts a language in charge for the length of a test.
func useLang(t *testing.T, lang i18n.Lang) {
	t.Helper()
	restore := i18n.Current()
	t.Cleanup(func() { i18n.Use(restore) })
	i18n.Use(lang)
}

// The start screen is the first thing a Russian user reads, so it has to
// be Russian the moment the language is.
func TestProfileScreenIsRussianUnderARussianInterface(t *testing.T) {
	useLang(t, i18n.LangEN)
	m := New(Deps{Profiles: testProfiles, AsciiFlag: true})

	view := m.View()
	for _, want := range []string{"Free space safely.", "Start:", "Rules & settings"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the English start screen does not say %q: %q", want, view)
		}
	}

	i18n.Use(i18n.LangRU)
	view = m.View()
	for _, want := range []string{"Безопасное освобождение места.", "Начать:", "Правила и настройки"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the Russian start screen does not say %q: %q", want, view)
		}
	}
}

// A count of rules is worded the way the language counts: Russian has
// three endings, and one of them for every number reads wrong.
func TestRuleCountIsWordedInRussian(t *testing.T) {
	useLang(t, i18n.LangRU)
	m := New(Deps{Profiles: testProfiles, AsciiFlag: true})

	for _, one := range []struct {
		n    int
		want string
	}{
		{1, "1 правило"}, {3, "3 правила"}, {5, "5 правил"},
		{11, "11 правил"}, {21, "21 правило"}, {102, "102 правила"},
	} {
		if got := m.rulesCount(one.n); got != one.want {
			t.Errorf("rulesCount(%d) = %q, want %q", one.n, got, one.want)
		}
	}

	i18n.Use(i18n.LangEN)
	if got := m.rulesCount(1); got != "1 rules" {
		t.Errorf("rulesCount(1) in English = %q, want the one form the catalog holds", got)
	}
}
