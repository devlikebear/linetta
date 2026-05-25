#!/usr/bin/env bash
# Build the engine once, then start `tauri dev` (which runs `pnpm dev` for Vite).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"${ROOT}/scripts/build-engine.sh"

cd "${ROOT}/apps/desktop"
pnpm tauri dev "$@"
