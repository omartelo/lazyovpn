package model

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
)

const (
	authInnerW    = 64  // modal inner width (height auto-fits the content)
	inputWidth    = 34  // text input width (leaves room for the "user: " prompt)
	credCharLimit = 128 // max length accepted for username/password
)

// AuthModal is the credential prompt shown when a connection needs a
// username/password. It never persists the values — Reset clears them.
type AuthModal struct {
	username textinput.Model
	password textinput.Model
	focus    int  // 0 = username, 1 = password
	save     bool // opt-in: persist creds to the OS keyring on submit
	connName string
}

// NewAuthModal builds the modal with a masked password field.
func NewAuthModal() AuthModal {
	u := textinput.New()
	u.Placeholder = "username"
	u.Prompt = "user: "
	u.CharLimit = credCharLimit
	u.SetWidth(inputWidth)

	p := textinput.New()
	p.Placeholder = "password"
	p.Prompt = "pass: "
	p.CharLimit = credCharLimit
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
	a.save = false
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
	a.save = false
}

// Handle routes keys to the focused field and toggles focus on tab/arrows.
// Imperative: it mutates the modal in place and returns any command the focused
// field emits — see internal/ui/CLAUDE.md.
func (a *AuthModal) Handle(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "tab", "shift+tab", "up", "down":
			a.toggleFocus()
			return a.focusCmd()
		case "ctrl+s":
			a.save = !a.save
			return nil
		}
	}
	var cmd tea.Cmd
	if a.focus == 0 {
		a.username, cmd = a.username.Update(msg)
	} else {
		a.password, cmd = a.password.Update(msg)
	}
	return cmd
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

// Save reports whether the user opted to persist the credentials.
func (a AuthModal) Save() bool { return a.save }

// View renders the modal box. The root composites it over the main view.
func (a AuthModal) View() string {
	box := "[ ]"
	if a.save {
		box = "[x]"
	}
	saveLine := box + " save credentials to keyring"
	hint := common.Hint.Render("enter: connect · ctrl+s: save · tab: switch · esc: cancel")
	body := a.username.View() + "\n\n" + a.password.View() + "\n\n" + saveLine + "\n\n" + hint
	return common.Dialog{Title: "auth — " + a.connName, Width: authInnerW}.Render(body)
}
