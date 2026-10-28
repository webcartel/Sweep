package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/model"
)

// The look of the screens. The TUI speaks to people who have never cleaned a
// disk by hand, so every screen keeps one promise: a plain title, the one
// number that matters, and buttons that say what they do.
var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	bigStyle     = lipgloss.NewStyle().Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	primaryStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	// selectedStyle marks the row under the cursor without color, so the
	// highlight survives the ASCII skin.
	selectedStyle = lipgloss.NewStyle().Bold(true)
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
)

// clickBox is one place of the last drawn screen the mouse can press: a row
// or a button. The screen draws itself and remembers the boxes; the next
// press looks itself up here.
type clickBox struct {
	x1, y1, x2, y2 int
	action         string
	arg            string
}

// The actions a press can carry.
const (
	actStart       = "start"
	actPickProfile = "pick-profile"
	actHelp        = "help"
	actQuit        = "quit"
	actCloseHelp   = "close-help"
	actCancelScan  = "cancel-scan"
	actToggleRule  = "toggle-rule"
	actOpenDetail  = "open-detail"
	actSelectSafe  = "select-safe"
	actSelectNone  = "select-none"
	actToggleBin   = "toggle-bin"
	actConfirm     = "confirm"
	actClean       = "clean"
	actBack        = "back"
	actRestart     = "restart"
	actStop        = "stop"
	actSaveReport  = "save-report"
	actToggleItem  = "toggle-item"
	actRow         = "row"
	actShowEmpty   = "show-empty"
	actRules       = "rules"
	actBrowseRules = "browse-rules"
	actViewConfig  = "view-config"
	actUserRules   = "user-rules"
	actRuleOpen    = "rule-open"
	actOpenUser    = "open-user-file"
	actNewRule     = "new-rule"
	actSaveRule    = "save-rule"
	actPreviewRule = "preview-rule"
	actNotepad     = "notepad"
	actDeleteRule  = "delete-rule"
	actDeleteYes   = "delete-yes"
	actField       = "field"
	actPreviewBack = "preview-back"
	actRegOpen     = "registry-open"
	actRegToggle   = "registry-toggle"
	actRegContinue = "registry-continue"
	actRegClean    = "registry-clean"
	actRegRestart  = "registry-restart"
	actRegBack     = "registry-back"
	actLang        = "lang-open"
	actLangEN      = "lang-en"
	actLangRU      = "lang-ru"
)

// hitAt answers the box under a press.
func (m *Model) hitAt(x, y int) (clickBox, bool) {
	for _, box := range m.hits {
		if x >= box.x1 && x <= box.x2 && y >= box.y1 && y <= box.y2 {
			return box, true
		}
	}
	return clickBox{}, false
}

