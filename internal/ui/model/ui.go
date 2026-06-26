// ui.go holds UI, the sole bubbletea model and the app's brain: it owns every
// piece of global state (mode, layout, key routing, the live log stream) and
// drives the sidebar + log panels. Message routing is centralized in one Update
// switch; the panels are imperative helpers it calls (see the package CLAUDE.md).
package model

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
	"github.com/omartelo/lazyovpn/internal/ui/dialog"
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

var (
	helpStyle = common.Hint.Padding(0, 1)
	nameStyle = common.Hint
)

// helpKeys is the keybinding footer, lazydocker style. helpKeysLog
// replaces it while the log pane is focused for scrolling.
const (
	helpKeys    = "↑/↓ j/k: navigate · enter: connect · tab: log · a: add · d: disconnect · x: forget creds · q: quit"
	helpKeysLog = "↑/↓ j/k: scroll · pgup/pgdn: page · tab/esc: back · q: quit"
)

// appMode is the top-level interaction mode: which overlay (if any) currently
// owns input. modeNormal routes keys to the panels; every other mode means a
// modal or confirm popup is up and consuming input.
type appMode uint8

// Possible appMode values.
const (
	modeNormal  appMode = iota
	modeAuth            // credential modal is capturing input
	modeAdd             // import-connection modal is open
	modeConfirm         // a yes/no confirm popup is up (forget creds, disconnect)
)

// pane identifies which panel receives navigation keys while no overlay is up
// (modeNormal). focusLog hands keys to the log viewport for scrolling.
type pane uint8

// Possible pane values.
const (
	focusSidebar pane = iota // list navigation + connect
	focusLog                 // scroll the live log viewport
)

// shared is the global UI state the panels read by pointer (crush's
// common.Common, scaled to a single field). The sidebar reads connected live at
// render time, so flipping it is enough to repaint the ● marker — no setter, no
// delegate rebuild.
type shared struct {
	connected string // name of the live connection, "" if none
}

// UI is the sole bubbletea model: the central brain. It holds the panels it
// drives, the active interaction mode, and the shared runtime state (the VPN
// manager, the live log channel, terminal size). All routing and mode switching
// happens on this type; the panels only ever see a message through a method UI
// calls on them.
type UI struct {
	sidebar Sidebar
	log     Log
	creds   dialog.Credentials
	picker  dialog.FilePicker
	mode    appMode
	focus   pane           // which pane gets nav keys in modeNormal
	pending vpn.Config     // connection awaiting credentials
	confirm dialog.Confirm // the active yes/no popup (forget creds, disconnect)
	mgr     *vpn.Manager
	logCh   <-chan string // live stream of the active connection
	sh      shared        // global state the panels read by pointer
	w, h    int
}

// New builds the initial UI from the already-discovered configs.
func New(configs []vpn.Config, mgr *vpn.Manager) *UI {
	m := &UI{mgr: mgr}
	m.sidebar = NewSidebar(configs, &m.sh) // sidebar reads m.sh.connected by pointer
	m.log = NewLog()
	m.creds = dialog.NewCredentials()
	m.picker = dialog.NewFilePicker("add connection")
	return m
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
		m.log.AppendLog(msg.Line)
		m.syncConnected() // tunnel-up flips the sidebar marker green
		return m, utils.WaitForLog(m.logCh)

	case utils.LogClosedMsg:
		if msg.Ch == m.logCh {
			m.log.MarkClosed()
			m.logCh = nil
			m.syncConnected()
		}
		return m, nil

	case tea.MouseWheelMsg:
		// The wheel scrolls the log viewport regardless of which pane has key
		// focus — the sidebar doesn't scroll, the log is the only target.
		// Scrolling off the bottom pauses tailing, same as keyboard scroll.
		return m, m.log.Scroll(msg)
	}

	// An open modal owns all other input while it is up.
	switch m.mode {
	case modeAuth:
		return m.updateAuth(msg)
	case modeAdd:
		return m.updateAdd(msg)
	case modeConfirm:
		return m.updateConfirm(msg)
	}

	// In normal mode the focused pane captures navigation. The log pane
	// takes keys for scrolling; the sidebar keeps the connect/add/etc bindings.
	if m.focus == focusLog {
		return m.updateLogNav(msg)
	}

	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "q", "ctrl+c":
			_ = m.mgr.Disconnect()
			return m, tea.Quit
		case "a":
			m.mode = modeAdd
			return m, m.picker.Open()
		case "enter":
			return m.enter()
		case "tab":
			// Move focus to the log pane so it can be scrolled.
			m.focus = focusLog
			return m, nil
		case "d":
			// Disconnecting tears down a live tunnel, so confirm first. Only prompt
			// when something is actually connected (logCh != nil); otherwise d is a
			// no-op — nothing to disconnect.
			if m.logCh != nil {
				m.confirm = dialog.NewConfirm("disconnect",
					"Disconnect from\n\""+m.log.ActiveName()+"\"?", m.disconnect)
				m.mode = modeConfirm
			}
			return m, nil
		case "x":
			// Forget saved creds for the selected connection (e.g. after a
			// password change) so the next connect prompts again. Only ask when
			// there is actually something stored — no popup for a no-op.
			if cfg, ok := m.sidebar.SelectedConfig(); ok {
				if _, _, has, _ := vpn.LoadCreds(cfg.Name); has {
					name := cfg.Name
					m.confirm = dialog.NewConfirm("forget credentials",
						"Forget saved credentials for\n\""+name+"\"?",
						func() { _ = vpn.ForgetCreds(name) })
					m.mode = modeConfirm
				}
			}
			return m, nil
		}
	}

	// Navigation: move the cursor on j/k/arrows; show the newly selected
	// connection's output when the selection changes.
	prev := m.sidebar.SelectedName()
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "up", "k":
			m.sidebar.Move(-1)
		case "down", "j":
			m.sidebar.Move(1)
		}
	}
	if sel := m.sidebar.SelectedName(); sel != prev {
		m.log.ShowBuffer(sel)
	}
	return m, nil
}

