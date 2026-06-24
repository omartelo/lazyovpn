package models

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func typeRunes(a AuthModal, s string) AuthModal {
	a, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return a
}

func TestAuthModalTypingAcrossFields(t *testing.T) {
	a := NewAuthModal()
	a.Open("vpn1")

	a = typeRunes(a, "bob")                       // username is focused first
	a, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab}) // move to password
	a = typeRunes(a, "hunter2")

	if a.Username() != "bob" {
		t.Errorf("Username() = %q, want bob", a.Username())
	}
	if a.Password() != "hunter2" {
		t.Errorf("Password() = %q, want hunter2", a.Password())
	}
}

func TestAuthModalResetClears(t *testing.T) {
	a := NewAuthModal()
	a.Open("vpn1")
	a = typeRunes(a, "bob")
	a, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
	a = typeRunes(a, "hunter2")

	a.Reset()
	if a.Username() != "" || a.Password() != "" {
		t.Errorf("after Reset: user=%q pass=%q, want empty", a.Username(), a.Password())
	}
}

func TestAuthModalPasswordMasked(t *testing.T) {
	a := NewAuthModal()
	a.Open("vpn1")
	a, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab}) // focus password
	a = typeRunes(a, "secret")

	view := a.View()
	// The raw password must never appear in the rendered modal.
	if strings.Contains(view, "secret") {
		t.Error("rendered modal leaks the plaintext password")
	}
}
