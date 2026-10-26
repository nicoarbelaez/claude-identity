#!/bin/sh
# Install the cid CLI from GitHub Releases, verifying its SHA-256 checksum.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/nicoarbelaez/claude-identity/main/scripts/install.sh | sh
#   sh install.sh [VERSION]            # e.g. 1.2.3 or v1.2.3
#
# Environment:
#   CID_VERSION      Version to install (default: latest release).
#   CID_INSTALL_DIR  Install directory (default: $HOME/.local/bin).
#   CID_VERIFY_ATTESTATION=0  Skip `gh attestation verify` even if gh exists.
set -eu

REPO="nicoarbelaez/claude-identity"
BIN="cid"

err() { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }

fetch() { # url dest
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$1" -O "$2"
  else
    err "curl or wget is required"
  fi
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    err "sha256sum or shasum is required"
  fi
}

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) err "unsupported OS: $(uname -s) (use install.ps1 on Windows)" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) err "unsupported architecture: $(uname -m)" ;;
esac

version="${1:-${CID_VERSION:-}}"
if [ -z "$version" ]; then
  tmp_latest="$(mktemp)"
  fetch "https://api.github.com/repos/$REPO/releases/latest" "$tmp_latest"
  version="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_latest" | head -n 1)"
  rm -f "$tmp_latest"
  [ -n "$version" ] || err "could not determine the latest release"
fi
tag="v${version#v}"
version="${tag#v}"

archive="${BIN}_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"
dest="${CID_INSTALL_DIR:-$HOME/.local/bin}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT INT TERM

info "Downloading $archive ($tag)..."
fetch "$base/$archive" "$work/$archive"
fetch "$base/checksums.txt" "$work/checksums.txt"

expected="$(awk -v f="$archive" '$2 == f {print $1}' "$work/checksums.txt")"
[ -n "$expected" ] || err "no checksum entry for $archive in checksums.txt"
actual="$(sha256_of "$work/$archive")"
if [ "$expected" != "$actual" ]; then
  err "checksum mismatch for $archive (expected $expected, got $actual); aborting"
fi
info "Checksum OK."

if [ "${CID_VERIFY_ATTESTATION:-1}" != "0" ] && command -v gh >/dev/null 2>&1; then
  info "Verifying build provenance with gh..."
  if ! gh attestation verify "$work/$archive" --repo "$REPO" >/dev/null 2>&1; then
    err "gh attestation verify failed for $archive; aborting (set CID_VERIFY_ATTESTATION=0 to skip)"
  fi
  info "Attestation OK."
fi

tar -xzf "$work/$archive" -C "$work" "$BIN"
mkdir -p "$dest"
install -m 0755 "$work/$BIN" "$dest/$BIN"
info "Installed $BIN $version to $dest/$BIN"

case ":$PATH:" in
  *":$dest:"*) ;;
  *)
    info ""
    info "$dest is not on your PATH. Add it, then restart your terminal (and Claude Code):"
    case "$(basename "${SHELL:-}")" in
      fish) info "  fish_add_path \"$dest\"" ;;
      zsh) info "  echo 'export PATH=\"$dest:\$PATH\"' >> ~/.zshrc" ;;
      bash) info "  echo 'export PATH=\"$dest:\$PATH\"' >> ~/.bashrc" ;;
      *) info "  export PATH=\"$dest:\$PATH\"" ;;
    esac
    ;;
esac
