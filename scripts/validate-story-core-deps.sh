#!/usr/bin/env bash
# The MCP-first pivot (#47) extracted internal/storycontext and
# internal/storyops as the LLM-free story core. This gate keeps them that way:
# if either package ever links tars' LLM client, agent loop, or chat sessions,
# the pivot's removal phase would delete code the MCP tools stand on.
set -euo pipefail
cd "$(dirname "$0")/../engine"

banned='github.com/devlikebear/tars/pkg/(llm|agentloop|session)'
if go list -deps ./internal/storycontext ./internal/storyops | grep -E "$banned"; then
  echo "error: internal/storycontext and internal/storyops must not depend on LLM/agent-loop/session code" >&2
  exit 1
fi
echo "story core deps OK: no tars llm/agentloop/session linkage"
