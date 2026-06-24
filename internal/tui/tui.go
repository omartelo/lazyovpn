// Package tui is the lazyovpn bubbletea interface: a connection sidebar + a log pane.
// It owns global app state (layout, key routing, live stream) and composes the
// per-panel sub-models in internal/tui/models.
package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/tui/components"
	"github.com/omartelo/lazyovpn/internal/tui/models"
	"github.com/omartelo/lazyovpn/internal/tui/utils"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

const (
	sidebarWidth = 42 // sidebar pane outer width in columns

	// paneChromeW/H are the cells a components.TitledBox adds around its inner
	// content: left+right border+padding (4) and top+bottom border (2). dims()
	// subtracts them to get each pane's inner content size.
	paneChromeW = 4
	paneChromeH = 2

	footerRows = 2 // rows reserved below the panes: status line + help line

	forgetInnerW = 54 // confirm-forget popup inner width
	forgetInnerH = 4  // confirm-forget popup inner height: prompt + name + hint
)

var (
	helpStyle = components.Hint.Padding(0, 1)
	nameStyle = components.Hint
)

// helpKeys is the keybinding footer, lazydocker style.
const helpKeys = "↑/↓ j/k: navigate · /: filter · enter: connect · a: add · d: disconnect · x: forget creds · q: quit"

// appMode is the top-level interaction mode.
type appMode int

const (
	modeNormal appMode = iota
	modeAuth           // credential modal is capturing input
	modeAdd            // import-connection modal is open
	modeForget         // confirm forgetting saved credentials
)

type model struct {
	sidebar    models.Sidebar
	terminal   models.Terminal
	auth       models.AuthModal
	add        models.AddModal
	mode       appMode
	pending    vpn.Config // connection awaiting credentials
	forgetName string     // connection whose saved creds the forget modal targets
	mgr        *vpn.Manager
	logCh      <-chan string // live stream of the active connection
	markedConn string        // connection currently flagged connected in the sidebar
	w, h       int
}

// New builds the initial model from the already-discovered configs.
func New(configs []vpn.Config, mgr *vpn.Manager) model {
	return model{
		sidebar:  models.NewSidebar(configs),
		terminal: models.NewTerminal(),
		auth:     models.NewAuthModal(),
		add:      models.NewAddModal(),
		mgr:      mgr,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Global messages: layout and the live log stream flow regardless of mode.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout(msg.Width, msg.Height)
		return m, nil

	case utils.LogMsg:
		if msg.Ch != m.logCh {
			return m, nil // log from an old connection
		}
		m.terminal.AppendLog(msg.Line)
		m.syncSidebar() // tunnel-up flips the sidebar marker green
		return m, utils.WaitForLog(m.logCh)

	case utils.LogClosedMsg:
		if msg.Ch == m.logCh {
			m.terminal.MarkClosed()
			m.logCh = nil
			m.syncSidebar()
		}
		return m, nil
	}

	// An open modal owns all other input while it is up.
	switch m.mode {
	case modeAuth:
		return m.updateAuth(msg)
	case modeAdd:
		return m.updateAdd(msg)
	case modeForget:
		return m.updateForget(msg)
	}

	if key, ok := msg.(tea.KeyPressMsg); ok && !m.sidebar.Filtering() {
		switch key.String() {
		case "q", "ctrl+c":
			_ = m.mgr.Disconnect()
			return m, tea.Quit
		case "a":
			m.mode = modeAdd
			return m, m.add.Open()
		case "enter":
			return m.enter()
		case "d":
			_ = m.mgr.Disconnect()
			m.terminal.MarkDisconnected()
			m.logCh = nil
			m.syncSidebar()
			return m, nil
		case "x":
			// Forget saved creds for the selected connection (e.g. after a
			// password change) so the next connect prompts again. Only ask when
			// there is actually something stored — no popup for a no-op.
			if cfg, ok := m.sidebar.SelectedConfig(); ok {
				if _, _, has, _ := vpn.LoadCreds(cfg.Name); has {
					m.forgetName = cfg.Name
					m.mode = modeForget
				}
			}
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

// updateAuth handles input while the credential modal is open.
func (m model) updateAuth(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.auth.Reset()
			m.mode = modeNormal
			return m, nil
		case "enter":
			user, pass := m.auth.Username(), m.auth.Password()
			save := m.auth.Save()
			cfg := m.pending
			m.auth.Reset() // drop the password as soon as it is handed off
			m.mode = modeNormal
			if save {
				// Best-effort: a keyring write failure must not block the connect.
				_ = vpn.SaveCreds(cfg.Name, user, pass)
			}
			return m.connect(cfg, user, pass)
		}
	}
	var cmd tea.Cmd
	m.auth, cmd = m.auth.Update(msg)
	return m, cmd
}

// updateAdd handles input while the import-connection modal is open.
func (m model) updateAdd(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.add.Reset()
			m.mode = modeNormal
			return m, nil
		case "r":
			return m, m.add.Open() // launch the chooser again
		case "enter":
			return m.addConfirm()
		}
	}
	var cmd tea.Cmd
	m.add, cmd = m.add.Update(msg) // records the file-chooser result
	return m, cmd
}

