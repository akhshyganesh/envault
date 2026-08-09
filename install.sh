#!/bin/sh
set -e

REPO="akhshyganesh/envault"
BIN_NAME="envault"
DATA_DIR="${HOME}/.envault"

# Override with ENVAULT_INSTALL_DIR to install somewhere you own and skip sudo:
#   curl -fsSL <url> | ENVAULT_INSTALL_DIR="$HOME/.local/bin" sh
INSTALL_DIR="${ENVAULT_INSTALL_DIR:-/usr/local/bin}"

# Prompt helper — reads from /dev/tty so it works even when piped via curl | sh
ask() {
  printf "%s [y/N] " "$1"
  read -r ans </dev/tty
  case "$ans" in [yY]*) return 0 ;; *) return 1 ;; esac
}

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *)
    echo "Unsupported OS: $OS"
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64 | amd64)  arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

echo "Detected: ${os}/${arch}"

# Decide up front whether writing to INSTALL_DIR needs elevation. The directory
# may not exist yet — a stock macOS has no /usr/local/bin — so judge writability
# by the nearest ancestor that does exist, since that is where we would create it.
probe="$INSTALL_DIR"
while [ ! -d "$probe" ]; do
  parent="$(dirname "$probe")"
  [ "$parent" = "$probe" ] && break
  probe="$parent"
done

SUDO=""
if [ ! -w "$probe" ]; then
  if command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
  else
    echo "Cannot write to ${INSTALL_DIR} and sudo is not available."
    echo "Re-run with an install directory you own:"
    echo "  curl -fsSL https://raw.githubusercontent.com/${REPO}/develop/install.sh | ENVAULT_INSTALL_DIR=\"\$HOME/.local/bin\" sh"
    exit 1
  fi
fi

# Fetch latest version from GitHub
echo "Checking latest version..."
LATEST="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' \
  | sed 's/.*"tag_name": *"//;s/".*//' \
  | sed 's/^v//')"

if [ -z "$LATEST" ]; then
  echo "Could not fetch latest version. Check your internet connection."
  exit 1
fi

# Check if envault is already installed
CURRENT=""
if command -v "$BIN_NAME" >/dev/null 2>&1; then
  CURRENT="$("$BIN_NAME" version 2>/dev/null | grep 'Version:' | awk '{print $NF}' | sed 's/^v//')"
fi

FRESH_INSTALL=1

if [ -n "$CURRENT" ]; then
  FRESH_INSTALL=0
  echo ""
  echo "  Installed: v${CURRENT}"
  echo "  Latest:    v${LATEST}"
  echo ""

  if [ "$CURRENT" = "$LATEST" ]; then
    echo "You already have the latest version (v${LATEST})."
    if ! ask "Reinstall anyway?"; then
      echo "Nothing to do."
      exit 0
    fi
  else
    echo "A new version is available: v${LATEST} (current: v${CURRENT})"
    if ! ask "Upgrade now?"; then
      echo "Upgrade cancelled."
      exit 0
    fi
  fi

  echo ""
  if ask "Clean vault data at ${DATA_DIR}? (this deletes ALL backups — cannot be undone)"; then
    rm -rf "$DATA_DIR"
    echo "  Vault data removed."
  fi
fi

# Fresh install: check for leftover data
if [ "$FRESH_INSTALL" = "1" ] && [ -d "$DATA_DIR" ]; then
  echo ""
  echo "Found existing vault data at ${DATA_DIR}."
  if ask "Delete old backups and start fresh?"; then
    rm -rf "$DATA_DIR"
    echo "  Old data removed."
  fi
fi

# Download
ASSET="${BIN_NAME}-${os}-${arch}"
BASE_URL="https://github.com/${REPO}/releases/latest/download"

echo ""
echo "Downloading ${ASSET}..."
TMP="$(mktemp)"
trap 'rm -f "$TMP" "${TMP}.sums"' EXIT INT TERM
curl -fsSL "${BASE_URL}/${ASSET}" -o "$TMP"

# Verify against the published checksums. Skipped only if the host has neither
# checksum tool, which would otherwise fail an install that is fine.
if command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
elif command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | awk '{print $1}'; }
else
  sha256() { return 1; }
fi

if curl -fsSL "${BASE_URL}/checksums.txt" -o "${TMP}.sums" 2>/dev/null; then
  expected="$(grep " ${ASSET}\$" "${TMP}.sums" | awk '{print $1}')"
  actual="$(sha256 "$TMP" || true)"
  if [ -n "$expected" ] && [ -n "$actual" ]; then
    if [ "$expected" != "$actual" ]; then
      echo "Checksum mismatch for ${ASSET}."
      echo "  expected: ${expected}"
      echo "  actual:   ${actual}"
      echo "Refusing to install. Please report this at https://github.com/${REPO}/issues"
      exit 1
    fi
    echo "Checksum verified."
  fi
fi

# Install. `install` will not create the target directory, and a missing
# /usr/local/bin makes it fail with a bare "No such file or directory" that
# reads like a broken download — so create it first.
if [ ! -d "$INSTALL_DIR" ]; then
  echo "Creating ${INSTALL_DIR}..."
  $SUDO install -d -m 755 "$INSTALL_DIR"
fi

if [ -n "$SUDO" ]; then
  echo "Installing to ${INSTALL_DIR}/${BIN_NAME} (requires sudo)..."
else
  echo "Installing to ${INSTALL_DIR}/${BIN_NAME}..."
fi
$SUDO install -m 755 "$TMP" "${INSTALL_DIR}/${BIN_NAME}"

echo ""
echo "envault v${LATEST} installed to ${INSTALL_DIR}/${BIN_NAME}"

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*)
    echo "  Run 'envault' to get started."
    ;;
  *)
    echo ""
    echo "  ${INSTALL_DIR} is not on your PATH. Add it to your shell profile:"
    echo "    export PATH=\"${INSTALL_DIR}:\$PATH\""
    ;;
esac
