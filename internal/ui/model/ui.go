// ui.go holds UI, the sole bubbletea model and the app's brain: it owns every
// piece of global state (mode, layout, key routing, the live log stream) and
// drives the sidebar + log panels. Message routing is centralized in one Update
// switch; the panels are imperative helpers it calls (see the package CLAUDE.md).
package model

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
	"github.com/omartelo/lazyovpn/internal/ui/dialog"
	"github.com/omartelo/lazyovpn/internal/ui/utils"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

// Auto-reconnect tuning. When a live tunnel's process exits on its own (not a
// user disconnect), lazyovpn redials the same config up to maxReconnects times,
// waiting reconnectDelay between tries. A connection that stayed up at least
// stableUptime is treated as healthy and earns the full retry budget back, so a
// rare nightly drop keeps reconnecting forever while a tight flap gives up.
const (
	maxReconnects  = 5
	reconnectDelay = 3 * time.Second
	stableUptime   = 30 * time.Second
)

// reconnectMsg fires reconnectDelay after a drop to redial the connection.
type reconnectMsg struct{}

// statePollInterval is how often the UI polls openvpn's management state to
// detect tunnel-up while a connection is coming up.
const statePollInterval = 1 * time.Second

// statePollMsg ticks the management-state poll; stateResultMsg carries the state
// name a poll read (tagged with its socket so a poll from a previous connection
// can't flip the badge for the current one — the stale-channel guard, by socket).
type (
	statePollMsg   struct{}
	stateResultMsg struct{ sock, state string }
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
	helpKeys    = "↑/↓ j/k: navigate · enter: connect · tab: log · a: add · d: disconnect · x: menu · q: quit"
	helpKeysLog = "↑/↓ j/k: scroll · pgup/pgdn: page · tab/esc: back · q: quit"
)

// appMode is the top-level interaction mode: which overlay (if any) currently
// owns input. modeNormal routes keys to the panels; every other mode means a
// modal or confirm popup is up and consuming input.
type appMode uint8

const (
	modeNormal  appMode = iota
	modeAuth            // credential modal is capturing input
	modeAdd             // import-connection modal is open
	modeConfirm         // a yes/no confirm popup is up (forget creds, disconnect)
	modeMenu            // the per-connection action menu is open
	modeRename          // the rename-connection prompt is open
)

// pane identifies which panel receives navigation keys while no overlay is up
// (modeNormal). focusLog hands keys to the log viewport for scrolling.
type pane uint8

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
	menu    dialog.Menu
	rename  dialog.Rename
	mode    appMode
	focus   pane           // which pane gets nav keys in modeNormal
	pending vpn.Config     // connection awaiting credentials
	confirm dialog.Confirm // the active yes/no popup (forget creds, disconnect)
	mgr     *vpn.Manager
	logCh   <-chan string // live stream of the active connection
	sh      shared        // global state the panels read by pointer
	w, h    int

	// Auto-reconnect state for the live connection. No credentials are held in
	// memory: reArmed is true only for a config we can both track (not daemon)
	// and silently redial — no-auth, or creds saved in the keyring, fetched fresh
	// at redial time via redialCreds (never retained). connectedAt stamps
	// tunnel-up so a stable session earns its retry budget back.
	reCfg       vpn.Config
	reArmed     bool
	reAttempts  int
	connectedAt time.Time
}

