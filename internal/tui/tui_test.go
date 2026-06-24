package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/lazyovpn/internal/tui/utils"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

func TestDims(t *testing.T) {
	m := model{w: 100, h: 30}
	sideW, sideH, outW, outH := m.dims()

	if sideW != sidebarWidth-paneChromeW {
		t.Errorf("sideW = %d, want %d", sideW, sidebarWidth-paneChromeW)
	}
	if outW != 100-sidebarWidth-paneChromeW {
		t.Errorf("outW = %d, want %d", outW, 100-sidebarWidth-paneChromeW)
	}
	if sideH != outH {
		t.Errorf("sideH (%d) != outH (%d), panes must match height", sideH, outH)
	}
	if sideH != 30-footerRows-paneChromeH {
		t.Errorf("sideH = %d, want %d", sideH, 30-footerRows-paneChromeH)
	}
}

func TestDimsClampsToMin(t *testing.T) {
	m := model{w: 1, h: 1} // tiny terminal
	sideW, sideH, outW, outH := m.dims()
	for _, v := range []int{sideW, sideH, outW, outH} {
		if v < 1 {
			t.Errorf("dimension %d below minimum 1", v)
		}
	}
}

// Invariant #4: a log message from an old connection must be dropped.
func TestStaleLogIgnored(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())

	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(model)

	foreign := make(chan string) // not the model's logCh (which is nil here)
	out, cmd := m.Update(utils.LogMsg{Ch: foreign, Line: "ghost"})
	if cmd != nil {
		t.Error("stale LogMsg produced a command, want nil")
	}
	if out.(model).terminal.State().Badge() == "" {
		t.Error("model corrupted by stale log")
	}
}

// A config with a bare auth-user-pass directive opens the credential modal on
// enter (and esc cancels it) — without spawning openvpn.
func TestEnterOpensAuthModal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vpn.ovpn")
	if err := os.WriteFile(path, []byte("client\nauth-user-pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "vpn", Path: path}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(model)

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mm := out.(model)
	if mm.mode != modeAuth {
		t.Fatalf("mode = %v after enter on auth config, want modeAuth", mm.mode)
	}

	out, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := out.(model).mode; got != modeNormal {
		t.Errorf("mode = %v after esc, want modeNormal", got)
	}
}

// Pressing "a" opens the import-connection modal; esc closes it. The returned
// command (the file chooser) is never run here, so no dialog is spawned.
func TestAddKeyOpensModal(t *testing.T) {
	m := New([]vpn.Config{{Name: "vpn", Path: "/x/vpn.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(model)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	mm := out.(model)
	if mm.mode != modeAdd {
		t.Fatalf("mode = %v after 'a', want modeAdd", mm.mode)
	}

	out, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := out.(model).mode; got != modeNormal {
		t.Errorf("mode = %v after esc, want modeNormal", got)
	}
}

// Picking a file then pressing enter imports it and appends it to the sidebar —
// no dialog is spawned (the FilePickedMsg is delivered directly).
func TestAddConfirmImports(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	src := filepath.Join(t.TempDir(), "new.ovpn")
	if err := os.WriteFile(src, []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(nil, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(model)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"}) // open modal
	m = out.(model)
	out, _ = m.Update(utils.FilePickedMsg{Path: src}) // chooser result
	m = out.(model)
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // import + add
	m = out.(model)

	if m.mode != modeNormal {
		t.Errorf("mode = %v after import, want modeNormal", m.mode)
	}
	cfg, ok := m.sidebar.SelectedConfig()
	if !ok || cfg.Name != "new" {
		t.Errorf("sidebar config = %+v ok=%v, want name new", cfg, ok)
	}
}

// statusLine surfaces the error message when in error state, otherwise the
// active connection name.
func TestStatusLine(t *testing.T) {
	m := New(nil, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(model)

	m.terminal.SetError("boom")
	if got := m.statusLine(); !strings.Contains(got, "boom") {
		t.Errorf("statusLine = %q, want it to contain the error", got)
	}

	m.terminal.StartConnection("alpha")
	if got := m.statusLine(); !strings.Contains(got, "alpha") {
		t.Errorf("statusLine = %q, want it to contain the active name", got)
	}
}
