#!/bin/sh
# Install the latest __TITLE__ release: downloads the archive for this OS/arch from GitHub Releases,
# verifies it against checksums.txt, and puts the binary in $BIN_DIR (default ~/.local/bin).
#   curl -fsSL https://raw.githubusercontent.com/__OWNER__/__REPO__/main/install.sh | sh
#   VERSION=v0.1.0 BIN_DIR=/usr/local/bin sh install.sh
set -eu
REPO="__OWNER__/__REPO__"
NAME="__DIST__"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;; esac
if [ -n "${VERSION:-}" ]; then base="https://github.com/$REPO/releases/download/$VERSION"; else base="https://github.com/$REPO/releases/latest/download"; fi
archive="${NAME}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
curl -fsSL -o "$tmp/$archive" "$base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"
(cd "$tmp" && grep " $archive\$" checksums.txt | sha256sum -c - >/dev/null 2>&1 || grep " $archive\$" checksums.txt | shasum -a 256 -c - >/dev/null) \
  || { echo "checksum mismatch for $archive" >&2; exit 1; }
tar -xzf "$tmp/$archive" -C "$tmp" "$NAME"
mkdir -p "$BIN_DIR" && install -m 755 "$tmp/$NAME" "$BIN_DIR/$NAME"
echo "installed $("$BIN_DIR/$NAME" version 2>/dev/null || echo "$NAME") → $BIN_DIR/$NAME"
case ":$PATH:" in *":$BIN_DIR:"*) ;; *) echo "note: $BIN_DIR is not on your PATH" ;; esac
