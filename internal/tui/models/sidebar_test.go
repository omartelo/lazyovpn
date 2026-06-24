package models

import (
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

func TestSidebarEmpty(t *testing.T) {
	s := NewSidebar(nil)
	if got := s.SelectedName(); got != "" {
		t.Errorf("SelectedName() = %q, want empty", got)
	}
	if _, ok := s.SelectedConfig(); ok {
		t.Error("SelectedConfig() ok = true on empty list, want false")
	}
}
