#!/usr/bin/env bash
# Local macOS release: build the engine + Tauri app, then sign (Developer ID),
# notarize, and staple the .app and .dmg using credentials kept outside the repo.
#
# Requirements:
#   - A "Developer ID Application" identity in the login keychain
#   - Apple credentials in ~/.linetta/apple/config.env (App Store Connect API key)
#
# Tauri performs signing + notarization + stapling automatically once the
# APPLE_SIGNING_IDENTITY / APPLE_API_* environment variables are set.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_FILE="${LINETTA_APPLE_CONFIG:-${HOME}/.linetta/apple/config.env}"

if [ ! -f "${CONFIG_FILE}" ]; then
  echo "Apple config not found: ${CONFIG_FILE}" >&2
  echo "Create it with APP_STORE_CONNECT_KEY_ID / APP_STORE_CONNECT_ISSUER_ID / APPLE_TEAM_ID / AUTH_KEY_PATH." >&2
  exit 1
fi

# shellcheck disable=SC1090
set -a; . "${CONFIG_FILE}"; set +a

: "${APPLE_TEAM_ID:?APPLE_TEAM_ID missing in ${CONFIG_FILE}}"
: "${APP_STORE_CONNECT_KEY_ID:?APP_STORE_CONNECT_KEY_ID missing in ${CONFIG_FILE}}"
: "${APP_STORE_CONNECT_ISSUER_ID:?APP_STORE_CONNECT_ISSUER_ID missing in ${CONFIG_FILE}}"
: "${AUTH_KEY_PATH:?AUTH_KEY_PATH missing in ${CONFIG_FILE}}"

if [ ! -f "${AUTH_KEY_PATH}" ]; then
  echo "App Store Connect API key not found: ${AUTH_KEY_PATH}" >&2
  exit 1
fi

# Resolve the Developer ID Application identity (override via APPLE_SIGNING_IDENTITY).
SIGNING_IDENTITY="${APPLE_SIGNING_IDENTITY:-}"
if [ -z "${SIGNING_IDENTITY}" ]; then
  SIGNING_IDENTITY="$(
    security find-identity -v -p codesigning \
      | awk -F '"' '/Developer ID Application/ { print $2; exit }'
  )"
fi
if [ -z "${SIGNING_IDENTITY}" ]; then
  echo "No 'Developer ID Application' signing identity found in the keychain." >&2
  security find-identity -v -p codesigning >&2 || true
  exit 1
fi
echo "Signing identity: ${SIGNING_IDENTITY}"

# Tauri v2 signing + notarization configuration.
export APPLE_SIGNING_IDENTITY="${SIGNING_IDENTITY}"
export APPLE_API_ISSUER="${APP_STORE_CONNECT_ISSUER_ID}"
export APPLE_API_KEY="${APP_STORE_CONNECT_KEY_ID}"
export APPLE_API_KEY_PATH="${AUTH_KEY_PATH}"

echo "Building engine sidecar"
bash "${ROOT}/scripts/build-engine.sh"

echo "Building, signing, notarizing, and stapling the macOS app + dmg"
cd "${ROOT}/apps/desktop"
pnpm tauri build --bundles app,dmg

BUNDLE_DIR="${ROOT}/apps/desktop/src-tauri/target/release/bundle"
APP_PATH="${BUNDLE_DIR}/macos/Linetta.app"
DMG_PATH="$(find "${BUNDLE_DIR}/dmg" -maxdepth 1 -name '*.dmg' 2>/dev/null | head -n1 || true)"

# Tauri signs + notarizes + staples the .app, but only signs the .dmg.
# Notarize and staple the dmg explicitly so Gatekeeper accepts it offline.
if [ -n "${DMG_PATH}" ]; then
  echo "Notarizing + stapling dmg: ${DMG_PATH}"
  xcrun notarytool submit "${DMG_PATH}" \
    --keychain-profile "${NOTARY_PROFILE:-linetta-notary}" --wait
  xcrun stapler staple "${DMG_PATH}"
fi

echo "=== Verification ==="
codesign --verify --deep --strict --verbose=2 "${APP_PATH}"
spctl --assess --type exec -vvv "${APP_PATH}"
stapler validate "${APP_PATH}"
if [ -n "${DMG_PATH}" ]; then
  stapler validate "${DMG_PATH}"
fi

echo ""
echo "Done."
echo "  App: ${APP_PATH}"
[ -n "${DMG_PATH}" ] && echo "  DMG: ${DMG_PATH}"
