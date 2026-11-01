#!/usr/bin/env bash
# `tauri ios init` writes Tauri's own icon into the generated project. Copy
# Linetta's over it; run this after every init, before building.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="${ROOT}/apps/desktop/src-tauri/icons/ios"
DEST="${ROOT}/apps/desktop/src-tauri/gen/apple/Assets.xcassets/AppIcon.appiconset"

[ -d "${DEST}" ] || { echo "missing ${DEST}: run tauri ios init first" >&2; exit 1; }
cp "${SRC}"/*.png "${DEST}/"
