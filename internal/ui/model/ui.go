// ui.go holds UI, the sole bubbletea model and the app's brain: it owns every
// piece of global state (mode, layout, key routing, the live log stream) and
// drives the sidebar + log panels. Message routing is centralized in one Update
// switch; the panels are imperative helpers it calls (see the package CLAUDE.md).
package model

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
	"github.com/omartelo/lazyovpn/internal/ui/utils"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

// sidebarWidth is the sidebar pane's outer width, in columns; the log pane
// takes the rest of the terminal width.
const sidebarWidth = 42

// paneChromeW and paneChromeH are the extra cells a common.TitledBox draws
// around its inner content: left+right border+padding (W) and top+bottom
// border (H). dims() subtracts them to size each pane's content area.
const (
	paneChromeW = 4
	paneChromeH = 2
)

// footerRows is the number of rows reserved below the panes for the status
// line and the help line.
const footerRows = 2

// Confirm-popup inner widths, in columns. Height auto-fits the content via
// common.Dialog, so only the width is pinned.
const (
	forgetInnerW     = 54
	disconnectInnerW = 54
)

var (
	helpStyle = common.Hint.Padding(0, 1)
	nameStyle = common.Hint
)

// helpKeys is the keybinding footer, lazydocker style.
const helpKeys = "↑/↓ j/k: navigate · /: filter · enter: connect · a: add · d: disconnect · x: forget creds · q: quit"

// appMode is the top-level interaction mode: which overlay (if any) currently
// owns input. modeNormal routes keys to the panels; every other mode means a
// modal or confirm popup is up and consuming input.
type appMode uint8

// Possible appMode values.
const (
	modeNormal     appMode = iota
	modeAuth               // credential modal is capturing input
	modeAdd                // import-connection modal is open
	modeForget             // confirm forgetting saved credentials
	modeDisconnect         // confirm tearing down the live connection
)

// UI is the sole bubbletea model: the central brain. It holds the panels it
// drives, the active interaction mode, and the shared runtime state (the VPN
// manager, the live log channel, terminal size). All routing and mode switching
// happens on this type; the panels only ever see a message through a method UI
// calls on them.
type UI struct {
	sidebar    Sidebar
	terminal   Terminal
	auth       AuthModal
	add        AddModal
	mode       appMode
	pending    vpn.Config // connection awaiting credentials
	forgetName string     // connection whose saved creds the forget modal targets
	mgr        *vpn.Manager
	logCh      <-chan string // live stream of the active connection
	markedConn string        // connection currently flagged connected in the sidebar
	w, h       int
}

// New builds the initial UI from the already-discovered configs.
func New(configs []vpn.Config, mgr *vpn.Manager) *UI {
	return &UI{
		sidebar:  NewSidebar(configs),
		terminal: NewTerminal(),
		auth:     NewAuthModal(),
		add:      NewAddModal(),
		mgr:      mgr,
	}
}

func (m *UI) Init() tea.Cmd { return nil }

func (m *UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	case modeDisconnect:
		return m.updateDisconnect(msg)
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
			// Disconnecting tears down a live tunnel, so confirm first. Only prompt
			// when something is actually connected (logCh != nil); otherwise d is a
			// no-op — nothing to disconnect.
			if m.logCh != nil {
				m.mode = modeDisconnect
			}
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
	cmd := m.sidebar.Handle(msg)
	if sel := m.sidebar.SelectedName(); sel != prev {
		m.terminal.ShowBuffer(sel)
	}
	return m, cmd
}

// updateAuth handles input while the credential modal is open.
func (m *UI) updateAuth(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	cmd := m.auth.Handle(msg)
	return m, cmd
}

// updateAdd handles input while the import-connection modal is open.
func (m *UI) updateAdd(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	m.add.Handle(msg) // records the file-chooser result
	return m, nil
}

// updateForget handles the confirm-forget popup. y/enter deletes the saved
// credentials; n/esc backs out without touching the keyring.
func (m *UI) updateForget(msg tea.Msg) (tea.Model, tea.Cmd) {
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

// updateDisconnect handles the confirm-disconnect popup. y/enter tears down the
// live connection; n/esc backs out and leaves it running.
func (m *UI) updateDisconnect(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "enter":
		_ = m.mgr.Disconnect()
		m.terminal.MarkDisconnected()
		m.logCh = nil
		m.syncSidebar()
		m.mode = modeNormal
	case "n", "esc":
		m.mode = modeNormal
	}
	return m, nil
}

// addConfirm imports the picked file into the connections dir and appends it to
// the sidebar. Stays open on error so the user can pick again.
func (m *UI) addConfirm() (tea.Model, tea.Cmd) {
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
func (m *UI) enter() (tea.Model, tea.Cmd) {
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
func (m *UI) connect(cfg vpn.Config, username, password string) (tea.Model, tea.Cmd) {
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
func (m *UI) syncSidebar() {
	name := ""
	if m.terminal.State() == StateConnected {
		name = m.terminal.ActiveName()
	}
	if name != m.markedConn {
		m.markedConn = name
		m.sidebar.SetConnected(name)
	}
}

// layout recomputes pane sizes. Reserves footerRows below the panes.
func (m *UI) layout(w, h int) {
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
func (m *UI) dims() (sideW, sideH, outW, outH int) {
	bodyH := m.h - footerRows
	sideW = max(sidebarWidth-paneChromeW, 1)
	outW = max(m.w-sidebarWidth-paneChromeW, 1)
	sideH = max(bodyH-paneChromeH, 1)
	outH = sideH
	return
}

func (m *UI) View() tea.View {
	if !m.terminal.Ready() {
		return altView("loading...")
	}

	left := m.sidebar.View(true) // sidebar is the focused pane
	right := m.terminal.View(false)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	switch m.mode { // popup floating over the view
	case modeAuth:
		body = common.Center(body, m.auth.View())
	case modeAdd:
		body = common.Center(body, m.add.View())
	case modeForget:
		body = common.Center(body, m.forgetView())
	case modeDisconnect:
		body = common.Center(body, m.disconnectView())
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
func (m *UI) forgetView() string {
	body := "Forget saved credentials for\n\"" + m.forgetName + "\"?\n\n" +
		common.Hint.Render("y/enter: forget · n/esc: cancel")
	return common.Dialog{Title: "forget credentials", Width: forgetInnerW}.Render(body)
}

// disconnectView renders the confirm-disconnect popup floating over the main view.
func (m *UI) disconnectView() string {
	body := "Disconnect from\n\"" + m.terminal.ActiveName() + "\"?\n\n" +
		common.Hint.Render("y/enter: disconnect · n/esc: cancel")
	return common.Dialog{Title: "disconnect", Width: disconnectInnerW}.Render(body)
}

func (m *UI) statusLine() string {
	line := " " + m.terminal.State().Badge()
	if m.terminal.State() == StateError && m.terminal.Err() != "" {
		line += nameStyle.Render(": " + m.terminal.Err())
	} else if name := m.terminal.ActiveName(); name != "" {
		line += "  " + nameStyle.Render(name)
	}
	return line
}
