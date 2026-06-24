package models

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"

	"github.com/omartelo/lazyovpn/internal/vpn"
)

func sampleConfigs() []vpn.Config {
	return []vpn.Config{
		{Name: "alpha", Path: "/etc/openvpn/alpha.ovpn"},
		{Name: "beta", Path: "/etc/openvpn/beta.conf"},
	}
}

func TestSidebarSelection(t *testing.T) {
	s := NewSidebar(sampleConfigs())

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

	if s.Filtering() {
		t.Error("Filtering() = true on a fresh list, want false")
	}
}

func TestConnDelegateMarksConnected(t *testing.T) {
	items := []list.Item{item{Name: "alpha"}, item{Name: "beta"}}
	l := list.New(items, connDelegate{connectedName: "alpha"}, 20, 4)
	d := connDelegate{connectedName: "alpha"}

	var connected, plain strings.Builder
	d.Render(&connected, l, 0, item{Name: "alpha"})
	d.Render(&plain, l, 1, item{Name: "beta"})

	if !strings.Contains(connected.String(), "●") {
		t.Error("connected item should render the ● marker")
	}
	if strings.Contains(plain.String(), "●") {
		t.Error("non-connected item should not render the ● marker")
	}
}

func TestSidebarAddConfig(t *testing.T) {
	s := NewSidebar(sampleConfigs()) // alpha, beta
	s.AddConfig(vpn.Config{Name: "gamma", Path: "/etc/openvpn/gamma.ovpn"})
	if got := len(s.list.Items()); got != 3 {
		t.Fatalf("items = %d after add, want 3", got)
	}

	// re-adding the same path is a no-op (dedup)
	s.AddConfig(vpn.Config{Name: "gamma", Path: "/etc/openvpn/gamma.ovpn"})
	if got := len(s.list.Items()); got != 3 {
		t.Errorf("items = %d after duplicate add, want 3", got)
	}
}

func TestSidebarEmpty(t *testing.T) {
	s := NewSidebar(nil)
	if got := s.SelectedName(); got != "" {
		t.Errorf("SelectedName() = %q, want empty", got)
	}
	if _, ok := s.SelectedConfig(); ok {
		t.Error("SelectedConfig() ok = true on empty list, want false")
	}
}
