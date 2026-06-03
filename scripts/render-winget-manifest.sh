#!/usr/bin/env bash
set -euo pipefail

if [[ "$#" -ne 5 ]]; then
  cat >&2 <<'USAGE'
Usage: scripts/render-winget-manifest.sh VERSION RELEASE_URL_BASE INSTALLER_FILENAME INSTALLER_SHA256 OUT_DIR

Example:
  scripts/render-winget-manifest.sh \
    0.4.0 \
    https://github.com/devlikebear/linetta/releases/download/v0.4.0 \
    Linetta_0.4.0_x64-setup.exe \
    ABCDEF... \
    dist/winget
USAGE
  exit 2
fi

VERSION="$1"
RELEASE_URL_BASE="${2%/}"
INSTALLER_FILENAME="$3"
INSTALLER_SHA256="$(printf '%s' "$4" | tr '[:lower:]' '[:upper:]')"
OUT_DIR="$5"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMPLATE_DIR="${ROOT}/packaging/winget"

PACKAGE_ID="${PACKAGE_ID:-Devlikebear.Linetta}"
PACKAGE_NAME="${PACKAGE_NAME:-Linetta}"
MANIFEST_VERSION="${MANIFEST_VERSION:-1.12.0}"
INSTALLER_URL="${RELEASE_URL_BASE}/${INSTALLER_FILENAME}"

mkdir -p "${OUT_DIR}"

render_template() {
  local template="$1"
  local output="$2"

  awk \
    -v package_id="${PACKAGE_ID}" \
    -v package_name="${PACKAGE_NAME}" \
    -v version="${VERSION}" \
    -v manifest_version="${MANIFEST_VERSION}" \
    -v installer_url="${INSTALLER_URL}" \
    -v installer_sha256="${INSTALLER_SHA256}" \
    '{
      gsub(/\{\{PACKAGE_ID\}\}/, package_id)
      gsub(/\{\{PACKAGE_NAME\}\}/, package_name)
      gsub(/\{\{VERSION\}\}/, version)
      gsub(/\{\{MANIFEST_VERSION\}\}/, manifest_version)
      gsub(/\{\{INSTALLER_URL\}\}/, installer_url)
      gsub(/\{\{INSTALLER_SHA256\}\}/, installer_sha256)
      print
    }' "${template}" > "${output}"
}

render_template \
  "${TEMPLATE_DIR}/Devlikebear.Linetta.yaml.template" \
  "${OUT_DIR}/Devlikebear.Linetta.yaml"
render_template \
  "${TEMPLATE_DIR}/Devlikebear.Linetta.installer.yaml.template" \
  "${OUT_DIR}/Devlikebear.Linetta.installer.yaml"
render_template \
  "${TEMPLATE_DIR}/Devlikebear.Linetta.locale.en-US.yaml.template" \
  "${OUT_DIR}/Devlikebear.Linetta.locale.en-US.yaml"

echo "Rendered winget manifests in ${OUT_DIR}"
