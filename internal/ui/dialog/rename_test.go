package dialog

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Open prefills the current name with the cursor at the end, so typing appends
// (rather than landing at the front).
func TestRenamePrefillsAndAppends(t *testing.T) {
	r := NewRename()
	r.Open("home-vpn")
	if r.Value() != "home-vpn" {
		t.Fatalf("Value() = %q, want prefilled home-vpn", r.Value())
	}
	r.Handle(tea.KeyPressMsg{Text: "2"})
	if r.Value() != "home-vpn2" {
		t.Errorf("Value() = %q, want home-vpn2 (cursor at end)", r.Value())
	}
}

// Reset clears the field for the next open.
func TestRenameResetClears(t *testing.T) {
	r := NewRename()
	r.Open("vpn")
	r.Handle(tea.KeyPressMsg{Text: "x"})
	r.Reset()
	if r.Value() != "" {
		t.Errorf("Value() = %q after Reset, want empty", r.Value())
	}
}

// A set error surfaces in the rendered box so the user sees why a rename failed.
func TestRenameShowsError(t *testing.T) {
	r := NewRename()
	r.Open("vpn")
	r.SetError(`a connection named "x" already exists`)
	if !strings.Contains(r.View(), "already exists") {
		t.Errorf("View missing the error:\n%s", r.View())
	}
}
