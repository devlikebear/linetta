import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  status: vi.fn(),
  enable: vi.fn(),
  disable: vi.fn(),
  regenerateToken: vi.fn(),
  activity: vi.fn(),
  settingsGet: vi.fn(),
  settingsSet: vi.fn(),
  bridgePath: vi.fn(),
  projectsList: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  mcp: {
    status: rpc.status,
    enable: rpc.enable,
    disable: rpc.disable,
    regenerateToken: rpc.regenerateToken,
    activity: rpc.activity,
  },
  settings: { get: rpc.settingsGet, set: rpc.settingsSet },
  projects: { list: rpc.projectsList },
  // The pane asks the shell for the installed bridge path when none is given.
  mcpBridgePath: rpc.bridgePath,
}));

vi.mock("../../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
}));

import { McpSection } from "./McpSection";

const OFF = { running: false, mode: "off", token_set: false };
const RUNNING = { running: true, mode: "full", port: 7391, token_set: true };

function settingsWith(overrides: Record<string, unknown> = {}) {
  return { mcp_mode: "read_only", mcp_port: 7391, mcp_project_id: "", ...overrides };
}

describe("McpSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.status.mockResolvedValue(OFF);
    rpc.settingsGet.mockResolvedValue(settingsWith());
    rpc.settingsSet.mockResolvedValue(settingsWith());
    rpc.activity.mockResolvedValue([]);
    rpc.bridgePath.mockResolvedValue(null);
    rpc.projectsList.mockResolvedValue([{ id: "work-1", title: "첫 작품" }]);
  });

  it("refuses to enable until the writer has explicitly consented", async () => {
    render(<McpSection />);
    const enable = await screen.findByTestId("mcp-enable");

    // Consent is a decision, not something a toggle implies.
    expect(enable).toBeDisabled();
    expect(rpc.enable).not.toHaveBeenCalled();

    await userEvent.click(screen.getByTestId("mcp-consent"));
    await waitFor(() => expect(rpc.settingsSet).toHaveBeenCalled());
    expect(rpc.settingsSet.mock.calls[0][0]).toMatchObject({ mcp_consent_version: 1 });
    await waitFor(() => expect(screen.getByTestId("mcp-enable")).toBeEnabled());
  });

  it("starts enabled when consent was already given", async () => {
    rpc.settingsGet.mockResolvedValue(settingsWith({ mcp_consent_version: 1 }));
    render(<McpSection />);
    await waitFor(() => expect(screen.getByTestId("mcp-enable")).toBeEnabled());
  });

  it("puts the token in the run-it-yourself command and a header helper in .mcp.json", async () => {
    rpc.settingsGet.mockResolvedValue(settingsWith({ mcp_consent_version: 1 }));
    rpc.enable.mockResolvedValue({ token: "SECRET-TOKEN", status: RUNNING });
    render(<McpSection bridgePath="/opt/linetta/linetta-mcp" />);

    await userEvent.click(await screen.findByTestId("mcp-enable"));
    await screen.findByTestId("mcp-snippets");

    const command = screen.getByTestId("mcp-snippet-command").textContent ?? "";
    expect(command).toContain("claude mcp add --transport http linetta");
    expect(command).toContain("http://127.0.0.1:7391/mcp");
    expect(command).toContain("Bearer SECRET-TOKEN");

    // .mcp.json is a file people commit, so the token must never land in it.
    const projectConfig = screen.getByTestId("mcp-snippet-project").textContent ?? "";
    expect(projectConfig).not.toContain("SECRET-TOKEN");
    expect(projectConfig).toContain("headersHelper");
    expect(projectConfig).toContain("/opt/linetta/linetta-mcp --print-headers");

    const desktop = screen.getByTestId("mcp-snippet-desktop").textContent ?? "";
    expect(desktop).not.toContain("SECRET-TOKEN");
    expect(desktop).toContain("/opt/linetta/linetta-mcp");
  });

  it("enables full by default: one decision, no mode dropdown in the way", async () => {
    rpc.settingsGet.mockResolvedValue(settingsWith({ mcp_mode: "off", mcp_consent_version: 1 }));
    rpc.enable.mockResolvedValue({ token: "t", status: RUNNING });
    render(<McpSection />);

    await userEvent.click(await screen.findByTestId("mcp-enable"));

    await waitFor(() => expect(rpc.enable).toHaveBeenCalled());
    const calls = rpc.settingsSet.mock.calls;
    expect(calls[calls.length - 1]?.[0]).toMatchObject({ mcp_mode: "full" });
  });

  it("enables read-only and a work restriction when chosen under Advanced", async () => {
    rpc.settingsGet.mockResolvedValue(settingsWith({ mcp_mode: "off", mcp_consent_version: 1 }));
    rpc.enable.mockResolvedValue({ token: "t", status: RUNNING });
    render(<McpSection />);

    // The work is picked by title from a list, never typed as a UUID.
    await userEvent.selectOptions(await screen.findByLabelText("settings.mcp.projectLimit"), "work-1");
    await userEvent.click(screen.getByTestId("mcp-readonly"));
    await userEvent.click(screen.getByTestId("mcp-enable"));

    await waitFor(() => expect(rpc.enable).toHaveBeenCalled());
    const calls = rpc.settingsSet.mock.calls;
    const patch = calls[calls.length - 1]?.[0];
    expect(patch).toMatchObject({ mcp_mode: "read_only", mcp_project_id: "work-1" });
  });

  it("keeps an unknown restricted work visible instead of silently dropping it", async () => {
    rpc.settingsGet.mockResolvedValue(
      settingsWith({ mcp_consent_version: 1, mcp_project_id: "gone-work" }),
    );
    render(<McpSection />);

    const select = (await screen.findByLabelText("settings.mcp.projectLimit")) as HTMLSelectElement;
    await waitFor(() => expect(select.value).toBe("gone-work"));
  });

  it("kills the listener, persists off, and drops the token from the pane", async () => {
    rpc.status.mockResolvedValue(RUNNING);
    rpc.settingsGet.mockResolvedValue(settingsWith({ mcp_consent_version: 1 }));
    rpc.disable.mockResolvedValue(OFF);
    render(<McpSection />);

    await userEvent.click(await screen.findByTestId("mcp-disable"));

    await waitFor(() => expect(rpc.disable).toHaveBeenCalledTimes(1));
    // Off must reach the disk BEFORE the stop, or the server resurrects on
    // the next launch (#74).
    const offIndex = rpc.settingsSet.mock.calls.findIndex((c) => c[0]?.mcp_mode === "off");
    expect(offIndex).toBeGreaterThanOrEqual(0);
    expect(rpc.settingsSet.mock.invocationCallOrder[offIndex]).toBeLessThan(
      rpc.disable.mock.invocationCallOrder[0],
    );
    await waitFor(() => expect(screen.queryByTestId("mcp-snippets")).toBeNull());
  });

  it("says how to recover the command when the token is not in hand", async () => {
    rpc.status.mockResolvedValue(RUNNING);
    rpc.settingsGet.mockResolvedValue(settingsWith({ mcp_consent_version: 1 }));
    render(<McpSection />);

    // Reopening Settings on a running server: the token was minted earlier and
    // settings.get redacts it, so the pane must not pretend to have it.
    await screen.findByTestId("mcp-token-hidden");
    expect(screen.queryByTestId("mcp-snippet-command")).toBeNull();
  });

  it("surfaces a refused enable instead of silently doing nothing", async () => {
    rpc.settingsGet.mockResolvedValue(settingsWith({ mcp_consent_version: 1 }));
    rpc.enable.mockRejectedValue(new Error("mcp port in use"));
    render(<McpSection />);

    await userEvent.click(await screen.findByTestId("mcp-enable"));
    const error = await screen.findByTestId("mcp-error");
    expect(error.textContent).toContain("port in use");
  });

  it("says where to get the bridge on a build that ships without one", async () => {
    rpc.status.mockResolvedValue(RUNNING);
    rpc.settingsGet.mockResolvedValue(settingsWith({ mcp_consent_version: 1 }));
    rpc.bridgePath.mockResolvedValue(null);
    render(<McpSection />);

    // Mac App Store builds leave the bridge out, so the Desktop snippet points
    // at a command the writer does not have yet. Saying so beats a silent fail.
    await screen.findByTestId("mcp-bridge-missing");
  });

  it("stays quiet about the bridge when the build actually bundles one", async () => {
    rpc.status.mockResolvedValue(RUNNING);
    rpc.settingsGet.mockResolvedValue(settingsWith({ mcp_consent_version: 1 }));
    rpc.bridgePath.mockResolvedValue("/Applications/Linetta.app/resources/linetta-mcp");
    render(<McpSection />);

    const desktop = await screen.findByTestId("mcp-snippet-desktop");
    await waitFor(() =>
      expect(desktop.textContent).toContain("/Applications/Linetta.app/resources/linetta-mcp"),
    );
    expect(screen.queryByTestId("mcp-bridge-missing")).toBeNull();
  });

  it("names the taken port as something the writer can fix", async () => {
    rpc.settingsGet.mockResolvedValue(settingsWith({ mcp_consent_version: 1 }));
    // The engine's English sentence says nothing about what to do next; the
    // reason code is what turns it into "pick another port".
    const refused = Object.assign(new Error("mcp port in use: 7391"), {
      data: { reason: "mcp_port_in_use" },
    });
    rpc.enable.mockRejectedValue(refused);
    render(<McpSection />);

    await userEvent.click(await screen.findByTestId("mcp-enable"));
    const error = await screen.findByTestId("mcp-error");
    expect(error.textContent).toBe("errors.mcpPortInUse");

    // The port field stays editable so the fix is one keystroke away.
    expect(screen.getByLabelText("settings.mcp.port")).not.toBeDisabled();
  });
});
