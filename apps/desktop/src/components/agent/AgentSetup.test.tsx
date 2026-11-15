import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ProviderStatus } from "../../lib/types";

const rpc = vi.hoisted(() => ({
  connectStatus: vi.fn(),
  trialStatus: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  publishApi: { connectStart: vi.fn(), connectStatus: rpc.connectStatus, disconnect: vi.fn() },
  openExternalUrl: vi.fn(),
  trial: { status: rpc.trialStatus },
  settings: { set: vi.fn() },
}));

vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

import { AgentSetup } from "./AgentSetup";

const row = (o: Partial<ProviderStatus>): ProviderStatus => ({
  id: "anthropic",
  auth: "api_key",
  active: false,
  configured: false,
  consented: false,
  ...o,
});

const trialRow = row({ id: "linetta-trial", auth: "works" });

function renderSetup(rows: ProviderStatus[]) {
  return render(
    <MemoryRouter>
      <AgentSetup rows={rows} onStarted={vi.fn()} />
    </MemoryRouter>,
  );
}

describe("AgentSetup", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.connectStatus.mockResolvedValue({ state: "disconnected", site_url: "https://x" });
    rpc.trialStatus.mockResolvedValue({ state: "not_connected", granted_tokens: 0, used_tokens: 0, remaining_tokens: 0 });
  });

  it("with nothing ready, offers the trial first and the writer's own key second", async () => {
    renderSetup([trialRow, row({ active: true })]);
    const cards = await screen.findAllByRole("button");
    expect(cards[0]).toBe(screen.getByTestId("agent-try-trial"));
    expect(screen.getByTestId("agent-own-key").getAttribute("href")).toBe("/settings");
    await userEvent.click(screen.getByTestId("agent-try-trial"));
    expect(await screen.findByTestId("trial-setup-panel")).toBeTruthy();
  });

  it("goes back to the two cards on cancel", async () => {
    renderSetup([trialRow, row({ active: true })]);
    await userEvent.click(await screen.findByTestId("agent-try-trial"));
    await userEvent.click(await screen.findByTestId("trial-cancel"));
    expect(await screen.findByTestId("agent-try-trial")).toBeTruthy();
  });

  it("a spent trial's card says so and cannot be chosen", async () => {
    rpc.trialStatus.mockResolvedValue({ state: "exhausted", granted_tokens: 3, used_tokens: 3, remaining_tokens: 0 });
    renderSetup([trialRow]);
    expect(await screen.findByText("agentPanel.trial.used")).toBeTruthy();
    expect(screen.getByTestId("agent-try-trial")).toBeDisabled();
  });

  it("keeps the plain notice when another provider is ready", async () => {
    renderSetup([trialRow, row({ configured: true, consented: true })]);
    expect(screen.getByText("agentPanel.unconfigured")).toBeTruthy();
    expect(screen.queryByTestId("agent-try-trial")).toBeNull();
    expect(rpc.trialStatus).not.toHaveBeenCalled();
  });
});
