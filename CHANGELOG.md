# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.7.0] - 2026-07-01

### Added

- Rename a connection from the action menu (`r`). A prompt asks for the new
  name; on confirm, any saved credentials are migrated to the new name in the
  keyring first (a migration failure aborts the rename untouched) and then the
  config file is renamed. Renaming a connection that is connected (or waiting to
  auto-reconnect) is refused — disconnect first.
- Delete a connection from the action menu (`d`). A confirmation popup guards
  the action; on confirm the config file and any saved credentials are removed
  and the connection drops off the list. Deleting a connection that is active
  disconnects it first.

### Changed

- `x` now opens a per-connection action menu instead of forgetting credentials
  directly. Forget saved credentials moved into the menu under `f` (it still
  asks for confirmation). The menu is the home for upcoming per-connection
  actions.

## [0.6.0] - 2026-06-30

### Added

- Quitting (`q` / `ctrl+c`) while a connection is active now asks for
  confirmation before tearing the tunnel down, so an accidental keypress no
  longer drops a live VPN. With nothing connected it quits immediately.

### Fixed

- Disconnecting (and quitting) now actually tears the tunnel down. openvpn runs
  as root via `pkexec` while the TUI is unprivileged, so a direct `kill` was
  silently denied (`EPERM`) — every connect/switch/quit leaked a live openvpn
  process, and a second connection with the same certificate flapped against the
  server. lazyovpn now opens openvpn's management socket
  (`--management … unix --management-client-user`) and asks openvpn to terminate
  itself (`signal SIGTERM`) on teardown; a `defer` in the launcher guarantees
  this on every exit path, including ctrl+c.
- The status badge no longer hangs on `connecting…` after a tunnel is actually
  up. Tunnel-up is now detected from openvpn's **management state**
  (`CONNECTED`), polled on the management socket, instead of scraping the log for
  the `Initialization Sequence Completed` line — a low-`verb`/`mute` config can
  suppress that line entirely, leaving the badge stuck forever despite a working
  connection. The management state is authoritative regardless of log verbosity.
- The log pane now streams in real time. openvpn is given a pseudo-terminal
  instead of a plain pipe, so it line-buffers its output (over a pipe it
  block-buffers in ~4KB chunks, so lines arrived late and bunched up).

## [0.5.0] - 2026-06-29

### Added

- A `doctor` command (`lazyovpn doctor`) that checks the external programs
  lazyovpn relies on: `openvpn` and `pkexec` (required — the command exits
  non-zero if either is missing) and a file chooser (`zenity`/`kdialog`,
  optional — only needed to import configs from inside the TUI). Each
  dependency is reported with its resolved path or an install hint.

## [0.4.0] - 2026-06-28

### Added

- Auto-reconnect. When a live tunnel's `openvpn` process exits on its own
  (a crash, an external kill, or a hard server drop that outlasts openvpn's own
  retry loop), lazyovpn redials the same connection automatically — up to 5
  times, 3s apart. A connection that stayed up at least 30s is treated as
  healthy and earns the full retry budget back, so an occasional drop keeps
  reconnecting while a tight flap gives up instead of spinning. The status badge
  shows `reconnecting...`; press `d` to cancel a pending reconnect. Auto-reconnect
  is a keyring feature: connections that need no authentication, and those whose
  credentials are saved in the OS keyring, come back automatically — the saved
  creds are fetched fresh from the keyring at redial time and never held in
  memory. A connection whose password was typed once but not saved is not
  auto-reconnected (save it via the modal's toggle to enable this). Configs that
  `daemon` into the background are also excluded — their foreground process exits
  by design and the tunnel is untrackable.

## [0.3.0] - 2026-06-25

### Added

- The log panel is now scrollable. Press `tab` to focus the log pane, then
  scroll with `↑`/`↓`/`j`/`k` (line) or `pgup`/`pgdn` (page); `tab` or `esc`
  returns to the connection list. The mouse wheel scrolls the log too (no need
  to focus it first). New lines keep tailing the bottom only while you are
  already at the bottom — scrolling up to read history no longer yanks the view
  down. The panel is also renamed from "terminal" to "log".

### Changed

- Disconnecting (`d`) now asks for confirmation before tearing down a live
  connection — `y`/`enter` confirms, `n`/`esc` cancels. With nothing connected,
  `d` stays a no-op (no popup).

### Removed

- Connection-list filtering (`/`). The sidebar is now rendered directly instead
  of through a list widget; filtering will return if config lists ever grow
  large enough to need it.

## [0.2.0] - 2026-06-24

### Added

- Optional credential storage in the OS keyring (Secret Service / libsecret).
  The auth modal gains a save toggle (`ctrl+s`, off by default); when enabled,
  the username/password are stored under the `lazyovpn` service. A connection
  with saved credentials skips the prompt and connects directly. Press `x` in
  the connection list to forget a saved entry (e.g. after a password change) —
  a confirmation popup guards the deletion. Credentials still never touch a
  plaintext file — only the encrypted keyring.

## [0.1.0] - 2026-06-24

First release — a terminal UI for managing OpenVPN connections on Linux,
lazydocker-style.

### Added

- Connection sidebar listing `.ovpn`/`.conf` configs discovered from
  `/etc/openvpn/client`, `/etc/openvpn`, and `~/.config/lazyovpn/connections`,
  with keyboard navigation and filtering.
- Connect / disconnect — `openvpn` runs as root via `pkexec`; the TUI process
  itself stays unprivileged.
- Live per-connection log streaming with a connection-state badge
  (connecting / connected / disconnected / error); the connected entry is
  marked green in the sidebar.
- Credential prompt for `auth-user-pass` configs — username + masked password,
  written to a 0600 tmpfs file passed via `--auth-user-pass` and removed when
  the connection ends, never persisted to durable storage.
- Import a config through the native file chooser (`zenity` / `kdialog`),
  copied into `~/.config/lazyovpn/connections`.
- GoReleaser release pipeline (Linux amd64/arm64) triggered on `v*` tags, and a
  CI workflow running `gofmt`/`vet`/`go test -race` with coverage on every PR.
- MIT license and README.

[Unreleased]: https://github.com/omartelo/lazyovpn/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/omartelo/lazyovpn/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/omartelo/lazyovpn/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/omartelo/lazyovpn/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/omartelo/lazyovpn/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/omartelo/lazyovpn/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/omartelo/lazyovpn/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/omartelo/lazyovpn/releases/tag/v0.1.0
