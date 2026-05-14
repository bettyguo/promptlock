#!/bin/sh
# Cross-platform installer for promptlock.
# Usage:
#   curl -sSfL https://promptlock.dev/install.sh | sh
#   curl -sSfL https://promptlock.dev/install.sh | PROMPTLOCK_VERSION=v0.1.0 sh
#
# Honors $PROMPTLOCK_INSTALL_DIR (default /usr/local/bin or $HOME/.local/bin).
set -eu

OWNER=promptlock
REPO=promptlock
VERSION=${PROMPTLOCK_VERSION:-latest}
INSTALL_DIR=${PROMPTLOCK_INSTALL_DIR:-}

# Resolve install dir.
if [ -z "${INSTALL_DIR}" ]; then
  if [ -w /usr/local/bin ]; then
    INSTALL_DIR=/usr/local/bin
  else
    INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "${INSTALL_DIR}"
  fi
fi

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "${OS}" in
  linux|darwin) ;;
  *) echo "promptlock: unsupported OS ${OS}; use the Windows .exe download." >&2; exit 1;;
esac

ARCH=$(uname -m)
case "${ARCH}" in
  x86_64|amd64) ARCH=amd64;;
  arm64|aarch64) ARCH=arm64;;
  *) echo "promptlock: unsupported arch ${ARCH}." >&2; exit 1;;
esac

if [ "${VERSION}" = "latest" ]; then
  URL_BASE="https://github.com/${OWNER}/${REPO}/releases/latest/download"
else
  URL_BASE="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}"
fi

PKG="promptlock_${VERSION#v}_${OS}_${ARCH}.tar.gz"
if [ "${VERSION}" = "latest" ]; then
  # name carries the version; can't predict — use redirect-aware download.
  PKG="promptlock_*_${OS}_${ARCH}.tar.gz"
  echo "promptlock: downloading latest ${OS}/${ARCH}" >&2
fi

TMP=$(mktemp -d)
trap 'rm -rf "${TMP}"' EXIT
( cd "${TMP}" && curl -sSfL "${URL_BASE}/${PKG}" -o promptlock.tar.gz && tar -xzf promptlock.tar.gz )

mv "${TMP}/promptlock" "${INSTALL_DIR}/promptlock"
chmod +x "${INSTALL_DIR}/promptlock"

echo "Installed ${INSTALL_DIR}/promptlock"
"${INSTALL_DIR}/promptlock" --version
echo "Add ${INSTALL_DIR} to your PATH if it isn't already."
