package model

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newSizedLog() Log {
	t := NewLog()
	t.SetSize(40, 10)
	return t
}

func TestLogInitialState(t *testing.T) {
	term := newSizedLog()
	if term.State() != StateIdle {
		t.Errorf("initial State() = %v, want StateIdle", term.State())
	}
	if !term.Ready() {
		t.Error("Ready() = false after SetSize, want true")
	}
}

func TestLogStartConnection(t *testing.T) {
	term := newSizedLog()
	term.StartConnection("alpha")

	if term.State() != StateConnecting {
		t.Errorf("State() = %v, want StateConnecting", term.State())
	}
	if term.ActiveName() != "alpha" {
		t.Errorf("ActiveName() = %q, want alpha", term.ActiveName())
	}
}

// Tunnel-up is no longer scraped from the log (a low-verb/mute config omits the
// marker): AppendLog leaves the state alone, MarkConnected (driven by the
// management state poll) is what flips it to connected.
func TestLogConnectedDetection(t *testing.T) {
	term := newSizedLog()
	term.StartConnection("alpha")

	term.AppendLog("Mon Jun 24 ... Initialization Sequence Completed")
	if term.State() != StateConnecting {
		t.Errorf("State() = %v, want StateConnecting — log content must not flip the badge", term.State())
	}

	term.MarkConnected()
	if term.State() != StateConnected {
		t.Errorf("State() = %v after MarkConnected, want StateConnected", term.State())
	}
}

func TestLogBufferAccumulates(t *testing.T) {
	term := newSizedLog()
	term.StartConnection("alpha")
	term.AppendLog("line one")
	term.AppendLog("line two")

	got := term.buffers["alpha"].String()
	if !strings.Contains(got, "line one") || !strings.Contains(got, "line two") {
		t.Errorf("buffer = %q, want both lines", got)
	}
}

func TestLogStartConnectionClearsBuffer(t *testing.T) {
	term := newSizedLog()
	term.StartConnection("alpha")
	term.AppendLog("stale line")
	term.StartConnection("alpha") // reconnect

	if got := term.buffers["alpha"].String(); got != "" {
		t.Errorf("buffer after reconnect = %q, want empty", got)
	}
}

func TestLogMarkClosed(t *testing.T) {
	term := newSizedLog()
	term.StartConnection("alpha")
	term.AppendLog("running")
	term.MarkClosed()

	if term.State() != StateDisconnected {
		t.Errorf("State() = %v, want StateDisconnected", term.State())
	}
	if term.ActiveName() != "" {
		t.Errorf("ActiveName() = %q after close, want empty", term.ActiveName())
	}
	if got := term.buffers["alpha"].String(); !strings.Contains(got, "[process exited]") {
		t.Errorf("buffer = %q, want exit marker", got)
	}
}

func TestLogMarkDisconnected(t *testing.T) {
	term := newSizedLog()
	term.StartConnection("alpha")
	term.MarkDisconnected()

	if term.State() != StateDisconnected {
		t.Errorf("State() = %v, want StateDisconnected", term.State())
	}
	if term.ActiveName() != "" {
		t.Errorf("ActiveName() = %q after disconnect, want empty", term.ActiveName())
	}
}

// SetError always records the message, but flips the badge to error only when
// no active connection owns it — an operation error (failed delete, unreadable
// config) must not repaint a live tunnel's badge or derail the state machine
// keyed on it (state poll, auto-reconnect).
func TestLogSetError(t *testing.T) {
	tests := []struct {
		name  string
		state ConnState
		want  ConnState
	}{
		{"idle flips to error", StateIdle, StateError},
		{"disconnected flips to error", StateDisconnected, StateError},
		{"connecting keeps its badge", StateConnecting, StateConnecting},
		{"connected keeps its badge", StateConnected, StateConnected},
		{"reconnecting keeps its badge", StateReconnecting, StateReconnecting},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := newSizedLog()
			term.state = tt.state
			term.SetError("boom")
			if term.State() != tt.want {
				t.Errorf("State() = %v, want %v", term.State(), tt.want)
			}
			if term.Err() != "boom" {
				t.Errorf("Err() = %q, want boom", term.Err())
			}
		})
	}
}

