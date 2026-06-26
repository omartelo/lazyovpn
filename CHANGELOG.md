# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- The log panel is now scrollable. Press `tab` to focus the log pane, then
  scroll with `↑`/`↓`/`j`/`k` (line) or `pgup`/`pgdn` (page); `tab` or `esc`
  returns to the connection list. New lines keep tailing the bottom only while
  you are already at the bottom — scrolling up to read history no longer yanks
  the view down. The panel is also renamed from "terminal" to "log".

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

[Unreleased]: https://github.com/omartelo/lazyovpn/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/omartelo/lazyovpn/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/omartelo/lazyovpn/releases/tag/v0.1.0
