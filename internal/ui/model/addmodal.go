package model

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/lazyovpn/internal/ui/common"
	"github.com/omartelo/lazyovpn/internal/ui/utils"
)

const addInnerW = 56 // modal inner width (height auto-fits the content)

// AddModal hosts the "import a connection" flow: it launches the native file
// chooser and shows the picked path until the user confirms with enter.
type AddModal struct {
	path   string
	errMsg string
}

func NewAddModal() AddModal { return AddModal{} }

// Open clears the modal and returns the command that launches the file chooser.
// Also used to pick again (the dialog is relaunched from a clean state).
func (a *AddModal) Open() tea.Cmd {
	a.path, a.errMsg = "", ""
	return utils.PickFile()
}

// Reset clears the modal state.
func (a *AddModal) Reset() { a.path, a.errMsg = "", "" }

// SetError shows an import error in the modal.
func (a *AddModal) SetError(msg string) { a.errMsg = msg }

// Path returns the chosen file path, or "" if nothing is picked yet.
func (a AddModal) Path() string { return a.path }

// Update records the file-chooser result.
func (a AddModal) Update(msg tea.Msg) (AddModal, tea.Cmd) {
	if picked, ok := msg.(utils.FilePickedMsg); ok {
		switch {
		case picked.Err != nil:
			a.errMsg = picked.Err.Error()
		case picked.Canceled:
			// keep current state — the user can pick again (r) or cancel (esc)
		default:
			a.path, a.errMsg = picked.Path, ""
		}
	}
	return a, nil
}

// View renders the modal box. The root composites it over the main view.
func (a AddModal) View() string {
	hint := common.Hint.Render("enter: add · r: pick again · esc: cancel")
	fit := lipgloss.NewStyle().MaxWidth(addInnerW)

	var body string
	switch {
	case a.errMsg != "":
		body = fit.Render("error: "+a.errMsg) + "\n\n" + hint
	case a.path == "":
		body = "opening file chooser…\n\n" + hint
	default:
		body = "selected:\n" + fit.Render(a.path) + "\n\n" + hint
	}
	return common.Dialog{Title: "add connection", Width: addInnerW}.Render(body)
}
