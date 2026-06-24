package models

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/tui/components"
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
	label, color := "idle", components.Muted
	switch s {
	case StateConnecting:
		label, color = "connecting...", components.Warn
	case StateConnected:
		label, color = "connected", components.Connected
	case StateDisconnected:
		label, color = "disconnected", components.Muted
	case StateError:
		label, color = "error", components.Danger
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render("● " + label)
}

// Terminal is the output panel: per-connection log buffers, a viewport, and connection state.
type Terminal struct {
	vp         viewport.Model
	buffers    map[string]*strings.Builder // accumulated output per connection
	activeName string                      // connection feeding the live stream
	shownName  string                      // connection shown in the viewport
	state      ConnState
	errMsg     string
	w, h       int // inner content size (inside the border)
	ready      bool
}

// NewTerminal builds an empty output panel.
func NewTerminal() Terminal {
	return Terminal{
		buffers: map[string]*strings.Builder{},
		state:   StateIdle,
	}
}

// SetSize sets the inner content size (inside the border), creating the viewport
// on first call.
func (t *Terminal) SetSize(w, h int) {
	t.w, t.h = w, h
	if !t.ready {
		t.vp = viewport.New() // v2: size via setters, not constructor args
		t.ready = true
	}
	t.vp.SetWidth(w)
	t.vp.SetHeight(h)
}

// Ready reports whether the viewport has been sized at least once.
func (t Terminal) Ready() bool { return t.ready }

// StartConnection resets the buffer for name and enters the connecting state.
func (t *Terminal) StartConnection(name string) {
	t.activeName = name
	t.buffers[name] = &strings.Builder{} // reconnecting clears the old log
	t.shownName = name
	t.state = StateConnecting
	t.errMsg = ""
	t.vp.SetContent("")
	t.vp.GotoTop()
}

// AppendLog records one line from the active stream and detects tunnel-up.
func (t *Terminal) AppendLog(line string) {
	if strings.Contains(line, connectedMarker) {
		t.state = StateConnected
	}
	b := t.buffers[t.activeName]
	if b == nil {
		return
	}
	b.WriteString(line + "\n")
	if t.shownName == t.activeName {
		t.vp.SetContent(b.String())
		t.vp.GotoBottom()
	}
}

// MarkClosed records that the active stream ended (process exited).
func (t *Terminal) MarkClosed() {
	if b := t.buffers[t.activeName]; b != nil {
		b.WriteString("\n[process exited]\n")
	}
	if t.shownName == t.activeName {
		t.ShowBuffer(t.activeName)
	}
	t.activeName = ""
	t.state = StateDisconnected
}

// MarkDisconnected enters the disconnected state (user-initiated).
func (t *Terminal) MarkDisconnected() {
	t.activeName = ""
	t.state = StateDisconnected
}

// SetError enters the error state with a message.
func (t *Terminal) SetError(msg string) {
	t.state = StateError
	t.errMsg = msg
}

// ShowBuffer renders connection name's output into the viewport (placeholder if empty).
func (t *Terminal) ShowBuffer(name string) {
	t.shownName = name
	if b, ok := t.buffers[name]; ok {
		t.vp.SetContent(b.String())
	} else {
		t.vp.SetContent("(no output — press enter to connect)")
	}
	t.vp.GotoBottom()
}

// View renders the bordered panel.
func (t Terminal) View(focused bool) string {
	title := "terminal"
	if t.shownName != "" {
		title = "terminal — " + t.shownName
	}
	return components.TitledBox(title, t.vp.View(), t.w, t.h, focused)
}

// State returns the current connection state.
func (t Terminal) State() ConnState { return t.state }

// ActiveName returns the connection feeding the live stream, or "".
func (t Terminal) ActiveName() string { return t.activeName }

// Err returns the last error message.
func (t Terminal) Err() string { return t.errMsg }
