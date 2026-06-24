# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`lazyovpn` — a TUI for managing OpenVPN connections on Linux, in the spirit of `lazydocker`/`lazygit`. Cobra CLI entry → Bubble Tea TUI.

## Commands

```bash
go run .              # launch the TUI (needs .ovpn/.conf in /etc/openvpn* or ~/.config/lazyovpn)
go build ./...        # build
go vet ./...          # static check
gofmt -w <file>       # format (mandatory before considering work done)
go test -race ./...   # run all tests
go test -race -run TestName ./internal/vpn   # run a single test
go test -cover ./...  # coverage
```

The TUI cannot be exercised non-interactively (needs a TTY, a real `.ovpn`, and a polkit password prompt) — verify logic with tests + `go build` + `go vet`, and run manually for end-to-end behavior.

## What lives where

```
main.go                          cobra root cmd → discovers configs → launches the Bubble Tea program
internal/vpn/vpn.go              config discovery, auth detection, privileged openvpn process management
internal/tui/tui.go              root model: global state, layout, key routing, mode switch, composition
internal/tui/components/box.go   reusable TitledBox (title inlined into the top border)
internal/tui/utils/stream.go     shared plumbing: LogMsg/LogClosedMsg/WaitForLog
internal/tui/models/sidebar.go   connection-list panel (bubbles/list + selection/filter)
internal/tui/models/terminal.go  output panel (viewport + per-connection buffers + ConnState/Badge)
internal/tui/models/authmodal.go credential modal (two textinputs; password masked, never persisted)
```

The TUI is split into packages so each panel owns its own state and business
rules, and new views can be added without touching the others. The import graph
has no cycles: `components` and `utils` depend on nothing internal; `models`
depends on `components` (+ `vpn`); the root `tui` package depends on `models`,
`utils`, and `vpn`. Each panel renders its own bordered box; the root only
places panels and routes messages.

### `internal/vpn` — process management

- `Manager` holds **one** active connection (`ponytail:` comment marks the upgrade path to multi-connection).
- `Connect(c, username, password)` spawns `openvpn` via `pkexec` and wires the child's combined stdout+stderr through an `os.Pipe` into a **`<-chan string`** of log lines, returned to the caller. A single scanner goroutine owns the channel and is the **only** caller of `cmd.Wait()` (reaper).
- **Auth**: `NeedsAuth(c)` reports whether a config has a bare `auth-user-pass` directive (no creds file → needs a prompt). When credentials are supplied, `Connect` writes them to a 0600 temp file on tmpfs (`$XDG_RUNTIME_DIR`), passes it via `--auth-user-pass`, and removes it when the connection ends — the password is never persisted to durable storage (`ponytail:` ceiling notes the same-user read window).
- Teardown is **async**: `stop()` only `Kill`s and closes a `done` channel; the scanner goroutine reaps on EOF. Switching connections can briefly overlap two `openvpn` processes (documented `ponytail:` ceiling).

### `internal/tui` — root model + panels

- Layout: bordered sidebar (`models.Sidebar`) + bordered output pane (`models.Terminal`), plus a status line and help footer. `layout()` reserves 2 rows (status + help) and accounts for each pane's border+padding when sizing.
- **Modes**: the root `model` has an `appMode` (`modeNormal`/`modeAuth`). In `modeAuth` the credential modal (`models.AuthModal`) owns all input except the global log stream; `enter` on a connection that `NeedsAuth` opens the modal, `enter` inside it hands creds to `Connect` and clears them, `esc` cancels. The modal floats over the live view via `components.Center` (lipgloss v2 `Canvas`/`Layer` compositing).
- **Charm v2 stack**: bubbletea, bubbles, and lipgloss are all **v2** (`charm.land/...`). v2 gotchas: `Model.View()` returns a `tea.View` (not a string) — wrap content with the `altView` helper, which also sets `v.AltScreen = true` (alt screen is declarative per-frame in v2, not a `NewProgram` option); key presses arrive as `tea.KeyPressMsg` (`tea.KeyMsg` is now an interface), matched via `.String()`. Compositing for the popup uses lipgloss v2 `Canvas`/`Layer` behind `components.Overlay`/`Center`.
- **Log streaming**: `utils.WaitForLog(ch)` is a `tea.Cmd` that blocks on the channel and emits `utils.LogMsg`/`utils.LogClosedMsg`; each handler re-issues it to pump the next line.
- **Per-connection output**: `Terminal.buffers` keeps each connection's log. The active connection keeps filling its buffer even when another is selected; navigating the list (`Terminal.ShowBuffer`) swaps which buffer the viewport shows.
- **Connection state**: `models.ConnState` badge (`connecting…`→`connected`→`disconnected`/`error`). `connected` is detected by scanning log lines for `connectedMarker` (`"Initialization Sequence Completed"`), openvpn's real tunnel-up signal — not by process start.

## Hard invariants — never break these

1. **Tests, as much coverage as possible.** Every non-trivial change ships with tests (table-driven, `go test -race`). Aim to cover the maximum of the code you can — logic in `internal/vpn` (config discovery, lifecycle) is fully testable; cover it. Don't ship logic with no test behind it.
2. **The TUI process stays unprivileged.** Only `openvpn` runs as root, spawned via `pkexec`. Never add a "run as root" dependency or escalate the TUI itself — the privilege boundary is the whole design.
3. **Exactly one `cmd.Wait()` per process** (the scanner goroutine's reaper). A second `Wait` panics.
4. **Stale-channel guard.** Every `utils.LogMsg`/`utils.LogClosedMsg` carries its source channel; the root handlers drop it if `msg.Ch != m.logCh`. This is what keeps a connection switch from mixing old and new output — keep it on any new log-channel message.
5. **English only.** All code, comments, and user-facing strings are in English. (The user converses in Portuguese; the codebase stays English.)
6. **Passwords never persist.** Credentials go to a 0600 tmpfs temp file removed when the connection ends, never to durable storage or logs; the modal clears its fields right after handoff. Don't add caching/saving without an explicit decision.

## Release checklist

When cutting a version:

- [ ] Bump `version` in `main.go`.
- [ ] Update `CHANGELOG.md` (keep-a-changelog style: move `Unreleased` entries under the new version + date).
- [ ] `go test -race ./...` passes.
- [ ] `go vet ./...` clean and `gofmt` applied.
- [ ] Tag matches the bumped version.

## Known ceilings (search `ponytail:`)

- Single connection only.
- `pkexec` hard-coded (no `sudo` fallback).
- Async teardown → brief two-process overlap on switch.
- A self-exiting openvpn leaves `Manager.current` stale until the next connect/disconnect.
- Auth is `auth-user-pass` (username + password) only — no key-passphrase (`askpass`) prompt, no saved/remembered credentials.
- Creds temp file lives for the whole connection on tmpfs (same-user read window); tighten to a FIFO / delete-after-read if it matters.
