# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/omartelo/lazyovpn/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/omartelo/lazyovpn/releases/tag/v0.1.0
