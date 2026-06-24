package models

import (
	"errors"
	"strings"
	"testing"

	"github.com/omartelo/lazyovpn/internal/tui/utils"
)

func TestAddModalStoresPath(t *testing.T) {
	a := NewAddModal()
	a, _ = a.Update(utils.FilePickedMsg{Path: "/tmp/x.ovpn"})
	if a.Path() != "/tmp/x.ovpn" {
		t.Errorf("Path() = %q, want /tmp/x.ovpn", a.Path())
	}
}

func TestAddModalCanceledKeepsEmpty(t *testing.T) {
	a := NewAddModal()
	a, _ = a.Update(utils.FilePickedMsg{Canceled: true})
	if a.Path() != "" {
		t.Errorf("Path() = %q after cancel, want empty", a.Path())
	}
}

func TestAddModalShowsError(t *testing.T) {
	a := NewAddModal()
	a, _ = a.Update(utils.FilePickedMsg{Err: errors.New("boom")})
	if !strings.Contains(a.View(), "boom") {
		t.Error("View() should show the error message")
	}
}

func TestAddModalResetClears(t *testing.T) {
	a := NewAddModal()
	a, _ = a.Update(utils.FilePickedMsg{Path: "/tmp/x.ovpn"})
	a.Reset()
	if a.Path() != "" {
		t.Errorf("Path() = %q after Reset, want empty", a.Path())
	}
}
