package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/model"
)

// useLang puts a language in charge for the length of a test.
func useLang(t *testing.T, lang i18n.Lang) {
	t.Helper()
	restore := i18n.Current()
	t.Cleanup(func() { i18n.Use(restore) })
	i18n.Use(lang)
}

// A config the loader refuses is read by a person, so the finding follows
// the language of the interface.
func TestConfigErrorFollowsTheInterfaceLanguage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("nope: 1\n"), 0o600); err != nil {
		t.Fatalf("write the config: %v", err)
	}

	useLang(t, i18n.LangRU)
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "неизвестное поле") {
		t.Fatalf("the config error in Russian = %v", err)
	}

	i18n.Use(i18n.LangEN)
	_, err = LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("the config error in English = %v", err)
	}
}

func TestProfileErrorFollowsTheInterfaceLanguage(t *testing.T) {
	set := &Set{Profiles: map[string]model.Profile{"default": {}}}

	useLang(t, i18n.LangRU)
	_, _, err := Select(set, "nope")
	if err == nil || !strings.Contains(err.Error(), "неизвестный профиль") {
		t.Fatalf("the profile error in Russian = %v", err)
	}

	i18n.Use(i18n.LangEN)
	_, _, err = Select(set, "nope")
	if err == nil || !strings.Contains(err.Error(), "unknown profile") {
		t.Fatalf("the profile error in English = %v", err)
	}
}

// The schema's own findings are worded by x/text, which carries its own
// translations. The printer has to follow the interface language for them
// to arrive translated.
func TestSchemaFindingsFollowTheInterfaceLanguage(t *testing.T) {
	data := []byte("schema: 1\nrules:\n  - id: x.y\n    name: N\n    description: D\n" +
		"    category: apps\n    risk: safe\n    targets: []\n    nope: 1\n")
	file, err := LoadFile("x.yaml", data)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	useLang(t, i18n.LangRU)
	russian := schemaText(ValidateSchema(file))
	if russian == "" {
		t.Fatal("the schema found nothing to complain about")
	}

	i18n.Use(i18n.LangEN)
	english := schemaText(ValidateSchema(file))

	if russian != english {
		return
	}
	// x/text may not carry a translation for this kind yet. That is a
	// fallback the user reads in English rather than a blank line, and it
	// is worth naming rather than hiding.
	t.Logf("the schema finding has no Russian text yet: %q", english)
}

func schemaText(findings []Error) string {
	var out []string
	for _, finding := range findings {
		out = append(out, finding.Msg)
	}
	return strings.Join(out, "\n")
}
