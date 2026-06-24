package utils

import (
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/lazyovpn/internal/files"
)

// FilePickedMsg carries the result of the native file-chooser dialog.
type FilePickedMsg struct {
	Path     string
	Canceled bool
	Err      error
}

// PickFile opens the native file-chooser dialog off the UI goroutine and
// reports the chosen path (or cancel/error) as a FilePickedMsg.
func PickFile() tea.Cmd {
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
