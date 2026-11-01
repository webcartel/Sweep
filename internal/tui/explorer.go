package tui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/charmbracelet/bubbletea"

	"github.com/webcartel/sweep/internal/explore"
	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/report"
)

// The screens of the disk overview.
const (
	screenExRoot = iota + 21
	screenExScan
	screenExTree
	screenExConfirm
	screenExDone
)

// exploreState is what the overview remembers between frames.
type exploreState struct {
	// exRoot is the path being measured; exHome is the fallback the field
	// shows while it is empty.
	exRoot string
	exHome string

	exTree    *explore.Node
	exTop     []*explore.Node
	exShowTop bool
	// exStack is the drilled path: the last node is the one on screen.
	exStack []*explore.Node

	exCursor int
	exOffset int

	// exTicked holds the paths the user asked to remove.
	exTicked map[string]bool

	exProgress explore.Progress
	exScanning bool
	exCancel   context.CancelFunc
	exEvents   <-chan tea.Msg
	exErrText  string
	exRemovals []explore.Removal
	// guard refuses a path the engine may not touch.
	guard *guard.Guard
}

// exploreProgress carries the running totals of a walk.
type exploreProgress struct{ progress explore.Progress }

// exploreDone carries what a walk found.
type exploreDone struct {
	tree *explore.Node
	top  []*explore.Node
	err  error
}

// exploreRemoved carries what the removal came to.
type exploreRemoved struct{ removals []explore.Removal }

// openExplorer starts the overview. The field opens empty on purpose: a field
// that already holds a path swallows what the user types after it, and the
// home folder is offered as the suggestion it is.
func (m *Model) openExplorer() {
	m.exHome = m.exploreProfile()
	m.exRoot = ""
	m.exTree = nil
	m.exTop = nil
	m.exStack = nil
	m.exTicked = map[string]bool{}
	m.exShowTop = false
	m.exCursor, m.exOffset = 0, 0
	m.exErrText = ""
	m.exRemovals = nil
	m.screen = screenExRoot
}

// normalizeRoot turns what a user types or pastes into the path the walk
// uses. Spaces and quotes around a path are not part of it, and a path that
// starts with one names nothing at all: the device spelling keeps the space,
// so the folder is not found.
func normalizeRoot(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Trim(text, `"`)
	return strings.TrimSpace(text)
}

// exploreProfile is the home folder of the machine's user.
func (m *Model) exploreProfile() string {
	if home, ok := m.env.Get("USERPROFILE"); ok && home != "" {
		return home
	}
	return ""
}

