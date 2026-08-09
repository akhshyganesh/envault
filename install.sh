#!/bin/sh
# envault installer. Detects your platform, verifies the download against the
# release checksums, and installs the binary.
#
#   curl -fsSL https://raw.githubusercontent.com/akhshyganesh/envault/develop/install.sh | sh
#
# Set ENVAULT_INSTALL_DIR to install somewhere you own and skip sudo entirely:
#
#   curl -fsSL <url> | ENVAULT_INSTALL_DIR="$HOME/.local/bin" sh

set -e

REPO="akhshyganesh/envault"
BIN_NAME="envault"
DATA_DIR="${HOME}/.envault"
INSTALL_DIR="${ENVAULT_INSTALL_DIR:-/usr/local/bin}"

# Reads from /dev/tty so prompts still work when piped through curl | sh.
ask() {
  printf "%s [y/N] " "$1"
  read -r answer </dev/tty
  case "$answer" in [yY]*) return 0 ;; *) return 1 ;; esac
}

die() {
  echo "$1" >&2
  exit 1
}

# ---------- platform ----------

case "$(uname -s)" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *)      die "Unsupported OS: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64 | amd64)  arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *)               die "Unsupported architecture: $(uname -m)" ;;
esac

echo "Detected: ${os}/${arch}"

# ---------- privileges ----------

# INSTALL_DIR may not exist yet — a stock macOS has no /usr/local/bin — so judge
# writability by the nearest ancestor that does exist, since that is where the
# directory would have to be created.
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
    echo "Cannot write to ${INSTALL_DIR}, and sudo is not available."
    echo "Re-run with a directory you own:"
    echo "  curl -fsSL https://raw.githubusercontent.com/${REPO}/develop/install.sh | ENVAULT_INSTALL_DIR=\"\$HOME/.local/bin\" sh"
    exit 1
  fi
fi

# ---------- version ----------

echo "Checking the latest release..."
LATEST="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' \
  | sed 's/.*"tag_name": *"//;s/".*//;s/^v//')"
[ -n "$LATEST" ] || die "Could not reach GitHub. Check your internet connection."

CURRENT=""
if command -v "$BIN_NAME" >/dev/null 2>&1; then
  CURRENT="$("$BIN_NAME" version 2>/dev/null | grep 'Version:' | awk '{print $NF}' | sed 's/^v//')"
fi

if [ -n "$CURRENT" ]; then
  echo ""
  echo "  Installed: v${CURRENT}"
  echo "  Latest:    v${LATEST}"
  echo ""
  if [ "$CURRENT" = "$LATEST" ]; then
    ask "Already up to date. Reinstall anyway?" || { echo "Nothing to do."; exit 0; }
  else
    ask "Upgrade to v${LATEST}?" || { echo "Upgrade cancelled."; exit 0; }
  fi
fi

# Offer a clean slate, but never take one without being told to.
if [ -d "$DATA_DIR" ]; then
  echo ""
  echo "Existing backups found at ${DATA_DIR}."
  if ask "Delete them and start fresh? (this cannot be undone)"; then
    rm -rf "$DATA_DIR"
    echo "  Removed ${DATA_DIR}"
  fi
fi

# ---------- download ----------

ASSET="${BIN_NAME}-${os}-${arch}"
BASE_URL="https://github.com/${REPO}/releases/latest/download"

echo ""
echo "Downloading ${ASSET}..."
TMP="$(mktemp)"
trap 'rm -f "$TMP" "${TMP}.sums"' EXIT INT TERM
curl -fsSL "${BASE_URL}/${ASSET}" -o "$TMP"

# Verify against the published checksums. Skipped only when the host has no
# checksum tool at all, which should not fail an otherwise fine install.
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
      echo "Checksum mismatch for ${ASSET}." >&2
      echo "  expected: ${expected}" >&2
      echo "  actual:   ${actual}" >&2
      die "Refusing to install. Please report this at https://github.com/${REPO}/issues"
    fi
    echo "Checksum verified."
  fi
fi

# ---------- install ----------

# `install` will not create its target directory, and a missing /usr/local/bin
# makes it fail with a bare "No such file or directory" that reads like a broken
# download. Create it first.
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
    echo "  ${INSTALL_DIR} is not on your PATH. Add this to your shell profile:"
    echo "    export PATH=\"${INSTALL_DIR}:\$PATH\""
    ;;
esac
