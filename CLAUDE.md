# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`lazyovpn` — a TUI for managing OpenVPN connections on Linux, in the spirit of `lazydocker`/`lazygit`. Cobra CLI entry → Bubble Tea TUI.

## Commands

```bash
go run .              # launch the TUI (discovers configs in /etc/openvpn* + ~/.config/lazyovpn/connections)
go build ./...        # build
go vet ./...          # static check
gofmt -w <file>       # format (mandatory before considering work done)
go test -race ./...   # run all tests
go test -race -run TestName ./internal/vpn   # run a single test
go test -cover ./...  # coverage
gremlins unleash      # mutation testing (config: .gremlins.yaml; install: go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0)
```

Mutation testing gotchas: `gremlins unleash` does **not** expand `./...` — run it
with no path (whole module) or a concrete dir (`./internal/vpn`). The very fast
suite needs `timeout-coefficient` high (set in `.gremlins.yaml`) or mutants get
wrongly flagged TIMED OUT under CPU contention. The efficacy gate lives in the
nested `threshold.efficacy` config key (the `--threshold-efficacy` CLI flag is
silently ignored by the current build).

The TUI cannot be exercised non-interactively (needs a TTY, a real `.ovpn`, and a polkit password prompt) — verify logic with tests + `go build` + `go vet`, and run manually for end-to-end behavior.

## What lives where

```
main.go                          cobra root cmd → discovers configs → launches the Bubble Tea program
internal/files/picker.go         native file-chooser (zenity/kdialog) resolution + Pick()
internal/files/copy.go           generic Copy(src, dst, perm)
internal/vpn/vpn.go              config discovery + import, auth detection, privileged openvpn process management
internal/vpn/rename.go           RenameConfig: migrate keyring creds + rename the config file
internal/vpn/keyring.go          opt-in credential storage in the OS keyring (Save/Load/ForgetCreds)
internal/ui/common/box.go        reusable TitledBox (title inlined into the top border)
internal/ui/common/overlay.go    Overlay/Center: lipgloss v2 layer compositing for floating popups
internal/ui/common/popup.go      Popup: auto-sizing titled box for overlay content (wraps TitledBox)
internal/ui/common/theme.go      shared color palette + Hint style
internal/ui/dialog/confirm.go    Confirm: reusable yes/no popup (title + message + onYes closure)
internal/ui/dialog/menu.go       Menu: per-connection action menu (aligned key/label columns)
internal/ui/dialog/filepicker.go FilePicker: native file-chooser wrapper (Open cmd + FilePickedMsg + status render)
internal/ui/dialog/credentials.go credential prompt (two textinputs; password masked, never persisted)
internal/ui/dialog/rename.go     rename prompt (one textinput, prefilled with the current name)
internal/ui/utils/stream.go      shared plumbing: LogMsg/LogClosedMsg/WaitForLog
internal/ui/model/ui.go          root model: global state, layout, key routing, mode switch, composition
internal/ui/model/sidebar.go     connection-list panel (hand-rolled rows + cursor; reads shared.connected)
internal/ui/model/log.go         log panel (scrollable viewport + per-connection buffers + ConnState/Badge)
internal/ui/model/add.go         import-connection flow (drives dialog.FilePicker + vpn.ImportConfig)
internal/ui/model/rename.go      rename-connection flow (drives dialog.Rename + vpn.RenameConfig; in-use guard)
```

The TUI is split into packages so each panel owns its own state and business
rules, and new views can be added without touching the others. The import graph
has no cycles: `files` (OS file helpers) and `common` depend on nothing
internal; `dialog` builds on `common` + `files` (the file chooser); `utils`
depends on nothing internal (just the log pump now); `vpn` depends on `files`;
`model` (the root model `ui.go` plus the panels) depends on `common`, `dialog`,
`utils`, `vpn`. The split line for files:
`vpn` knows *what an OpenVPN config is* (discover/import/auth); `files` knows
*how to talk to the OS about files* (chooser/copy). Each panel renders its own
bordered box; the root only places panels and routes messages.

### `internal/files` — OS file helpers