// Every state transition supersedes a stale operation error — an old message
// left behind would read as the reason for the new state (e.g. a failed delete
// presented as why the tunnel closed).
func TestLogErrorClearing(t *testing.T) {
	tests := []struct {
		name       string
		transition func(*Log)
	}{
		{"MarkConnected", func(l *Log) { l.MarkConnected() }},
		{"MarkClosed", func(l *Log) { l.MarkClosed() }},
		{"MarkDisconnected", func(l *Log) { l.MarkDisconnected() }},
		{"MarkReconnecting", func(l *Log) { l.MarkReconnecting(1) }},
		{"StartConnection", func(l *Log) { l.StartConnection("alpha") }},
		{"ClearError", func(l *Log) { l.ClearError() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := newSizedLog()
			term.state = StateConnected // a live tunnel keeps the badge on SetError
			term.SetError("boom")
			tt.transition(&term)
			if term.Err() != "" {
				t.Errorf("Err() = %q after %s, want cleared", term.Err(), tt.name)
			}
		})
	}
}

// ClearError on an error that owns the badge settles the badge back to idle —
// a bare "error" badge with no message behind it leaves nothing to act on. A
// live connection's badge is untouched.
func TestLogClearErrorSettlesBadge(t *testing.T) {
	term := newSizedLog()
	term.SetError("boom") // idle -> StateError
	term.ClearError()
	if term.State() != StateIdle {
		t.Errorf("State() = %v after ClearError, want StateIdle", term.State())
	}

	term = newSizedLog()
	term.state = StateConnected
	term.SetError("boom") // badge stays connected
	term.ClearError()
	if term.State() != StateConnected {
		t.Errorf("State() = %v after ClearError while connected, want StateConnected", term.State())
	}
}

func TestLogStaleLogIgnoredWhenSwitched(t *testing.T) {
	// Showing another connection: active still buffers, but the viewport is not
	// the active one — the active buffer keeps filling regardless.
	term := newSizedLog()
	term.StartConnection("alpha")
	term.ShowBuffer("beta") // user navigated away
	term.AppendLog("still arriving")

	if got := term.buffers["alpha"].String(); !strings.Contains(got, "still arriving") {
		t.Errorf("active buffer = %q, want the line even while showing another", got)
	}
}

// While tailing (viewport at the bottom) new log lines keep the view pinned to
// the bottom; once the user scrolls up, new lines must not yank it back down.
func TestAppendLogTailFollowAndLock(t *testing.T) {
	term := NewLog()
	term.SetSize(40, 5) // content will exceed 5 rows
	term.StartConnection("alpha")
	for i := 0; i < 50; i++ {
		term.AppendLog(fmt.Sprintf("line %d", i))
	}
	if !term.vp.AtBottom() {
		t.Fatal("tailing should keep the viewport at the bottom")
	}

	term.vp.GotoTop() // user scrolls up to read history
	term.AppendLog("a fresh line")
	if term.vp.AtBottom() {
		t.Error("new log yanked the viewport to the bottom while scrolled up")
	}
}

// Scroll forwards a navigation key to the viewport: pgup moves off the bottom.
func TestLogScroll(t *testing.T) {
	term := NewLog()
	term.SetSize(40, 5)
	term.StartConnection("alpha")
	for i := 0; i < 50; i++ {
		term.AppendLog(fmt.Sprintf("line %d", i))
	}
	if !term.vp.AtBottom() {
		t.Fatal("expected to start at the bottom")
	}

	term.Scroll(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if term.vp.AtBottom() {
		t.Error("pgup via Scroll did not move the viewport off the bottom")
	}
}

func TestConnStateBadge(t *testing.T) {
	// Every state renders a non-empty, distinct badge.
	states := []ConnState{StateIdle, StateConnecting, StateConnected, StateDisconnected, StateError, StateReconnecting}
	seen := map[string]bool{}
	for _, s := range states {
		b := s.Badge()
		if b == "" {
			t.Errorf("Badge() for state %d is empty", s)
		}
		if seen[b] {
			t.Errorf("Badge() for state %d duplicates another", s)
		}
		seen[b] = true
	}
}
