package model

import (
	"strings"
	"testing"

	"github.com/omartelo/lazyovpn/internal/vpn"
)

func sampleConfigs() []vpn.Config {
	return []vpn.Config{
		{Name: "alpha", Path: "/etc/openvpn/alpha.ovpn"},
		{Name: "beta", Path: "/etc/openvpn/beta.conf"},
	}
}

func TestSidebarSelection(t *testing.T) {
	s := NewSidebar(sampleConfigs(), &shared{})

	if got := s.SelectedName(); got != "alpha" {
		t.Errorf("SelectedName() = %q, want alpha", got)
	}

	cfg, ok := s.SelectedConfig()
	if !ok {
		t.Fatal("SelectedConfig() ok = false, want true")
	}
	if cfg.Name != "alpha" || cfg.Path != "/etc/openvpn/alpha.ovpn" {
		t.Errorf("SelectedConfig() = %+v, want alpha config", cfg)
	}
}

func TestSidebarMoveClamps(t *testing.T) {
	s := NewSidebar(sampleConfigs(), &shared{}) // alpha, beta

	s.Move(-1) // already at the top
	if got := s.SelectedName(); got != "alpha" {
		t.Errorf("Move(-1) at top = %q, want alpha (clamped)", got)
	}

	s.Move(1)
	if got := s.SelectedName(); got != "beta" {
		t.Errorf("Move(1) = %q, want beta", got)
	}

	s.Move(5) // past the end
	if got := s.SelectedName(); got != "beta" {
		t.Errorf("Move past end = %q, want beta (clamped)", got)
	}
}

// rowKind decides the per-row style; the cursor wins when it overlaps the
// connected row.
func TestSidebarRowKind(t *testing.T) {
	s := NewSidebar(sampleConfigs(), &shared{}) // cursor = 0

	if k := s.rowKind(0, false); k != rowCursor {
		t.Errorf("rowKind(cursor) = %d, want rowCursor", k)
	}
	if k := s.rowKind(1, true); k != rowConnected {
		t.Errorf("rowKind(connected, non-cursor) = %d, want rowConnected", k)
	}
	if k := s.rowKind(0, true); k != rowCursor {
		t.Errorf("rowKind(cursor that is also connected) = %d, want rowCursor (cursor wins)", k)
	}
	if k := s.rowKind(1, false); k != rowPlain {
		t.Errorf("rowKind(plain) = %d, want rowPlain", k)
	}
}

// The ● marker tracks shared.connected and lands on the connected row only;
// flipping the shared field repaints it without any setter call.
func TestSidebarMarksConnected(t *testing.T) {
	sh := &shared{connected: "alpha"}
	s := NewSidebar(sampleConfigs(), sh)
	s.SetSize(20, 4)

	var alphaLine, betaLine string
	for _, ln := range strings.Split(s.View(true), "\n") {
		if strings.Contains(ln, "alpha") {
			alphaLine = ln
		}
		if strings.Contains(ln, "beta") {
			betaLine = ln
		}
	}
	if !strings.Contains(alphaLine, "●") {
		t.Error("connected row (alpha) should show the ● marker")
	}
	if strings.Contains(betaLine, "●") {
		t.Error("non-connected row (beta) should not show the ● marker")
	}

	sh.connected = "" // pull, not push: clearing shared clears the marker
	if strings.Contains(s.View(true), "●") {
		t.Error("no ● expected once nothing is connected")
	}
}

func TestSidebarAddConfig(t *testing.T) {
	s := NewSidebar(sampleConfigs(), &shared{}) // alpha, beta
	s.AddConfig(vpn.Config{Name: "gamma", Path: "/etc/openvpn/gamma.ovpn"})
	if got := len(s.configs); got != 3 {
		t.Fatalf("configs = %d after add, want 3", got)
	}

	// re-adding the same path is a no-op (dedup)
	s.AddConfig(vpn.Config{Name: "gamma", Path: "/etc/openvpn/gamma.ovpn"})
	if got := len(s.configs); got != 3 {
		t.Errorf("configs = %d after duplicate add, want 3", got)
	}
}

func TestSidebarRemoveConfig(t *testing.T) {
	t.Run("cursor on removed last row clamps to previous", func(t *testing.T) {
		s := NewSidebar(sampleConfigs(), &shared{}) // alpha, beta
		s.Move(1)                                   // cursor on beta (last)
		s.RemoveConfig(vpn.Config{Name: "beta", Path: "/etc/openvpn/beta.conf"})

		cfg, ok := s.SelectedConfig()
		if !ok {
			t.Fatal("SelectedConfig() ok = false after remove, want true")
		}
		if cfg.Name != "alpha" {
			t.Errorf("SelectedConfig() = %q after removing last row, want alpha", cfg.Name)
		}
	})

	t.Run("removing the only config leaves a sane empty list", func(t *testing.T) {
		s := NewSidebar([]vpn.Config{{Name: "solo", Path: "/etc/openvpn/solo.ovpn"}}, &shared{})
		s.RemoveConfig(vpn.Config{Name: "solo", Path: "/etc/openvpn/solo.ovpn"})

		if _, ok := s.SelectedConfig(); ok {
			t.Error("SelectedConfig() ok = true on emptied list, want false")
		}
		s.Move(1) // must not panic after the clamp to an empty list
		if got := s.SelectedName(); got != "" {
			t.Errorf("SelectedName() = %q on emptied list, want empty", got)
		}
	})

	t.Run("cursor before the removed row is untouched", func(t *testing.T) {
		s := NewSidebar(sampleConfigs(), &shared{}) // cursor on alpha
		s.RemoveConfig(vpn.Config{Name: "beta", Path: "/etc/openvpn/beta.conf"})
		if got := s.SelectedName(); got != "alpha" {
			t.Errorf("SelectedName() = %q, want alpha", got)
		}
	})
}

func TestSidebarEmpty(t *testing.T) {
	s := NewSidebar(nil, &shared{})
	if got := s.SelectedName(); got != "" {
		t.Errorf("SelectedName() = %q, want empty", got)
	}
	if _, ok := s.SelectedConfig(); ok {
		t.Error("SelectedConfig() ok = true on empty list, want false")
	}
	s.Move(1) // must not panic on an empty list
}
