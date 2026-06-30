// Package dialog holds the app's overlay components: stateful popups the UI
// builds, drives, and renders over the main view. Each is imperative (no Elm
// Update) and renders itself through common.Popup; centering over the view is
// the UI's job. Modelled on crush's internal/ui/dialog package.
package dialog

import (
	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
)

// confirmWidth is the inner width of a confirm popup; the height auto-fits.
const confirmWidth = 54

// Confirm is a yes/no overlay: a title, a message, and the action to run when
// the user accepts. The UI builds one, routes y/n to it, and renders it. onYes
// returns a tea.Cmd so an accepted action can have an effect (e.g. quitting).
type Confirm struct {
	title   string
	message string
	onYes   func() tea.Cmd
}

// NewConfirm builds a confirm popup that runs onYes when accepted.
func NewConfirm(title, message string, onYes func() tea.Cmd) Confirm {
	return Confirm{title: title, message: message, onYes: onYes}
}

// Yes runs the confirm action and returns its command (nil if none was set).
func (c Confirm) Yes() tea.Cmd {
	if c.onYes != nil {
		return c.onYes()
	}
	return nil
}

// View renders the popup box. The UI centers it over the main view.
func (c Confirm) View() string {
	body := c.message + "\n\n" + common.Hint.Render("y/enter: confirm · n/esc: cancel")
	return common.Popup{Title: c.title, Width: confirmWidth}.Render(body)
}
