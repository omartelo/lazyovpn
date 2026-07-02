package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// "x" opens the per-connection action menu; esc closes it back to normal mode.
func TestMenuOpensAndCloses(t *testing.T) {
	m := New([]vpn.Config{{Name: "vpn", Path: "/x/vpn.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := out.(*UI).mode; got != modeMenu {
		t.Fatalf("mode = %v after x, want modeMenu", got)
	}
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := out.(*UI).mode; got != modeNormal {
		t.Errorf("mode = %v after esc, want modeNormal", got)
	}
}

// Menu "f" with no saved credentials closes the menu without a confirm — the
// forget popup only appears when there is actually something to forget.
func TestForgetNoSavedCredsNoPopup(t *testing.T) {
	keyring.MockInit()

	m := New([]vpn.Config{{Name: "vpn", Path: "/x/vpn.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // open menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'f', Text: "f"}) // forget
	if got := out.(*UI).mode; got != modeNormal {
		t.Errorf("mode = %v with no saved creds, want modeNormal (no popup)", got)
	}
}

// Menu "f" on a connection with saved credentials opens the confirm popup;
// confirming deletes the keyring entry.
func TestForgetConfirmDeletes(t *testing.T) {
	keyring.MockInit()
	if err := vpn.SaveCreds("vpn", "u", "p"); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "vpn", Path: "/x/vpn.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // open menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'f', Text: "f"}) // forget
	mm := out.(*UI)
	if mm.mode != modeConfirm {
		t.Fatalf("mode = %v after menu forget with saved creds, want modeConfirm", mm.mode)
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

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // open menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'f', Text: "f"}) // forget → confirm
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEsc})     // cancel
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
	m.reAttempts = 2 // a spent retry budget the no-op must not touch

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
	// A true no-op stops before the connect path: the retry budget keeps its
	// value and no operation error is recorded (the config's path is unreadable,
	// so falling through would surface a NeedsAuth error).
	if mm.reAttempts != 2 {
		t.Errorf("reAttempts = %d after no-op enter, want 2 (untouched)", mm.reAttempts)
	}
	if mm.log.Err() != "" {
		t.Errorf("Err() = %q after no-op enter, want empty (connect path must not run)", mm.log.Err())
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

// The mouse wheel scrolls the log viewport even without key focus on it.
func TestMouseWheelScrollsLog(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 8})
	m = sized.(*UI)
	m.log.StartConnection("alpha")
	for i := 0; i < 80; i++ {
		m.log.AppendLog("line")
	}
	if !m.log.vp.AtBottom() {
		t.Fatal("expected to start tailing at the bottom")
	}

	// Focus stays on the sidebar — the wheel must still reach the log.
	out, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	mm := out.(*UI)
	if mm.focus != focusSidebar {
		t.Errorf("focus = %v, wheel must not change focus", mm.focus)
	}
	if mm.log.vp.AtBottom() {
		t.Error("mouse wheel up did not scroll the log off the bottom")
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

// armedConnected builds a UI with a live, connected, auto-reconnect-armed
// connection — without spawning openvpn. It returns the UI and its live channel
// so a test can deliver a LogClosedMsg for that exact channel (a drop).
func armedConnected(t *testing.T, name string) (*UI, chan string) {
	t.Helper()
	cfg := vpn.Config{Name: name, Path: "/x/" + name + ".ovpn"}
	m := New([]vpn.Config{cfg}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.log.StartConnection(name)
	m.log.state = StateConnected // simulate tunnel-up without the marker stream
	ch := make(chan string)
	m.logCh = ch
	m.reCfg = cfg
	m.reArmed = true
	return m, ch
}

// A live tunnel's process exiting on its own schedules an auto-reconnect:
// the channel clears, the attempt counter advances, and a timer cmd is returned.
func TestDropSchedulesReconnect(t *testing.T) {
	m, ch := armedConnected(t, "alpha")

	out, cmd := m.Update(utils.LogClosedMsg{Ch: ch})
	mm := out.(*UI)
	if cmd == nil {
		t.Error("drop while connected produced no reconnect timer")
	}
	if mm.logCh != nil {
		t.Error("logCh not cleared after drop")
	}
	if mm.reAttempts != 1 {
		t.Errorf("reAttempts = %d after first drop, want 1", mm.reAttempts)
	}
	if mm.log.State() != StateReconnecting {
		t.Errorf("state = %v after drop, want StateReconnecting", mm.log.State())
	}
}

// A close from a stale channel must not trigger a reconnect.
func TestForeignDropIgnored(t *testing.T) {
	m, ch := armedConnected(t, "alpha")

	out, cmd := m.Update(utils.LogClosedMsg{Ch: make(chan string)})
	mm := out.(*UI)
	if cmd != nil {
		t.Error("close from a foreign channel produced a command")
	}
	if mm.logCh != ch {
		t.Error("foreign close cleared the live channel")
	}
}

// A process that exits before the tunnel ever came up (bad config, auth failure)
// must NOT be retried — that would spin pkexec on a hopeless connection.
func TestDropBeforeConnectedGivesUp(t *testing.T) {
	m, ch := armedConnected(t, "alpha")
	m.log.state = StateConnecting // never reached the connected marker

	out, cmd := m.Update(utils.LogClosedMsg{Ch: ch})
	mm := out.(*UI)
	if cmd != nil {
		t.Error("reconnect scheduled for a connection that never came up")
	}
	if mm.log.State() != StateDisconnected {
		t.Errorf("state = %v, want StateDisconnected", mm.log.State())
	}
	if mm.reArmed {
		t.Error("reArmed still set after giving up")
	}
}

// A daemon config forks out of our reach, so a drop must not be retried (it would
// stack a second openvpn on the still-running daemon).
func TestDropDaemonNotReconnected(t *testing.T) {
	m, ch := armedConnected(t, "alpha")
	m.reArmed = false // daemon config: untrackable

	out, cmd := m.Update(utils.LogClosedMsg{Ch: ch})
	if cmd != nil {
		t.Error("daemon config was auto-reconnected")
	}
	if out.(*UI).log.State() != StateDisconnected {
		t.Error("daemon drop did not settle to disconnected")
	}
}

// Once the retry budget is spent, a further drop gives up instead of looping.
func TestDropBudgetExhaustedGivesUp(t *testing.T) {
	m, ch := armedConnected(t, "alpha")
	m.reAttempts = maxReconnects // budget already spent (recent, so no stable reset)

	out, cmd := m.Update(utils.LogClosedMsg{Ch: ch})
	mm := out.(*UI)
	if cmd != nil {
		t.Error("reconnect scheduled past the retry cap")
	}
	if mm.log.State() != StateDisconnected {
		t.Errorf("state = %v at the cap, want StateDisconnected", mm.log.State())
	}
}

// A connection that stayed up past stableUptime is healthy: a later drop earns
// the full retry budget back rather than counting against the spent one.
func TestStableUptimeResetsBudget(t *testing.T) {
	m, ch := armedConnected(t, "alpha")
	m.reAttempts = maxReconnects
	m.connectedAt = time.Now().Add(-2 * stableUptime) // proven stable

	out, cmd := m.Update(utils.LogClosedMsg{Ch: ch})
	mm := out.(*UI)
	if cmd == nil {
		t.Error("stable session did not earn its retry budget back")
	}
	if mm.reAttempts != 1 {
		t.Errorf("reAttempts = %d after stable reset, want 1", mm.reAttempts)
	}
}

// Pressing "d" while a reconnect is pending cancels it — no confirm popup (there
// is no live tunnel to tear down), straight to disconnected.
func TestDisconnectCancelsPendingReconnect(t *testing.T) {
	m, ch := armedConnected(t, "alpha")
	out, _ := m.Update(utils.LogClosedMsg{Ch: ch}) // drop -> reconnecting
	m = out.(*UI)
	if m.log.State() != StateReconnecting {
		t.Fatalf("setup: state = %v, want StateReconnecting", m.log.State())
	}

	out, _ = m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	mm := out.(*UI)
	if mm.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal (no confirm for a pending reconnect)", mm.mode)
	}
	if mm.log.State() != StateDisconnected {
		t.Errorf("state = %v after cancel, want StateDisconnected", mm.log.State())
	}
	if mm.reArmed {
		t.Error("reArmed still set after cancelling the reconnect")
	}
}

// A reconnect timer that fires after the user has moved on (not reconnecting any
// more) is stale and must not start a connection.
func TestStaleReconnectMsgIgnored(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, cmd := m.Update(reconnectMsg{})
	if cmd != nil {
		t.Error("stale reconnectMsg produced a command")
	}
	if out.(*UI).logCh != nil {
		t.Error("stale reconnectMsg started a connection")
	}
}

// A user disconnect disarms auto-reconnect.
func TestDisconnectDisarmsReconnect(t *testing.T) {
	m, _ := armedConnected(t, "alpha")

	m.disconnect()
	if m.reArmed {
		t.Error("reArmed still set after a user disconnect")
	}
}

// Quitting with a live connection confirms first (q/ctrl+c are easy to
// fat-finger). Confirming tears down and disarms auto-reconnect — otherwise a
// drop in flight as the program exits could reschedule a redial, respawning the
// (root) openvpn the user just asked to quit — and returns tea.Quit.
func TestQuitConfirmsThenTearsDown(t *testing.T) {
	m, _ := armedConnected(t, "alpha")

	out, cmd := m.quit()
	mm := out.(*UI)
	if mm.mode != modeConfirm {
		t.Fatalf("quit with a live connection did not confirm (mode=%v)", mm.mode)
	}
	if cmd != nil {
		t.Error("quit ran the teardown before the user confirmed")
	}
	if !mm.reArmed || mm.logCh == nil {
		t.Error("quit tore the connection down before the user confirmed")
	}

	out2, cmd2 := mm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	done := out2.(*UI)
	if done.reArmed {
		t.Error("reArmed still set after confirming quit — a late drop could respawn openvpn")
	}
	if done.logCh != nil {
		t.Error("logCh not cleared after confirming quit")
	}
	if cmd2 == nil {
		t.Error("confirming quit returned no command, expected tea.Quit")
	}
}

// Cancelling the quit confirm keeps the connection and the program running.
func TestQuitConfirmCancelKeepsConnection(t *testing.T) {
	m, _ := armedConnected(t, "alpha")

	out, _ := m.quit()
	out, cmd := out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	kept := out.(*UI)
	if kept.mode != modeNormal {
		t.Errorf("mode = %v after cancel, want modeNormal", kept.mode)
	}
	if kept.logCh == nil {
		t.Error("logCh cleared after cancelling quit, want the connection kept")
	}
	if cmd != nil {
		t.Error("cancelling quit returned a cmd, want none")
	}
}

// Quitting with nothing connected goes straight out — no confirm popup.
func TestQuitNoConnectionImmediate(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, cmd := m.quit()
	if got := out.(*UI).mode; got == modeConfirm {
		t.Error("quit asked for confirmation with no active connection")
	}
	if cmd == nil {
		t.Error("quit with no connection returned no command, expected tea.Quit")
	}
}

// connectingUI builds a UI mid-connect: state StateConnecting with a live logCh,
// the state the management-state poll resolves. MgmtSock() is "" for a Manager
// with no real connection — the poll tags its result with that same value.
func connectingUI(t *testing.T, name string) *UI {
	t.Helper()
	cfg := vpn.Config{Name: name, Path: "/x/" + name + ".ovpn"}
	m := New([]vpn.Config{cfg}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.log.StartConnection(name)
	m.logCh = make(chan string)
	return m
}

// A CONNECTED management state flips the badge to connected and starts the
// stability clock — with no "Initialization Sequence Completed" in the log.
func TestStateResultConnects(t *testing.T) {
	m := connectingUI(t, "alpha")

	out, _ := m.Update(stateResultMsg{sock: m.mgr.MgmtSock(), state: "CONNECTED"})
	mm := out.(*UI)

	if mm.log.State() != StateConnected {
		t.Errorf("State() = %v after CONNECTED, want StateConnected", mm.log.State())
	}
	if mm.connectedAt.IsZero() {
		t.Error("connectedAt not started when the tunnel came up")
	}
}

// A result tagged with a different socket (a poll from a previous connection
// after a switch/reconnect) must not flip the current connection's badge.
func TestStateResultStaleDropped(t *testing.T) {
	m := connectingUI(t, "alpha")

	out, _ := m.Update(stateResultMsg{sock: "/some/other.sock", state: "CONNECTED"})
	mm := out.(*UI)

	if mm.log.State() != StateConnecting {
		t.Errorf("a stale-socket result flipped the badge: State() = %v", mm.log.State())
	}
}

// A poll that is not yet CONNECTED keeps polling (returns a tick cmd) and leaves
// the badge connecting.
func TestStateResultReschedulesWhileConnecting(t *testing.T) {
	m := connectingUI(t, "alpha")

	out, cmd := m.Update(stateResultMsg{sock: m.mgr.MgmtSock(), state: "CONNECTING"})
	mm := out.(*UI)

	if mm.log.State() != StateConnecting {
		t.Errorf("State() = %v, want StateConnecting (not up yet)", mm.log.State())
	}
	if cmd == nil {
		t.Error("a not-yet-connected poll returned no cmd — polling stopped early")
	}
}

// A poll tick while connecting keeps polling even when the socket isn't open yet
// (MgmtSock is "" here): it must reschedule, not give up.
func TestStatePollReschedulesWhileConnecting(t *testing.T) {
	m := connectingUI(t, "alpha")

	_, cmd := m.Update(statePollMsg{})
	if cmd == nil {
		t.Fatal("poll tick while connecting returned no cmd — polling stopped early")
	}
	// With no socket yet the cmd must be the reschedule tick, not a query: a
	// query resolves (to a stateResultMsg) immediately, the tick only after
	// statePollInterval.
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if _, ok := msg.(stateResultMsg); ok {
			t.Error("poll tick with no socket queried the state instead of rescheduling")
		}
	case <-time.After(150 * time.Millisecond):
		// still pending — the tick, as wanted
	}
}

// A poll tick stops (no cmd) once nothing is coming up — no busy-loop after the
// connection settles.
func TestStatePollStopsWhenNotConnecting(t *testing.T) {
	m := New([]vpn.Config{{Name: "alpha", Path: "/x/alpha.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI) // state Idle, logCh nil

	_, cmd := m.Update(statePollMsg{})
	if cmd != nil {
		t.Error("poll tick with no live connection returned a cmd — should stop")
	}
}

// syncConnected mirrors the log state into the shared field the sidebar's ●
// marker reads: the active name while connected, empty in any other state.
func TestSyncConnected(t *testing.T) {
	m := connectingUI(t, "alpha")

	m.syncConnected() // still connecting
	if got := m.sh.connected; got != "" {
		t.Errorf("sh.connected = %q while connecting, want empty", got)
	}

	m.log.MarkConnected()
	m.syncConnected()
	if got := m.sh.connected; got != "alpha" {
		t.Errorf("sh.connected = %q while connected, want alpha", got)
	}

	m.log.MarkDisconnected()
	m.syncConnected()
	if got := m.sh.connected; got != "" {
		t.Errorf("sh.connected = %q after disconnect, want empty", got)
	}
}

// writeTempConfig writes a throwaway .ovpn and returns its Config (Name = name).
func writeTempConfig(t *testing.T, name, body string) vpn.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".ovpn")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return vpn.Config{Name: name, Path: path}
}

// redialCreds is the contract for auto-reconnect: a no-auth config is redialable
// with no creds; a needs-auth config is redialable only when its creds are in
// the keyring (fetched on demand), and not at all otherwise.
func TestRedialCreds(t *testing.T) {
	keyring.MockInit()
	plain := writeTempConfig(t, "plain", "client\ndev tun\n")
	secured := writeTempConfig(t, "secured", "client\nauth-user-pass\n")

	if u, p, ok := redialCreds(plain); !ok || u != "" || p != "" {
		t.Errorf("redialCreds(no-auth) = %q,%q,%v; want \"\",\"\",true", u, p, ok)
	}
	if _, _, ok := redialCreds(secured); ok {
		t.Error("redialCreds(needs-auth, no keyring) reported redialable; want not")
	}

	if err := vpn.SaveCreds(secured.Name, "bob", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if u, p, ok := redialCreds(secured); !ok || u != "bob" || p != "s3cret" {
		t.Errorf("redialCreds(needs-auth, saved) = %q,%q,%v; want bob,s3cret,true", u, p, ok)
	}
}

// If the keyring entry is forgotten between the drop and the redial timer firing,
// the redial gives up instead of spawning openvpn (no creds to use).
func TestReconnectFireGivesUpWithoutKeyring(t *testing.T) {
	keyring.MockInit()
	cfg := writeTempConfig(t, "secured", "client\nauth-user-pass\n") // needs auth, nothing saved

	m := New([]vpn.Config{cfg}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.log.StartConnection(cfg.Name)
	m.log.MarkReconnecting(1) // -> StateReconnecting
	m.reCfg = cfg
	m.reArmed = true
	m.logCh = nil

	out, cmd := m.Update(reconnectMsg{})
	mm := out.(*UI)
	if cmd != nil {
		t.Error("redial with no keyring creds issued a command (spawned openvpn)")
	}
	if mm.log.State() != StateDisconnected {
		t.Errorf("state = %v, want StateDisconnected after give-up", mm.log.State())
	}
	if mm.reArmed {
		t.Error("reArmed still set after give-up")
	}
}

// A redial whose Connect fails (creds unwritable, config gone) settles instead
// of leaving the badge stuck on "reconnecting…" with no retry pending — nothing
// else would ever fire (no stream, no timer, state poll gated on connecting).
func TestReconnectFireFailedRedialSettles(t *testing.T) {
	keyring.MockInit()
	cfg := writeTempConfig(t, "secured", "client\nauth-user-pass\n")
	if err := vpn.SaveCreds(cfg.Name, "bob", "s3cret"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", "") // creds write refused -> Connect fails, no spawn

	m := New([]vpn.Config{cfg}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.log.StartConnection(cfg.Name)
	m.log.MarkReconnecting(1) // -> StateReconnecting
	m.reCfg = cfg
	m.reArmed = true
	m.logCh = nil

	out, cmd := m.Update(reconnectMsg{})
	mm := out.(*UI)
	if cmd != nil {
		t.Error("failed redial issued a command (spawned openvpn)")
	}
	if mm.log.State() != StateError {
		t.Errorf("state = %v after failed redial, want StateError (settled)", mm.log.State())
	}
	if mm.log.Err() == "" {
		t.Error("failed redial lost its error message")
	}
	if mm.reArmed {
		t.Error("reArmed still set after a failed redial settled")
	}
}

// An operation error while a tunnel is live (a failed delete, an unreadable
// config on enter) must not repaint the badge or disable auto-reconnect: the
// state stays connected and a later real drop still schedules the redial.
func TestOpErrorWhileConnectedKeepsReconnect(t *testing.T) {
	m, ch := armedConnected(t, "alpha")

	m.log.SetError("delete config: permission denied") // op unrelated to the tunnel
	if m.log.State() != StateConnected {
		t.Fatalf("op error repainted the badge: State() = %v, want StateConnected", m.log.State())
	}

	out, cmd := m.Update(utils.LogClosedMsg{Ch: ch})
	mm := out.(*UI)
	if cmd == nil {
		t.Error("drop after an op error produced no reconnect timer")
	}
	if mm.log.State() != StateReconnecting {
		t.Errorf("state = %v after drop, want StateReconnecting", mm.log.State())
	}
}

// statusLine surfaces an operation error even while the badge shows a live
// connection — and keeps showing which connection is live next to it.
func TestStatusLineShowsOpErrorWhileConnected(t *testing.T) {
	m, _ := armedConnected(t, "alpha")
	m.log.SetError("boom")
	got := m.statusLine()
	if !strings.Contains(got, "boom") {
		t.Errorf("statusLine = %q, want it to contain the op error", got)
	}
	if !strings.Contains(got, "alpha") {
		t.Errorf("statusLine = %q, want the live connection name alongside the error", got)
	}
}

// A new connect attempt supersedes a stale operation error.
func TestEnterClearsStaleError(t *testing.T) {
	keyring.MockInit()
	cfg := writeTempConfig(t, "secured", "client\nauth-user-pass\n")
	m := New([]vpn.Config{cfg}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.log.SetError("boom")

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // opens the auth modal
	if got := out.(*UI).log.Err(); got != "" {
		t.Errorf("Err() = %q after enter, want cleared", got)
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

// armReconnect's decision is the whole "auto-reconnect is a keyring feature"
// contract: armed only for a trackable (non-daemon) config that can be silently
// redialed. The reconnect tests set reArmed by hand, so the decision itself
// needs its own coverage.
func TestArmReconnectDecision(t *testing.T) {
	keyring.MockInit()
	tests := []struct {
		name    string
		content string
		saved   bool // pre-save creds in the keyring for this config
		want    bool
	}{
		{"no-auth, trackable", "client\ndev tun\n", false, true},
		{"daemon forks out of reach", "client\ndaemon\n", false, false},
		{"needs auth, nothing saved", "client\nauth-user-pass\n", false, false},
		{"needs auth, creds saved", "client\nauth-user-pass\n", true, true},
		{"daemon wins even with saved creds", "client\ndaemon\nauth-user-pass\n", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := writeTempConfig(t, tt.name, tt.content)
			if tt.saved {
				if err := vpn.SaveCreds(cfg.Name, "u", "p"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = vpn.ForgetCreds(cfg.Name) })
			}

			m := &UI{connectedAt: time.Now()} // non-zero clock to prove the reset
			m.armReconnect(cfg)

			if m.reArmed != tt.want {
				t.Errorf("reArmed = %v, want %v", m.reArmed, tt.want)
			}
			if m.reCfg.Name != cfg.Name {
				t.Errorf("reCfg = %q, want %q", m.reCfg.Name, cfg.Name)
			}
			if !m.connectedAt.IsZero() {
				t.Error("connectedAt not reset — the stability clock must restart per attempt")
			}
		})
	}
}

// A config whose file cannot be read is never armed (NeedsAuth errors → not
// redialable), so a drop on it does not spin pkexec on a hopeless connection.
func TestArmReconnectUnreadableConfig(t *testing.T) {
	m := &UI{}
	m.armReconnect(vpn.Config{Name: "ghost", Path: "/does/not/exist.ovpn"})
	if m.reArmed {
		t.Error("reArmed = true for an unreadable config, want false")
	}
}

// "r" in the menu opens the rename prompt, prefilled with the selected name.
func TestMenuRenameOpensPrompt(t *testing.T) {
	m := New([]vpn.Config{{Name: "vpn", Path: "/x/vpn.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // open menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'r', Text: "r"}) // rename
	mm := out.(*UI)
	if mm.mode != modeRename {
		t.Fatalf("mode = %v after menu rename, want modeRename", mm.mode)
	}
	if got := mm.rename.Value(); got != "vpn" {
		t.Errorf("rename field = %q, want prefilled vpn", got)
	}
}

// The full rename path: x → r, edit the name, enter renames the file on disk and
// updates the sidebar entry.
func TestRenameConfirmRenamesFileAndSidebar(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	path := filepath.Join(dir, "old.ovpn")
	if err := os.WriteFile(path, []byte("client\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "old", Path: path}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'r', Text: "r"}) // rename (prefill "old")
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Text: "-2"})           // → "old-2"
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEnter})   // confirm
	mm := out.(*UI)

	if mm.mode != modeNormal {
		t.Fatalf("mode = %v after rename, want modeNormal", mm.mode)
	}
	cfg, _ := mm.sidebar.SelectedConfig()
	if cfg.Name != "old-2" {
		t.Errorf("sidebar name = %q, want old-2", cfg.Name)
	}
	if _, err := os.Stat(filepath.Join(dir, "old-2.ovpn")); err != nil {
		t.Errorf("renamed file missing: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("old file still present after rename")
	}
	// The log pane retitles to the new name (buffer remapped + shown).
	if v := mm.log.View(false); !strings.Contains(v, "old-2") {
		t.Errorf("log pane not refreshed to the new name:\n%s", v)
	}
}

// A pending auto-reconnect also counts as in use: renaming that config is
// refused (its reCfg is keyed by the old path/name).
func TestRenameBlockedWhileReconnectArmed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conn.ovpn")
	if err := os.WriteFile(path, []byte("client\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := vpn.Config{Name: "conn", Path: path}

	m := New([]vpn.Config{cfg}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.reArmed = true // a redial is pending (logCh nil, no live tunnel)
	m.reCfg = cfg

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Text: "-2"})
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mm := out.(*UI)

	if mm.mode != modeRename {
		t.Errorf("mode = %v, want modeRename (blocked by pending reconnect)", mm.mode)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file renamed despite a pending reconnect: %v", err)
	}
}

// Renaming a connection that is live is refused: the prompt stays open with an
// error and the file is untouched (name-keyed state would otherwise desync).
func TestRenameBlockedWhenConnected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live.ovpn")
	if err := os.WriteFile(path, []byte("client\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "live", Path: path}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.log.StartConnection("live")
	m.logCh = make(chan string) // simulate a live stream

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'r', Text: "r"}) // rename
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Text: "-2"})           // edit
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEnter})   // confirm → blocked
	mm := out.(*UI)

	if mm.mode != modeRename {
		t.Errorf("mode = %v, want modeRename (blocked, stays open)", mm.mode)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("live config file renamed despite being in use: %v", err)
	}
}

// An invalid new name keeps the prompt open and leaves the file untouched.
func TestRenameInvalidNameStaysOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.ovpn")
	if err := os.WriteFile(path, []byte("client\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "old", Path: path}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'r', Text: "r"}) // rename
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Text: "/evil"})        // invalid (path sep)
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEnter})   // confirm → error
	mm := out.(*UI)

	if mm.mode != modeRename {
		t.Errorf("mode = %v after invalid name, want modeRename (stays open)", mm.mode)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("original file gone after a refused rename: %v", err)
	}
}

// Menu "d" opens the delete-connection confirm.
func TestMenuDeleteOpensConfirm(t *testing.T) {
	m := New([]vpn.Config{{Name: "vpn", Path: "/x/vpn.ovpn"}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'd', Text: "d"}) // delete
	if got := out.(*UI).mode; got != modeConfirm {
		t.Fatalf("mode = %v after menu delete, want modeConfirm", got)
	}
}

// Confirming a delete removes the file, its saved creds, and the sidebar entry;
// the cursor clamps to a remaining connection.
func TestDeleteConfirmRemovesEverything(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	gone := filepath.Join(dir, "gone.ovpn")
	keep := filepath.Join(dir, "keep.ovpn")
	for _, p := range []string{gone, keep} {
		if err := os.WriteFile(p, []byte("client\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := vpn.SaveCreds("gone", "u", "p"); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "gone", Path: gone}, {Name: "keep", Path: keep}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.sidebar.Move(1) // select "keep"... then back to delete "gone"
	m.sidebar.Move(-1)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // menu on "gone"
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'd', Text: "d"}) // delete
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'y', Text: "y"}) // confirm
	mm := out.(*UI)

	if mm.mode != modeNormal {
		t.Fatalf("mode = %v after delete, want modeNormal", mm.mode)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Error("deleted file still present")
	}
	if _, _, ok, _ := vpn.LoadCreds("gone"); ok {
		t.Error("credentials still present after delete")
	}
	if cfg, ok := mm.sidebar.SelectedConfig(); !ok || cfg.Name != "keep" {
		t.Errorf("selected = %+v ok=%v, want keep", cfg, ok)
	}
}

// A successful delete supersedes a stale operation error — the old message must
// not survive into the post-delete status line.
func TestDeleteClearsStaleError(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "gone.ovpn")
	if err := os.WriteFile(path, []byte("client\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "gone", Path: path}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.log.SetError("boom")

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'd', Text: "d"}) // delete
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'y', Text: "y"}) // confirm

	if got := out.(*UI).log.Err(); got != "" {
		t.Errorf("Err() = %q after a successful delete, want cleared", got)
	}
}

// Cancelling the delete confirm leaves the file, creds, and list untouched.
func TestDeleteCancelKeeps(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	path := filepath.Join(dir, "keep.ovpn")
	if err := os.WriteFile(path, []byte("client\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := vpn.SaveCreds("keep", "u", "p"); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "keep", Path: path}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'd', Text: "d"}) // delete
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEsc})     // cancel
	mm := out.(*UI)

	if mm.mode != modeNormal {
		t.Errorf("mode = %v after cancel, want modeNormal", mm.mode)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file removed after cancelling delete: %v", err)
	}
	if _, _, ok, _ := vpn.LoadCreds("keep"); !ok {
		t.Error("credentials removed after cancelling delete")
	}
	if _, ok := mm.sidebar.SelectedConfig(); !ok {
		t.Error("connection removed from the list after cancelling delete")
	}
}

// Deleting a live connection tears the tunnel down first, then removes it.
func TestDeleteLiveConnectionDisconnectsAndRemoves(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	path := filepath.Join(dir, "live.ovpn")
	if err := os.WriteFile(path, []byte("client\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "live", Path: path}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)
	m.log.StartConnection("live")
	m.logCh = make(chan string) // simulate a live stream

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'd', Text: "d"}) // delete
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'y', Text: "y"}) // confirm
	mm := out.(*UI)

	if mm.logCh != nil {
		t.Error("logCh still set after deleting the live connection")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("deleted file still present")
	}
	if _, ok := mm.sidebar.SelectedConfig(); ok {
		t.Error("connection still listed after delete")
	}
}

