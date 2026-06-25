package dialog

import (
	"errors"
	"strings"
	"testing"
)

func TestFilePickerStoresPath(t *testing.T) {
	p := NewFilePicker("add connection")
	p.Handle(FilePickedMsg{Path: "/tmp/x.ovpn"})
	if p.Path() != "/tmp/x.ovpn" {
		t.Errorf("Path() = %q, want /tmp/x.ovpn", p.Path())
	}
	if !strings.Contains(p.View(), "/tmp/x.ovpn") {
		t.Error("View() should show the selected path")
	}
}

func TestFilePickerCanceledKeepsEmpty(t *testing.T) {
	p := NewFilePicker("add connection")
	p.Handle(FilePickedMsg{Canceled: true})
	if p.Path() != "" {
		t.Errorf("Path() = %q after cancel, want empty", p.Path())
	}
}

func TestFilePickerShowsError(t *testing.T) {
	p := NewFilePicker("add connection")
	p.Handle(FilePickedMsg{Err: errors.New("boom")})
	if !strings.Contains(p.View(), "boom") {
		t.Error("View() should show the error message")
	}
}

func TestFilePickerSetError(t *testing.T) {
	p := NewFilePicker("add connection")
	p.SetError("import failed")
	if !strings.Contains(p.View(), "import failed") {
		t.Error("View() should show the error set via SetError")
	}
}

func TestFilePickerResetClears(t *testing.T) {
	p := NewFilePicker("add connection")
	p.Handle(FilePickedMsg{Path: "/tmp/x.ovpn"})
	p.Reset()
	if p.Path() != "" {
		t.Errorf("Path() = %q after Reset, want empty", p.Path())
	}
}

func TestFilePickerViewUsesTitle(t *testing.T) {
	if v := NewFilePicker("add connection").View(); !strings.Contains(v, "add connection") {
		t.Error("View() should show the title")
	}
}
