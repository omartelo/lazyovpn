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
	if out.(*UI).log.State().Badge() == "" {
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
	m.log.StartConnection("alpha")
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
	if done.log.State() != StateDisconnected {
		t.Errorf("state = %v after disconnect, want disconnected", done.log.State())
	}
}

// Cancelling the confirm-disconnect popup leaves the connection live.
func TestDisconnectCancelKeepsConnection(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.log.StartConnection("alpha")
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

// Enter on the already-connected connection is a no-op: it must not reconnect
// (no command, the live channel and state are untouched). The config path does
// not exist, so without the guard enter would fall through to NeedsAuth and
// error out — which the StateConnecting assertion catches.
func TestEnterWhileConnectedIsNoOp(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.log.StartConnection("alpha") // -> StateConnecting, active = alpha
	ch := make(chan string)
	m.logCh = ch

	out, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mm := out.(*UI)
	if cmd != nil {
		t.Error("enter while already connected issued a command, want no-op")
	}
	if mm.logCh != ch {
		t.Error("enter while connected replaced the live channel (reconnected)")
	}
	if mm.log.State() != StateConnecting {
		t.Errorf("state = %v after enter, want unchanged StateConnecting (no reconnect)", mm.log.State())
	}
}

// Tab moves focus to the log pane (to scroll the log); tab/esc return it to
// the sidebar. Focus is orthogonal to connection state — no live stream needed.
func TestTabTogglesLogFocus(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	mm := out.(*UI)
	if mm.focus != focusLog {
		t.Fatalf("focus = %v after tab, want focusLog", mm.focus)
	}
	if mm.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal (pane focus is not an overlay)", mm.mode)
	}

	out, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := out.(*UI).focus; got != focusSidebar {
		t.Errorf("focus = %v after esc, want focusSidebar", got)
	}

	out, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // focus again
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := out.(*UI).focus; got != focusSidebar {
		t.Errorf("focus = %v after tab toggle back, want focusSidebar", got)
	}
}

// While the log is focused, navigation keys scroll the log and must NOT
// move the sidebar cursor.
func TestLogFocusScrollLeavesSidebar(t *testing.T) {
	m := New([]vpn.Config{
		{Name: "alpha", Path: "/x/alpha.ovpn"},
		{Name: "beta", Path: "/x/beta.ovpn"},
	}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // focus log
	m = out.(*UI)
	before := m.sidebar.SelectedName()

	out, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}) // would move the cursor if unfocused
	m = out.(*UI)
	if got := m.sidebar.SelectedName(); got != before {
		t.Errorf("sidebar moved to %q while log focused, want %q", got, before)
	}
}

// The help footer swaps to scroll hints while the log pane is focused.
func TestHelpFooterFollowsFocus(t *testing.T) {
	m := &UI{}
	if got := m.helpFooter(); got != helpKeys {
		t.Errorf("sidebar-focus footer = %q, want the normal bindings", got)
	}
	m.focus = focusLog
	if got := m.helpFooter(); got != helpKeysLog {
		t.Errorf("log-focus footer = %q, want the scroll bindings", got)
	}
}

// statusLine surfaces the error message when in error state, otherwise the
// active connection name.
func TestStatusLine(t *testing.T) {
	m := New(nil, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	m.log.SetError("boom")
	if got := m.statusLine(); !strings.Contains(got, "boom") {
		t.Errorf("statusLine = %q, want it to contain the error", got)
	}

	m.log.StartConnection("alpha")
	if got := m.statusLine(); !strings.Contains(got, "alpha") {
		t.Errorf("statusLine = %q, want it to contain the active name", got)
	}
}
