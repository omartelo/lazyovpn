// Package tui is the lazyovpn bubbletea interface: a connection sidebar + a log pane.
// It owns global app state (layout, key routing, live stream) and composes the
// per-panel sub-models in internal/tui/models.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/omartelo/lazyovpn/internal/tui/models"
	"github.com/omartelo/lazyovpn/internal/tui/utils"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

const sidebarWidth = 42

var (
	helpStyle = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	nameStyle = lipgloss.NewStyle().Faint(true)
)

// helpKeys is the keybinding footer, lazydocker style.
const helpKeys = "↑/↓ j/k: navigate · /: filter · enter: connect · d: disconnect · q: quit"

type model struct {
	sidebar  models.Sidebar
	terminal models.Terminal
	mgr      *vpn.Manager
	logCh    <-chan string // live stream of the active connection
	w, h     int
}

// New builds the initial model from the already-discovered configs.
func New(configs []vpn.Config, mgr *vpn.Manager) model {
	return model{
		sidebar:  models.NewSidebar(configs),
		terminal: models.NewTerminal(),
		mgr:      mgr,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout(msg.Width, msg.Height)
		return m, nil

	case utils.LogMsg:
		if msg.Ch != m.logCh {
			return m, nil // log from an old connection
		}
		m.terminal.AppendLog(msg.Line)
		return m, utils.WaitForLog(m.logCh)

	case utils.LogClosedMsg:
		if msg.Ch == m.logCh {
			m.terminal.MarkClosed()
			m.logCh = nil
		}
		return m, nil

	case tea.KeyMsg:
		if m.sidebar.Filtering() {
			break // let the filter consume the keys
		}
		switch msg.String() {
		case "q", "ctrl+c":
			_ = m.mgr.Disconnect()
			return m, tea.Quit
		case "enter":
			return m.connectSelected()
		case "d":
			_ = m.mgr.Disconnect()
			m.terminal.MarkDisconnected()
			m.logCh = nil
			return m, nil
		}
	}

	// Navigation (j/k/arrows come from the list). On selection change, show that
	// connection's output.
	prev := m.sidebar.SelectedName()
	var cmd tea.Cmd
	m.sidebar, cmd = m.sidebar.Update(msg)
	if sel := m.sidebar.SelectedName(); sel != prev {
		m.terminal.ShowBuffer(sel)
	}
	return m, cmd
}

func (m model) connectSelected() (tea.Model, tea.Cmd) {
	cfg, ok := m.sidebar.SelectedConfig()
	if !ok {
		return m, nil
	}
	ch, err := m.mgr.Connect(cfg)
	if err != nil {
		m.terminal.SetError(err.Error())
		return m, nil
	}
	m.logCh = ch
	m.terminal.StartConnection(cfg.Name)
	return m, utils.WaitForLog(ch)
}

// layout recomputes both pane sizes. Reserves 2 rows: status + help.
func (m *model) layout(w, h int) {
	m.w, m.h = w, h
	sideW, sideH, outW, outH := m.dims()
	firstReady := !m.terminal.Ready()
	m.sidebar.SetSize(sideW, sideH)
	m.terminal.SetSize(outW, outH)
	if firstReady {
		m.terminal.ShowBuffer(m.sidebar.SelectedName()) // initial state
	}
}

// dims returns the inner content size of each pane (excluding border + padding).
func (m model) dims() (sideW, sideH, outW, outH int) {
	bodyH := m.h - 2 // status + help rows
	sideW = max(sidebarWidth-4, 1)
	outW = max(m.w-sidebarWidth-4, 1)
	sideH = max(bodyH-2, 1) // top + bottom border
	outH = sideH
	return
}

func (m model) View() string {
	if !m.terminal.Ready() {
		return "loading..."
	}
	left := m.sidebar.View(true) // sidebar is the focused pane
	right := m.terminal.View(false)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return body + "\n" + m.statusLine() + "\n" + helpStyle.Render(helpKeys)
}

func (m model) statusLine() string {
	line := " " + m.terminal.State().Badge()
	if m.terminal.State() == models.StateError && m.terminal.Err() != "" {
		line += nameStyle.Render(": " + m.terminal.Err())
	} else if name := m.terminal.ActiveName(); name != "" {
		line += "  " + nameStyle.Render(name)
	}
	return line
}
