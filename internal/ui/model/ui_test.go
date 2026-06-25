package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zalando/go-keyring"

	"github.com/omartelo/lazyovpn/internal/ui/dialog"
	"github.com/omartelo/lazyovpn/internal/ui/utils"
	"github.com/omartelo/lazyovpn/internal/vpn"
)

func TestDims(t *testing.T) {
	m := &UI{w: 100, h: 30}
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
	m := &UI{w: 1, h: 1} // tiny terminal
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
	m = sized.(*UI)

	foreign := make(chan string) // not the UI's logCh (which is nil here)
	out, cmd := m.Update(utils.LogMsg{Ch: foreign, Line: "ghost"})
	if cmd != nil {
		t.Error("stale LogMsg produced a command, want nil")
	}
	if out.(*UI).terminal.State().Badge() == "" {
		t.Error("UI corrupted by stale log")
	}
}

// A config with a bare auth-user-pass directive opens the credential modal on
// enter (and esc cancels it) — without spawning openvpn.
func TestEnterOpensCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vpn.ovpn")
	if err := os.WriteFile(path, []byte("client\nauth-user-pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "vpn", Path: path}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mm := out.(*UI)
	if mm.mode != modeAuth {
		t.Fatalf("mode = %v after enter on auth config, want modeAuth", mm.mode)
	}

	out, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := out.(*UI).mode; got != modeNormal {
		t.Errorf("mode = %v after esc, want modeNormal", got)
	}
}

// Pressing "a" opens the import-connection modal; esc closes it. The returned
// command (the file chooser) is never run here, so no dialog is spawned.
func TestAddKeyOpensModal(t *testing.T) {
	m := New([]vpn.Config{{Name: "vpn", Path: "/x/vpn.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	mm := out.(*UI)
	if mm.mode != modeAdd {
		t.Fatalf("mode = %v after 'a', want modeAdd", mm.mode)
	}

	out, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := out.(*UI).mode; got != modeNormal {
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
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"}) // open modal
	m = out.(*UI)
	out, _ = m.Update(dialog.FilePickedMsg{Path: src}) // chooser result
	m = out.(*UI)
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // import + add
	m = out.(*UI)

	if m.mode != modeNormal {
		t.Errorf("mode = %v after import, want modeNormal", m.mode)
	}
	cfg, ok := m.sidebar.SelectedConfig()
	if !ok || cfg.Name != "new" {
		t.Errorf("sidebar config = %+v ok=%v, want name new", cfg, ok)
	}
}

// Pressing "x" with no saved credentials is a no-op — the confirm popup only
// appears when there is actually something to forget.
func TestForgetNoSavedCredsNoPopup(t *testing.T) {
	keyring.MockInit()

	m := New([]vpn.Config{{Name: "vpn", Path: "/x/vpn.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := out.(*UI).mode; got != modeNormal {
		t.Errorf("mode = %v with no saved creds, want modeNormal (no popup)", got)
	}
}

// "x" on a connection with saved credentials opens the confirm popup; confirming
// deletes the keyring entry.
func TestForgetConfirmDeletes(t *testing.T) {
	keyring.MockInit()
	if err := vpn.SaveCreds("vpn", "u", "p"); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "vpn", Path: "/x/vpn.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	mm := out.(*UI)
	if mm.mode != modeConfirm {
		t.Fatalf("mode = %v after x with saved creds, want modeConfirm", mm.mode)
	}

	out, _ = mm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if got := out.(*UI).mode; got != modeNormal {
		t.Errorf("mode = %v after confirm, want modeNormal", got)
	}
	if _, _, ok, _ := vpn.LoadCreds("vpn"); ok {
		t.Error("credentials still present after confirming forget")
	}
}

// Cancelling the confirm popup leaves the saved credentials untouched.
func TestForgetCancelKeeps(t *testing.T) {
	keyring.MockInit()
	if err := vpn.SaveCreds("vpn", "u", "p"); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "vpn", Path: "/x/vpn.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})    // open confirm
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEsc}) // cancel
	if got := out.(*UI).mode; got != modeNormal {
		t.Errorf("mode = %v after cancel, want modeNormal", got)
	}
	if _, _, ok, _ := vpn.LoadCreds("vpn"); !ok {
		t.Error("credentials deleted after cancelling forget")
	}
}

// "d" with a live connection opens the confirm-disconnect popup; confirming
// tears the connection down (clears the live channel, flips to disconnected).
func TestDisconnectConfirmTearsDown(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.terminal.StartConnection("alpha")
	m.logCh = make(chan string) // simulate a live stream without spawning openvpn

	out, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	mm := out.(*UI)
	if mm.mode != modeConfirm {
		t.Fatalf("mode = %v after d while connected, want modeConfirm", mm.mode)
	}

	out, _ = mm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	done := out.(*UI)
	if done.mode != modeNormal {
		t.Errorf("mode = %v after confirm, want modeNormal", done.mode)
	}
	if done.logCh != nil {
		t.Error("logCh still set after confirming disconnect, want nil")
	}
	if done.terminal.State() != StateDisconnected {
		t.Errorf("state = %v after disconnect, want disconnected", done.terminal.State())
	}
}

// Cancelling the confirm-disconnect popup leaves the connection live.
func TestDisconnectCancelKeepsConnection(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.terminal.StartConnection("alpha")
	m.logCh = make(chan string)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})    // open confirm
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEsc}) // cancel
	kept := out.(*UI)
	if kept.mode != modeNormal {
		t.Errorf("mode = %v after cancel, want modeNormal", kept.mode)
	}
	if kept.logCh == nil {
		t.Error("logCh cleared after cancelling disconnect, want it kept")
	}
}

// "d" with nothing connected is a no-op — no popup (logCh is nil).
func TestDisconnectNoConnectionNoPopup(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if got := out.(*UI).mode; got != modeNormal {
		t.Errorf("mode = %v with no connection, want modeNormal (no popup)", got)
	}
}

// statusLine surfaces the error message when in error state, otherwise the
// active connection name.
func TestStatusLine(t *testing.T) {
	m := New(nil, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	m.terminal.SetError("boom")
	if got := m.statusLine(); !strings.Contains(got, "boom") {
		t.Errorf("statusLine = %q, want it to contain the error", got)
	}

	m.terminal.StartConnection("alpha")
	if got := m.statusLine(); !strings.Contains(got, "alpha") {
		t.Errorf("statusLine = %q, want it to contain the active name", got)
	}
}
