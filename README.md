# lazyovpn

A terminal UI for managing OpenVPN connections, written in Go.

[![Release](https://img.shields.io/github/v/release/omartelo/lazyovpn?sort=semver)](https://github.com/omartelo/lazyovpn/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/omartelo/lazyovpn)](go.mod)
[![License](https://img.shields.io/github/license/omartelo/lazyovpn)](LICENSE)

<p align="center">
  <img src="docs/lazyovpn.gif" alt="lazyovpn" width="820">
</p>

## Features

- Lists OpenVPN configs from `/etc/openvpn/client`, `/etc/openvpn`, and `~/.config/lazyovpn/connections`.
- Connect / disconnect — `openvpn` runs as root via `pkexec`; the TUI itself stays unprivileged.
- Live per-connection log streaming with a connection-state badge (connecting / connected / disconnected / error).
- The connected entry is marked in the sidebar.
- Prompts for credentials on `auth-user-pass` configs — the password is masked and never written to plaintext storage. Optionally save it to the OS keyring (Secret Service / libsecret) so later connects skip the prompt.
- Import a config through the native file chooser (`zenity` / `kdialog`); it's copied into `~/.config/lazyovpn/connections`.
- Scrollable log pane: focus it with `tab` (or the mouse wheel) and page back through history.

## Quick Start

### Requirements

- Linux with `openvpn` and `pkexec` (polkit) on `PATH`.
- `zenity` or `kdialog` — only needed to import a config from the file chooser.

### Install

#### Script

Download the latest release binary, verify its checksum, and install it:

```bash
curl -fsSL https://raw.githubusercontent.com/omartelo/lazyovpn/main/install.sh | sh
```

#### Go

```bash
go install github.com/omartelo/lazyovpn@latest
```

#### Manual

Download a prebuilt binary from the [releases](https://github.com/omartelo/lazyovpn/releases) page, extract it, and put `lazyovpn` on your `PATH`:

```bash
tar -xzf lazyovpn_*_linux_amd64.tar.gz
sudo install lazyovpn /usr/local/bin/
```

### Usage

Call `lazyovpn` in your terminal to launch:

```bash
lazyovpn
```

It discovers your OpenVPN configs on startup and opens the connection list. Pick one and press `enter` to connect, or press `a` to import a new `.ovpn`/`.conf`. No configs yet? It still launches — just add one with `a`.

### Keys

| Key | Action |
|-----|--------|
| `↑`/`↓`, `j`/`k` | navigate (sidebar) / scroll (log pane) |
| `enter` | connect |
| `a` | add a connection |
| `d` | disconnect |
| `x` | forget saved credentials |
| `tab` | focus the log pane to scroll (`tab`/`esc` to go back) |
| `pgup`/`pgdn` | page the log (while focused) |
| `q` | quit |

When prompted for credentials, `ctrl+s` toggles saving them to the OS keyring.

## License

[MIT](LICENSE)