// updateAuth handles input while the credential modal is open.
func (m *UI) updateAuth(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.creds.Reset()
			m.mode = modeNormal
			return m, nil
		case "enter":
			user, pass := m.creds.Username(), m.creds.Password()
			save := m.creds.Save()
			cfg := m.pending
			m.creds.Reset() // drop the password as soon as it is handed off
			m.mode = modeNormal
			if save {
				// Best-effort: a keyring write failure must not block the connect.
				_ = vpn.SaveCreds(cfg.Name, user, pass)
			}
			return m.connect(cfg, user, pass)
		}
	}
	cmd := m.creds.Handle(msg)
	return m, cmd
}

// updateConfirm handles any yes/no confirm popup (forget creds, disconnect).
// y/enter runs the action the popup was built with; n/esc backs out.
func (m *UI) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "enter":
		m.confirm.Yes()
		m.mode = modeNormal
	case "n", "esc":
		m.mode = modeNormal
	}
	return m, nil
}

// updateLogNav handles input while the log pane is focused: esc
// returns focus to the sidebar, q/ctrl+c quit, everything else scrolls the log
// viewport. The global log stream keeps flowing — it is handled before this.
func (m *UI) updateLogNav(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc", "tab":
			m.focus = focusSidebar
			return m, nil
		case "q", "ctrl+c":
			_ = m.mgr.Disconnect()
			return m, tea.Quit
		}
	}
	return m, m.log.Scroll(msg)
}

// disconnect tears down the live tunnel; it is the action behind the disconnect
// confirm popup.
func (m *UI) disconnect() {
	_ = m.mgr.Disconnect()
	m.log.MarkDisconnected()
	m.logCh = nil
	m.syncConnected()
}

// enter connects the selected config, prompting for credentials first if needed.
func (m *UI) enter() (tea.Model, tea.Cmd) {
	cfg, ok := m.sidebar.SelectedConfig()
	if !ok {
		return m, nil
	}
	// Already streaming this connection? Enter is a no-op — don't tear down a
	// live tunnel just to reconnect to the same config.
	if m.logCh != nil && cfg.Name == m.log.ActiveName() {
		return m, nil
	}
	needs, err := vpn.NeedsAuth(cfg)
	if err != nil {
		m.log.SetError(err.Error())
		return m, nil
	}
	if needs {
		// Saved creds skip the prompt. On any keyring error fall back to asking.
		if user, pass, ok, err := vpn.LoadCreds(cfg.Name); ok && err == nil {
			return m.connect(cfg, user, pass)
		}
		m.pending = cfg
		m.mode = modeAuth
		return m, m.creds.Open(cfg.Name)
	}
	return m.connect(cfg, "", "")
}

// connect starts the connection and begins pumping its log stream.
func (m *UI) connect(cfg vpn.Config, username, password string) (tea.Model, tea.Cmd) {
	ch, err := m.mgr.Connect(cfg, username, password)
	if err != nil {
		m.log.SetError(err.Error())
		return m, nil
	}
	m.logCh = ch
	m.log.StartConnection(cfg.Name)
	m.syncConnected() // a fresh connect clears any previous green marker
	return m, utils.WaitForLog(ch)
}

// syncConnected mirrors the log's connected state into the shared state
// the sidebar reads, so the ● marker tracks the live tunnel. Pull, not push:
// the sidebar re-reads m.sh.connected every frame, so there is nothing to
// rebuild — just flip the field.
func (m *UI) syncConnected() {
	if m.log.State() == StateConnected {
		m.sh.connected = m.log.ActiveName()
	} else {
		m.sh.connected = ""
	}
}

// layout recomputes pane sizes. Reserves footerRows below the panes.
func (m *UI) layout(w, h int) {
	m.w, m.h = w, h
	sideW, sideH, outW, outH := m.dims()
	firstReady := !m.log.Ready()
	m.sidebar.SetSize(sideW, sideH)
	m.log.SetSize(outW, outH)
	if firstReady {
		m.log.ShowBuffer(m.sidebar.SelectedName()) // initial state
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
	if !m.log.Ready() {
		return altView("loading...")
	}

	left := m.sidebar.View(m.focus == focusSidebar)
	right := m.log.View(m.focus == focusLog)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	switch m.mode { // popup floating over the view
	case modeAuth:
		body = common.Center(body, m.creds.View())
	case modeAdd:
		body = common.Center(body, m.picker.View())
	case modeConfirm:
		body = common.Center(body, m.confirm.View())
	}
	return altView(body + "\n" + m.statusLine() + "\n" + helpStyle.Render(m.helpFooter()))
}

// helpFooter is the key-hint line for the current focus: scroll hints while the
// log pane is focused, the normal bindings otherwise.
func (m *UI) helpFooter() string {
	if m.focus == focusLog {
		return helpKeysLog
	}
	return helpKeys
}

// altView wraps content in a tea.View on the alt screen (v2: alt screen is
// declarative, set per-frame instead of via a program option).
func altView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion // enable mouse wheel for log scrolling
	return v
}

func (m *UI) statusLine() string {
	line := " " + m.log.State().Badge()
	if m.log.State() == StateError && m.log.Err() != "" {
		line += nameStyle.Render(": " + m.log.Err())
	} else if name := m.log.ActiveName(); name != "" {
		line += "  " + nameStyle.Render(name)
	}
	return line
}
