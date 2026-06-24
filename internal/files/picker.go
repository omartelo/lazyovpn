// Package files holds OS-facing file helpers — the native file-chooser dialog
// and small file operations — kept free of TUI concerns so they stay testable.
package files

import (
	"errors"
	"os/exec"
	"strings"
)

// ErrCanceled is returned by Pick when the user dismisses the dialog.
var ErrCanceled = errors.New("file selection canceled")

// ErrNoChooser is returned when no supported file-chooser dialog is installed.
var ErrNoChooser = errors.New("no file chooser found (install zenity or kdialog)")

// chooser is a native file-selection dialog and the args that make it print the
// chosen path to stdout.
type chooser struct {
	bin  string
	args []string
}

// choosers are the supported native dialogs, tried in order — the first one on
// PATH wins.
//
// ponytail: Linux desktop dialogs only. Add osascript (macOS) / PowerShell
// (Windows) entries here if lazyovpn ever leaves Linux.
var choosers = []chooser{
	{"zenity", []string{
		"--file-selection",
		"--title=Select an OpenVPN config",
		"--file-filter=OpenVPN configs (ovpn, conf) | *.ovpn *.conf",
	}},
	{"kdialog", []string{
		"--getopenfilename", ".", "*.ovpn *.conf|OpenVPN configs",
	}},
}

// resolveChooser returns the first installed file-chooser dialog.
func resolveChooser() (chooser, bool) {
	for _, c := range choosers {
		if _, err := exec.LookPath(c.bin); err == nil {
			return c, true
		}
	}
	return chooser{}, false
}

// Pick opens the native file-chooser dialog and returns the path the user
// selected. ErrCanceled if dismissed, ErrNoChooser if none is installed.
//
// The dialog is a separate GUI window, so this blocks the calling goroutine
// until the user acts — run it off the UI goroutine (see utils.PickFile).
func Pick() (string, error) {
	c, ok := resolveChooser()
	if !ok {
		return "", ErrNoChooser
	}
	out, err := exec.Command(c.bin, c.args...).Output()
	if err != nil {
		// zenity and kdialog both exit 1 when the dialog is canceled.
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return "", ErrCanceled
		}
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", ErrCanceled
	}
	return path, nil
}
