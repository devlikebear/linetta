import { useCallback, useState } from "react";

import { useEngineEvent } from "./useEngineEvent";

/** What an agent just changed. Emitted by the engine after every applied
 *  mutation so the workspace can refetch instead of showing text the agent
 *  already replaced. */
export type McpChangedPayload = {
  project_id?: string;
  tool?: string;
  node_ids?: string[];
  batch_id?: string;
  /** Who wrote it: "external" (an MCP client over HTTP) or "agent" (the
   *  writer's own built-in panel). Set engine-side from the composed tool
   *  deps, so an external client cannot claim to be the agent. Optional
   *  because an engine older than #93 does not send it. */
  source?: string;
};

export type McpChangeOptions = {
  /** The work currently open, so changes to other works are ignored. */
  projectId: string | null;
  /** The scene currently open in the editor, or null. */
  openNodeId: string | null;
  /** Whether the editor holds unsaved edits for the open scene. */
  editorDirty: boolean;
  /** Refetch the outline tree. */
  onOutlineChanged: () => void;
  /** Reload the open scene's body from the engine. */
  onSceneChanged: (nodeId: string) => void;
};

/** Keeps the workspace in step with an external agent.
 *
 *  The one rule that matters: when the editor has unsaved edits for the scene
 *  the agent touched, the buffer is NOT replaced. The writer's in-progress
 *  sentence outranks the agent's version, so the change is surfaced as a
 *  banner they can act on instead. */
export function useMcpChanges({
  projectId,
  openNodeId,
  editorDirty,
  onOutlineChanged,
  onSceneChanged,
}: McpChangeOptions) {
  // The conflict and the source it came from move together: a banner that
  // names the wrong writer is worse than a generic one, and they can only
  // disagree if they are stored apart.
  const [conflict, setConflict] = useState<{ nodeId: string; source: string | null } | null>(null);

  const dismissConflict = useCallback(() => setConflict(null), []);

  useEngineEvent<McpChangedPayload>("mcp-changed", (payload) => {
    // A change to a work the writer is not looking at needs no UI reaction.
    if (payload.project_id && projectId && payload.project_id !== projectId) {
      return;
    }
    onOutlineChanged();

    const touched = payload.node_ids ?? [];
    if (!openNodeId || !touched.includes(openNodeId)) {
      return;
    }
    if (editorDirty) {
      setConflict({ nodeId: openNodeId, source: payload.source ?? null });
      return;
    }
    onSceneChanged(openNodeId);
  });

  return {
    conflictNodeId: conflict?.nodeId ?? null,
    conflictSource: conflict?.source ?? null,
    dismissConflict,
  };
}