- `Pick()` resolves the first installed native file-chooser dialog (`zenity`, then `kdialog`) and runs it, returning the chosen path, `ErrCanceled`, or `ErrNoChooser`. It's a separate GUI window, so it blocks — run it off the UI goroutine (`dialog.FilePicker.Open` does). Linux desktop dialogs only.
- `Copy(src, dst, perm)` is a generic file copy that forces `dst`'s mode to `perm` (so re-copies stay private).

### `internal/vpn` — process management

- `Manager` holds **one** active connection at a time.
- `Connect(c, username, password)` spawns `openvpn` via `pkexec` and wires the child's combined stdout+stderr through a **pty** (`openPTY`, Linux `/dev/ptmx`) into a **`<-chan string`** of log lines, returned to the caller. The pty (not a plain pipe) makes openvpn see a tty so libc **line-buffers** its log, so the log pane streams in real time — over a pipe it block-buffers (~4KB) and lines arrive late and bunched. (Tunnel-up detection does **not** depend on log content — see Connection state.) A single scanner goroutine owns the channel and is the **only** caller of `cmd.Wait()` (reaper); on child exit the pty master read returns EIO, which the scanner treats as a normal end-of-stream.
- **Discovery + import**: `Discover()` scans the system dirs (`/etc/openvpn/client`, `/etc/openvpn`) plus `ConnectionsDir()` (`~/.config/lazyovpn/connections`, the default home for user-added configs). `ImportConfig(src)` validates the `.ovpn/.conf` extension, copies the file into `ConnectionsDir()` at 0600 (configs can carry inline keys), and returns the new `Config`.
- **Rename**: a connection's name **is** its config filename, so `RenameConfig(c, newName)` (`rename.go`) renames the file (keeping its extension) and returns the updated `Config`. It refuses a non-bare name (empty, `.`/`..`, or containing a path separator — traversal) and refuses to overwrite an existing config. **Credentials migrate first**: if the old name has a keyring entry it is copied to the new name *before* the file moves, so a keyring failure aborts with nothing changed; only once both the migration and the file rename succeed is the old entry removed, and a file-rename failure rolls the migrated entry back. A config with no saved creds just gets its file renamed. (The UI blocks rename of an in-use connection — see `model` Modes.)
- **Auth**: `NeedsAuth(c)` reports whether a config has a bare `auth-user-pass` directive (no creds file → needs a prompt). When credentials are supplied, `Connect` writes them to a 0600 temp file on tmpfs (`$XDG_RUNTIME_DIR`), passes it via `--auth-user-pass`, and removes it when the connection ends — the password is never written to a plaintext file on durable storage.
- **Saved credentials (opt-in)**: `keyring.go` wraps `github.com/zalando/go-keyring` (Secret Service / libsecret on Linux, pure-Go D-Bus, no cgo). `SaveCreds(name, user, pass)` stores `"user\npass"` under service `lazyovpn`; `LoadCreds(name)` returns `(user, pass, ok, err)` — `ok=false` with nil err when nothing is stored; `ForgetCreds(name)` deletes (missing is a no-op). Saving is only ever triggered by the modal's explicit save toggle. Keyring calls run **inline** on the UI goroutine — fine while the login keyring is unlocked (the usual case); move to a `tea.Cmd` if a locked keyring ever hangs the UI. Tested via `keyring.MockInit()`.
- **Teardown via the management socket**: `openvpn` runs as **root** (pkexec) while the TUI is **unprivileged**, so a plain `kill(2)` on it is `EPERM` — it would leak the process (and a duplicate-cert openvpn flaps the server). So `Connect` passes `--management <sock> unix --management-client-user <me>` (socket in `$XDG_RUNTIME_DIR`, reachable only by the running user), and `stop()` calls `signalQuit` → dials the socket, writes `signal SIGTERM`, then **drains until EOF**: openvpn replies and exits, closing the socket, so `signalQuit` blocks until the process is actually gone (bounded: 1s dial + 2s drain deadline). That makes teardown deterministic instead of racing the caller's next step (a switch's new process, the program quitting). `stop()` still `Kill`s afterward as a fallback (works only for a non-pkexec/test process; `EPERM` and harmless in real use) and closes the `done` channel; the scanner goroutine reaps the pipe on EOF. `cmd/app.go` `defer mgr.Disconnect()` guarantees teardown on **every** exit path (`q`, ctrl+c, error) — without it a ctrl+c left the root openvpn running; the `q`/`ctrl+c` key path uses `UI.quit`, which disarms auto-reconnect first so a late drop can't respawn the process being torn down. `signalQuit` runs inline on the UI goroutine. **Manual-verify only** (privileged path, no unit test for the live socket): connect, then `q`/`d`, confirm `pgrep openvpn` is empty.

