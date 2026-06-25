package dialog

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
)

const (
	credentialsWidth = 64  // inner width; height auto-fits the content
	inputWidth       = 34  // text input width (leaves room for the "user: " prompt)
	credCharLimit    = 128 // max length accepted for username/password
)

// Credentials is the username/password prompt shown when a connection needs an
// auth-user-pass login. The save toggle opts into storing them in the OS
// keyring; the typed values themselves never persist — Reset clears them. It
// wraps two bubbles textinputs (cursor, editing, masking) rather than
// hand-rolling input.
type Credentials struct {
	username   textinput.Model
	password   textinput.Model
	onPassword bool // false: username focused; true: password focused
	save       bool // opt-in: persist creds to the OS keyring on submit
	connName   string
}

// NewCredentials builds the prompt with a masked password field.
func NewCredentials() Credentials {
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

	return Credentials{username: u, password: p}
}

// Open clears the prompt for connName and focuses the username field.
func (c *Credentials) Open(connName string) tea.Cmd {
	c.Reset()
	c.connName = connName
	return c.username.Focus()
}

// Reset clears both fields and the save toggle (call after submit/cancel —
// passwords must not linger).
func (c *Credentials) Reset() {
	c.username.Reset()
	c.password.Reset()
	c.username.Blur()
	c.password.Blur()
	c.onPassword = false
	c.save = false
}

// Handle routes keys to the focused field and toggles focus on tab/arrows.
// Imperative: it mutates in place and returns any command the field emits.
func (c *Credentials) Handle(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "tab", "shift+tab", "up", "down":
			return c.toggleFocus()
		case "ctrl+s":
			c.save = !c.save
			return nil
		}
	}
	var cmd tea.Cmd
	if c.onPassword {
		c.password, cmd = c.password.Update(msg)
	} else {
		c.username, cmd = c.username.Update(msg)
	}
	return cmd
}

// toggleFocus moves focus to the other field and returns its focus command.
func (c *Credentials) toggleFocus() tea.Cmd {
	c.onPassword = !c.onPassword
	if c.onPassword {
		c.username.Blur()
		return c.password.Focus()
	}
	c.password.Blur()
	return c.username.Focus()
}

// Username returns the typed username.
func (c Credentials) Username() string { return c.username.Value() }

// Password returns the typed password.
func (c Credentials) Password() string { return c.password.Value() }

// Save reports whether the user opted to persist the credentials.
func (c Credentials) Save() bool { return c.save }

// View renders the prompt box. The UI centers it over the main view.
func (c Credentials) View() string {
	box := "[ ]"
	if c.save {
		box = "[x]"
	}
	saveLine := box + " save credentials to keyring"
	hint := common.Hint.Render("enter: connect · ctrl+s: save · tab: switch · esc: cancel")
	body := c.username.View() + "\n\n" + c.password.View() + "\n\n" + saveLine + "\n\n" + hint
	return common.Popup{Title: "auth — " + c.connName, Width: credentialsWidth}.Render(body)
}
