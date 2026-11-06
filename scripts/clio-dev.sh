#!/usr/bin/env bash
# Start Clio's `tauri dev` (which links the embedded Go engine via Cargo
# build.rs and runs `pnpm dev` for Vite on port 1430).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}/apps/clio"
pnpm install --frozen-lockfile
pnpm tauri dev "$@"