// updateForget handles the confirm-forget popup. y/enter deletes the saved
// credentials; n/esc backs out without touching the keyring.
func (m model) updateForget(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "enter":
		_ = vpn.ForgetCreds(m.forgetName)
		m.forgetName = ""
		m.mode = modeNormal
	case "n", "esc":
		m.forgetName = ""
		m.mode = modeNormal
	}
	return m, nil
}

// addConfirm imports the picked file into the connections dir and appends it to
// the sidebar. Stays open on error so the user can pick again.
func (m model) addConfirm() (tea.Model, tea.Cmd) {
	path := m.add.Path()
	if path == "" {
		return m, nil // nothing picked yet
	}
	cfg, err := vpn.ImportConfig(path)
	if err != nil {
		m.add.SetError(err.Error())
		return m, nil
	}
	m.sidebar.AddConfig(cfg)
	m.add.Reset()
	m.mode = modeNormal
	return m, nil
}

// enter connects the selected config, prompting for credentials first if needed.
func (m model) enter() (tea.Model, tea.Cmd) {
	cfg, ok := m.sidebar.SelectedConfig()
	if !ok {
		return m, nil
	}
	needs, err := vpn.NeedsAuth(cfg)
	if err != nil {
		m.terminal.SetError(err.Error())
		return m, nil
	}
	if needs {
		// Saved creds skip the prompt. On any keyring error fall back to asking.
		if user, pass, ok, err := vpn.LoadCreds(cfg.Name); ok && err == nil {
			return m.connect(cfg, user, pass)
		}
		m.pending = cfg
		m.mode = modeAuth
		return m, m.auth.Open(cfg.Name)
	}
	return m.connect(cfg, "", "")
}

// connect starts the connection and begins pumping its log stream.
func (m model) connect(cfg vpn.Config, username, password string) (tea.Model, tea.Cmd) {
	ch, err := m.mgr.Connect(cfg, username, password)
	if err != nil {
		m.terminal.SetError(err.Error())
		return m, nil
	}
	m.logCh = ch
	m.terminal.StartConnection(cfg.Name)
	m.syncSidebar() // a fresh connect clears any previous green marker
	return m, utils.WaitForLog(ch)
}

// syncSidebar flags the connected connection in the list, but only on a real
// state change (avoids rebuilding the delegate on every log line).
func (m *model) syncSidebar() {
	name := ""
	if m.terminal.State() == models.StateConnected {
		name = m.terminal.ActiveName()
	}
	if name != m.markedConn {
		m.markedConn = name
		m.sidebar.SetConnected(name)
	}
}

// layout recomputes pane sizes. Reserves footerRows below the panes.
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
	bodyH := m.h - footerRows
	sideW = max(sidebarWidth-paneChromeW, 1)
	outW = max(m.w-sidebarWidth-paneChromeW, 1)
	sideH = max(bodyH-paneChromeH, 1)
	outH = sideH
	return
}

func (m model) View() tea.View {
	if !m.terminal.Ready() {
		return altView("loading...")
	}

	left := m.sidebar.View(true) // sidebar is the focused pane
	right := m.terminal.View(false)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	switch m.mode { // popup floating over the view
	case modeAuth:
		body = components.Center(body, m.auth.View())
	case modeAdd:
		body = components.Center(body, m.add.View())
	case modeForget:
		body = components.Center(body, m.forgetView())
	}
	return altView(body + "\n" + m.statusLine() + "\n" + helpStyle.Render(helpKeys))
}

// altView wraps content in a tea.View on the alt screen (v2: alt screen is
// declarative, set per-frame instead of via a program option).
func altView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// forgetView renders the confirm-forget popup floating over the main view.
func (m model) forgetView() string {
	body := "Forget saved credentials for\n\"" + m.forgetName + "\"?\n\n" +
		components.Hint.Render("y/enter: forget · n/esc: cancel")
	return components.TitledBox("forget credentials", body, forgetInnerW, forgetInnerH, true)
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
