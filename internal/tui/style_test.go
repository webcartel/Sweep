package tui

import (
	"strings"
	"testing"
)

type fakeEnv map[string]string

func (e fakeEnv) Get(key string) (string, bool) {
	value, ok := e[key]
	return value, ok
}

// The spec names the order of the ASCII say: the hand force of the
// environment first, then the flags, then what the terminal looks like.
func TestPickASCII(t *testing.T) {
	cases := []struct {
		name    string
		env     fakeEnv
		ascii   bool
		noASCII bool
		want    bool
		windows bool // the default the auto answer depends on
	}{
		{"SWEEP_ASCII forces on", fakeEnv{"SWEEP_ASCII": "1"}, false, false, true, true},
		{"SWEEP_ASCII wins over --no-ascii", fakeEnv{"SWEEP_ASCII": "1"}, false, true, true, true},
		{"--ascii forces on", fakeEnv{}, true, false, true, true},
		{"--no-ascii forces off", fakeEnv{}, false, true, false, true},
		{"Windows Terminal auto off", fakeEnv{"WT_SESSION": "x"}, false, false, false, true},
		{"conhost on Windows auto on", fakeEnv{}, false, false, true, true},
		{"other systems auto off", fakeEnv{}, false, false, false, false},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			got := pickASCII(one.env, one.ascii, one.noASCII, one.windows)
			if got != one.want {
				t.Fatalf("pickASCII = %v, want %v", got, one.want)
			}
		})
	}
}

// A checkbox reads the same in both skins: brackets the screen always draws.
func TestCheckbox(t *testing.T) {
	for _, skin := range []skin{newSkin(fakeEnv{}, false, true), newSkin(fakeEnv{}, true, true)} {
		if got := skin.checkbox(true); got != "[x]" {
			t.Fatalf("checkbox(on) = %q, want [x]", got)
		}
		if got := skin.checkbox(false); got != "[ ]" {
			t.Fatalf("checkbox(off) = %q, want [ ]", got)
		}
	}
}

// The bar fills by its portion: full answers the whole width, and the cells
// it did not reach stay of the skin's background.
func TestBar(t *testing.T) {
	skin := newSkin(fakeEnv{}, true, true) // ASCII: the cell of work is '#'
	bar := skin.bar(10, 0.4)
	if strings.Count(bar, "#") != 4 {
		t.Fatalf("bar(%d, 0.4) = %q, want 4 cells of work", 10, bar)
	}
	full := skin.bar(10, 1)
	if strings.Count(full, "#") != 10 {
		t.Fatalf("bar(10, 1) = %q, want a full bar", full)
	}
}
