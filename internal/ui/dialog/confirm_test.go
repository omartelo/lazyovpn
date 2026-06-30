package dialog

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestConfirmYesRunsAction(t *testing.T) {
	ran := false
	cmd := NewConfirm("t", "msg", func() tea.Cmd { ran = true; return nil }).Yes()
	if !ran {
		t.Error("Yes() should run the onYes action")
	}
	if cmd != nil {
		t.Error("Yes() should return the action's cmd (nil here)")
	}
}

// Yes() must hand back the action's command so an accepted confirm can have an
// effect (e.g. tea.Quit on the quit popup).
func TestConfirmYesReturnsCmd(t *testing.T) {
	want := tea.Quit
	cmd := NewConfirm("t", "msg", func() tea.Cmd { return want }).Yes()
	if cmd == nil {
		t.Error("Yes() dropped the action's cmd")
	}
}

func TestConfirmYesNilSafe(t *testing.T) {
	NewConfirm("t", "msg", nil).Yes() // must not panic with no action
}

func TestConfirmViewShowsTitleAndMessage(t *testing.T) {
	v := NewConfirm("disconnect", "Disconnect from x?", nil).View()
	if !strings.Contains(v, "Disconnect from x?") {
		t.Error("View should contain the message")
	}
	if !strings.Contains(v, "disconnect") {
		t.Error("View should contain the title")
	}
}
