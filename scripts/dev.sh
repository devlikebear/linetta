#!/usr/bin/env bash
# Start `tauri dev` (which links the embedded Go engine via Cargo build.rs and
# runs `pnpm dev` for Vite).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}/apps/desktop"
pnpm tauri dev "$@"
