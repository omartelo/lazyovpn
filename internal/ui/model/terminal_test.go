package model

import (
	"strings"
	"testing"
)

func newSizedTerminal() Terminal {
	t := NewTerminal()
	t.SetSize(40, 10)
	return t
}

func TestTerminalInitialState(t *testing.T) {
	term := newSizedTerminal()
	if term.State() != StateIdle {
		t.Errorf("initial State() = %v, want StateIdle", term.State())
	}
	if !term.Ready() {
		t.Error("Ready() = false after SetSize, want true")
	}
}

func TestTerminalStartConnection(t *testing.T) {
	term := newSizedTerminal()
	term.StartConnection("alpha")

	if term.State() != StateConnecting {
		t.Errorf("State() = %v, want StateConnecting", term.State())
	}
	if term.ActiveName() != "alpha" {
		t.Errorf("ActiveName() = %q, want alpha", term.ActiveName())
	}
}

func TestTerminalConnectedDetection(t *testing.T) {
	term := newSizedTerminal()
	term.StartConnection("alpha")

	term.AppendLog("Mon Jun 24 ... TLS handshake")
	if term.State() != StateConnecting {
		t.Errorf("State() = %v before marker, want StateConnecting", term.State())
	}

	term.AppendLog("Mon Jun 24 ... " + connectedMarker)
	if term.State() != StateConnected {
		t.Errorf("State() = %v after marker, want StateConnected", term.State())
	}
}

func TestTerminalBufferAccumulates(t *testing.T) {
	term := newSizedTerminal()
	term.StartConnection("alpha")
	term.AppendLog("line one")
	term.AppendLog("line two")

	got := term.buffers["alpha"].String()
	if !strings.Contains(got, "line one") || !strings.Contains(got, "line two") {
		t.Errorf("buffer = %q, want both lines", got)
	}
}

func TestTerminalStartConnectionClearsBuffer(t *testing.T) {
	term := newSizedTerminal()
	term.StartConnection("alpha")
	term.AppendLog("stale line")
	term.StartConnection("alpha") // reconnect

	if got := term.buffers["alpha"].String(); got != "" {
		t.Errorf("buffer after reconnect = %q, want empty", got)
	}
}

func TestTerminalMarkClosed(t *testing.T) {
	term := newSizedTerminal()
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

func TestTerminalMarkDisconnected(t *testing.T) {
	term := newSizedTerminal()
	term.StartConnection("alpha")
	term.MarkDisconnected()

	if term.State() != StateDisconnected {
		t.Errorf("State() = %v, want StateDisconnected", term.State())
	}
	if term.ActiveName() != "" {
		t.Errorf("ActiveName() = %q after disconnect, want empty", term.ActiveName())
	}
}

func TestTerminalSetError(t *testing.T) {
	term := newSizedTerminal()
	term.SetError("boom")
	if term.State() != StateError {
		t.Errorf("State() = %v, want StateError", term.State())
	}
	if term.Err() != "boom" {
		t.Errorf("Err() = %q, want boom", term.Err())
	}
}

func TestTerminalStaleLogIgnoredWhenSwitched(t *testing.T) {
	// Showing another connection: active still buffers, but the viewport is not
	// the active one — the active buffer keeps filling regardless.
	term := newSizedTerminal()
	term.StartConnection("alpha")
	term.ShowBuffer("beta") // user navigated away
	term.AppendLog("still arriving")

	if got := term.buffers["alpha"].String(); !strings.Contains(got, "still arriving") {
		t.Errorf("active buffer = %q, want the line even while showing another", got)
	}
}

func TestConnStateBadge(t *testing.T) {
	// Every state renders a non-empty, distinct badge.
	states := []ConnState{StateIdle, StateConnecting, StateConnected, StateDisconnected, StateError}
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
