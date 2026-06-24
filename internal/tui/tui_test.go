package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/omartelo/lazyovpn/internal/tui/utils"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

func TestDims(t *testing.T) {
	m := model{w: 100, h: 30}
	sideW, sideH, outW, outH := m.dims()

	if sideW != sidebarWidth-4 {
		t.Errorf("sideW = %d, want %d", sideW, sidebarWidth-4)
	}
	if outW != 100-sidebarWidth-4 {
		t.Errorf("outW = %d, want %d", outW, 100-sidebarWidth-4)
	}
	if sideH != outH {
		t.Errorf("sideH (%d) != outH (%d), panes must match height", sideH, outH)
	}
	if sideH != 30-2-2 {
		t.Errorf("sideH = %d, want %d", sideH, 30-2-2)
	}
}

func TestDimsClampsToMin(t *testing.T) {
	m := model{w: 1, h: 1} // tiny terminal
	sideW, sideH, outW, outH := m.dims()
	for _, v := range []int{sideW, sideH, outW, outH} {
		if v < 1 {
			t.Errorf("dimension %d below minimum 1", v)
		}
	}
}

// Invariant #4: a log message from an old connection must be dropped.
func TestStaleLogIgnored(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())

	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(model)

	foreign := make(chan string) // not the model's logCh (which is nil here)
	out, cmd := m.Update(utils.LogMsg{Ch: foreign, Line: "ghost"})
	if cmd != nil {
		t.Error("stale LogMsg produced a command, want nil")
	}
	if out.(model).terminal.State().Badge() == "" {
		t.Error("model corrupted by stale log")
	}
}
