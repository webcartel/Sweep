// Package tui is the terminal frontend of sweep: it scans, shows and cleans
// through the same core packages the CLI commands call, and adds nothing to
// the work itself.
package tui

import (
	"context"

	"github.com/charmbracelet/bubbletea"
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
	// AsciiFlag and NoASCIIFlag are the say of the command line.
	AsciiFlag, NoASCIIFlag bool
	// Profiles names every profile of the rules, in the order the TUI
	// lists them.
	Profiles []profileRow
	// Scanner is the scanner of the core the run reads the disk through.
	Scanner *scan.Scanner
	// RulesFor answers the rules of one profile of the set.
	RulesFor func(profile string) ([]*model.Rule, error)
}

// Model is the one Bubble Tea model of the TUI. Every screen of the run is a
// state of it.
type Model struct {
	screen int
	skin   skin
	profileState
	scanState
	listState
	detailState
	confirmState
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
}

// New puts the model together. The TUI opens on the screen of the profile.
func New(d Deps) *Model {
	m := &Model{screen: screenProfile}
	m.profiles = d.Profiles
	m.scanner = d.Scanner
	m.rulesFor = d.RulesFor
	env := d.Env
	if env == nil {
		env = envMap{}
	}
	m.skin = newSkin(env, d.AsciiFlag, d.NoASCIIFlag)
	m.sort = "size"
	return m
}

// Init starts the first command of the program; none yet.
func (m *Model) Init() tea.Cmd { return nil }

// Update handles one message and answers with the model and the next
// command.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	}
	return m, cmd
}

// View draws the screen the model is on.
func (m *Model) View() string {
	switch m.screen {
	case screenProfile:
		return m.viewProfile()
	case screenList:
		return m.viewList(80)
	case screenDetail:
		return m.viewDetail()
	case screenConfirm:
		return m.viewConfirm()
	}
	return ""
}
