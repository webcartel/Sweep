package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// explorerModel is the overview over a fake machine with one big folder.
func explorerModel(t *testing.T) (*Model, *fake.FS) {
	t.Helper()
	home := `C:\Users\probe`
	fsys := fake.NewFS()
	fsys.Add(platform.Info{Path: home, IsDir: true})
	fsys.Add(platform.Info{Path: home + `\big`, IsDir: true})
	fsys.Add(platform.Info{Path: home + `\big\a.bin`, Size: 100})
	fsys.Add(platform.Info{Path: home + `\big\b.bin`, Size: 50})
	fsys.Add(platform.Info{Path: home + `\small`, IsDir: true})
	fsys.Add(platform.Info{Path: home + `\small\c.bin`, Size: 10})

	env := fake.NewEnv()
	env.Set("USERPROFILE", home)
	env.Set("SystemRoot", `C:\Windows`)
	env.Set("SystemDrive", `C:`)
	env.Set("APPDATA", home+`\AppData\Roaming`)
	env.Set("LOCALAPPDATA", home+`\AppData\Local`)
	env.Set("TEMP", home+`\AppData\Local\Temp`)

	g, err := guard.New(fsys, env, home, nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	m := New(Deps{Profiles: testProfiles, AsciiFlag: true,
		FS: fsys, Env: env, System: fake.NewSystem(), Guard: g})
	return m, fsys
}

// startScan opens the overview and runs the walk to its end.
func startScan(t *testing.T) (*Model, *fake.FS) {
	t.Helper()
	m, fsys := explorerModel(t)

	m = keyIn(m, "f", 0)
	if m.screen != screenExRoot {
		t.Fatalf("f left the screen at %d, want the root screen", m.screen)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(next.(*Model), cmd)
	if m.screen != screenExTree {
		t.Fatalf("the walk left the screen at %d, want the tree: %s", m.screen, m.exErrText)
	}
	return m, fsys
}

// The start screen offers the overview, and it opens on the home folder as a
// suggestion rather than as text already in the field.
func TestOverviewOpensOnTheHomeFolder(t *testing.T) {
	m, _ := explorerModel(t)

	m = keyIn(m, "f", 0)

	if m.screen != screenExRoot {
		t.Fatalf("screen = %d, want the root screen", m.screen)
	}
	if m.exRoot != "" {
		t.Fatalf("the field opens with %q, want it empty", m.exRoot)
	}
	if m.exHome != `C:\Users\probe` {
		t.Fatalf("home = %q, want the home folder as the suggestion", m.exHome)
	}
	view := m.View()
	if !strings.Contains(view, "What takes space?") || !strings.Contains(view, `C:\Users\probe`) {
		t.Fatalf("the root screen does not offer the home folder: %q", view)
	}
}

// What the user types is the whole path: the field does not carry a stale one
// in front of it.
func TestTypingIsTheWholePath(t *testing.T) {
	m, _ := explorerModel(t)
	m = keyIn(m, "f", 0)

	for _, r := range `C:\Windows\Temp` {
		m = keyIn(m, string(r), 0)
	}

	if m.exRoot != `C:\Windows\Temp` {
		t.Fatalf("typed %q, want the path alone", m.exRoot)
	}
}

// A path typed with a stray space or a quote around it still names the folder:
// the walk trims it and scans what the user meant.
func TestRootIsTrimmedBeforeTheWalk(t *testing.T) {
	m, _ := explorerModel(t)
	m = keyIn(m, "f", 0)

	for _, r := range " C:\\Users\\probe " {
		m = keyIn(m, string(r), 0)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(next.(*Model), cmd)

	if m.screen != screenExTree {
		t.Fatalf("the walk left the screen at %d: %s", m.screen, m.exErrText)
	}
	if m.exRoot != `C:\Users\probe` {
		t.Fatalf("root = %q, want the path without the spaces", m.exRoot)
	}
}

// A path that is not there says so, rather than blaming the folders inside it.
func TestMissingRootSaysItIsMissing(t *testing.T) {
	m, _ := explorerModel(t)
	m = keyIn(m, "f", 0)

	for _, r := range `C:\Users\probe\nope` {
		m = keyIn(m, string(r), 0)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(next.(*Model), cmd)

	if m.screen != screenExRoot {
		t.Fatalf("screen = %d, want the root screen", m.screen)
	}
	if !strings.Contains(m.exErrText, "does not exist") {
		t.Fatalf("error = %q, want it to say the path is missing", m.exErrText)
	}
}

// Ctrl+V reads the clipboard: a terminal in raw mode sends the key, not the
// text, so the app has to fetch it.
func TestPasteFromTheClipboard(t *testing.T) {
	m, _ := explorerModel(t)
	m = keyIn(m, "f", 0)

	sys := fake.NewSystem()
	sys.SetClipboard("C:\\Windows\\Temp\r\n")
	m.system = sys

	m = keyIn(m, "", tea.KeyCtrlV)

	if m.exRoot != `C:\Windows\Temp` {
		t.Fatalf("the paste left %q, want the path", m.exRoot)
	}
}

// An empty clipboard says so rather than leaving the user guessing why the
// field stayed empty.
func TestPasteOfAnEmptyClipboardSaysSo(t *testing.T) {
	m, _ := explorerModel(t)
	m = keyIn(m, "f", 0)
	m.system = fake.NewSystem()

	m = keyIn(m, "", tea.KeyCtrlV)

	if m.exRoot != "" {
		t.Fatalf("the field holds %q after an empty paste", m.exRoot)
	}
	if !strings.Contains(m.exErrText, "holds no text") {
		t.Fatalf("the screen says %q, want the empty clipboard notice", m.exErrText)
	}
}

// The walk lands on a tree, largest folder first, with the totals of the folder.
func TestOverviewShowsTheTree(t *testing.T) {
	m, _ := startScan(t)

	if m.exTree == nil || m.exTree.Size != 160 {
		t.Fatalf("tree = %+v, want 160 bytes", m.exTree)
	}
	rows := m.exRows()
	if len(rows) != 2 || rows[0].Name != "big" || rows[0].Size != 150 {
		t.Fatalf("rows = %+v, want big first", rows)
	}
	view := m.View()
	for _, want := range []string{"big", "small"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the tree hides %q: %q", want, view)
		}
	}
	// A fresh walk opens on the tree, not on the largest files.
	if m.exShowTop {
		t.Fatal("the walk opened on the largest files, want the tree")
	}
	if !strings.Contains(view, "big") || !strings.Contains(view, "Up/Down move") {
		t.Fatalf("the view is not the tree: %q", view)
	}
}

// Space marks a row, and the header says how much is marked.
func TestOverviewMarksARow(t *testing.T) {
	m, _ := startScan(t)

	m = keyIn(m, " ", 0)

	if len(m.exTicked) != 1 {
		t.Fatalf("marked %d rows, want one", len(m.exTicked))
	}
	if !strings.Contains(m.View(), "150") {
		t.Fatalf("the header does not show the marked size: %q", m.View())
	}
}

// I inverts the view, which is how a user says what to keep: mark all, take
// the keepers back.
func TestOverviewInvertsTheMarks(t *testing.T) {
	m, _ := startScan(t)

	m = keyIn(m, "i", 0)
	if len(m.exTicked) != 2 {
		t.Fatalf("invert marked %d rows, want the two on screen", len(m.exTicked))
	}
	m = keyIn(m, "i", 0)
	if len(m.exTicked) != 0 {
		t.Fatalf("a second invert left %d rows marked", len(m.exTicked))
	}
}

// Enter opens a folder and Backspace leaves it.
func TestOverviewDrillsInAndOut(t *testing.T) {
	m, _ := startScan(t)

	m = keyIn(m, "", tea.KeyEnter)
	if len(m.exStack) != 2 || m.exStack[1].Name != "big" {
		t.Fatalf("the walk did not enter big: %+v", m.exStack)
	}
	if got := len(m.exRows()); got != 2 {
		t.Fatalf("big holds %d rows, want its two files", got)
	}

	m = keyIn(m, "", tea.KeyBackspace)
	if len(m.exStack) != 1 {
		t.Fatalf("Backspace left the stack at %d", len(m.exStack))
	}
}

// The largest files are one key away, and the tree is one key back.
func TestOverviewShowsTheLargestFiles(t *testing.T) {
	m, _ := startScan(t)

	m = keyIn(m, "t", 0)
	if !m.exShowTop || len(m.exRows()) != 3 {
		t.Fatalf("the largest files are not on screen: %d rows", len(m.exRows()))
	}
	if m.exRows()[0].Name != "a.bin" {
		t.Fatalf("the largest file is %q, want a.bin", m.exRows()[0].Name)
	}

	m = keyIn(m, "t", 0)
	if m.exShowTop {
		t.Fatal("t did not return to the tree")
	}
}

// The confirmation sends the marks to the Recycle Bin.
func TestOverviewRemovesTheMarks(t *testing.T) {
	m, fsys := startScan(t)

	m = keyIn(m, " ", 0) // big
	m = keyIn(m, "c", 0)
	if m.screen != screenExConfirm {
		t.Fatalf("c left the screen at %d, want the confirmation", m.screen)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(next.(*Model), cmd)
	if m.screen != screenExDone {
		t.Fatalf("the removal left the screen at %d", m.screen)
	}
	if len(m.exRemovals) != 1 || m.exRemovals[0].Action != "trashed" {
		t.Fatalf("removals = %+v", m.exRemovals)
	}
	if _, err := fsys.Lstat(`C:\Users\probe\big`); err == nil {
		t.Fatal("the folder is still there")
	}
}
