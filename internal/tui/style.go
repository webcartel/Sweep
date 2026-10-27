package tui

import (
	"runtime"

	"github.com/webcartel/sweep/internal/platform"
)

// envMap stands for an environment the tests and the fallback of New answer
// through.
type envMap map[string]string

func (e envMap) Get(key string) (string, bool) {
	value, ok := e[key]
	return value, ok
}

// pickASCII answers whether the screens draw with the plain characters of
// ASCII. The environment says first (SWEEP_ASCII=1 over everything), then
// the flags --ascii and --no-ascii, and what is left is the terminal
// itself: Windows Terminal names itself in WT_SESSION and draws every
// character, while a conhost on a Windows without it falls short of the
// frames, so ASCII stands in. A terminal on another system never asks.
func pickASCII(env platform.Env, asciiFlag, noASCIIFlag, windows bool) bool {
	if value, ok := env.Get("SWEEP_ASCII"); ok && value == "1" {
		return true
	}
	switch {
	case asciiFlag:
		return true
	case noASCIIFlag:
		return false
	}
	_, advanced := env.Get("WT_SESSION")
	return windows && !advanced
}

// skin is the set of characters one skin of the screens draws with. The
// plain skin keeps to ASCII the way the spec's --ascii asks; the rich one
// draws a rounded frame, full bars and shaded pieces.
type skin struct {
	name     string
	plain    bool
	checkOn  string
	checkOff string
	barCell  string
	barBare  string
	tl, tr   string
	bl, br   string
	h, v     string
	teeL     string
	teeR     string
}

// newSkin puts a skin together. The flags and the environment that named
// ASCII answer once; the skins differ in nothing else.
func newSkin(env platform.Env, asciiFlag, noASCIIFlag bool) skin {
	if pickASCII(env, asciiFlag, noASCIIFlag, runtime.GOOS == "windows") {
		return skin{
			name: "Sweep", plain: true, checkOn: "[x]", checkOff: "[ ]",
			barCell: "#", barBare: "-",
			tl: "+", tr: "+", bl: "+", br: "+", h: "-", v: "|", teeL: "+", teeR: "+",
		}
	}
	return skin{
		name: "Sweep", checkOn: "[x]", checkOff: "[ ]",
		barCell: "█", barBare: "─",
		tl: "╭", tr: "╮", bl: "╰", br: "╯", h: "─", v: "│", teeL: "├", teeR: "┤",
	}
}

// pointer marks the row under the cursor.
func (s skin) pointer() string {
	if s.plain {
		return ">"
	}
	return "▸"
}

// sep separates short facts in one line.
func (s skin) sep() string {
	if s.plain {
		return " - "
	}
	return " · "
}

// chevron marks the way into the details of a row.
func (s skin) chevron() string {
	if s.plain {
		return ">"
	}
	return "›"
}

// bullet marks one item of a list.
func (s skin) bullet() string {
	if s.plain {
		return "*"
	}
	return "•"
}

// ellipsis cuts a long text.
func (s skin) ellipsis() string {
	if s.plain {
		return "..."
	}
	return "…"
}

// arrows names the up and down keys in the hints.
func (s skin) arrows() string {
	if s.plain {
		return "Up/Down"
	}
	return "↑↓"
}

// caret marks the place the next letter lands in.
func (s skin) caret() string {
	if s.plain {
		return "_"
	}
	return "▌"
}

// checkbox draws the mark of a chosen rule or item.
func (s skin) checkbox(on bool) string {
	if on {
		return s.checkOn
	}
	return s.checkOff
}

// bar fills width cells by portion; what the work did not reach stays of
// the bare filler.
func (s skin) bar(width int, portion float64) string {
	if width <= 0 {
		return ""
	}
	if portion < 0 {
		portion = 0
	}
	if portion > 1 {
		portion = 1
	}
	filled := int(portion*float64(width) + 0.5)
	var out []rune
	for range width {
		if filled > 0 {
			out = append(out, []rune(s.barCell)...)
			filled--
			continue
		}
		out = append(out, []rune(s.barBare)...)
	}
	return string(out)
}