// A delete that fails (the file can't be removed) surfaces the error and leaves
// the connection in the list.
func TestDeleteFailureKeepsConnection(t *testing.T) {
	keyring.MockInit()
	// A path with no file on disk makes vpn.DeleteConfig's os.Remove fail.
	ghost := filepath.Join(t.TempDir(), "ghost.ovpn")
	m := New([]vpn.Config{{Name: "ghost", Path: ghost}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'd', Text: "d"}) // delete
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'y', Text: "y"}) // confirm → fails
	mm := out.(*UI)

	if _, ok := mm.sidebar.SelectedConfig(); !ok {
		t.Error("connection removed from the list despite the delete failing")
	}
	if mm.log.State() != StateError {
		t.Errorf("state = %v after a failed delete, want StateError", mm.log.State())
	}
}

// esc while the rename prompt is open cancels back to normal: the file keeps
// its name, the sidebar entry is untouched, and the typed edit is dropped.
func TestRenameEscCancels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keep.ovpn")
	if err := os.WriteFile(path, []byte("client\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := New([]vpn.Config{{Name: "keep", Path: path}}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})        // menu
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 'r', Text: "r"}) // rename
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Text: "-nope"})        // edit
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEsc})     // cancel
	mm := out.(*UI)

	if mm.mode != modeNormal {
		t.Fatalf("mode = %v after esc, want modeNormal", mm.mode)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file renamed despite cancel: %v", err)
	}
	if cfg, _ := mm.sidebar.SelectedConfig(); cfg.Name != "keep" {
		t.Errorf("sidebar name = %q after cancel, want keep", cfg.Name)
	}
}