// exploreDrive is the root of the system drive.
func (m *Model) exploreDrive() string {
	if drive, ok := m.env.Get("SystemDrive"); ok && drive != "" {
		return strings.TrimSuffix(drive, `\`) + `\`
	}
	return `C:\`
}

// startExplore measures the chosen root in one command and reports progress
// through a channel, so the screen is not blank for a whole drive.
func (m *Model) startExplore() tea.Cmd {
	root := normalizeRoot(m.exRoot)
	if root == "" {
		root = m.exHome
	}
	m.exRoot = root
	if root == "" {
		m.exErrText = i18n.T("tui.explore.noroot")
		return nil
	}
	if m.fsys == nil {
		m.exErrText = i18n.T("tui.explore.nofs")
		return nil
	}
	m.exErrText = ""

	ctx, cancel := context.WithCancel(context.Background())
	m.exCancel = cancel
	m.exScanning = true
	m.exStack = nil
	m.exCursor, m.exOffset = 0, 0

	ch := make(chan tea.Msg, 32)
	m.exEvents = ch
	root, fsys := m.exRoot, m.fsys
	go func() {
		defer close(ch)
		result, err := explore.Scan(ctx, fsys, root, explore.Options{
			Depth: 5, TopFiles: 100,
			Progress: func(progress explore.Progress) {
				select {
				case ch <- exploreProgress{progress: progress}:
				default: // the screen is behind; dropping a tick is fine
				}
			},
		})
		ch <- exploreDone{tree: result.Root, top: result.Top, err: err}
	}()
	m.screen = screenExScan
	return waitExplore(ch)
}

// waitExplore reads the next message of a running walk.
func waitExplore(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// updateExRoot takes the keys of the root screen: the field takes text, Enter
// starts, Esc leaves.
func (m *Model) updateExRoot(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "esc", "q":
		m.screen = screenProfile
	case "enter":
		return m.startExplore(), true
	case "backspace":
		if runes := []rune(m.exRoot); len(runes) > 0 {
			m.exRoot = string(runes[:len(runes)-1])
		}
	case "ctrl+u":
		m.exRoot = ""
	case "ctrl+v":
		// A terminal in raw mode sends Ctrl+V as a key, not as the text:
		// the clipboard is read here and appended as if it were typed.
		if m.system != nil {
			if text, err := m.system.Clipboard(); err == nil {
				text = strings.ReplaceAll(strings.ReplaceAll(text, "\r", ""), "\n", "")
				m.exRoot += strings.TrimSpace(text)
			} else {
				m.exErrText = i18n.T("tui.explore.root.pastefailed")
			}
		}
	default:
		if runes := key.Runes; len(runes) != 0 {
			m.exRoot += string(runes)
		}
	}
	return nil, true
}

// updateExScan takes the progress of a walk and its end.
func (m *Model) updateExScan(msg tea.Msg) (tea.Cmd, bool) {
	switch one := msg.(type) {
	case exploreProgress:
		m.exProgress = one.progress
		return waitExplore(m.exEvents), true
	case exploreDone:
		m.exScanning = false
		m.exCancel = nil
		m.exEvents = nil
		if one.err != nil && one.tree == nil {
			if errors.Is(one.err, fs.ErrNotExist) {
				m.exErrText = i18n.T("tui.explore.notfound", m.exRoot)
			} else {
				m.exErrText = i18n.T("tui.explore.unreadable", m.exRoot)
			}
			m.screen = screenExRoot
			return nil, true
		}
		m.exTree = one.tree
		m.exTop = one.top
		m.exStack = []*explore.Node{one.tree}
		m.exShowTop = false
		m.exCursor, m.exOffset = 0, 0
		m.exErrText = ""
		m.screen = screenExTree
	case tea.KeyMsg:
		switch one.String() {
		case "esc", "q":
			if m.exCancel != nil {
				m.exCancel()
			}
			m.screen = screenProfile
		}
	}
	return nil, true
}

// exRows answers the rows of the tree screen: the largest files, or the
// children of the folder being looked at.
func (m *Model) exRows() []*explore.Node {
	if m.exShowTop {
		return m.exTop
	}
	if len(m.exStack) == 0 {
		return nil
	}
	return m.exStack[len(m.exStack)-1].Kids
}

// exHere is the folder on screen, or nil at the top of the largest files.
func (m *Model) exHere() *explore.Node {
	if m.exShowTop || len(m.exStack) == 0 {
		return nil
	}
	return m.exStack[len(m.exStack)-1]
}

// updateExTree moves, ticks and drills.
func (m *Model) updateExTree(msg tea.Msg) (tea.Cmd, bool) {
	if wheel, ok := msg.(tea.MouseMsg); ok {
		switch wheel.Button {
		case tea.MouseButtonWheelUp:
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case tea.MouseButtonWheelDown:
			msg = tea.KeyMsg{Type: tea.KeyDown}
		}
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	rows := m.exRows()
	switch key.String() {
	case "esc", "q":
		m.screen = screenProfile
	case "up", "k":
		if m.exCursor > 0 {
			m.exCursor--
		}
	case "down", "j":
		if m.exCursor < len(rows)-1 {
			m.exCursor++
		}
	case " ":
		m.toggleTick(m.exCursor)
	case "enter", "right", "l":
		m.drill(m.exCursor)
	case "backspace", "left", "h":
		m.goUp()
	case "i":
		m.invertTicks()
	case "t":
		m.exShowTop = !m.exShowTop
		m.exCursor, m.exOffset = 0, 0
	case "c":
		if len(m.exTicked) != 0 {
			m.screen = screenExConfirm
		}
	}
	return nil, true
}

// toggleTick flips one row of the current view.
func (m *Model) toggleTick(index int) {
	rows := m.exRows()
	if index < 0 || index >= len(rows) {
		return
	}
	path := rows[index].Path
	m.exTicked[path] = !m.exTicked[path]
	if !m.exTicked[path] {
		delete(m.exTicked, path)
	}
}

// invertTicks flips every row of the current view, which is the "choose what
// to keep" the overview offers: tick all, then take the keepers back.
func (m *Model) invertTicks() {
	for _, row := range m.exRows() {
		if m.exTicked[row.Path] {
			delete(m.exTicked, row.Path)
			continue
		}
		m.exTicked[row.Path] = true
	}
}

// clearTicks drops every mark.
func (m *Model) clearTicks() { m.exTicked = map[string]bool{} }

// drill enters a folder row.
func (m *Model) drill(index int) {
	rows := m.exRows()
	if m.exShowTop || index < 0 || index >= len(rows) {
		return
	}
	node := rows[index]
	if !node.IsDir || node.Link {
		return
	}
	m.exStack = append(m.exStack, node)
	m.exCursor, m.exOffset = 0, 0
}

// goUp leaves the folder on screen for its parent.
func (m *Model) goUp() {
	if m.exShowTop {
		m.exShowTop = false
		m.exCursor, m.exOffset = 0, 0
		return
	}
	if len(m.exStack) <= 1 {
		return
	}
	m.exStack = m.exStack[:len(m.exStack)-1]
	m.exCursor, m.exOffset = 0, 0
}

// exTicks answers what the user marked, as the removal takes it.
func (m *Model) exTicks() []explore.Tick {
	var ticks []explore.Tick
	for path := range m.exTicked {
		ticks = append(ticks, explore.Tick{Path: path, Size: m.exSizeOf(path)})
	}
	return ticks
}

// exSizeOf answers the measured size of a marked path.
func (m *Model) exSizeOf(path string) int64 {
	if size, ok := m.exSizes()[strings.ToLower(path)]; ok {
		return size
	}
	return 0
}

// exSizes flattens the measured tree into path -> size, for the marked paths
// and the confirmation total.
func (m *Model) exSizes() map[string]int64 {
	out := map[string]int64{}
	var walk func(node *explore.Node)
	walk = func(node *explore.Node) {
		if node == nil {
			return
		}
		out[strings.ToLower(node.Path)] = node.Size
		for _, kid := range node.Kids {
			walk(kid)
		}
	}
	walk(m.exTree)
	for _, one := range m.exTop {
		out[strings.ToLower(one.Path)] = one.Size
	}
	return out
}

// exTickedSize is the whole of what the marks cover.
func (m *Model) exTickedSize() int64 {
	var total int64
	for _, tick := range m.exTicks() {
		total += tick.Size
	}
	return total
}

// updateExConfirm takes the word on the removal.
func (m *Model) updateExConfirm(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "esc", "q":
		m.screen = screenExTree
	case "enter":
		return m.removeTicked(), true
	}
	return nil, true
}

// removeTicked sends the marks to the Recycle Bin.
func (m *Model) removeTicked() tea.Cmd {
	ticks := m.exTicks()
	guard := m.guard
	fsys := m.fsys
	m.screen = screenExDone
	return func() tea.Msg {
		removals := explore.Remove(context.Background(), fsys, ticks, explore.RemoveOptions{
			Guard: guard,
			Log: func(action, path, note string) {
				_ = action
				_ = path
				_ = note
			},
		})
		return exploreRemoved{removals: removals}
	}
}

// updateExDone takes what the removal came to.
func (m *Model) updateExDone(msg tea.Msg) (tea.Cmd, bool) {
	switch one := msg.(type) {
	case exploreRemoved:
		m.exRemovals = one.removals
		m.exTicked = map[string]bool{}
	case tea.KeyMsg:
		switch one.String() {
		case "esc", "q", "enter":
			m.screen = screenProfile
		}
	}
	return nil, true
}

// viewExRoot draws the field and the two suggestions.
func (m *Model) viewExRoot() string {
	b := &body{}
	b.add(titleStyle.Render(i18n.T("tui.explore.root.title")))
	b.add(mutedStyle.Render(i18n.T("tui.explore.root.intro")))
	b.add("")
	b.add(i18n.T("tui.explore.root.field"))
	if m.exRoot == "" {
		b.add(mutedStyle.Render("  " + m.exHome + m.skin.caret()))
	} else {
		b.add("  " + m.exRoot + m.skin.caret())
	}
	if m.exErrText != "" {
		b.add("")
		b.add(dangerStyle.Render(m.exErrText))
	}
	b.add("")
	b.add(mutedStyle.Render(i18n.T("tui.explore.root.hint")))

	return m.renderFrame(i18n.T("tui.explore.root.step"), *b,
		mutedStyle.Render(i18n.T("tui.explore.root.keys")), makeButtons(
			buttonSpec{label: i18n.T("tui.explore.root.start"), primary: true, action: actExStart},
			buttonSpec{label: i18n.T("tui.explore.root.profile"), action: actExRootProfile},
			buttonSpec{label: i18n.T("tui.explore.root.drive"), action: actExRootDrive},
			buttonSpec{label: i18n.T("tui.button.back"), action: actExBack},
		))
}

// viewExScan draws the walk at work.
func (m *Model) viewExScan() string {
	b := &body{}
	b.add(titleStyle.Render(i18n.T("tui.explore.scan.title")))
	b.add(mutedStyle.Render(m.exRoot))
	b.add("")
	b.add(fmt.Sprintf(i18n.T("tui.explore.scan.totals"),
		humanCount(int64(m.exProgress.Dirs)), humanCount(int64(m.exProgress.Files)),
		report.FormatBytes(m.exProgress.Bytes)))
	b.add("")
	b.add(mutedStyle.Render(i18n.T("tui.explore.scan.keepopen")))

	return m.renderFrame(i18n.T("tui.explore.scan.step"), *b,
		mutedStyle.Render(i18n.T("tui.explore.scan.cancel")), makeButtons(
			buttonSpec{label: i18n.T("tui.explore.scan.cancelbutton"), action: actExCancel},
		))
}

// viewExTree draws the folder on screen, or the largest files.
func (m *Model) viewExTree() string {
	b := &body{}
	here := m.exHere()
	if m.exShowTop {
		b.add(titleStyle.Render(i18n.T("tui.explore.tree.top.title")))
		b.add(mutedStyle.Render(i18n.T("tui.explore.tree.top.intro")))
	} else if here != nil {
		b.add(titleStyle.Render(fit(here.Path, m.screenWidth()-8)))
		b.add(mutedStyle.Render(fmt.Sprintf(i18n.T("tui.explore.tree.folder"),
			report.FormatBytes(here.Size), humanCount(int64(here.Files)))))
		if here.Denied != 0 {
			b.add(warningStyle().Render(fmt.Sprintf(i18n.T("tui.explore.tree.denied"), here.Denied)))
		}
	}
	b.add("")

	rows := m.exRows()
	if len(rows) == 0 {
		b.add(mutedStyle.Render(i18n.T("tui.explore.tree.empty")))
	}

	room := m.screenHeight() - 14
	if room < 2 {
		room = 2
	}
	if m.exCursor < m.exOffset {
		m.exOffset = m.exCursor
	}
	if m.exCursor >= m.exOffset+room {
		m.exOffset = m.exCursor - room + 1
	}
	shown := rows
	if m.exOffset < len(shown) {
		shown = shown[m.exOffset:]
	} else {
		shown = nil
	}
	if len(shown) > room {
		shown = shown[:room]
	}

	for i, row := range shown {
		index := m.exOffset + i
		pick := "  "
		if index == m.exCursor {
			pick = m.skin.pointer() + " "
		}
		mark := m.skin.checkbox(m.exTicked[row.Path])
		name := row.Name
		if row.Link {
			name += " " + i18n.T("tui.explore.tree.link")
		}
		line := fmt.Sprintf("%s%s %9s  %s", pick, mark, report.FormatBytes(row.Size), fit(name, m.screenWidth()-24))
		if index == m.exCursor {
			line = selectedStyle.Render(line)
		}
		lineIndex := b.add(line)
		b.hit(lineIndex, 0, m.screenWidth()-8, actExTick, fmt.Sprint(index))
	}

	b.add("")
	ticked := len(m.exTicked)
	if ticked == 0 {
		b.add(mutedStyle.Render(i18n.T("tui.explore.tree.none")))
	} else {
		b.add(fmt.Sprintf(i18n.T("tui.explore.tree.ticked"),
			ticked, report.FormatBytes(m.exTickedSize())))
	}

	up := i18n.T("tui.explore.tree.up")
	if m.exShowTop {
		up = i18n.T("tui.explore.tree.totree")
	}
	hint := i18n.T("tui.explore.tree.hint.move") + m.skin.sep() +
		i18n.T("tui.explore.tree.hint.tick") + m.skin.sep() +
		i18n.T("tui.explore.tree.hint.invert") + m.skin.sep() +
		i18n.T("tui.explore.tree.hint.enter") + m.skin.sep() +
		i18n.T("tui.hint.back")

	return m.renderFrame(i18n.T("tui.explore.tree.step"), *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: i18n.T("tui.explore.tree.remove"), primary: true, action: actExContinue},
		buttonSpec{label: i18n.T("tui.explore.tree.invert"), action: actExInvert},
		buttonSpec{label: i18n.T("tui.explore.tree.clear"), action: actExClear},
		buttonSpec{label: up, action: actExUp},
		buttonSpec{label: i18n.T("tui.explore.tree.files"), action: actExTop},
		buttonSpec{label: i18n.T("tui.button.back"), action: actExBack},
	))
}

// viewExConfirm restates what the removal will take.
func (m *Model) viewExConfirm() string {
	b := &body{}
	b.add(titleStyle.Render(fmt.Sprintf(i18n.T("tui.explore.confirm.title"), len(m.exTicked))))
	b.add(mutedStyle.Render(i18n.T("tui.explore.confirm.bin")))
	b.add("")
	b.add(fmt.Sprintf(i18n.T("tui.explore.confirm.total"), report.FormatBytes(m.exTickedSize())))

	var names []string
	for _, tick := range m.exTicks() {
		name := tick.Path
		if i := strings.LastIndexAny(name, `\/`); i >= 0 {
			name = name[i+1:]
		}
		names = append(names, name)
	}
	if len(names) > 5 {
		names = names[:5]
	}
	b.add("")
	for _, name := range names {
		b.add("  " + m.skin.bullet() + " " + name)
	}
	if m.exErrText != "" {
		b.add("")
		b.add(dangerStyle.Render(m.exErrText))
	}

	return m.renderFrame(i18n.T("tui.explore.confirm.step"), *b,
		mutedStyle.Render(i18n.T("tui.explore.confirm.keys")), makeButtons(
			buttonSpec{label: i18n.T("tui.explore.confirm.yes"), primary: true, action: actExClean},
			buttonSpec{label: i18n.T("tui.button.back"), action: actExBack},
		))
}

// viewExDone answers what the removal came to.
func (m *Model) viewExDone() string {
	b := &body{}
	var taken, refused, failed int
	for _, one := range m.exRemovals {
		switch one.Action {
		case "trashed":
			taken++
		case "refused":
			refused++
		default:
			failed++
		}
	}
	b.add(titleStyle.Render(fmt.Sprintf(i18n.T("tui.explore.done.title"), taken)))
	if refused != 0 {
		b.add(warningStyle().Render(fmt.Sprintf(i18n.T("tui.explore.done.refused"), refused)))
	}
	if failed != 0 {
		b.add(dangerStyle.Render(fmt.Sprintf(i18n.T("tui.explore.done.failed"), failed)))
	}
	b.add("")
	b.add(mutedStyle.Render(i18n.T("tui.explore.done.bin")))
	for _, one := range m.exRemovals {
		if one.Action == "trashed" {
			continue
		}
		b.add(mutedStyle.Render(fmt.Sprintf("%s: %s (%s)", one.Name, one.Action, one.Note)))
	}

	return m.renderFrame(i18n.T("tui.explore.done.step"), *b,
		mutedStyle.Render(i18n.T("tui.explore.done.keys")), makeButtons(
			buttonSpec{label: i18n.T("tui.explore.done.close"), primary: true, action: actExBack},
		))
}
