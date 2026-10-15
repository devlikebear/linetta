import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ProviderStatus } from "../../lib/types";

const rpc = vi.hoisted(() => ({
  providersList: vi.fn(),
  listModels: vi.fn(),
  settingsSet: vi.fn(),
}));

const events = vi.hoisted(() => ({ dispatch: vi.fn() }));

vi.mock("../../lib/rpc", () => ({
  providers: { list: rpc.providersList, listModels: rpc.listModels },
  settings: { set: rpc.settingsSet },
}));

vi.mock("../../lib/appEvents", () => ({ dispatchAppEvent: events.dispatch }));

vi.mock("../../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars
        ? `${key}:${Object.entries(vars)
            .map(([name, value]) => `${name}=${value}`)
            .join(",")}`
        : key,
  }),
}));

import { ProviderPicker } from "./ProviderPicker";

function row(overrides: Partial<ProviderStatus>): ProviderStatus {
  return {
    id: "anthropic",
    auth: "api_key",
    active: false,
    configured: true,
    consented: true,
    ...overrides,
  };
}

/** Gemini active with a stored model; Anthropic ready; Codex and the
 *  OpenAI-compatible slot not set up. */
const ROWS: ProviderStatus[] = [
  row({ id: "openai-codex", auth: "oauth", configured: false, consented: false }),
  row({ id: "anthropic" }),
  row({ id: "gemini-native", active: true, model: "gemini-3.8-flash" }),
  row({ id: "openai", configured: true, consented: false }),
];

function renderPicker(props: Partial<Parameters<typeof ProviderPicker>[0]> = {}) {
  const onRows = vi.fn();
  const onError = vi.fn();
  render(<ProviderPicker rows={ROWS} disabled={false} onRows={onRows} onError={onError} {...props} />);
  return { onRows, onError };
}

const providerSelect = () => screen.getByTestId("agent-provider-select") as HTMLSelectElement;
const modelSelect = () => screen.getByTestId("agent-model-select") as HTMLSelectElement;
const option = (select: HTMLSelectElement, value: string) =>
  [...select.options].find((o) => o.value === value)!;

beforeEach(() => {
  vi.clearAllMocks();
  rpc.settingsSet.mockImplementation(() => Promise.resolve({ provider: "anthropic" }));
  rpc.providersList.mockImplementation(() => Promise.resolve(ROWS));
});

describe("ProviderPicker", () => {
  it("shows the active provider and its stored model", () => {
    renderPicker();
    expect(providerSelect().value).toBe("gemini-native");
    expect(modelSelect().value).toBe("gemini-3.8-flash");
  });

  it("offers only providers a turn could start on", () => {
    renderPicker();
    const select = providerSelect();
    expect(option(select, "anthropic").disabled).toBe(false);
    // No credential, and a credential without consent: both would be
    // refused by agent.run, so neither is offered.
    expect(option(select, "openai-codex").disabled).toBe(true);
    expect(option(select, "openai").disabled).toBe(true);
    expect(option(select, "openai").textContent).toBe(
      "agentPanel.provider.needsSetup:provider=settings.providers.name.openai",
    );
  });

  it("renders nothing without an active row", () => {
    renderPicker({ rows: [] });
    expect(screen.queryByTestId("agent-provider-picker")).toBeNull();
  });

  it("switches the provider, announces the settings, and adopts the fresh list", async () => {
    const { onRows } = renderPicker();
    await act(async () => {
      fireEvent.change(providerSelect(), { target: { value: "anthropic" } });
    });
    expect(rpc.settingsSet).toHaveBeenCalledWith({ provider: "anthropic" });
    expect(events.dispatch).toHaveBeenCalledWith("linetta:settings-updated", { provider: "anthropic" });
    expect(onRows).toHaveBeenCalledWith(ROWS);
  });

  it("saves a model for the active provider only", async () => {
    rpc.listModels.mockImplementation(() =>
      Promise.resolve({ models: ["gemini-3.8-flash", "gemini-3.8-pro"] }),
    );
    renderPicker();
    await act(async () => {
      fireEvent.focus(modelSelect());
    });
    expect(rpc.listModels).toHaveBeenCalledWith("gemini-native");
    expect(option(modelSelect(), "gemini-3.8-pro")).toBeTruthy();

    await act(async () => {
      fireEvent.change(modelSelect(), { target: { value: "gemini-3.8-pro" } });
    });
    expect(rpc.settingsSet).toHaveBeenCalledWith({
      providers: { "gemini-native": { model: "gemini-3.8-pro" } },
    });
  });

  it("treats the empty model as the provider default, a real choice", async () => {
    renderPicker();
    await act(async () => {
      fireEvent.change(modelSelect(), { target: { value: "" } });
    });
    expect(rpc.settingsSet).toHaveBeenCalledWith({ providers: { "gemini-native": { model: "" } } });
  });

  it("fetches the model list once per provider", async () => {
    rpc.listModels.mockImplementation(() => Promise.resolve({ models: [] }));
    renderPicker();
    await act(async () => {
      fireEvent.focus(modelSelect());
    });
    await act(async () => {
      fireEvent.focus(modelSelect());
    });
    expect(rpc.listModels).toHaveBeenCalledTimes(1);
  });

  it("keeps the stored model and says so quietly when the list fails", async () => {
    rpc.listModels.mockImplementation(() => Promise.reject(new Error("boom")));
    renderPicker();
    await act(async () => {
      fireEvent.focus(modelSelect());
    });
    expect(screen.getByTestId("agent-model-failed")).toBeTruthy();
    expect(modelSelect().value).toBe("gemini-3.8-flash");
  });

  it("hands a failed save to the panel", async () => {
    const err = new Error("refused");
    rpc.settingsSet.mockImplementation(() => Promise.reject(err));
    const { onError, onRows } = renderPicker();
    await act(async () => {
      fireEvent.change(providerSelect(), { target: { value: "anthropic" } });
    });
    expect(onError).toHaveBeenCalledWith(err);
    expect(onRows).not.toHaveBeenCalled();
  });

  it("is locked while a turn runs", () => {
    renderPicker({ disabled: true });
    expect(providerSelect().disabled).toBe(true);
    expect(modelSelect().disabled).toBe(true);
  });
});
