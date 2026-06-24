package models

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/tui/components"
)

const (
	authInnerW = 46
	authInnerH = 5
	inputWidth = 34
)

// AuthModal is the credential prompt shown when a connection needs a
// username/password. It never persists the values — Reset clears them.
type AuthModal struct {
	username textinput.Model
	password textinput.Model
	focus    int // 0 = username, 1 = password
	connName string
}

// NewAuthModal builds the modal with a masked password field.
func NewAuthModal() AuthModal {
	u := textinput.New()
	u.Placeholder = "username"
	u.Prompt = "user: "
	u.CharLimit = 128
	u.SetWidth(inputWidth)

	p := textinput.New()
	p.Placeholder = "password"
	p.Prompt = "pass: "
	p.CharLimit = 128
	p.SetWidth(inputWidth)
	p.EchoMode = textinput.EchoPassword
	p.EchoCharacter = '•'

	return AuthModal{username: u, password: p}
}

// Open resets the modal for connection name and focuses the username field.
func (a *AuthModal) Open(connName string) tea.Cmd {
	a.connName = connName
	a.username.Reset()
	a.password.Reset()
	a.focus = 0
	a.password.Blur()
	return a.username.Focus()
}

// Reset clears both fields (call after submit/cancel — passwords must not linger).
func (a *AuthModal) Reset() {
	a.username.Reset()
	a.password.Reset()
	a.username.Blur()
	a.password.Blur()
	a.focus = 0
}

// Update routes keys to the focused field and toggles focus on tab/arrows.
func (a AuthModal) Update(msg tea.Msg) (AuthModal, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "tab", "shift+tab", "up", "down":
			a.toggleFocus()
			return a, a.focusCmd()
		}
	}
	var cmd tea.Cmd
	if a.focus == 0 {
		a.username, cmd = a.username.Update(msg)
	} else {
		a.password, cmd = a.password.Update(msg)
	}
	return a, cmd
}

func (a *AuthModal) toggleFocus() { a.focus = (a.focus + 1) % 2 }

func (a *AuthModal) focusCmd() tea.Cmd {
	if a.focus == 0 {
		a.password.Blur()
		return a.username.Focus()
	}
	a.username.Blur()
	return a.password.Focus()
}

// Username returns the typed username.
func (a AuthModal) Username() string { return a.username.Value() }

// Password returns the typed password.
func (a AuthModal) Password() string { return a.password.Value() }

// View renders the modal box. The root composites it over the main view.
func (a AuthModal) View() string {
	hint := lipgloss.NewStyle().Faint(true).Render("enter: connect · tab: switch · esc: cancel")
	body := a.username.View() + "\n\n" + a.password.View() + "\n" + hint
	return components.TitledBox("auth — "+a.connName, body, authInnerW, authInnerH, true)
}
