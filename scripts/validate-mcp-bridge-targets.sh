#!/usr/bin/env bash
# Cross-compile the linetta-mcp bridge for every platform it ships to.
#
# The bridge is built CGO_ENABLED=0 so one runner can produce all three
# binaries. That excludes cgo-gated files, and a package the bridge pulls in
# only transitively can lose a symbol on one GOOS and nowhere else.
#
# That is not hypothetical: the macOS Keychain backend in internal/settings is
# cgo, and with it excluded darwin had no defaultSecretStore at all. Nothing
# caught it, because the desktop build has cgo and the bridge was only built
# for the host — so the break surfaced on a release tag, after the tag was
# already pushed.
#
# Compile only. No output is kept; scripts/build-mcp-bridge.sh does the real
# build.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}/engine"

for target in darwin/arm64 darwin/amd64 linux/amd64 windows/amd64; do
  goos="${target%%/*}"
  goarch="${target##*/}"
  echo "  checking ${goos}/${goarch}"
  if ! CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -o /dev/null ./cmd/linetta-mcp; then
    echo "MCP bridge does not build for ${goos}/${goarch}" >&2
    exit 1
  fi
done

echo "mcp bridge builds for every shipped target"
