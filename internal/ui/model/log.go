package model

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
)

// ConnState is the lifecycle of the active connection.
type ConnState int

const (
	StateIdle ConnState = iota
	StateConnecting
	StateConnected
	StateDisconnected
	StateError
	StateReconnecting // tunnel dropped on its own; waiting to auto-redial
)

// Badge renders the colored status indicator for a state.
func (s ConnState) Badge() string {
	label, color := "idle", common.Muted
	switch s {
	case StateConnecting:
		label, color = "connecting...", common.Warn
	case StateConnected:
		label, color = "connected", common.Connected
	case StateDisconnected:
		label, color = "disconnected", common.Muted
	case StateError:
		label, color = "error", common.Danger
	case StateReconnecting:
		label, color = "reconnecting...", common.Warn
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render("● " + label)
}

// Log is the output panel: per-connection log buffers, a viewport, and connection state.
type Log struct {
	vp         viewport.Model
	buffers    map[string]*strings.Builder // accumulated output per connection
	activeName string                      // connection feeding the live stream
	shownName  string                      // connection shown in the viewport
	state      ConnState
	errMsg     string
	w, h       int // inner content size (inside the border)
	ready      bool
}

// NewLog builds an empty log panel.
func NewLog() Log {
	return Log{
		buffers: map[string]*strings.Builder{},
		state:   StateIdle,
	}
}

// SetSize sets the inner content size (inside the border), creating the viewport
// on first call.
func (l *Log) SetSize(w, h int) {
	l.w, l.h = w, h
	if !l.ready {
		l.vp = viewport.New() // v2: size via setters, not constructor args
		l.ready = true
	}
	l.vp.SetWidth(w)
	l.vp.SetHeight(h)
}

// Ready reports whether the viewport has been sized at least once.
func (l Log) Ready() bool { return l.ready }

// StartConnection resets the buffer for name and enters the connecting state.
func (l *Log) StartConnection(name string) {
	l.activeName = name
	l.buffers[name] = &strings.Builder{} // reconnecting clears the old log
	l.shownName = name
	l.state = StateConnecting
	l.errMsg = ""
	l.vp.SetContent("")
	l.vp.GotoTop()
}

// AppendLog records one line from the active stream. Tunnel-up is detected
// separately via the management state poll (see vpn.QueryState), not by scanning
// log content — a low-verb/mute config can omit the "Initialization Sequence
// Completed" line entirely while still being connected.
func (l *Log) AppendLog(line string) {
	b := l.buffers[l.activeName]
	if b == nil {
		return
	}
	b.WriteString(line + "\n")
	if l.shownName == l.activeName {
		// Tail the live stream, but only while the user is already at the bottom.
		// If they have scrolled up to read back, leave the viewport where it is.
		follow := l.vp.AtBottom()
		l.vp.SetContent(b.String())
		if follow {
			l.vp.GotoBottom()
		}
	}
}

// Scroll forwards a navigation message to the viewport (line/page scrolling via
// its default keymap). The UI calls this while the log pane is focused so the
// user can read back through the output.
func (l *Log) Scroll(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.vp, cmd = l.vp.Update(msg)
	return cmd
}

// MarkConnected enters the connected state — the tunnel is up. Driven by the
// management state poll (vpn.QueryState), not by log content. A stale operation
// error is superseded by the tunnel coming up.
func (l *Log) MarkConnected() {
	l.state = StateConnected
	l.errMsg = ""
}

// MarkClosed records that the active stream ended (process exited). Like every
// state transition, it supersedes a stale operation error — otherwise the old
// message would read as the reason the connection ended.
func (l *Log) MarkClosed() {
	if b := l.buffers[l.activeName]; b != nil {
		b.WriteString("\n[process exited]\n")
	}
	if l.shownName == l.activeName {
		l.ShowBuffer(l.activeName)
	}
	l.activeName = ""
	l.state = StateDisconnected
	l.errMsg = ""
}

// MarkReconnecting notes the live tunnel dropped and is being auto-redialed
// (attempt n of maxReconnects). It keeps activeName — the same connection is
// coming back — and the buffer/badge show the gap.
func (l *Log) MarkReconnecting(n int) {
	if b := l.buffers[l.activeName]; b != nil {
		fmt.Fprintf(b, "\n[connection lost — reconnecting (%d/%d)]\n", n, maxReconnects)
		if l.shownName == l.activeName {
			l.ShowBuffer(l.activeName)
		}
	}
	l.state = StateReconnecting
	l.errMsg = "" // the transition supersedes a stale operation error
}

// MarkDisconnected enters the disconnected state (user-initiated). The
// transition supersedes a stale operation error — it must not read as the
// reason for the disconnect.
func (l *Log) MarkDisconnected() {
	l.activeName = ""
	l.state = StateDisconnected
	l.errMsg = ""
}

// SetError records msg, flipping the badge to the error state only when no
// active connection owns it. An operation error (failed delete, unreadable
// config) while a tunnel is live/coming up/redialing must not repaint the badge
// or derail the state machine keyed on it (state poll, auto-reconnect); the
// message still surfaces in the status line.
func (l *Log) SetError(msg string) {
	l.errMsg = msg
	switch l.state {
	case StateConnecting, StateConnected, StateReconnecting:
		// the live connection keeps its badge
	default:
		l.state = StateError
	}
}

// ClearError drops a stale operation error — a new action supersedes it. When
// the error owned the badge, the badge settles back to idle too: a bare "error"
// with no message behind it would leave nothing to act on.
func (l *Log) ClearError() {
	l.errMsg = ""
	if l.state == StateError {
		l.state = StateIdle
	}
}

// ShowBuffer renders connection name's output into the viewport (placeholder if empty).
func (l *Log) ShowBuffer(name string) {
	l.shownName = name
	if b, ok := l.buffers[name]; ok {
		l.vp.SetContent(b.String())
	} else {
		l.vp.SetContent("(no output — press enter to connect)")
	}
	l.vp.GotoBottom()
}

// RenameBuffer moves a connection's stored output from one name to another so
// its log history stays visible after a rename. No-op when nothing is stored.
func (l *Log) RenameBuffer(from, to string) {
	if from == to {
		return
	}
	if b, ok := l.buffers[from]; ok {
		l.buffers[to] = b
		delete(l.buffers, from)
	}
	if l.shownName == from {
		l.shownName = to
	}
	if l.activeName == from {
		l.activeName = to
	}
}

// DropBuffer discards a connection's stored output (a delete). No-op if nothing
// is stored for it.
func (l *Log) DropBuffer(name string) {
	delete(l.buffers, name)
}

// View renders the bordered panel.
func (l Log) View(focused bool) string {
	title := "log"
	if l.shownName != "" {
		title = "log — " + l.shownName
	}
	return common.TitledBox(title, l.vp.View(), l.w, l.h, focused)
}

// State returns the current connection state.
func (l Log) State() ConnState { return l.state }

// ActiveName returns the connection feeding the live stream, or "".
func (l Log) ActiveName() string { return l.activeName }

// Err returns the last error message.
func (l Log) Err() string { return l.errMsg }
