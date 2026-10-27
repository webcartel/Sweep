// Package tui is the terminal frontend of sweep: it scans, shows and cleans
// through the same core packages the CLI commands call, and adds nothing to
// the work itself.
package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/exec"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/scan"
)

// The screens of the TUI, in the order a run walks them. A screen is a state
// of one Model, not a type of its own: the plan of the spec.
const (
	screenProfile = iota // the choice of the profile
)

// Deps collects what the TUI reaches the machine through; the frontend that
// builds it is the root command of the CLI.
type Deps struct {
	// Env is the environment of the machine, for the say of --ascii and
	// the terminal that screens draw in.
	Env platform.Env
	// FS is what the scanner and the executor read and change.
	FS platform.FS
	// AsciiFlag and NoASCIIFlag are the say of the command line.
	AsciiFlag, NoASCIIFlag bool
	// Executor is the executor of the core the run works through.
	Executor *exec.Executor
	// LogDir is where the log of the run goes, and next to it the answer
	// it can leave behind.
	LogDir string
	// Profiles names every profile of the rules, in the order the TUI
	// lists them.
	Profiles []ProfileRow
	// Scanner is the scanner of the core the run reads the disk through.
	Scanner *scan.Scanner
	// System is the machine, for the say of the elevation.
	System platform.System
	// Preselect names the rules a run were already agreed to: they land
	// in the choice once the plan does.
	Preselect []string
	// Restart restarts the whole run as the administrator, with the
	// rules named in the choice.
	Restart func(preselect []string) error
	// RulesFor answers the rules of one profile of the set.
	RulesFor func(profile string) ([]*model.Rule, error)
}

// Model is the one Bubble Tea model of the TUI. Every screen of the run is a
// state of it.
type Model struct {
	screen        int
	screenBehind  int
	width, height int
	skin          skin
	hits          []clickBox
	profileState
	scanState
	listState
	detailState
	confirmState
	uacState
	logDir string
	run    runState
}

// scanState is what the screen of the scan and its neighbours read and
// write.
type scanState struct {
	scanError  error
	progress   map[string]model.RulePlan
	notes      []string
	scanner    *scan.Scanner
	rulesFor   func(profile string) ([]*model.Rule, error)
	scanEvents <-chan model.Event
	cancelScan context.CancelFunc
	rules      []*model.Rule
	plan       *model.Plan
	selection  model.Selection
	executor   *exec.Executor
}

// New puts the model together. The TUI opens on the screen of the profile.
func New(d Deps) *Model {
	m := &Model{screen: screenProfile}
	m.profiles = d.Profiles
	m.scanner = d.Scanner
	m.executor = d.Executor
	m.rulesFor = d.RulesFor
	m.system = d.System
	m.preselect = d.Preselect
	m.restart = d.Restart
	m.logDir = d.LogDir
	env := d.Env
	if env == nil {
		env = envMap{}
	}
	m.skin = newSkin(env, d.AsciiFlag, d.NoASCIIFlag)
	m.sort = "size"
	return m
}

// Init starts the first command of the program: a run whose preselect
// named the rules goes straight to the scan.
func (m *Model) Init() tea.Cmd {
	title := tea.SetWindowTitle("sweep - free space safely")
	if len(m.preselect) != 0 && m.scanner != nil && m.rulesFor != nil {
		m.screen = screenScanning
		return tea.Batch(title, m.startScan())
	}
	return title
}

// Update handles one message and answers with the model and the next
// command.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		return m, nil
	}
	if click, ok := msg.(tea.MouseMsg); ok {
		if cmd, took := m.handleClick(click); took {
			return m, cmd
		}
	}
	if m.screen == screenHelp {
		cmd, _ := m.updateHelp(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	switch m.screen {
	case screenProfile:
		cmd, _ = m.updateProfile(msg)
	case screenScanning:
		cmd, _ = m.updateScanning(msg)
	case screenList:
		cmd, _ = m.updateList(msg)
	case screenDetail:
		cmd, _ = m.updateDetail(msg)
	case screenConfirm:
		cmd, _ = m.updateConfirm(msg)
	case screenRunning:
		cmd, _ = m.updateRunning(msg)
	case screenDone:
		cmd, _ = m.updateDone(msg)
	}
	return m, cmd
}

// View draws the screen the model is on.
func (m *Model) View() string {
	if m.width != 0 && (m.width < 80 || m.height < 24) {
		return "Please make the window larger: sweep needs at least 80 x 24.\n" +
			fmt.Sprintf("This window is %d x %d.", m.width, m.height)
	}
	switch m.screen {
	case screenProfile:
		return m.viewProfile()
	case screenScanning:
		return m.viewScanning()
	case screenList:
		return m.viewList()
	case screenDetail:
		return m.viewDetail()
	case screenConfirm:
		return m.viewConfirm()
	case screenRunning:
		return m.viewRunning()
	case screenDone:
		return m.viewDone()
	case screenHelp:
		return m.viewHelp()
	}
	return ""
}
