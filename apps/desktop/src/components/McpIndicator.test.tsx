import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { act } from "react";
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

const rpc = vi.hoisted(() => ({ status: vi.fn() }));
vi.mock("../lib/rpc", () => ({ mcp: { status: rpc.status } }));
vi.mock("../lib/i18n", () => ({ useI18n: () => ({ t: (k: string) => k }) }));

import { McpIndicator } from "./McpIndicator";

function renderIndicator() {
  return render(
    <MemoryRouter>
      <McpIndicator />
    </MemoryRouter>,
  );
}

describe("McpIndicator", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
  });

  it("stays out of the way when nothing is listening", async () => {
    rpc.status.mockResolvedValue({ running: false, mode: "off", token_set: false });
    renderIndicator();
    await waitFor(() => expect(rpc.status).toHaveBeenCalled());
    expect(screen.queryByTestId("mcp-indicator")).toBeNull();
  });

  it("shows while an agent can reach the work, and links to Settings", async () => {
    rpc.status.mockResolvedValue({ running: true, mode: "full", port: 7391, token_set: true });
    renderIndicator();

    const dot = await screen.findByTestId("mcp-indicator");
    // The label has to say what the agent can do, not just that something is on.
    expect(dot.getAttribute("aria-label")).toBe("workspace.mcp.active.full");
    expect(dot.getAttribute("href")).toBe("/settings");
  });

  it("distinguishes read-only from full access", async () => {
    rpc.status.mockResolvedValue({ running: true, mode: "read_only", port: 7391, token_set: true });
    renderIndicator();
    const dot = await screen.findByTestId("mcp-indicator");
    expect(dot.getAttribute("aria-label")).toBe("workspace.mcp.active.readOnly");
  });

  it("appears as soon as an agent change arrives, without waiting for a poll", async () => {
    rpc.status.mockResolvedValue({ running: false, mode: "off", token_set: false });
    renderIndicator();
    await waitFor(() => expect(rpc.status).toHaveBeenCalled());
    expect(screen.queryByTestId("mcp-indicator")).toBeNull();

    const cb = ev.listeners.get("mcp-changed");
    expect(cb).toBeTruthy();
    await act(async () => {
      cb?.({ payload: { project_id: "p1", tool: "linetta_write_scene" } });
    });
    expect(screen.getByTestId("mcp-indicator")).toBeTruthy();
  });

  it("stays hidden on a build with no MCP method at all", async () => {
    rpc.status.mockRejectedValue(new Error("method not found: mcp.status"));
    renderIndicator();
    await waitFor(() => expect(rpc.status).toHaveBeenCalled());
    expect(screen.queryByTestId("mcp-indicator")).toBeNull();
  });
});
