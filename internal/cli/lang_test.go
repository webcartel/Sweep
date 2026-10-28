package cli

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/platform/fake"
	"github.com/webcartel/sweep/internal/settings"
)

// langMachine is a deps whose APPDATA is a directory of its own and whose
// system reports the language it is given.
func langMachine(t *testing.T, systemLang string) (deps, string) {
	t.Helper()
	dir := t.TempDir()
	env := fake.NewEnv()
	env.Set("APPDATA", dir)
	sys := fake.NewSystem()
	sys.SetUILanguage(systemLang)
	return deps{Env: env, System: sys}, dir
}

// saveLang writes a language where langEarly reads it: the settings file
// of the machine's own data directory.
func saveLang(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, "sweep", "settings.yaml")
	if err := settings.Write(path, name); err != nil {
		t.Fatalf("save the setting: %v", err)
	}
}

func TestLangFlagWinsOverTheSetting(t *testing.T) {
	d, dir := langMachine(t, "en")
	saveLang(t, dir, "en")

	lang, _, err := langEarly(d, []string{"--lang", "ru"})
	if err != nil {
		t.Fatalf("langEarly: %v", err)
	}
	if lang != i18n.LangRU {
		t.Fatalf("lang = %q, want the flag to win", lang)
	}
}

func TestSettingWinsOverTheSystem(t *testing.T) {
	d, dir := langMachine(t, "en")
	saveLang(t, dir, "ru")

	lang, _, err := langEarly(d, nil)
	if err != nil {
		t.Fatalf("langEarly: %v", err)
	}
	if lang != i18n.LangRU {
		t.Fatalf("lang = %q, want the setting to win over the system", lang)
	}
}

func TestSystemWinsOverEnglish(t *testing.T) {
	d, _ := langMachine(t, "ru")

	lang, _, err := langEarly(d, nil)
	if err != nil {
		t.Fatalf("langEarly: %v", err)
	}
	if lang != i18n.LangRU {
		t.Fatalf("lang = %q, want the system language", lang)
	}
}

func TestNothingSaysEnglish(t *testing.T) {
	d, _ := langMachine(t, "en")

	lang, _, err := langEarly(d, nil)
	if err != nil {
		t.Fatalf("langEarly: %v", err)
	}
	if lang != i18n.LangEN {
		t.Fatalf("lang = %q, want English as the fallback", lang)
	}
}

// A language sweep does not speak is a mistake to say, not a language to
// fall back on: the user asked for something and got silence instead.
func TestUnknownLangIsAUsageError(t *testing.T) {
	d, _ := langMachine(t, "en")

	_, _, err := langEarly(d, []string{"--lang", "de"})
	if err == nil {
		t.Fatal("langEarly accepted a language sweep does not speak")
	}
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitUsage {
		t.Fatalf("err = %v, want a usage error with code %d", err, ExitUsage)
	}
}

// The machine answer has to keep the same shape whatever the interface
// says, so a run carrying --json speaks English end to end.
func TestJSONRunSpeaksEnglish(t *testing.T) {
	d, _ := langMachine(t, "ru")

	lang, _, err := langEarly(d, []string{"scan", "--json"})
	if err != nil {
		t.Fatalf("langEarly: %v", err)
	}
	if lang != i18n.LangEN {
		t.Fatalf("lang = %q, want English for a --json run", lang)
	}
}

func TestLangEqualsSpellingAlsoWorks(t *testing.T) {
	d, _ := langMachine(t, "en")

	lang, _, err := langEarly(d, []string{"--lang=ru"})
	if err != nil {
		t.Fatalf("langEarly: %v", err)
	}
	if lang != i18n.LangRU {
		t.Fatalf("lang = %q, want the --lang=value spelling to count", lang)
	}
}

func TestSettingsPathSitsNextToTheConfig(t *testing.T) {
	d, dir := langMachine(t, "en")

	_, path, err := langEarly(d, []string{"--config", filepath.Join("D:", "x", "config.yaml")})
	if err != nil {
		t.Fatalf("langEarly: %v", err)
	}
	if want := filepath.Join("D:", "x", "settings.yaml"); path != want {
		t.Fatalf("settings path = %q, want %q", path, want)
	}

	_, path, err = langEarly(d, nil)
	if err != nil {
		t.Fatalf("langEarly: %v", err)
	}
	if want := filepath.Join(dir, "sweep", "settings.yaml"); path != want {
		t.Fatalf("settings path = %q, want %q next to the default config", path, want)
	}
}
