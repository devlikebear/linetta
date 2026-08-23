import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const ev = vi.hoisted(() => ({
  listeners: new Map<string, (e: { payload: unknown }) => void>(),
}));

vi.mock("@tauri-apps/api/event", () => ({
  listen: (event: string, cb: (e: { payload: unknown }) => void) => {
    ev.listeners.set(event, cb);
    return Promise.resolve(() => ev.listeners.delete(event));
  },
}));

import { useMcpChanges, type McpChangedPayload } from "./useMcpChanges";

async function emit(payload: McpChangedPayload) {
  const cb = ev.listeners.get("mcp-changed");
  if (!cb) throw new Error("mcp-changed listener was never registered");
  await act(async () => {
    cb({ payload });
  });
}

function setup(overrides: Partial<Parameters<typeof useMcpChanges>[0]> = {}) {
  const onOutlineChanged = vi.fn();
  const onSceneChanged = vi.fn();
  const view = renderHook(() =>
    useMcpChanges({
      projectId: "p1",
      openNodeId: "n1",
      editorDirty: false,
      onOutlineChanged,
      onSceneChanged,
      ...overrides,
    }),
  );
  return { view, onOutlineChanged, onSceneChanged };
}

describe("useMcpChanges", () => {
  beforeEach(() => {
    ev.listeners.clear();
  });

  it("refetches the outline and reloads the open scene when the buffer is clean", async () => {
    const { onOutlineChanged, onSceneChanged } = setup();
    await emit({ project_id: "p1", tool: "linetta_write_scene", node_ids: ["n1"] });

    expect(onOutlineChanged).toHaveBeenCalledTimes(1);
    expect(onSceneChanged).toHaveBeenCalledWith("n1");
  });

  it("never replaces a dirty buffer; it surfaces the change instead", async () => {
    const { view, onOutlineChanged, onSceneChanged } = setup({ editorDirty: true });
    await emit({ project_id: "p1", tool: "linetta_write_scene", node_ids: ["n1"] });

    // The outline is still safe to refresh — it is not what the writer is typing into.
    expect(onOutlineChanged).toHaveBeenCalledTimes(1);
    expect(onSceneChanged).not.toHaveBeenCalled();
    expect(view.result.current.conflictNodeId).toBe("n1");

    act(() => view.result.current.dismissConflict());
    expect(view.result.current.conflictNodeId).toBeNull();
  });

  it("ignores a change to a scene the writer does not have open", async () => {
    const { onOutlineChanged, onSceneChanged } = setup();
    await emit({ project_id: "p1", tool: "linetta_write_scene", node_ids: ["other"] });

    expect(onOutlineChanged).toHaveBeenCalledTimes(1);
    expect(onSceneChanged).not.toHaveBeenCalled();
  });

  it("ignores changes to another work entirely", async () => {
    const { onOutlineChanged, onSceneChanged } = setup();
    await emit({ project_id: "p2", tool: "linetta_write_scene", node_ids: ["n1"] });

    expect(onOutlineChanged).not.toHaveBeenCalled();
    expect(onSceneChanged).not.toHaveBeenCalled();
  });

  it("refetches the outline for a structural batch that names no scenes", async () => {
    const { onOutlineChanged, onSceneChanged } = setup();
    await emit({ project_id: "p1", tool: "linetta_apply_story_ops", batch_id: "b1" });

    expect(onOutlineChanged).toHaveBeenCalledTimes(1);
    expect(onSceneChanged).not.toHaveBeenCalled();
  });
});
