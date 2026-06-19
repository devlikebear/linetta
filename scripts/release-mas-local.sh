#!/usr/bin/env bash
# Build, sign, and package the Mac App Store .pkg locally (no upload).
#   engine (mas tag) -> Tauri build (mas config) -> embed provisioning profile ->
#   sign sidecar+app (Apple Distribution) -> productbuild (Mac Installer) -> validate
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIR="${HOME}/.linetta/apple"
CONFIG="${LINETTA_APPLE_CONFIG:-${DIR}/config.env}"
[ -f "${CONFIG}" ] || { echo "missing config: ${CONFIG}" >&2; exit 1; }
# shellcheck disable=SC1090
set -a; . "${CONFIG}"; set +a

ENT_APP="${ROOT}/apps/desktop/src-tauri/entitlements/linetta-mas.entitlements"
ENT_SIDECAR="${ROOT}/apps/desktop/src-tauri/entitlements/linetta-sidecar.entitlements"
PROFILE="${MAS_PROFILE_PATH:-${DIR}/linetta-mas.provisionprofile}"

# Resolve identities (override via config.env MAS_APP_IDENTITY / MAS_INSTALLER_IDENTITY).
APP_ID="${MAS_APP_IDENTITY:-}"
if [ -z "${APP_ID}" ]; then
  APP_ID="$(security find-identity -v -p codesigning | awk -F '"' '/Apple Distribution|3rd Party Mac Developer Application/ {print $2; exit}')"
fi
INST_ID="${MAS_INSTALLER_IDENTITY:-}"
if [ -z "${INST_ID}" ]; then
  INST_ID="$(security find-identity -v | awk -F '"' '/Mac Installer Distribution|3rd Party Mac Developer Installer/ {print $2; exit}')"
fi
[ -n "${APP_ID}" ] || { echo "no Apple Distribution identity" >&2; exit 1; }
[ -n "${INST_ID}" ] || { echo "no Mac Installer identity" >&2; exit 1; }
[ -f "${PROFILE}" ] || { echo "missing profile: ${PROFILE}" >&2; exit 1; }
echo "App identity:       ${APP_ID}"
echo "Installer identity: ${INST_ID}"

echo "Building engine (mas) + Tauri app"
LINETTA_BUILD_TAGS=mas bash "${ROOT}/scripts/build-engine.sh"
cd "${ROOT}/apps/desktop"
pnpm tauri build --config src-tauri/tauri.mas.conf.json --bundles app --features mas

APP="${ROOT}/apps/desktop/src-tauri/target/release/bundle/macos/Linetta.app"
SIDECAR="${APP}/Contents/MacOS/linetta-engine"
PKG="${ROOT}/apps/desktop/src-tauri/target/release/bundle/macos/Linetta.pkg"

echo "Embedding provisioning profile"
cp "${PROFILE}" "${APP}/Contents/embedded.provisionprofile"

echo "Signing sidecar then app (Apple Distribution, MAS entitlements)"
codesign --force --timestamp --sign "${APP_ID}" --entitlements "${ENT_SIDECAR}" "${SIDECAR}"
codesign --force --timestamp --sign "${APP_ID}" --entitlements "${ENT_APP}" "${APP}"

echo "Building installer package"
rm -f "${PKG}"
productbuild --component "${APP}" /Applications --sign "${INST_ID}" "${PKG}"

echo "=== Verification ==="
codesign --verify --deep --strict --verbose=2 "${APP}"
codesign -d --entitlements - "${APP}" 2>&1 | grep -E 'application-identifier|app-sandbox' || true
pkgutil --check-signature "${PKG}"
echo ""
echo "Built: ${PKG}"
echo "Upload: see docs/superpowers/plans/2026-06-18-mac-app-store-submission.md Task 8."
