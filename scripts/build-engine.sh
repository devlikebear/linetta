#!/usr/bin/env bash
# Build the Go engine into apps/desktop/src-tauri/binaries/ with the target-triple
# suffix Tauri's externalBin expects.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT}/apps/desktop/src-tauri/binaries"
mkdir -p "${OUT_DIR}"

# Determine the host target triple Tauri uses.
if [[ "$(uname -s)" == "Darwin" ]]; then
  if [[ "$(uname -m)" == "arm64" ]]; then
    TRIPLE="aarch64-apple-darwin"
  else
    TRIPLE="x86_64-apple-darwin"
  fi
elif [[ "$(uname -s)" == "Linux" ]]; then
  TRIPLE="x86_64-unknown-linux-gnu"
else
  echo "Unsupported host: $(uname -s)" >&2
  exit 1
fi

OUT="${OUT_DIR}/linetta-engine-${TRIPLE}"

echo "Building engine -> ${OUT}"
(
  cd "${ROOT}/engine"
  go build -o "${OUT}" ./cmd/linetta-engine
)
echo "ok"
