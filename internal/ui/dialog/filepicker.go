package dialog

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/files"
	"github.com/omartelo/lazyovpn/internal/ui/common"
)

const filePickerWidth = 56 // inner width; height auto-fits

// FilePickedMsg carries the result of the native file-chooser dialog.
type FilePickedMsg struct {
	Path     string
	Canceled bool
	Err      error
}

// FilePicker drives the native OS file chooser: Open launches it off the UI
// goroutine and the picker shows the chosen path (or an error) until the caller
// confirms. The actual browsing happens in a separate GUI window, so this only
// tracks and renders the result — it is not an in-TUI browser.
type FilePicker struct {
	title  string
	path   string
	errMsg string
}

// NewFilePicker returns an empty picker that renders under title.
func NewFilePicker(title string) FilePicker { return FilePicker{title: title} }

// Open clears the picker and returns the command that launches the native
// chooser off the UI goroutine. Also used to pick again.
func (p *FilePicker) Open() tea.Cmd {
	p.path, p.errMsg = "", ""
	return func() tea.Msg {
		path, err := files.Pick()
		switch {
		case errors.Is(err, files.ErrCanceled):
			return FilePickedMsg{Canceled: true}
		case err != nil:
			return FilePickedMsg{Err: err}
		default:
			return FilePickedMsg{Path: path}
		}
	}
}

// Handle records a file-chooser result. Imperative: it mutates in place.
func (p *FilePicker) Handle(msg FilePickedMsg) {
	switch {
	case msg.Err != nil:
		p.errMsg = msg.Err.Error()
	case msg.Canceled:
		// keep current state — the caller can pick again or cancel
	default:
		p.path, p.errMsg = msg.Path, ""
	}
}

// Path returns the chosen file path, or "" if nothing is picked yet.
func (p FilePicker) Path() string { return p.path }

// SetError shows an error in the picker (e.g. a failed import).
func (p *FilePicker) SetError(msg string) { p.errMsg = msg }

// Reset clears the picker state.
func (p *FilePicker) Reset() { p.path, p.errMsg = "", "" }

// View renders the picker box. The UI centers it over the main view.
func (p FilePicker) View() string {
	hint := common.Hint.Render("enter: confirm · r: pick again · esc: cancel")
	fit := lipgloss.NewStyle().MaxWidth(filePickerWidth)

	var body string
	switch {
	case p.errMsg != "":
		body = fit.Render("error: "+p.errMsg) + "\n\n" + hint
	case p.path == "":
		body = "opening file chooser…\n\n" + hint
	default:
		body = "selected:\n" + fit.Render(p.path) + "\n\n" + hint
	}
	return common.Popup{Title: p.title, Width: filePickerWidth}.Render(body)
}
