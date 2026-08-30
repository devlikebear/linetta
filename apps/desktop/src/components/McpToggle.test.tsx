import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

const rpc = vi.hoisted(() => ({
  status: vi.fn(),
  enable: vi.fn(),
  disable: vi.fn(),
  settingsGet: vi.fn(),
  settingsSet: vi.fn(),
  projectGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  mcp: { status: rpc.status, enable: rpc.enable, disable: rpc.disable },
  settings: { get: rpc.settingsGet, set: rpc.settingsSet },
  projects: { get: rpc.projectGet },
}));
vi.mock("../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
}));

import { McpToggle } from "./McpToggle";

const OFF = { running: false, mode: "off", token_set: false };
const RUNNING_THIS = { running: true, mode: "full", port: 7391, project_id: "p1", token_set: true };
const RUNNING_ALL = { running: true, mode: "full", port: 7391, project_id: "", token_set: true };
const RUNNING_OTHER = { running: true, mode: "full", port: 7391, project_id: "p2", token_set: true };

function renderToggle() {
  return render(
    <MemoryRouter>
      <McpToggle projectId="p1" projectTitle="악귀 호루" />
    </MemoryRouter>,
  );
}

describe("McpToggle", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    rpc.status.mockResolvedValue(OFF);
    rpc.settingsGet.mockResolvedValue({ mcp_consent_version: 1 });
    rpc.settingsSet.mockResolvedValue({});
    rpc.projectGet.mockResolvedValue({ id: "p2", title: "다른 작품" });
  });

  it("shows a quiet off state instead of disappearing", async () => {
    renderToggle();
    const btn = await screen.findByTestId("mcp-indicator");
    await waitFor(() => expect(btn.getAttribute("aria-label")).toBe("workspace.mcp.off"));
    expect(screen.queryByTestId("mcp-popover")).toBeNull();
  });

  it("stays hidden on a build with no MCP method at all", async () => {
    rpc.status.mockRejectedValue(new Error("method not found: mcp.status"));
    renderToggle();
    await waitFor(() => expect(rpc.status).toHaveBeenCalled());
    expect(screen.queryByTestId("mcp-indicator")).toBeNull();
  });

  it("never enables from a stray click: turning on goes through the popover", async () => {
    rpc.enable.mockResolvedValue({ token: "t", status: RUNNING_THIS });
    renderToggle();

    await userEvent.click(await screen.findByTestId("mcp-indicator"));
    // The click only opened the popover; nothing was enabled yet.
    expect(rpc.enable).not.toHaveBeenCalled();
    await screen.findByTestId("mcp-popover");

    await userEvent.click(screen.getByTestId("mcp-toggle-enable"));
    await waitFor(() => expect(rpc.enable).toHaveBeenCalledTimes(1));
    // Enabling from the work screen scopes the server to this work, full mode.
    expect(rpc.settingsSet).toHaveBeenCalledWith(
      expect.objectContaining({ mcp_mode: "full", mcp_project_id: "p1" }),
    );
    // The popover now names the open work.
    expect((await screen.findByTestId("mcp-scope")).textContent).toBe(
      "workspace.mcp.running.thisWork",
    );
  });

  it("carries consent in the first enable, and says so on the button", async () => {
    rpc.settingsGet.mockResolvedValue({});
    rpc.enable.mockResolvedValue({ token: "t", status: RUNNING_THIS });
    renderToggle();

    await userEvent.click(await screen.findByTestId("mcp-indicator"));
    // Not consented yet: the sentence is shown and the button says so.
    await screen.findByTestId("mcp-popover-consent");
    const enable = screen.getByTestId("mcp-toggle-enable");
    expect(enable.textContent).toBe("workspace.mcp.enable.consentConfirm");

    await userEvent.click(enable);
    await waitFor(() => expect(rpc.enable).toHaveBeenCalled());
    expect(rpc.settingsSet).toHaveBeenCalledWith(
      expect.objectContaining({ mcp_mode: "full", mcp_project_id: "p1", mcp_consent_version: 1 }),
    );
  });

  it("persists off BEFORE stopping the listener, so the kill switch survives a restart", async () => {
    rpc.status.mockResolvedValue(RUNNING_THIS);
    rpc.disable.mockResolvedValue(OFF);
    renderToggle();

    await userEvent.click(await screen.findByTestId("mcp-indicator"));
    await userEvent.click(await screen.findByTestId("mcp-toggle-disable"));

    await waitFor(() => expect(rpc.disable).toHaveBeenCalledTimes(1));
    const offPatch = rpc.settingsSet.mock.calls.find((c) => c[0]?.mcp_mode === "off");
    expect(offPatch).toBeTruthy();
    // settings.set({off}) must land before mcp.disable (#74).
    const setOrder = rpc.settingsSet.mock.invocationCallOrder[
      rpc.settingsSet.mock.calls.findIndex((c) => c[0]?.mcp_mode === "off")
    ];
    expect(setOrder).toBeLessThan(rpc.disable.mock.invocationCallOrder[0]);
  });

  it("says when a different work is the open one, and can re-scope to this one", async () => {
    rpc.status.mockResolvedValue(RUNNING_OTHER);
    rpc.enable.mockResolvedValue({ token: "t", status: RUNNING_THIS });
    renderToggle();

    await userEvent.click(await screen.findByTestId("mcp-indicator"));
    // A dot in work A while only work B is open would be a lie; the popover
    // names the actually-open work.
    await waitFor(() =>
      expect(screen.getByTestId("mcp-scope").textContent).toBe(
        "workspace.mcp.running.otherWork:다른 작품",
      ),
    );

    await userEvent.click(screen.getByTestId("mcp-rescope"));
    await waitFor(() => expect(rpc.enable).toHaveBeenCalled());
    expect(rpc.settingsSet).toHaveBeenCalledWith(
      expect.objectContaining({ mcp_mode: "full", mcp_project_id: "p1" }),
    );
  });

  it("reports the whole library being open as exactly that", async () => {
    rpc.status.mockResolvedValue(RUNNING_ALL);
    renderToggle();
    await userEvent.click(await screen.findByTestId("mcp-indicator"));
    expect((await screen.findByTestId("mcp-scope")).textContent).toBe(
      "workspace.mcp.running.allWorks",
    );
    expect(screen.queryByTestId("mcp-rescope")).toBeNull();
  });

  it("refreshes as soon as an agent change arrives, without waiting for a poll", async () => {
    renderToggle();
    const btn = await screen.findByTestId("mcp-indicator");
    await waitFor(() => expect(btn.getAttribute("aria-label")).toBe("workspace.mcp.off"));

    rpc.status.mockResolvedValue(RUNNING_THIS);
    const cb = ev.listeners.get("mcp-changed");
    expect(cb).toBeTruthy();
    await act(async () => {
      cb?.({ payload: { project_id: "p1", tool: "linetta_write_scene" } });
    });
    await waitFor(() =>
      expect(screen.getByTestId("mcp-indicator").getAttribute("aria-label")).toBe(
        "workspace.mcp.active.full",
      ),
    );
  });
});