// handleClick runs the action under the mouse and answers with a command when
// the action starts work, and with the model otherwise.
func (m *Model) handleClick(msg tea.MouseMsg) (tea.Cmd, bool) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil, false
	}
	box, ok := m.hitAt(msg.X, msg.Y)
	if !ok {
		return nil, false
	}
	switch box.action {
	case actPickProfile:
		m.cursor = atoiOrZero(box.arg)
		return nil, true
	case actStart:
		if m.screen == screenProfile {
			m.screen = screenScanning
			return m.startScan(), true
		}
		return nil, true
	case actHelp:
		m.openHelp()
		return nil, true
	case actCloseHelp:
		m.screen = m.screenBehind
		return nil, true
	case actQuit:
		return tea.Quit, true
	case actCancelScan:
		if m.cancelScan != nil {
			m.cancelScan()
		}
		return tea.Quit, true
	case actToggleRule:
		m.selection.Rules[box.arg] = !m.selection.Rules[box.arg]
		m.notice = ""
		return nil, true
	case actOpenDetail:
		m.row = atoiOrZero(box.arg)
		return m.openDetail()
	case actSelectSafe:
		m.chooseSafeOnly()
		return nil, true
	case actSelectNone:
		m.chooseNone()
		return nil, true
	case actToggleBin:
		m.trash = !m.trash
		return nil, true
	case actConfirm:
		m.screen = screenConfirm
		m.confirmText = ""
		return nil, true
	case actClean:
		if m.pendingWord() {
			return nil, true
		}
		return m.confirmEnter(), true
	case actBack:
		switch m.screen {
		case screenDetail, screenConfirm:
			m.screen = screenList
			m.confirmText = ""
		case screenHelp:
			m.screen = m.screenBehind
		case screenRules:
			m.screen = screenProfile
		case screenRuleList, screenConfig, screenUserRules:
			m.screen = screenRules
		case screenRuleDetail:
			if m.detail != nil && !isBuiltinSource(m.detail.Source) {
				m.screen = screenUserRules
			} else {
				m.screen = screenRuleList
			}
			m.pendingDelete = ""
		case screenEditor, screenPreview:
			m.screen = screenUserRules
		}
		return nil, true
	case actRestart:
		return m.confirmEnter(), true
	case actStop:
		if m.run.cancelExec != nil {
			m.run.cancelExec()
			m.run.cancelExec = nil
		}
		m.run.execEvents = nil
		m.run.execCmd = nil
		m.closeLog()
		m.screen = screenConfirm
		return nil, true
	case actSaveReport:
		m.saveNoteReport()
		return nil, true
	case actToggleItem:
		m.selection.Exclude[box.arg] = !m.selection.Exclude[box.arg]
		return nil, true
	case actShowEmpty:
		m.showEmpty = !m.showEmpty
		m.row = 0
		m.offset = 0
		return nil, true
	case actRules:
		m.openRules()
		return nil, true
	case actBrowseRules:
		m.ruleCursor, m.ruleOffset = 0, 0
		m.screen = screenRuleList
		return nil, true
	case actViewConfig:
		m.configOffset = 0
		m.screen = screenConfig
		return nil, true
	case actUserRules:
		m.refreshUserFiles()
		m.screen = screenUserRules
		return nil, true
	case actRuleOpen:
		all := m.sortedRules()
		index := atoiOrZero(box.arg)
		if index < len(all) {
			m.detail = all[index]
			m.screen = screenRuleDetail
		}
		return nil, true
	case actOpenUser:
		index := atoiOrZero(box.arg)
		if index < len(m.userFiles) {
			return m.openUserFile(m.userFiles[index]), true
		}
		return nil, true
	case actNewRule:
		m.startNewRule()
		return nil, true
	case actField:
		m.editor.cursor = atoiOrZero(box.arg)
		m.editor.editing = false
		return nil, true
	case actSaveRule:
		return m.saveEditor(), true
	case actPreviewRule:
		return m.previewEditor(), true
	case actPreviewBack:
		m.screen = screenEditor
		return nil, true
	case actLang:
		m.screen = screenLang
		return nil, true
	case actLangEN:
		return m.setLanguage(i18n.LangEN), true
	case actLangRU:
		return m.setLanguage(i18n.LangRU), true
	case actRegOpen:
		return m.openRegistry(), true
	case actRegToggle:
		m.toggleRegistryFinding(atoiOrZero(box.arg))
		return nil, true
	case actRegContinue:
		if m.selectedCount() != 0 {
			m.screen = screenRegConfirm
		}
		return nil, true
	case actRegClean:
		return m.startRegistryClean(), true
	case actRegRestart:
		return m.restartRegistry(), true
	case actRegBack:
		switch m.screen {
		case screenRegConfirm:
			m.screen = screenRegReview
		case screenRegReview, screenRegScan, screenRegDone:
			m.screen = screenProfile
		}
		return nil, true
	case actNotepad:
		return m.openInNotepad(), true
	case actDeleteRule:
		if m.detail != nil && !isBuiltinSource(m.detail.Source) {
			m.pendingDelete = m.detail.Source
		}
		return nil, true
	case actDeleteYes:
		return m.deleteUserRule(), true
	}
	return nil, false
}

