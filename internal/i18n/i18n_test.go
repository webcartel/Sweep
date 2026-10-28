package i18n

import "testing"

// seed puts a key into one catalog for the length of a test, so that a
// lookup, a fallback or a format can be watched without the shared
// catalogs carrying test keys.
func seed(t *testing.T, key, en, ru string) {
	t.Helper()
	catalogEN[key] = en
	if ru != "" {
		catalogRU[key] = ru
	}
	t.Cleanup(func() {
		delete(catalogEN, key)
		delete(catalogRU, key)
	})
}

// restore puts the language back after a test that switched it.
func restore(t *testing.T) {
	t.Helper()
	old := Current()
	t.Cleanup(func() { Use(old) })
}

func TestDefaultIsEnglish(t *testing.T) {
	if got := Current(); got != LangEN {
		t.Fatalf("Current() = %q, want %q", got, LangEN)
	}
}

func TestUseSwitchesTheLanguage(t *testing.T) {
	restore(t)
	Use(LangRU)
	if got := Current(); got != LangRU {
		t.Fatalf("Current() = %q, want %q", got, LangRU)
	}
}

func TestTFormatsArguments(t *testing.T) {
	restore(t)
	seed(t, "test.format", "hello %s", "привет %s")

	Use(LangEN)
	if got := T("test.format", "world"); got != "hello world" {
		t.Errorf("T in English = %q", got)
	}
	Use(LangRU)
	if got := T("test.format", "мир"); got != "привет мир" {
		t.Errorf("T in Russian = %q", got)
	}
}

func TestMissingKeyFallsBackToEnglish(t *testing.T) {
	restore(t)
	catalogEN["test.only.en"] = "English only"
	t.Cleanup(func() { delete(catalogEN, "test.only.en") })

	Use(LangRU)
	if got := T("test.only.en"); got != "English only" {
		t.Fatalf("T = %q, want the English text", got)
	}
}

func TestKeyMissingInBothAnswersTheKey(t *testing.T) {
	restore(t)
	Use(LangRU)
	if got := T("test.nowhere"); got != "test.nowhere" {
		t.Fatalf("T = %q, want the key itself", got)
	}
}

func TestSupportedReadsNames(t *testing.T) {
	for name, want := range map[string]bool{"en": true, "ru": true, "de": false, "": false} {
		lang, ok := Supported(name)
		if ok != want {
			t.Errorf("Supported(%q) ok = %v, want %v", name, ok, want)
		}
		if ok && string(lang) != name {
			t.Errorf("Supported(%q) = %q", name, lang)
		}
	}
}

func TestUseIgnoresUnknownName(t *testing.T) {
	restore(t)
	Use(LangRU)
	Use(Lang("de"))
	if got := Current(); got != LangRU {
		t.Fatalf("Current() = %q, want the language to stand", got)
	}
}

func TestMissingListsKeysTheCatalogLacks(t *testing.T) {
	restore(t)
	catalogEN["test.absent.ru"] = "no Russian"
	t.Cleanup(func() { delete(catalogEN, "test.absent.ru") })

	found := false
	for _, key := range Missing(LangRU) {
		if key == "test.absent.ru" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Missing(LangRU) = %v, want test.absent.ru", Missing(LangRU))
	}
	if got := Missing(LangEN); len(got) != 0 {
		t.Fatalf("Missing(LangEN) = %v, want none", got)
	}
}
