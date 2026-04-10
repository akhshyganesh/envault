#!/bin/sh
set -e

REPO="akhshyganesh/envault"
INSTALL_DIR="/usr/local/bin"
BIN_NAME="envault"
DATA_DIR="${HOME}/.envault"

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
    echo "  ✓ Vault data removed."
  fi
fi

# Fresh install: check for leftover data
if [ "$FRESH_INSTALL" = "1" ] && [ -d "$DATA_DIR" ]; then
  echo ""
  echo "Found existing vault data at ${DATA_DIR}."
  if ask "Delete old backups and start fresh?"; then
    rm -rf "$DATA_DIR"
    echo "  ✓ Old data removed."
  fi
fi

# Download and install
ASSET="${BIN_NAME}-${os}-${arch}"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo ""
echo "Downloading ${ASSET}..."
TMP="$(mktemp)"
curl -fsSL "$URL" -o "$TMP"
chmod +x "$TMP"

echo "Installing to ${INSTALL_DIR}/${BIN_NAME} (requires sudo)..."
sudo install -m 755 "$TMP" "${INSTALL_DIR}/${BIN_NAME}"
rm -f "$TMP"

echo ""
echo "✓ envault v${LATEST} installed!"
echo "  Run 'envault' to get started."
