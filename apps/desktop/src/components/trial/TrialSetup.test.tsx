import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  connectStart: vi.fn(),
  connectStatus: vi.fn(),
  disconnect: vi.fn(),
  openExternalUrl: vi.fn(),
  trialStatus: vi.fn(),
  settingsSet: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  publishApi: { connectStart: rpc.connectStart, connectStatus: rpc.connectStatus, disconnect: rpc.disconnect },
  openExternalUrl: rpc.openExternalUrl,
  trial: { status: rpc.trialStatus },
  settings: { set: rpc.settingsSet },
}));

vi.mock("../../lib/appEvents", () => ({ dispatchAppEvent: vi.fn() }));

vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
}));

import { TrialSetup } from "./TrialSetup";

const SITE = "https://works.marvin-42.com";
const DISCONNECTED = { state: "disconnected", site_url: SITE };
const WAITING = { state: "waiting", site_url: SITE, confirm_code: "NWCQ", verify_url: `${SITE}/connect` };
const CONNECTED = { state: "connected", site_url: SITE, pen_name: "마곰", handle: "devlikebear" };
const trialOf = (state: string, remaining = 0, granted = 3_000_000) => ({
  state,
  granted_tokens: granted,
  used_tokens: granted - remaining,
  remaining_tokens: remaining,
});

describe("TrialSetup", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.connectStatus.mockResolvedValue(DISCONNECTED);
    rpc.disconnect.mockResolvedValue({ ok: true });
    rpc.openExternalUrl.mockResolvedValue(undefined);
    rpc.trialStatus.mockResolvedValue(trialOf("available", 2_134_000));
    rpc.settingsSet.mockResolvedValue({});
  });

  it("not connected: offers the Works connection behind the publishing disclosure, and asks the site nothing", async () => {
    render(<TrialSetup mode="settings" />);
    const connect = await screen.findByTestId("trial-connect");
    expect(screen.getByTestId("trial-intro")).toBeTruthy();
    expect(screen.getByText("settings.publish.disclosure")).toBeTruthy();
    expect(connect).toBeDisabled();
    await userEvent.click(screen.getByTestId("trial-publish-consent"));
    expect(connect).toBeEnabled();
    rpc.connectStart.mockResolvedValue(WAITING);
    await userEvent.click(connect);
    expect(rpc.connectStart).toHaveBeenCalledWith(true);
    expect(rpc.openExternalUrl).toHaveBeenCalledWith(WAITING.verify_url);
    expect(await screen.findByTestId("trial-waiting")).toBeTruthy();
    expect(rpc.trialStatus).not.toHaveBeenCalled();
    // No key, model or connection test anywhere.
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it.each([
    ["available", trialOf("available", 2_134_000), "trial.remaining:2,134,000,3,000,000"],
    ["not_started", trialOf("not_started", 3_000_000), "trial.notStarted:3,000,000"],
    ["exhausted", trialOf("exhausted", 0), "trial.exhausted"],
    ["paused", trialOf("paused", 1_000_000), "trial.unavailable"],
    ["unreachable", trialOf("unreachable"), "trial.unavailable"],
  ])("connected, %s: says where the trial stands", async (_name, status, text) => {
    rpc.connectStatus.mockResolvedValue(CONNECTED);
    rpc.trialStatus.mockResolvedValue(status);
    render(<TrialSetup mode="settings" />);
    expect(await screen.findByText(text)).toBeTruthy();
  });

  it("settings: the consent box saves as it is ticked", async () => {
    rpc.connectStatus.mockResolvedValue(CONNECTED);
    const onConsentChange = vi.fn();
    render(<TrialSetup mode="settings" consented={false} onConsentChange={onConsentChange} />);
    await userEvent.click(await screen.findByTestId("trial-consent"));
    expect(onConsentChange).toHaveBeenCalledWith(true);
    expect(screen.getByText("trial.consent")).toBeTruthy();
  });

  it("panel: starting saves consent and the provider in one write, only after the box is ticked", async () => {
    rpc.connectStatus.mockResolvedValue(CONNECTED);
    const onStarted = vi.fn();
    render(<TrialSetup mode="panel" onStarted={onStarted} />);
    const startButton = await screen.findByTestId("trial-start");
    expect(startButton).toBeDisabled();
    await userEvent.click(screen.getByTestId("trial-consent"));
    await userEvent.click(startButton);
    expect(rpc.settingsSet).toHaveBeenCalledTimes(1);
    const patch = rpc.settingsSet.mock.calls[0][0];
    expect(patch.provider).toBe("linetta-trial");
    expect(patch.providers["linetta-trial"].consented_at).toBeGreaterThan(0);
    await waitFor(() => expect(onStarted).toHaveBeenCalled());
  });

  it("panel: an exhausted trial cannot be started", async () => {
    rpc.connectStatus.mockResolvedValue(CONNECTED);
    rpc.trialStatus.mockResolvedValue(trialOf("exhausted"));
    render(<TrialSetup mode="panel" />);
    expect(await screen.findByText("trial.exhausted")).toBeTruthy();
    expect(screen.queryByTestId("trial-start")).toBeNull();
  });

  it("panel: cancelling changes nothing, and abandons a connection still waiting", async () => {
    rpc.connectStatus.mockResolvedValue(WAITING);
    const onCancel = vi.fn();
    render(<TrialSetup mode="panel" onCancel={onCancel} />);
    await screen.findByTestId("trial-waiting");
    await userEvent.click(screen.getByTestId("trial-cancel"));
    expect(rpc.disconnect).toHaveBeenCalled();
    await waitFor(() => expect(onCancel).toHaveBeenCalled());
    expect(rpc.settingsSet).not.toHaveBeenCalled();
  });

  it("tells the caller when the connection comes, not when it first loads", async () => {
    rpc.connectStatus.mockResolvedValue(DISCONNECTED);
    const onConnectionChange = vi.fn();
    render(<TrialSetup mode="settings" onConnectionChange={onConnectionChange} />);
    await userEvent.click(await screen.findByTestId("trial-publish-consent"));
    expect(onConnectionChange).not.toHaveBeenCalled();
    rpc.connectStart.mockResolvedValue(CONNECTED);
    await userEvent.click(screen.getByTestId("trial-connect"));
    await waitFor(() => expect(onConnectionChange).toHaveBeenCalledTimes(1));
  });
});
