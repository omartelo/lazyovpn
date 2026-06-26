package model

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
)

// connectedMarker is the openvpn log line that signals the tunnel is up.
const connectedMarker = "Initialization Sequence Completed"

// ConnState is the lifecycle of the active connection.
type ConnState int

const (
	StateIdle ConnState = iota
	StateConnecting
	StateConnected
	StateDisconnected
	StateError
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

// AppendLog records one line from the active stream and detects tunnel-up.
func (l *Log) AppendLog(line string) {
	if strings.Contains(line, connectedMarker) {
		l.state = StateConnected
	}
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

// MarkClosed records that the active stream ended (process exited).
func (l *Log) MarkClosed() {
	if b := l.buffers[l.activeName]; b != nil {
		b.WriteString("\n[process exited]\n")
	}
	if l.shownName == l.activeName {
		l.ShowBuffer(l.activeName)
	}
	l.activeName = ""
	l.state = StateDisconnected
}

// MarkDisconnected enters the disconnected state (user-initiated).
func (l *Log) MarkDisconnected() {
	l.activeName = ""
	l.state = StateDisconnected
}

// SetError enters the error state with a message.
func (l *Log) SetError(msg string) {
	l.state = StateError
	l.errMsg = msg
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