### `internal/ui/model` — root model + panels

- Layout: bordered sidebar (`model.Sidebar`) + bordered log pane (`model.Log`), plus a status line and help footer. `layout()` reserves 2 rows (status + help) and accounts for each pane's border+padding when sizing.
- See `internal/ui/CLAUDE.md` for the UI-tree architecture guide (UI as the sole model, the imperative-panel convention, the constants/style rules).
- **Modes**: the root model (the `UI` struct in `ui.go`, the sole bubbletea model) has an `appMode` (`modeNormal`/`modeAuth`/`modeAdd`/`modeConfirm`/`modeMenu`/`modeRename`). An open overlay owns all input except the global log stream. `modeAuth`: `enter` on a connection that `NeedsAuth` first tries `vpn.LoadCreds` — saved creds skip the prompt and connect straight away; otherwise the credential prompt (`dialog.Credentials`) opens. Inside it, `enter` hands creds to `Connect` and clears them (and, if the save toggle is on — `ctrl+s` — calls `vpn.SaveCreds` best-effort first), `esc` cancels. `modeConfirm`: a reusable `dialog.Confirm` (title + message + an `onYes` closure returning a `tea.Cmd`) backs every yes/no popup. The forget-creds action lives in the action menu (`modeMenu`, below), not on a top-level key; `d` on a live connection (`logCh != nil`) confirms tearing the tunnel down (`onYes` = `m.disconnect`); `q`/`ctrl+c` on a live connection confirms before quitting (`onYes` disarms reconnect, tears the tunnel down, and returns `tea.Quit` — `onYes` returns a `tea.Cmd` precisely so the quit popup can emit `tea.Quit`). `y`/`enter` runs the action and propagates its cmd, `n`/`esc` backs out; with nothing stored/connected the key is a no-op (quit with nothing connected exits straight away, no popup). `modeAdd`: `a` opens the import flow (`add.go`), which launches the native file chooser via `dialog.FilePicker.Open`; when a `dialog.FilePickedMsg` arrives the picker shows the path, `enter` runs `vpn.ImportConfig` + `sidebar.AddConfig`, `r` re-picks, `esc` cancels. `modeMenu`: `x` opens the per-connection action menu (`dialog.Menu`) for the selected connection; its rows are keybind/label columns and the UI routes each action key — `f` forgets saved creds (opens the forget `modeConfirm`, no-op when nothing is stored), `r` opens rename, `esc`/`x` close. `modeRename`: `r` in the menu opens the rename prompt (`dialog.Rename`, prefilled with the current name); `enter` runs `vpn.RenameConfig` (migrate keyring creds, then rename the file — see the `vpn` section), `esc` cancels. A rename that fails (invalid name, target exists, keyring migration error) keeps the prompt open with the error; renaming a connection that is **in use** (`m.inUse` — live tunnel or a config with a pending auto-reconnect) is refused, since that state is keyed by name. On success `sidebar.RenameConfig` and `log.RenameBuffer` re-key the panels to the new name. Overlays float over the live view via `common.Center` (lipgloss v2 `Canvas`/`Layer` compositing).
- **Pane focus**: in normal mode a `pane` field (`focusSidebar`/`focusLog`) selects which panel gets navigation keys — orthogonal to `appMode` (an open overlay still takes precedence). Default is the sidebar (list nav + connect). `tab` moves focus to the log pane (and `tab`/`esc` move it back); there `up`/`down`/`j`/`k`/`pgup`/`pgdn` scroll the log viewport (forwarded to `Log.Scroll`), `q` still quits. The mouse wheel scrolls the log too, handled globally (a `tea.MouseWheelMsg` is routed to `Log.Scroll` regardless of focus — the sidebar doesn't scroll), which is why `altView` sets `v.MouseMode`. Focus is a dedicated toggle, **not** overloaded onto `enter` — `enter` would be ambiguous (connect vs view) and we cannot reliably tell "still connected" once the foreground openvpn exits (e.g. a `daemon` config closes `logCh` while the tunnel stays up). The help footer (`helpFooter()`) swaps to the scroll hints while focused. `enter` connects the selected config, or is a **no-op** when it is already the live connection (`logCh != nil` and the active one) — it never reconnects on top of itself.
- **Charm v2 stack**: bubbletea, bubbles, and lipgloss are all **v2** (`charm.land/...`). v2 gotchas: `Model.View()` returns a `tea.View` (not a string) — wrap content with the `altView` helper, which also sets `v.AltScreen = true` and `v.MouseMode` (both declarative per-frame in v2, not `NewProgram` options); key presses arrive as `tea.KeyPressMsg` (`tea.KeyMsg` is now an interface), matched via `.String()`. Compositing for the popup uses lipgloss v2 `Canvas`/`Layer` behind `common.Overlay`/`Center`.
- **Log streaming**: `utils.WaitForLog(ch)` is a `tea.Cmd` that blocks on the channel and emits `utils.LogMsg`/`utils.LogClosedMsg`; each handler re-issues it to pump the next line.
- **Per-connection output**: `Log.buffers` keeps each connection's log. The active connection keeps filling its buffer even when another is selected; navigating the list (`Log.ShowBuffer`) swaps which buffer the viewport shows. `AppendLog` tail-follows (auto-scrolls to the bottom) only while the viewport is already at the bottom — once the user scrolls up to read history, new lines no longer yank it down.
- **Connection state**: `model.ConnState` badge (`connecting…`→`connected`→`disconnected`/`error`, plus `reconnecting…`). `connected` is detected from openvpn's **management state**, not by scraping the log: while connecting, the UI polls `vpn.QueryState` on the management socket every `statePollInterval` (`statePollMsg` ticks → a cmd dials and emits `stateResultMsg`) and flips to `connected` on `CONNECTED`. This is authoritative regardless of the config's `verb`/`mute` — the old approach scraped the log for `"Initialization Sequence Completed"`, which a low-`verb`/`mute` config never prints, so the badge hung forever despite a live tunnel. `stateResultMsg` carries the socket it queried and is dropped unless it matches `mgr.MgmtSock()` — the stale-channel guard by socket, so a switch/reconnect can't flip the wrong connection. Polling stops once connected (or the connection is gone/settled).
- **Auto-reconnect**: the only `utils.LogClosedMsg` that survives the stale-channel guard (#4) is an *unexpected* process exit (user disconnect / switch / quit all null `logCh` first). `handleDrop` redials there: if the tunnel had reached `StateConnected` and the config is `reArmed` (everything except a `daemon` config, which forks out of reach — `vpn.HasDaemon`), it schedules a `reconnectMsg` via `tea.Tick(reconnectDelay)`, up to `maxReconnects` tries; a session up ≥ `stableUptime` resets the budget. **No credentials are held in memory** — auto-reconnect is a keyring feature: `reArmed` is true only when the config is trackable (not `daemon` — `vpn.HasDaemon`) **and** silently redialable (`redialCreds`: no-auth, or creds present in the keyring). `redialCreds` re-fetches the saved creds fresh from the keyring at redial time; a needs-auth config with nothing saved is never armed (drop → straight to `disconnected`, no `reconnecting…` flash), and if a keyring entry is forgotten mid-connection the redial gives up when the timer fires. `disarmReconnect` just clears the flags (nothing to wipe). `enter` resets the budget (manual intent); the auto path does not. `d` cancels a pending reconnect (no confirm — no live tunnel yet to tear down).

## Hard invariants — never break these

1. **Tests, as much coverage as possible — and they must have teeth.** Every non-trivial change ships with tests (table-driven, `go test -race`). Aim to cover the maximum of the code you can — logic in `internal/vpn` (config discovery, lifecycle) is fully testable; cover it. Don't ship logic with no test behind it. Tests assert observable behavior, not implementation mechanics, so a real regression fails them; **mutation testing (`gremlins unleash`) gates efficacy ≥85% in CI** to catch tautological tests. When a test breaks, fix the code — never rewrite the assertion to pass, unless the test itself encoded wrong behavior.
2. **The TUI process stays unprivileged.** Only `openvpn` runs as root, spawned via `pkexec`. Never add a "run as root" dependency or escalate the TUI itself — the privilege boundary is the whole design.
3. **Exactly one `cmd.Wait()` per process** (the scanner goroutine's reaper). A second `Wait` panics.
4. **Stale-channel guard.** Every `utils.LogMsg`/`utils.LogClosedMsg` carries its source channel; the root handlers drop it if `msg.Ch != m.logCh`. This is what keeps a connection switch from mixing old and new output — keep it on any new log-channel message.
5. **English only.** All code, comments, and user-facing strings are in English. (The user converses in Portuguese; the codebase stays English.)
6. **Passwords never touch plaintext storage.** During a connection, credentials go to a 0600 tmpfs temp file removed when the connection ends, never to a plaintext file on durable storage or to logs; the modal clears its fields right after handoff. The **only** allowed persistence is the OS keyring (Secret Service / libsecret), and **only** when the user explicitly opts in via the modal's save toggle (`internal/vpn/keyring.go`). Never write creds anywhere else, and never default the save toggle to on.

## Release checklist

Releases are built by **GoReleaser** (`.goreleaser.yaml`), triggered by the
`release` workflow on any `v*` tag push. The release version comes from the tag
via ldflags (`-X main.version`); the `version` var in `main.go` is only the
dev fallback for plain `go build`/`go run`. GoReleaser builds linux amd64+arm64,
runs `go test` as a pre-hook, and publishes tar.gz archives + checksums with a
commit-grouped changelog. To cut a version:

- [ ] `go test -race ./...` passes; `go vet ./...` clean and `gofmt` applied.
- [ ] `gremlins unleash` passes the efficacy gate (the release workflow blocks on it).
- [ ] Update `CHANGELOG.md`: move `[Unreleased]` entries under a new `[vX.Y.Z] - DATE` heading and refresh the compare links at the bottom.
- [ ] (Optional, local dry run) `goreleaser release --snapshot --clean`.
- [ ] Tag `vX.Y.Z` and push it — the workflow does the rest.

## Known ceilings

- Single connection only.
- `pkexec` hard-coded (no `sudo` fallback).
- Async teardown → brief two-process overlap on switch.
- Teardown goes through openvpn's management socket (`signal SIGTERM`); a hard SIGKILL of the TUI itself (vs `q`/ctrl+c) skips `defer mgr.Disconnect()` and would still leak the root openvpn.
- Tunnel-up is detected by **polling** the management `state` (~1s cadence) rather than subscribing to `>STATE:` events, so the badge can lag up to `statePollInterval` behind the actual connection; `disconnected`/`reconnecting` still come from the log stream closing, not management state.
- A self-exiting openvpn leaves `Manager.active` non-nil until the next connect/disconnect.
- Auth is `auth-user-pass` (username + password) only — no key-passphrase (`askpass`) prompt. Credentials can be saved to the OS keyring (opt-in); there is no per-config TTL/expiry on a saved entry — it lives until `x` forgets it.
- Creds temp file lives for the whole connection on tmpfs (same-user read window); tighten to a FIFO / delete-after-read if it matters.
- File chooser is Linux desktop dialogs only (`zenity`/`kdialog`); needs a display, with no in-TUI picker fallback.
- Auto-reconnect only covers connections that can be silently redialed: no-auth configs, or `auth-user-pass` configs whose credentials are saved in the keyring (fetched on demand, never held in memory). A password typed once but not saved is not auto-reconnected — save it to opt in. It is also off for `daemon` configs (untrackable), capped at `maxReconnects`, and on by default with no in-app toggle (press `d` to cancel a pending one).
- Auto-reconnect re-invokes `pkexec`, so polkit may re-prompt for the root password on a redial once its auth cache (`auth_admin_keep`, ~5 min) expires — it is not a silent background reconnect after that window.
