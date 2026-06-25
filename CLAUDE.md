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
internal/vpn/keyring.go          opt-in credential storage in the OS keyring (Save/Load/ForgetCreds)
internal/ui/common/box.go        reusable TitledBox (title inlined into the top border)
internal/ui/common/overlay.go    Overlay/Center: lipgloss v2 layer compositing for floating popups
internal/ui/common/dialog.go     Dialog: centered, auto-sizing popup box (wraps TitledBox + Center)
internal/ui/common/theme.go      shared color palette + Hint style
internal/ui/utils/stream.go      shared plumbing: LogMsg/LogClosedMsg/WaitForLog
internal/ui/utils/picker.go      bubbletea glue for the file chooser: PickFile cmd + FilePickedMsg
internal/ui/model/ui.go          root model: global state, layout, key routing, mode switch, composition
internal/ui/model/sidebar.go     connection-list panel (bubbles/list + selection/filter + AddConfig)
internal/ui/model/terminal.go    output panel (viewport + per-connection buffers + ConnState/Badge)
internal/ui/model/authmodal.go   credential modal (two textinputs; password masked, never persisted)
internal/ui/model/addmodal.go    import-connection modal (launches the file chooser, confirms on enter)
```

The TUI is split into packages so each panel owns its own state and business
rules, and new views can be added without touching the others. The import graph
has no cycles: `files` (OS file helpers) depends on nothing internal;
`common` depends on nothing internal; `utils` depends on `files`; `vpn`
depends on `files`; `model` (the root model `ui.go` plus the per-panel
sub-models) depends on `common`, `utils`, `vpn`. The split line for files:
`vpn` knows *what an OpenVPN config is* (discover/import/auth); `files` knows
*how to talk to the OS about files* (chooser/copy). Each panel renders its own
bordered box; the root only places panels and routes messages.

### `internal/files` — OS file helpers

- `Pick()` resolves the first installed native file-chooser dialog (`zenity`, then `kdialog`) and runs it, returning the chosen path, `ErrCanceled`, or `ErrNoChooser`. It's a separate GUI window, so it blocks — run it off the UI goroutine (`utils.PickFile` does). Linux desktop dialogs only.
- `Copy(src, dst, perm)` is a generic file copy that forces `dst`'s mode to `perm` (so re-copies stay private).

### `internal/vpn` — process management

- `Manager` holds **one** active connection at a time.
- `Connect(c, username, password)` spawns `openvpn` via `pkexec` and wires the child's combined stdout+stderr through an `os.Pipe` into a **`<-chan string`** of log lines, returned to the caller. A single scanner goroutine owns the channel and is the **only** caller of `cmd.Wait()` (reaper).
- **Discovery + import**: `Discover()` scans the system dirs (`/etc/openvpn/client`, `/etc/openvpn`) plus `ConnectionsDir()` (`~/.config/lazyovpn/connections`, the default home for user-added configs). `ImportConfig(src)` validates the `.ovpn/.conf` extension, copies the file into `ConnectionsDir()` at 0600 (configs can carry inline keys), and returns the new `Config`.
- **Auth**: `NeedsAuth(c)` reports whether a config has a bare `auth-user-pass` directive (no creds file → needs a prompt). When credentials are supplied, `Connect` writes them to a 0600 temp file on tmpfs (`$XDG_RUNTIME_DIR`), passes it via `--auth-user-pass`, and removes it when the connection ends — the password is never written to a plaintext file on durable storage.
- **Saved credentials (opt-in)**: `keyring.go` wraps `github.com/zalando/go-keyring` (Secret Service / libsecret on Linux, pure-Go D-Bus, no cgo). `SaveCreds(name, user, pass)` stores `"user\npass"` under service `lazyovpn`; `LoadCreds(name)` returns `(user, pass, ok, err)` — `ok=false` with nil err when nothing is stored; `ForgetCreds(name)` deletes (missing is a no-op). Saving is only ever triggered by the modal's explicit save toggle. Keyring calls run **inline** on the UI goroutine — fine while the login keyring is unlocked (the usual case); move to a `tea.Cmd` if a locked keyring ever hangs the UI. Tested via `keyring.MockInit()`.
- Teardown is **async**: `stop()` only `Kill`s and closes a `done` channel; the scanner goroutine reaps on EOF. Switching connections can briefly overlap two `openvpn` processes.

### `internal/ui/model` — root model + panels

- Layout: bordered sidebar (`model.Sidebar`) + bordered output pane (`model.Terminal`), plus a status line and help footer. `layout()` reserves 2 rows (status + help) and accounts for each pane's border+padding when sizing.
- **Modes**: the root model (the `app` struct in `ui.go`) has an `appMode` (`modeNormal`/`modeAuth`/`modeAdd`/`modeForget`/`modeDisconnect`). An open modal owns all input except the global log stream. `modeAuth`: `enter` on a connection that `NeedsAuth` first tries `vpn.LoadCreds` — saved creds skip the prompt and connect straight away; otherwise the credential modal (`model.AuthModal`) opens. Inside it, `enter` hands creds to `Connect` and clears them (and, if the save toggle is on — `ctrl+s` — calls `vpn.SaveCreds` best-effort first), `esc` cancels. `modeForget`: in normal mode `x` on a connection with saved creds opens a confirm popup (`forgetView`, no sub-model — rendered inline); `y`/`enter` calls `vpn.ForgetCreds`, `n`/`esc` backs out. With nothing stored `x` is a no-op (no popup). `modeDisconnect`: in normal mode `d` on a live connection (`logCh != nil`) opens a confirm popup (`disconnectView`, no sub-model — rendered inline like `forgetView`); `y`/`enter` tears the tunnel down, `n`/`esc` backs out, and with nothing connected `d` is a no-op (no popup). `modeAdd`: `a` opens the import modal (`model.AddModal`), which immediately launches the native file chooser (`utils.PickFile`); when `FilePickedMsg` arrives the modal shows the path, `enter` runs `vpn.ImportConfig` + `sidebar.AddConfig`, `r` re-picks, `esc` cancels. Modals float over the live view via `common.Center` (lipgloss v2 `Canvas`/`Layer` compositing).
- **Charm v2 stack**: bubbletea, bubbles, and lipgloss are all **v2** (`charm.land/...`). v2 gotchas: `Model.View()` returns a `tea.View` (not a string) — wrap content with the `altView` helper, which also sets `v.AltScreen = true` (alt screen is declarative per-frame in v2, not a `NewProgram` option); key presses arrive as `tea.KeyPressMsg` (`tea.KeyMsg` is now an interface), matched via `.String()`. Compositing for the popup uses lipgloss v2 `Canvas`/`Layer` behind `common.Overlay`/`Center`.
- **Log streaming**: `utils.WaitForLog(ch)` is a `tea.Cmd` that blocks on the channel and emits `utils.LogMsg`/`utils.LogClosedMsg`; each handler re-issues it to pump the next line.
- **Per-connection output**: `Terminal.buffers` keeps each connection's log. The active connection keeps filling its buffer even when another is selected; navigating the list (`Terminal.ShowBuffer`) swaps which buffer the viewport shows.
- **Connection state**: `model.ConnState` badge (`connecting…`→`connected`→`disconnected`/`error`). `connected` is detected by scanning log lines for `connectedMarker` (`"Initialization Sequence Completed"`), openvpn's real tunnel-up signal — not by process start.

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
- A self-exiting openvpn leaves `Manager.active` non-nil until the next connect/disconnect.
- Auth is `auth-user-pass` (username + password) only — no key-passphrase (`askpass`) prompt. Credentials can be saved to the OS keyring (opt-in); there is no per-config TTL/expiry on a saved entry — it lives until `x` forgets it.
- Creds temp file lives for the whole connection on tmpfs (same-user read window); tighten to a FIFO / delete-after-read if it matters.
- File chooser is Linux desktop dialogs only (`zenity`/`kdialog`); needs a display, with no in-TUI picker fallback.