// New builds the initial UI from the already-discovered configs.
func New(configs []vpn.Config, mgr *vpn.Manager) *UI {
	m := &UI{mgr: mgr}
	m.sidebar = NewSidebar(configs, &m.sh) // sidebar reads m.sh.connected by pointer
	m.log = NewLog()
	m.creds = dialog.NewCredentials()
	m.picker = dialog.NewFilePicker("add connection")
	m.menu = dialog.NewMenu()
	m.rename = dialog.NewRename()
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
		// Display only: a log line never changes connection state — tunnel-up is
		// detected by the management-state poll (stateResultMsg), not log content.
		m.log.AppendLog(msg.Line)
		return m, utils.WaitForLog(m.logCh)

	case utils.LogClosedMsg:
		if msg.Ch == m.logCh {
			return m, m.handleDrop()
		}
		return m, nil

	case statePollMsg:
		// Poll only while a connection is coming up; stop once it is connected,
		// gone, or settled. The dial runs off the UI goroutine in the returned cmd.
		if m.logCh == nil || m.log.State() != StateConnecting {
			return m, nil
		}
		sock := m.mgr.MgmtSock()
		if sock == "" {
			return m, m.pollStateTick() // openvpn hasn't opened the socket yet
		}
		return m, func() tea.Msg { return stateResultMsg{sock: sock, state: vpn.QueryState(sock)} }

	case stateResultMsg:
		// Drop a result from a previous connection (switch/reconnect) — the socket
		// identifies it, like the log stream's stale-channel guard.
		if msg.sock != m.mgr.MgmtSock() || m.logCh == nil || m.log.State() != StateConnecting {
			return m, nil
		}
		if msg.state == vpn.StateConnected {
			m.log.MarkConnected()
			if m.connectedAt.IsZero() {
				m.connectedAt = time.Now() // tunnel-up: start the stability clock
			}
			m.syncConnected()
			return m, nil // done polling
		}
		return m, m.pollStateTick() // still coming up — poll again

	case reconnectMsg:
		// Redial only if still waiting (not cancelled) and nothing else has
		// since connected — a stale timer after the user reconnected is dropped.
		if m.log.State() == StateReconnecting && m.logCh == nil {
			if user, pass, ok := redialCreds(m.reCfg); ok {
				return m.connect(m.reCfg, user, pass)
			}
			// Keyring entry vanished since the drop (forgotten mid-connection):
			// nothing to redial with, so settle as closed.
			m.settleClosed()
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
	case modeMenu:
		return m.updateMenu(msg)
	case modeRename:
		return m.updateRename(msg)
	}

	// In normal mode the focused pane captures navigation. The log pane
	// takes keys for scrolling; the sidebar keeps the connect/add/etc bindings.
	if m.focus == focusLog {
		return m.updateLogNav(msg)
	}

	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "q", "ctrl+c":
			return m.quit()
		case "a":
			m.mode = modeAdd
			return m, m.picker.Open()
		case "enter":
			return m.enter()
		case "tab":
			m.focus = focusLog
			return m, nil
		case "d":
			// Disconnecting tears down a live tunnel, so confirm first. Only prompt
			// when something is actually connected (logCh != nil); otherwise d is a
			// no-op — nothing to disconnect.
			if m.logCh != nil {
				m.confirm = dialog.NewConfirm("disconnect",
					"Disconnect from\n\""+m.log.ActiveName()+"\"?",
					func() tea.Cmd { m.disconnect(); return nil })
				m.mode = modeConfirm
			} else if m.log.State() == StateReconnecting {
				// Cancel a pending auto-reconnect (no tunnel yet, so no confirm).
				m.disarmReconnect()
				m.log.MarkDisconnected()
				m.syncConnected()
			}
			return m, nil
		case "x":
			// Open the per-connection action menu (forget creds lives in it).
			if name := m.sidebar.SelectedName(); name != "" {
				m.menu.Open(name)
				m.mode = modeMenu
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
		cmd := m.confirm.Yes()
		m.mode = modeNormal
		return m, cmd
	case "n", "esc":
		m.mode = modeNormal
	}
	return m, nil
}

// updateMenu handles input while the per-connection action menu is open. Each
// action has a key; esc/x close the menu. The selected connection was fixed when
// the menu opened, so the actions read it back through the sidebar selection.
func (m *UI) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "r":
		return m.openRename() // swaps modeMenu → modeRename (or no-op)
	case "f":
		m.mode = modeNormal
		m.forgetCreds() // may re-open as the forget-confirm popup
	case "esc", "x":
		m.mode = modeNormal
	}
	return m, nil
}

// forgetCreds opens the forget-credentials confirm for the selected connection,
// but only when it actually has creds saved (no-op otherwise — no popup for a
// no-op). The menu's "f" action.
func (m *UI) forgetCreds() {
	cfg, ok := m.sidebar.SelectedConfig()
	if !ok {
		return
	}
	if _, _, has, _ := vpn.LoadCreds(cfg.Name); !has {
		return
	}
	name := cfg.Name
	m.confirm = dialog.NewConfirm("forget credentials",
		"Forget saved credentials for\n\""+name+"\"?",
		func() tea.Cmd { _ = vpn.ForgetCreds(name); return nil })
	m.mode = modeConfirm
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
			return m.quit()
		}
	}
	return m, m.log.Scroll(msg)
}

// disconnect tears down the live tunnel; it is the action behind the disconnect
// confirm popup. A user disconnect must not trigger auto-reconnect, so it
// disarms first.
func (m *UI) disconnect() {
	m.disarmReconnect()
	_ = m.mgr.Disconnect()
	m.log.MarkDisconnected()
	m.logCh = nil
	m.syncConnected()
}

