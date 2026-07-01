package dialog

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
)

const (
	renameWidth     = 54  // inner width; height auto-fits
	renameInputW    = 40  // text input width (leaves room for the "name: " prompt)
	renameCharLimit = 128 // max length accepted for a connection name
)

// Rename is the popup that asks for a connection's new name. One text field,
// prefilled with the current name so it can be edited in place. The UI runs
// vpn.RenameConfig on submit and feeds any failure back via SetError, so the box
// shows it and stays open for another try.
type Rename struct {
	input    textinput.Model
	connName string
	errMsg   string
}

// NewRename builds the rename prompt.
func NewRename() Rename {
	in := textinput.New()
	in.Placeholder = "new name"
	in.Prompt = "name: "
	in.CharLimit = renameCharLimit
	in.SetWidth(renameInputW)
	return Rename{input: in}
}

// Open prefills the field with the current name, puts the cursor at the end, and
// focuses it.
func (r *Rename) Open(connName string) tea.Cmd {
	r.connName = connName
	r.errMsg = ""
	r.input.SetValue(connName)
	r.input.CursorEnd()
	return r.input.Focus()
}

// Reset clears the prompt (call after submit/cancel).
func (r *Rename) Reset() {
	r.input.Reset()
	r.input.Blur()
	r.errMsg = ""
	r.connName = ""
}

// Handle routes keys to the text field. Imperative: it mutates in place and
// returns any command the field emits.
func (r *Rename) Handle(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	r.input, cmd = r.input.Update(msg)
	return cmd
}

// Value returns the typed name.
func (r Rename) Value() string { return r.input.Value() }

// SetError shows an error in the box (e.g. a failed rename) and keeps it open.
func (r *Rename) SetError(msg string) { r.errMsg = msg }

// View renders the prompt box. The UI centers it over the main view.
func (r Rename) View() string {
	body := r.input.View()
	if r.errMsg != "" {
		fit := lipgloss.NewStyle().Foreground(common.Danger).MaxWidth(renameWidth)
		body += "\n\n" + fit.Render("error: "+r.errMsg)
	}
	body += "\n\n" + common.Hint.Render("enter: rename · esc: cancel")
	return common.Popup{Title: "rename — " + r.connName, Width: renameWidth}.Render(body)
}
