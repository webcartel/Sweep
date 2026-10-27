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
	"github.com/webcartel/sweep/internal/rules"
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
	// RulesDir is where the user's own rule files live.
	RulesDir string
	// ConfigPath is the config file the run read ("" when the machine has
	// none).
	ConfigPath string
	// Set is every rule the run knows, built-in and the user's own; the
	// rules screens read it.
	Set *rules.Set
	// Reload reads the sources again after an edit and answers the fresh
	// set.
	Reload func() (*rules.Set, error)
	// Registry is the registry the cleanup reads and changes.
	Registry platform.Registry
	// RegistryPreselect names the registry findings an elevated restart
	// brought back.
	RegistryPreselect []string
	// RestartRegistry restarts the whole run as the administrator with the
	// registry findings named.
	RestartRegistry func(ids []string) error
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
	rulesState
	regState
	registryPreselect []string
	logDir            string
	run               runState
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
	scanRules  []*model.Rule
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
	m.set = d.Set
	m.rulesDir = d.RulesDir
	m.configPath = d.ConfigPath
	m.reload = d.Reload
	m.registryPreselect = d.RegistryPreselect
	env := d.Env
	if env == nil {
		env = envMap{}
	}
	m.env = env
	m.reg = d.Registry
	m.fsys = d.FS
	m.backup = registryBackupDir(env)
	m.regEnv = env
	m.regElevated = elevatedOf(d.System)
	m.registryRestart = d.RestartRegistry
	m.skin = newSkin(env, d.AsciiFlag, d.NoASCIIFlag)
	m.sort = "size"
	return m
}

// Init starts the first command of the program: a run whose preselect
// named the rules goes straight to the scan.
func (m *Model) Init() tea.Cmd {
	title := tea.SetWindowTitle("Sweep - free space safely")
	if len(m.registryPreselect) != 0 && m.reg != nil {
		m.screen = screenRegScan
		return tea.Batch(title, m.scanRegistry())
	}
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
	if done, ok := msg.(notepadDone); ok {
		return m, m.afterNotepad(done)
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
	case screenRules:
		cmd, _ = m.updateRules(msg)
	case screenRuleList:
		cmd, _ = m.updateRuleList(msg)
	case screenRuleDetail:
		cmd, _ = m.updateRuleDetail(msg)
	case screenConfig:
		cmd, _ = m.updateConfig(msg)
	case screenUserRules:
		cmd, _ = m.updateUserRules(msg)
	case screenEditor:
		cmd, _ = m.updateEditor(msg)
	case screenPreview:
		cmd, _ = m.updatePreview(msg)
	case screenRegScan:
		cmd, _ = m.updateRegScan(msg)
	case screenRegReview:
		cmd, _ = m.updateRegReview(msg)
	case screenRegConfirm:
		cmd, _ = m.updateRegConfirm(msg)
	case screenRegRunning:
		cmd, _ = m.updateRegRunning(msg)
	case screenRegDone:
		cmd, _ = m.updateRegDone(msg)
	}
	return m, cmd
}

// View draws the screen the model is on.
func (m *Model) View() string {
	if m.width != 0 && (m.width < 80 || m.height < 24) {
		return "Please make the window larger: Sweep needs at least 80 x 24.\n" +
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
	case screenRules:
		return m.viewRules()
	case screenRuleList:
		return m.viewRuleList()
	case screenRuleDetail:
		return m.viewRuleDetail()
	case screenConfig:
		return m.viewConfig()
	case screenUserRules:
		return m.viewUserRules()
	case screenEditor:
		return m.viewEditor()
	case screenPreview:
		return m.viewPreview()
	case screenRegScan:
		return m.viewRegScan()
	case screenRegReview:
		return m.viewRegReview()
	case screenRegConfirm:
		return m.viewRegConfirm()
	case screenRegRunning:
		return m.viewRegRunning()
	case screenRegDone:
		return m.viewRegDone()
	}
	return ""
}
