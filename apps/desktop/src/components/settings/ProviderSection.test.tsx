import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  providersList: vi.fn(),
  settingsSet: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  providers: { list: rpc.providersList },
  settings: { set: rpc.settingsSet },
}));

vi.mock("../../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
}));

import { ProviderSection } from "./ProviderSection";

function statusRow(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "openai-codex",
    auth: "oauth",
    active: false,
    configured: false,
    consented: false,
    ...overrides,
  };
}

describe("ProviderSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.providersList.mockResolvedValue([
      statusRow({ id: "openai-codex", auth: "oauth", active: true }),
      statusRow({ id: "anthropic", auth: "api_key" }),
      statusRow({ id: "gemini-native", auth: "api_key" }),
      statusRow({ id: "openai", auth: "api_key" }),
    ]);
    rpc.settingsSet.mockResolvedValue({});
  });

  it("renders the four providers and marks the active one", async () => {
    render(<ProviderSection />);

    const codex = await screen.findByTestId("provider-choice-openai-codex");
    expect(codex).toHaveAttribute("aria-pressed", "true");

    for (const id of ["anthropic", "gemini-native", "openai"]) {
      const button = screen.getByTestId(`provider-choice-${id}`);
      expect(button).toHaveAttribute("aria-pressed", "false");
    }
  });

  it("saves the chosen provider and does not keep it local", async () => {
    render(<ProviderSection />);
    await screen.findByTestId("provider-choice-anthropic");

    await userEvent.click(screen.getByTestId("provider-choice-anthropic"));

    await waitFor(() => expect(rpc.settingsSet).toHaveBeenCalledWith({ provider: "anthropic" }));
  });

  it("shows configured and consent state for the active provider", async () => {
    rpc.providersList.mockResolvedValue([
      statusRow({ id: "openai-codex", auth: "oauth", active: true, configured: true, consented: true }),
      statusRow({ id: "anthropic", auth: "api_key" }),
      statusRow({ id: "gemini-native", auth: "api_key" }),
      statusRow({ id: "openai", auth: "api_key" }),
    ]);
    render(<ProviderSection />);

    const state = await screen.findByTestId("provider-state");
    expect(state.textContent).toContain("settings.providers.state.configured");
    expect(state.textContent).toContain("settings.providers.state.consented");
  });

  it("renders a reason code as a translated message", async () => {
    rpc.providersList.mockRejectedValue(
      Object.assign(new Error("x"), {
        data: { reason: "provider_not_configured" },
      }),
    );
    render(<ProviderSection />);

    const error = await screen.findByTestId("provider-error");
    expect(error.textContent).toBe("errors.providerNotConfigured");
  });
});
