package dialog

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func typeRunes(c Credentials, s string) Credentials {
	c.Handle(tea.KeyPressMsg{Text: s})
	return c
}

func TestCredentialsTypingAcrossFields(t *testing.T) {
	c := NewCredentials()
	c.Open("vpn1")

	c = typeRunes(c, "bob")                     // username is focused first
	c.Handle(tea.KeyPressMsg{Code: tea.KeyTab}) // move to password
	c = typeRunes(c, "hunter2")

	if c.Username() != "bob" {
		t.Errorf("Username() = %q, want bob", c.Username())
	}
	if c.Password() != "hunter2" {
		t.Errorf("Password() = %q, want hunter2", c.Password())
	}
}

func TestCredentialsResetClears(t *testing.T) {
	c := NewCredentials()
	c.Open("vpn1")
	c = typeRunes(c, "bob")
	c.Handle(tea.KeyPressMsg{Code: tea.KeyTab})
	c = typeRunes(c, "hunter2")

	c.Reset()
	if c.Username() != "" || c.Password() != "" {
		t.Errorf("after Reset: user=%q pass=%q, want empty", c.Username(), c.Password())
	}
}

func TestCredentialsSaveToggle(t *testing.T) {
	c := NewCredentials()
	c.Open("vpn1")

	if c.Save() {
		t.Fatal("save toggle starts on, must be opt-in")
	}
	ctrlS := tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	c.Handle(ctrlS)
	if !c.Save() {
		t.Fatal("ctrl+s did not turn the save toggle on")
	}
	if !strings.Contains(c.View(), "[x]") {
		t.Error("enabled toggle not rendered as [x]")
	}
	c.Handle(ctrlS)
	if c.Save() {
		t.Error("second ctrl+s did not turn the save toggle off")
	}
}

func TestCredentialsResetClearsSaveToggle(t *testing.T) {
	c := NewCredentials()
	c.Open("vpn1")
	c.Handle(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	c.Reset()
	if c.Save() {
		t.Error("save toggle survived Reset")
	}
}

func TestCredentialsFocusRoundTrip(t *testing.T) {
	c := NewCredentials()
	c.Open("vpn1")

	c.Handle(tea.KeyPressMsg{Code: tea.KeyTab}) // -> password
	c.Handle(tea.KeyPressMsg{Code: tea.KeyTab}) // -> back to username
	c = typeRunes(c, "bob")

	if c.Username() != "bob" {
		t.Errorf("Username() = %q after tab round trip, want bob", c.Username())
	}
	if c.Password() != "" {
		t.Errorf("Password() = %q, want empty (typing must land on username)", c.Password())
	}
}

func TestCredentialsPasswordMasked(t *testing.T) {
	c := NewCredentials()
	c.Open("vpn1")
	c.Handle(tea.KeyPressMsg{Code: tea.KeyTab}) // focus password
	c = typeRunes(c, "secret")

	// The raw password must never appear in the rendered prompt.
	if strings.Contains(c.View(), "secret") {
		t.Error("rendered prompt leaks the plaintext password")
	}
}
