#!/usr/bin/env bash
# Build the standalone Go JSONRPC engine for debug and compatibility checks.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT}/engine/bin"
mkdir -p "${OUT_DIR}"

# Determine the host target triple for the compatibility binary suffix.
TRIPLE=""
if command -v rustc >/dev/null 2>&1; then
  TRIPLE="$(rustc --print host-tuple 2>/dev/null || true)"
  if [[ -z "${TRIPLE}" ]]; then
    TRIPLE="$(rustc -Vv | awk '/^host:/ {print $2; exit}')"
  fi
fi

if [[ -z "${TRIPLE}" ]]; then
  case "$(uname -s)" in
    Darwin)
      if [[ "$(uname -m)" == "arm64" ]]; then
        TRIPLE="aarch64-apple-darwin"
      else
        TRIPLE="x86_64-apple-darwin"
      fi
      ;;
    Linux)
      TRIPLE="x86_64-unknown-linux-gnu"
      ;;
    MINGW*|MSYS*|CYGWIN*)
      TRIPLE="x86_64-pc-windows-msvc"
      ;;
    *)
      echo "Unsupported host: $(uname -s)" >&2
      exit 1
      ;;
  esac
fi

EXT=""
case "${TRIPLE}" in
  aarch64-apple-darwin)
    GOOS=darwin
    GOARCH=arm64
    ;;
  x86_64-apple-darwin)
    GOOS=darwin
    GOARCH=amd64
    ;;
  x86_64-unknown-linux-gnu)
    GOOS=linux
    GOARCH=amd64
    ;;
  aarch64-unknown-linux-gnu)
    GOOS=linux
    GOARCH=arm64
    ;;
  x86_64-pc-windows-msvc)
    GOOS=windows
    GOARCH=amd64
    EXT=".exe"
    ;;
  *)
    echo "Unsupported target triple: ${TRIPLE}" >&2
    exit 1
    ;;
esac

OUT="${OUT_DIR}/linetta-engine-${TRIPLE}${EXT}"

TAGS="${LINETTA_BUILD_TAGS:-}"
echo "Building engine -> ${OUT}${TAGS:+ (tags: ${TAGS})}"
(
  cd "${ROOT}/engine"
  if [ -n "${TAGS}" ]; then
    GOOS="${GOOS}" GOARCH="${GOARCH}" go build -tags "${TAGS}" -o "${OUT}" ./cmd/linetta-engine
  else
    GOOS="${GOOS}" GOARCH="${GOARCH}" go build -o "${OUT}" ./cmd/linetta-engine
  fi
)
echo "ok"
