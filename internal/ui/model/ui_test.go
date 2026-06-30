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

// A close from a stale channel (invariant #4) must not trigger a reconnect.
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

// Quitting must disarm auto-reconnect and clear logCh too — otherwise a drop in
// flight as the program exits could reschedule a redial, respawning the (root)
// openvpn the user just asked to quit. It also returns tea.Quit.
func TestQuitDisarmsReconnect(t *testing.T) {
	m, _ := armedConnected(t, "alpha")

	out, cmd := m.quit()
	mm := out.(*UI)
	if mm.reArmed {
		t.Error("reArmed still set after quit — a late drop could respawn openvpn")
	}
	if mm.logCh != nil {
		t.Error("logCh not cleared after quit")
	}
	if cmd == nil {
		t.Error("quit returned no command, expected tea.Quit")
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
