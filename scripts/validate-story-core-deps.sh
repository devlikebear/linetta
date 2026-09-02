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
# deliberately still linked — neither carries a model. Test files are included.
set -euo pipefail
cd "$(dirname "$0")/../engine"

# Every check below captures `go list` into a variable first and only then
# matches. Piping straight into `if ... | grep` fails open: `if` suspends
# errexit for its condition, and with pipefail a `go list` that errors (a
# renamed package, a build break) yields the same "no match" the clean case
# does, so the gate would pass green while checking nothing. A capture is a
# plain assignment, so errexit still aborts on a failed `go list`.

banned='github.com/devlikebear/tars/pkg/(agentloop|session)'
all_deps=$(go list -test -deps ./...)
if grep -E "$banned" <<<"$all_deps"; then
  echo "error: the engine must not link tars agentloop/session code" >&2
  echo "       the built-in agent's loop lives in internal/agent" >&2
  exit 1
fi

llm='github.com/devlikebear/tars/pkg/llm'
for pkg in ./internal/storycontext ./internal/storyops ./internal/mcphost ./internal/rpc/handlers; do
  deps=$(go list -test -deps "$pkg")
  if grep -qx "$llm" <<<"$deps"; then
    echo "error: $pkg must not link $llm — it is shared by every agent" >&2
    exit 1
  fi
done

allowed='internal/provider|internal/agent'
# `go list` separated from the filtering on purpose: a `go list` failure must
# still abort, while an empty filter result (nobody imports it) is the pass
# case and keeps its `|| true`.
imports=$(go list -test -f '{{.ImportPath}}: {{join .Imports " "}}' ./...)
importers=$(grep -F "$llm" <<<"$imports" | cut -d: -f1 | grep -Ev "/($allowed)(\s+\[.*\])?$" || true)
if [ -n "$importers" ]; then
  echo "error: only internal/provider and internal/agent may import $llm; found:" >&2
  echo "$importers" >&2
  exit 1
fi

echo "engine deps OK: pkg/llm only in provider/agent; no agentloop/session; story core clean"
