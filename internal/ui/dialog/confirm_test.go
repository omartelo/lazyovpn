package dialog

import (
	"strings"
	"testing"
)

func TestConfirmYesRunsAction(t *testing.T) {
	ran := false
	NewConfirm("t", "msg", func() { ran = true }).Yes()
	if !ran {
		t.Error("Yes() should run the onYes action")
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
