#!/usr/bin/env bash
# Linetta's story core does not talk to a language model. The MCP-first pivot
# (#47) moved AI collaboration behind the MCP boundary; the built-in agent
# (#90) brought a model client back — into two packages only, as one more
# client of the same MCP tools.
#
# This gate keeps both facts true:
#   1. tars' agent loop and session code never link into the engine at all.
#      The built-in agent's loop is written here (internal/agent) so that
#      cancellation, the activity log and the writer's limits stay in hand.
#   2. tars/pkg/llm is imported only by internal/provider and internal/agent,
#      and never reaches the story core, the MCP tool layer or the RPC
#      handlers — they are what every agent, external or built-in, runs on.
#
# tars' pkg/tools (fact-book URL capture) and pkg/memory (remembered facts) are
# deliberately still linked — neither carries a model.
set -euo pipefail
cd "$(dirname "$0")/../engine"

banned='github.com/devlikebear/tars/pkg/(agentloop|session)'
if go list -deps ./... | grep -E "$banned"; then
  echo "error: the engine must not link tars agentloop/session code" >&2
  echo "       the built-in agent's loop lives in internal/agent" >&2
  exit 1
fi

llm='github.com/devlikebear/tars/pkg/llm'
for pkg in ./internal/storycontext ./internal/storyops ./internal/mcphost ./internal/rpc/handlers; do
  if go list -deps "$pkg" | grep -qx "$llm"; then
    echo "error: $pkg must not link $llm — it is shared by every agent" >&2
    exit 1
  fi
done

allowed='internal/provider|internal/agent'
importers=$(go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./... \
  | grep -F "$llm" | cut -d: -f1 | grep -Ev "/($allowed)$" || true)
if [ -n "$importers" ]; then
  echo "error: only internal/provider and internal/agent may import $llm; found:" >&2
  echo "$importers" >&2
  exit 1
fi

echo "engine deps OK: pkg/llm only in provider/agent; no agentloop/session; story core clean"
