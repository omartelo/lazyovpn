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
