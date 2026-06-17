#!/usr/bin/env bash
# Local Mac App Store (sandbox) build: build the engine with the `mas` tag (no
# git sync) + Tauri app with the MAS config overlay, then sign the sidecar with
# inherit entitlements and the app with sandbox entitlements.
#
# This signs with the existing Developer ID identity for LOCAL sandbox
# verification. Submitting to the App Store (Apple Distribution cert + .pkg) is a
# later phase.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_FILE="${LINETTA_APPLE_CONFIG:-${HOME}/.linetta/apple/config.env}"

ENT_APP="${ROOT}/apps/desktop/src-tauri/entitlements/linetta.entitlements"
ENT_SIDECAR="${ROOT}/apps/desktop/src-tauri/entitlements/linetta-sidecar.entitlements"

# Resolve the Developer ID Application identity (override via APPLE_SIGNING_IDENTITY).
SIGNING_IDENTITY="${APPLE_SIGNING_IDENTITY:-}"
if [ -z "${SIGNING_IDENTITY}" ] && [ -f "${CONFIG_FILE}" ]; then
  # shellcheck disable=SC1090
  set -a; . "${CONFIG_FILE}"; set +a
  SIGNING_IDENTITY="${APPLE_SIGNING_IDENTITY:-${SIGNING_IDENTITY}}"
fi
if [ -z "${SIGNING_IDENTITY}" ]; then
  SIGNING_IDENTITY="$(
    security find-identity -v -p codesigning \
      | awk -F '"' '/Developer ID Application/ { print $2; exit }'
  )"
fi
if [ -z "${SIGNING_IDENTITY}" ]; then
  echo "No 'Developer ID Application' signing identity found." >&2
  exit 1
fi
echo "Signing identity: ${SIGNING_IDENTITY}"

echo "Building engine sidecar (mas: git sync excluded)"
LINETTA_BUILD_TAGS=mas bash "${ROOT}/scripts/build-engine.sh"

echo "Building the sandboxed macOS app"
cd "${ROOT}/apps/desktop"
pnpm tauri build --config src-tauri/tauri.mas.conf.json --bundles app

APP="${ROOT}/apps/desktop/src-tauri/target/release/bundle/macos/Linetta.app"
SIDECAR="${APP}/Contents/MacOS/linetta-engine"

echo "Signing sidecar (inherit) then app (sandbox)"
codesign --force --options runtime --timestamp \
  --sign "${SIGNING_IDENTITY}" --entitlements "${ENT_SIDECAR}" "${SIDECAR}"
codesign --force --options runtime --timestamp \
  --sign "${SIGNING_IDENTITY}" --entitlements "${ENT_APP}" "${APP}"

echo "=== Verification ==="
codesign --verify --deep --strict --verbose=2 "${APP}"
echo "--- app entitlements ---"
codesign -d --entitlements - "${APP}"
echo "--- sidecar entitlements ---"
codesign -d --entitlements - "${SIDECAR}"

echo ""
echo "Done."
echo "  App: ${APP}"
echo "  Run it, then check for sandbox violations with:"
echo "    log stream --style compact --predicate 'sender == \"sandboxd\"'"