// The rename flow with nothing selected (empty list) backs out to normal mode
// instead of opening a prompt for a connection that does not exist. Both the
// open and the confirm guard are exercised directly — the menu key path cannot
// reach them (the menu refuses to open with no selection).
func TestRenameNoSelectionBacksOut(t *testing.T) {
	m := New(nil, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, cmd := m.openRename()
	if mode := out.(*UI).mode; mode != modeNormal {
		t.Errorf("mode = %v after openRename with no selection, want modeNormal", mode)
	}
	if cmd != nil {
		t.Error("openRename with no selection returned a command")
	}

	m.mode = modeRename // force the prompt open, then confirm with no selection
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if mode := out.(*UI).mode; mode != modeNormal {
		t.Errorf("mode = %v after confirm with no selection, want modeNormal", mode)
	}
}

// Submitting the credential modal with the save toggle on stores the creds in
// the keyring, hands them to connect, and clears the fields. Connect is forced
// to fail before spawning anything (XDG_RUNTIME_DIR unset refuses the creds
// temp file), so the flow is exercised without openvpn.
func TestAuthSubmitSavesCredsAndClears(t *testing.T) {
	keyring.MockInit()
	cfg := writeTempConfig(t, "secured", "client\nauth-user-pass\n")
	t.Setenv("XDG_RUNTIME_DIR", "")

	m := New([]vpn.Config{cfg}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open the modal
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Text: "bob"})
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyTab})
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Text: "s3cret"})
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}) // save on
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEnter})          // submit
	mm := out.(*UI)

	if mm.mode != modeNormal {
		t.Fatalf("mode = %v after submit, want modeNormal", mm.mode)
	}
	user, pass, has, err := vpn.LoadCreds("secured")
	if err != nil || !has {
		t.Fatalf("LoadCreds: has=%v err=%v, want saved creds", has, err)
	}
	if user != "bob" || pass != "s3cret" {
		t.Errorf("saved creds = %q/%q, want bob/s3cret", user, pass)
	}
	if mm.creds.Username() != "" || mm.creds.Password() != "" {
		t.Error("modal fields not cleared after submit")
	}
	if mm.log.Err() == "" {
		t.Error("failed connect after submit surfaced no error")
	}
}

// Without the save toggle, submitting connects but writes nothing to the
// keyring — persistence is strictly opt-in.
func TestAuthSubmitWithoutToggleSavesNothing(t *testing.T) {
	keyring.MockInit()
	cfg := writeTempConfig(t, "secured", "client\nauth-user-pass\n")
	t.Setenv("XDG_RUNTIME_DIR", "")

	m := New([]vpn.Config{cfg}, vpn.NewManager())
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(*UI)

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open the modal
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Text: "bob"})
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyTab})
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Text: "s3cret"})
	out, _ = out.(*UI).Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // submit
	mm := out.(*UI)

	if mm.mode != modeNormal {
		t.Fatalf("mode = %v after submit, want modeNormal", mm.mode)
	}
	if _, _, has, _ := vpn.LoadCreds("secured"); has {
		t.Error("creds saved to the keyring without the opt-in toggle")
	}
	if mm.creds.Username() != "" || mm.creds.Password() != "" {
		t.Error("modal fields not cleared after submit")
	}
}
