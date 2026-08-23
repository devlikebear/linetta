#!/usr/bin/env bash
# Build the linetta-mcp stdio bridge for the host platform.
#
# The bridge is what Claude Desktop launches: that client cannot reach a
# loopback HTTP MCP server, so the stdio front door has to ship with the app.
# Pure Go with no cgo, so cross-compiling is a matter of GOOS/GOARCH.
#
# Output goes to the Tauri resource directory, where the desktop bundle picks
# it up and the shell resolves its path at runtime.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT}/apps/desktop/src-tauri/resources"
mkdir -p "${OUT_DIR}"

# GOOS/GOARCH default to the host; override for cross builds in CI.
GOOS="${GOOS:-}"
GOARCH="${GOARCH:-}"
if [[ -z "${GOOS}" ]]; then
  case "$(uname -s)" in
    Darwin) GOOS=darwin ;;
    Linux) GOOS=linux ;;
    MINGW* | MSYS* | CYGWIN*) GOOS=windows ;;
    *)
      echo "Unsupported host: $(uname -s)" >&2
      exit 1
      ;;
  esac
fi
if [[ -z "${GOARCH}" ]]; then
  case "$(uname -m)" in
    arm64 | aarch64) GOARCH=arm64 ;;
    *) GOARCH=amd64 ;;
  esac
fi

EXT=""
if [[ "${GOOS}" == "windows" ]]; then
  EXT=".exe"
fi

# The name stays constant across platforms: the settings pane prints this path
# into the writer's client config, and a target-triple suffix would make that
# snippet differ per machine for no benefit.
OUT="${OUT_DIR}/linetta-mcp${EXT}"

echo "Building MCP bridge -> ${OUT} (${GOOS}/${GOARCH})"
cd "${ROOT}/engine"
CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build -o "${OUT}" ./cmd/linetta-mcp
echo "Built ${OUT}"