// atoiOrZero reads the small numbers the boxes carry as arguments.
func atoiOrZero(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// body is what one screen wants inside the frame: the lines, and the boxes of
// the rows and buttons it drew. Y of a box counts from the first line of the
// body; frame adds its own lines on top.
type body struct {
	lines []string
	boxes []clickBox
}

// add appends a line and answers the index it took.
func (b *body) add(line string) int {
	b.lines = append(b.lines, line)
	return len(b.lines) - 1
}

// hit records a pressable place of a line already added.
func (b *body) hit(line, x1, x2 int, action, arg string) {
	b.boxes = append(b.boxes, clickBox{y1: line, y2: line, x1: x1, x2: x2, action: action, arg: arg})
}

// screenWidth answers the width the frame draws in: one column short of the
// terminal, so the closing border never wraps to the next line.
func (m *Model) screenWidth() int {
	if m.width < 40 {
		return 90
	}
	return m.width - 1
}

// screenHeight answers the height of the terminal, or a sensible one.
func (m *Model) screenHeight() int {
	if m.height < 12 {
		return 28
	}
	return m.height
}

// renderFrame draws one screen: the frame of the product, the step, the body
// and the footer of a hint and buttons. It also stores the boxes of the body
// and of the buttons in the model, moved to the absolute places of the
// screen.
func (m *Model) renderFrame(step string, b body, hint string, btns footerButtons) string {
	width := m.screenWidth()
	height := m.screenHeight()
	inner := width - 2

	footerLines := 0
	if hint != "" {
		footerLines++
	}
	if btns.line != "" {
		footerLines++
	}

	// The body has to fit between the frame, the separator and the footer.
	maxBody := height - 4 - footerLines
	if maxBody < 1 {
		maxBody = 1
	}
	lines := b.lines
	if len(lines) > maxBody {
		lines = lines[:maxBody]
	}
	// The frame takes the whole window: the space below the content stays
	// part of the screen, not of the terminal behind it.
	for len(lines) < maxBody {
		lines = append(lines, "")
	}

	var out []string
	out = append(out, m.frameTop(width, step))
	out = append(out, m.frameLine("", inner))
	for _, line := range lines {
		out = append(out, m.frameLine(line, inner))
	}
	out = append(out, m.frameSep(width))
	if hint != "" {
		out = append(out, m.frameLine(hint, inner))
	}
	buttonsY := -1
	if btns.line != "" {
		buttonsY = len(out)
		out = append(out, m.frameLine(btns.line, inner))
	}
	out = append(out, m.frameBottom(width))

	// The body starts after the top border and one empty line.
	top := 2
	left := 3
	m.hits = m.hits[:0]
	for _, box := range b.boxes {
		m.hits = append(m.hits, clickBox{
			x1: box.x1 + left, x2: box.x2 + left,
			y1: box.y1 + top, y2: box.y2 + top,
			action: box.action, arg: box.arg,
		})
	}
	if buttonsY >= 0 {
		for i, span := range btns.spans {
			item := btns.items[i]
			m.hits = append(m.hits, clickBox{
				x1: span.x1 + left, x2: span.x2 + left,
				y1: buttonsY, y2: buttonsY,
				action: item.action, arg: item.arg,
			})
		}
	}
	return strings.Join(out, "\n")
}

// frameTop draws the line of the product name and the step.
func (m *Model) frameTop(width int, step string) string {
	inner := width - 2
	left := " " + m.skin.name + " "
	right := " " + step + " "
	fill := inner - len(left) - len(right)
	if fill < 0 {
		fill = 0
	}
	return m.skin.tl + left + strings.Repeat(m.skin.h, fill) + right + m.skin.tr
}

// frameSep draws the line between the body and the footer.
func (m *Model) frameSep(width int) string {
	return m.skin.teeL + strings.Repeat(m.skin.h, width-2) + m.skin.teeR
}

// frameBottom draws the closing line.
func (m *Model) frameBottom(width int) string {
	return m.skin.bl + strings.Repeat(m.skin.h, width-2) + m.skin.br
}

// frameLine draws one line of the inside, padded to the inner width.
func (m *Model) frameLine(line string, inner int) string {
	return m.skin.v + " " + fit(line, inner-2) + " " + m.skin.v
}

// fit cuts a line to the width and pads it with spaces.
func fit(line string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(line) > width {
		runes := []rune(line)
		if len(runes) > width {
			runes = runes[:width]
		}
		line = string(runes)
	}
	pad := width - lipgloss.Width(line)
	if pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line
}

// buttonSpec is one button of a footer row.
type buttonSpec struct {
	label   string
	primary bool
	action  string
	arg     string
}

// buttonsLine draws a row of buttons separated by three spaces, and answers
// the line and the x ranges of each button.
func buttonsLine(items ...buttonSpec) (string, []struct{ x1, x2 int }) {
	var out strings.Builder
	var spans []struct{ x1, x2 int }
	for i, item := range items {
		if i != 0 {
			out.WriteString("   ")
		}
		x1 := lipgloss.Width(out.String())
		text := "[ " + item.label + " ]"
		if item.primary {
			text = primaryStyle.Render(text)
		}
		out.WriteString(text)
		spans = append(spans, struct{ x1, x2 int }{x1, x1 + lipgloss.Width(text) - 1})
	}
	return out.String(), spans
}

// footerButtons is a row of buttons of the footer.
type footerButtons struct {
	line  string
	spans []struct{ x1, x2 int }
	items []buttonSpec
}

// makeButtons builds a footer row; the frame registers its boxes.
func makeButtons(items ...buttonSpec) footerButtons {
	line, spans := buttonsLine(items...)
	return footerButtons{line: line, spans: spans, items: items}
}

// humanProfile names a profile the way a person would say it, and says what
// it does in one line.
func humanProfile(name string) (title, about string) {
	switch name {
	case "default":
		return i18n.T("tui.profile.default.title"), i18n.T("tui.profile.default.about")
	case "dev":
		return i18n.T("tui.profile.dev.title"), i18n.T("tui.profile.dev.about")
	case "aggressive":
		return i18n.T("tui.profile.deep.title"), i18n.T("tui.profile.deep.about")
	}
	return name, i18n.T("tui.profile.otherabout")
}

// riskWord names a risk the way the badge of the spec writes it.
func riskWord(r model.Risk) string {
	switch r {
	case model.RiskDangerous:
		return "DANGER"
	case model.RiskModerate:
		return "MOD"
	default:
		return "SAFE"
	}
}

// riskLegend explains the badges in plain words.
func riskLegend() string {
	return i18n.T("tui.risk.legend")
}

// riskStyleOf colors the plain word of a risk.
func riskStyleOf(r model.Risk) lipgloss.Style {
	switch r {
	case model.RiskDangerous:
		return dangerStyle
	case model.RiskModerate:
		return moderateStyle
	default:
		return safeStyle
	}
}

// wrapText breaks a sentence into lines no wider than width, at spaces.
func wrapText(text string, width int) []string {
	if width < 10 {
		width = 10
	}
	words := strings.Fields(text)
	var lines []string
	var line string
	for _, word := range words {
		if line == "" {
			line = word
			continue
		}
		if len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = word
			continue
		}
		line += " " + word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// mutedLine keeps a long line inside the width, cutting it with the skin's
// ellipsis instead of letting the frame do it.
func mutedLine(text string, width int) string {
	if width <= 0 || len(text) <= width {
		return text
	}
	if width <= 3 {
		return text[:width]
	}
	return text[:width-3] + "..."
}

// ruleCount names a number of rules with its noun.
func ruleCount(n int) string {
	if i18n.Current() == i18n.LangRU {
		return fmt.Sprintf("%d %s", n, russianPlural(n, "правило", "правила", "правил"))
	}
	if n == 1 {
		return i18n.T("tui.rulescount.one")
	}
	return fmt.Sprintf(i18n.T("tui.rulescount.many"), n)
}

// humanCount writes an integer with thousands groups: 11623 -> 11,623.
func humanCount(n int64) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	out := strings.Join(parts, ",")
	if neg {
		out = "-" + out
	}
	return out
}

// rulesCount words a number of rules the way the language counts. Russian
// has three endings and picks one by the last digits; English has one
// form in the catalog today.
func (m *Model) rulesCount(n int) string {
	if i18n.Current() == i18n.LangRU {
		return fmt.Sprintf("%d %s", n, russianPlural(n, "правило", "правила", "правил"))
	}
	return fmt.Sprintf(i18n.T("tui.profile.rulescount"), n)
}

// filesCount words a number of files the same way.
func (m *Model) filesCount(n int64) string {
	if i18n.Current() == i18n.LangRU {
		return fmt.Sprintf("%d %s", n, russianPlural(int(n), "файл", "файла", "файлов"))
	}
	return fmt.Sprintf(i18n.T("tui.scan.filescount"), n)
}

// notesCount words a number of notes the same way.
func (m *Model) notesCount(n int) string {
	if i18n.Current() == i18n.LangRU {
		return fmt.Sprintf("%d %s", n, russianPlural(n, "заметка", "заметки", "заметок"))
	}
	return fmt.Sprintf(i18n.T("tui.scan.notescount"), n)
}

// russianPlural picks the ending a number takes: one, a few or many. The
// 11..14 range is the exception Russian keeps, where 11 is many rather
// than one.
func russianPlural(n int, one, few, many string) string {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs%100 >= 11 && abs%100 <= 14:
		return many
	case abs%10 == 1:
		return one
	case abs%10 >= 2 && abs%10 <= 4:
		return few
	}
	return many
}