// quit ends the program. With a live connection it confirms first — q/ctrl+c are
// easy to fat-finger and quitting tears the tunnel down. The teardown runs on
// accept: disarm auto-reconnect and signal openvpn to stop. Disarming and
// nulling logCh first is what stops the quit from leaving the (root) openvpn
// running — without the disarm a late drop could reschedule a redial, and the
// nulled logCh makes the stale-channel guard drop any in-flight LogClosedMsg.
func (m *UI) quit() (tea.Model, tea.Cmd) {
	if m.logCh == nil {
		return m, tea.Quit // nothing connected — just go
	}
	m.confirm = dialog.NewConfirm("quit",
		"\""+m.log.ActiveName()+"\" is still active.\nDisconnect and quit?",
		func() tea.Cmd {
			m.disarmReconnect()
			_ = m.mgr.Disconnect()
			m.logCh = nil
			return tea.Quit
		})
	m.mode = modeConfirm
	return m, nil
}

// handleDrop reacts to the live process exiting on its own (the only LogClosedMsg
// that survives the stale-channel guard). If the tunnel had come up and the
// config is trackable, it schedules an auto-reconnect and returns the timer cmd;
// otherwise it marks the connection closed and returns nil.
func (m *UI) handleDrop() tea.Cmd {
	m.logCh = nil
	wasUp := m.log.State() == StateConnected
	if m.reArmed && wasUp {
		if !m.connectedAt.IsZero() && time.Since(m.connectedAt) >= stableUptime {
			m.reAttempts = 0 // it proved stable: earn the retry budget back
		}
		if m.reAttempts < maxReconnects {
			m.reAttempts++
			m.log.MarkReconnecting(m.reAttempts)
			m.syncConnected()
			return tea.Tick(reconnectDelay, func(time.Time) tea.Msg { return reconnectMsg{} })
		}
	}
	m.settleClosed()
	return nil
}

// settleClosed marks the connection finished and abandons any reconnect effort —
// the shared give-up path (retry budget spent, or no creds left to redial with).
func (m *UI) settleClosed() {
	m.log.MarkClosed()
	m.disarmReconnect()
	m.syncConnected()
}

// armReconnect arms auto-reconnect for cfg, holding no credentials: enabled only
// when the connection is trackable (not a daemon, which forks out of reach) and
// silently redialable (no-auth, or creds in the keyring — see redialCreds). It
// resets the stability clock for the new attempt. Pairs with disarmReconnect.
func (m *UI) armReconnect(cfg vpn.Config) {
	m.reCfg = cfg
	daemon, _ := vpn.HasDaemon(cfg)
	_, _, redialable := redialCreds(cfg)
	m.reArmed = !daemon && redialable
	m.connectedAt = time.Time{}
}

// disarmReconnect forgets the live connection's redial state. Called on user
// disconnect/cancel and when the retry budget runs out. No credentials to wipe —
// none are ever held (see redialCreds).
func (m *UI) disarmReconnect() {
	m.reArmed = false
	m.reAttempts = 0
	m.reCfg = vpn.Config{}
	m.connectedAt = time.Time{}
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
	m.reAttempts = 0 // a manual connect starts with a full retry budget
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

// connect starts the connection, arms auto-reconnect for it (see armReconnect),
// and begins pumping its log stream.
func (m *UI) connect(cfg vpn.Config, username, password string) (tea.Model, tea.Cmd) {
	ch, err := m.mgr.Connect(cfg, username, password)
	if err != nil {
		m.log.SetError(err.Error())
		return m, nil
	}
	m.logCh = ch
	m.armReconnect(cfg)
	m.log.StartConnection(cfg.Name)
	m.syncConnected() // a fresh connect clears any previous green marker
	// Pump the log for display and poll the management state for tunnel-up.
	return m, tea.Batch(utils.WaitForLog(ch), m.pollStateTick())
}

// pollStateTick schedules the next management-state poll. Polling runs while the
// connection is coming up and stops once it reaches connected (see statePollMsg).
func (m *UI) pollStateTick() tea.Cmd {
	return tea.Tick(statePollInterval, func(time.Time) tea.Msg { return statePollMsg{} })
}

// redialCreds resolves the credentials to reconnect cfg with WITHOUT retaining
// them: a no-auth config needs none; a config with saved keyring creds loads
// them on demand. A config that needs auth but has nothing in the keyring is not
// redialable (ok=false) — auto-reconnect is a keyring feature, and credentials
// typed once but not saved are deliberately never held. Any error (unreadable
// config, keyring failure) resolves to not-redialable.
func redialCreds(cfg vpn.Config) (user, pass string, ok bool) {
	needs, err := vpn.NeedsAuth(cfg)
	if err != nil {
		return "", "", false
	}
	if !needs {
		return "", "", true
	}
	user, pass, has, err := vpn.LoadCreds(cfg.Name)
	if err != nil || !has {
		return "", "", false
	}
	return user, pass, true
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
	case modeMenu:
		body = common.Center(body, m.menu.View())
	case modeRename:
		body = common.Center(body, m.rename.View())
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
