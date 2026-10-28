// Package i18n carries the languages sweep speaks. Every string that
// leaves the program is asked for by key, and the package answers it in
// the language this run picked.
package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Lang is a language sweep knows.
type Lang string

const (
	// LangEN is English: the default, and the language the machine
	// answers in when nothing says otherwise.
	LangEN Lang = "en"
	// LangRU is Russian.
	LangRU Lang = "ru"
)

// active is the language this run speaks. Every screen and every message
// reads it, so it lives behind an atomic rather than a mutex.
var active atomic.Pointer[Lang]

func init() { use(LangEN) }

// Use puts a language in charge. A name sweep does not know is ignored,
// so a bad value can never blank the interface.
func Use(lang Lang) {
	if _, ok := Supported(string(lang)); !ok {
		return
	}
	use(lang)
}

// use stores the language without asking whether it is known; Use is the
// one that checks.
func use(lang Lang) {
	stored := lang
	active.Store(&stored)
}

// Current answers the language this run speaks.
func Current() Lang {
	stored := active.Load()
	if stored == nil {
		return LangEN
	}
	return *stored
}

// T answers the text of a key in the language this run speaks. A key the
// language has not got falls back to English, and a key neither catalog
// holds answers the key itself: a hole in a translation has to show on
// the screen rather than hide there as a blank line.
func T(key string, args ...any) string {
	text, ok := lookup(Current(), key)
	if !ok {
		text, ok = lookup(LangEN, key)
	}
	if !ok {
		return key
	}
	if len(args) == 0 || !strings.Contains(text, "%") {
		return text
	}
	return fmt.Sprintf(text, args...)
}

// lookup answers the text of a key in one catalog.
func lookup(lang Lang, key string) (string, bool) {
	if lang == LangRU {
		text, ok := catalogRU[key]
		return text, ok
	}
	text, ok := catalogEN[key]
	return text, ok
}

// Supported says whether a language name is one sweep speaks.
func Supported(name string) (Lang, bool) {
	switch Lang(name) {
	case LangEN:
		return LangEN, true
	case LangRU:
		return LangRU, true
	}
	return "", false
}

// Missing answers the keys of the English catalog that lang has not got.
// The English catalog is the source of truth, so English itself never
// misses anything.
func Missing(lang Lang) []string {
	if lang == LangEN {
		return nil
	}
	var out []string
	for key := range catalogEN {
		if _, ok := lookup(lang, key); !ok {
			out = append(out, key)
		}
	}
	return out
}
