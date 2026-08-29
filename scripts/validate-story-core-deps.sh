#!/usr/bin/env bash
# Linetta does not talk to a language model. The MCP-first pivot (#47) removed
# the built-in companion; every AI path now runs in the writer's own client,
# on the other side of the MCP boundary.
#
# This gate is what keeps that true. It used to guard only the extracted story
# core; with the removal phase done it covers the whole engine, so a stray
# import cannot quietly put a chat client back in the app.
#
# tars' pkg/tools (fact-book URL capture) and pkg/memory (remembered facts) are
# deliberately still linked — neither carries a model.
set -euo pipefail
cd "$(dirname "$0")/../engine"

banned='github.com/devlikebear/tars/pkg/(llm|agentloop|session)'
if go list -deps ./... | grep -E "$banned"; then
  echo "error: the engine must not link tars llm/agentloop/session code" >&2
  echo "       Linetta is a writing tool; AI collaboration happens over MCP." >&2
  exit 1
fi
echo "engine deps OK: no tars llm/agentloop/session linkage"
