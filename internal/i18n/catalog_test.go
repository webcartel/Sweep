package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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

// keyUse is how the code asks for a text: i18n.T("some.key", ...).
var keyUse = regexp.MustCompile(`i18n\.T\("([^"]+)"`)

// A key the code asks for but no catalog holds would print itself on the
// screen. The catalogs are compared with each other elsewhere; this test
// compares them with the code.
func TestEveryKeyUsedInTheCodeIsInTheCatalog(t *testing.T) {
	root := filepath.Join("..", "..")
	used := map[string]string{}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".superpowers":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range keyUse.FindAllSubmatch(data, -1) {
			used[string(match[1])] = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the sources: %v", err)
	}
	if len(used) == 0 {
		t.Fatal("no i18n.T key was found in the sources")
	}

	for key, path := range used {
		if _, ok := catalogEN[key]; !ok {
			t.Errorf("%s asks for %q, which no catalog holds", path, key)
		}
	}
}
