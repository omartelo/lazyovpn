#!/bin/sh
# lazyovpn installer — fetch the latest prebuilt binary from GitHub releases.
#
#   curl -fsSL https://raw.githubusercontent.com/omartelo/lazyovpn/main/install.sh | sh
#
# Overrides (env vars):
#   VERSION   pin a release tag (default: latest)        e.g. VERSION=v0.4.0
#   BIN_DIR   install destination (default: /usr/local/bin; falls back to
#             ~/.local/bin when /usr/local/bin is not writable and sudo is absent)
set -eu

REPO="omartelo/lazyovpn"
BIN="lazyovpn"

err() { echo "lazyovpn: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || err "missing required tool: $1"; }

# lazyovpn shells out to openvpn/pkexec/zenity — Linux only.
[ "$(uname -s)" = "Linux" ] || err "Linux only (got $(uname -s))"

case "$(uname -m)" in
  x86_64|amd64)   ARCH=amd64 ;;
  aarch64|arm64)  ARCH=arm64 ;;
  *)              err "unsupported arch: $(uname -m)" ;;
esac

need curl
need tar
need sha256sum

# Resolve the release tag (latest unless VERSION is pinned).
TAG="${VERSION:-}"
if [ -z "$TAG" ]; then
  TAG=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
        | grep '"tag_name":' | head -n1 | cut -d'"' -f4)
  [ -n "$TAG" ] || err "could not resolve latest release tag"
fi
VER="${TAG#v}" # archive filenames drop the leading v

BASE="https://github.com/$REPO/releases/download/$TAG"
TARBALL="${BIN}_${VER}_linux_${ARCH}.tar.gz"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading $TARBALL ($TAG)..."
curl -fsSL "$BASE/$TARBALL"      -o "$TMP/$TARBALL"
curl -fsSL "$BASE/checksums.txt" -o "$TMP/checksums.txt"

# Verify the archive against the published checksum before trusting it.
echo "Verifying checksum..."
( cd "$TMP" && grep " $TARBALL\$" checksums.txt | sha256sum -c - ) \
  || err "checksum verification failed"

tar -xzf "$TMP/$TARBALL" -C "$TMP" "$BIN"

# Pick a destination. An explicit BIN_DIR is honored as-is (created if needed).
# The default /usr/local/bin uses sudo when not writable, else ~/.local/bin.
if [ -n "${BIN_DIR:-}" ]; then
  DEST="$BIN_DIR"
  mkdir -p "$DEST"
  install -m755 "$TMP/$BIN" "$DEST/$BIN"
elif [ -w /usr/local/bin ]; then
  DEST=/usr/local/bin
  install -m755 "$TMP/$BIN" "$DEST/$BIN"
elif command -v sudo >/dev/null 2>&1; then
  DEST=/usr/local/bin
  echo "Installing to $DEST (sudo)..."
  sudo install -m755 "$TMP/$BIN" "$DEST/$BIN"
else
  DEST="$HOME/.local/bin"
  mkdir -p "$DEST"
  install -m755 "$TMP/$BIN" "$DEST/$BIN"
fi

case ":$PATH:" in
  *":$DEST:"*) ;;
  *) echo "note: $DEST is not on PATH — add it to use '$BIN' directly" ;;
esac

echo "Installed $BIN $TAG to $DEST/$BIN"
