package i18n

import (
	"regexp"
	"sort"
	"testing"
)

// keyShape is what a catalog key looks like: a dotted path in lower case,
// so that `tui.profile.start` and `registry.note.drive` read the same way.
var keyShape = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z0-9_]+)+$`)

// The English catalog is the source of truth. The Russian one must hold
// its translation of every key, so a new English line without its Russian
// twin fails here instead of reaching a Russian user in English.
func TestCatalogsHoldTheSameKeys(t *testing.T) {
	if missing := Missing(LangRU); len(missing) != 0 {
		sort.Strings(missing)
		t.Fatalf("the Russian catalog misses %d keys: %v", len(missing), missing)
	}
	for key := range catalogRU {
		if _, ok := catalogEN[key]; !ok {
			t.Errorf("the Russian catalog has %q, which English does not", key)
		}
	}
}

func TestNoCatalogEntryIsEmpty(t *testing.T) {
	for key, text := range catalogEN {
		if text == "" {
			t.Errorf("English %q is empty", key)
		}
	}
	for key, text := range catalogRU {
		if text == "" {
			t.Errorf("Russian %q is empty", key)
		}
	}
}

func TestCatalogKeysLookLikePaths(t *testing.T) {
	for key := range catalogEN {
		if !keyShape.MatchString(key) {
			t.Errorf("the key %q does not look like a dotted path", key)
		}
	}
}
