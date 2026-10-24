import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  list: vi.fn(),
  listModels: vi.fn(),
  settingsSet: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  imageProviders: { list: rpc.list, listModels: rpc.listModels },
  settings: { set: rpc.settingsSet },
}));

vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars
        ? `${key}:${Object.entries(vars)
            .map(([name, value]) => `${name}=${value}`)
            .join(",")}`
        : key,
  }),
}));

import { ImageGenSection } from "./ImageGenSection";

function rows(active: string | null, extras: Record<string, Record<string, unknown>> = {}) {
  return ["openai-codex", "gemini-native"].map((id) => ({
    id,
    active: id === active,
    configured: false,
    consented: false,
    ...extras[id],
  }));
}

describe("ImageGenSection", () => {
  beforeEach(() => {
    rpc.list.mockReset();
    rpc.listModels.mockReset().mockResolvedValue({ models: [] });
    rpc.settingsSet.mockReset().mockResolvedValue({});
  });

  it("starts off and turns a provider on", async () => {
    rpc.list.mockResolvedValue(rows(null));
    render(<ImageGenSection onOpenProviders={() => {}} />);
    const off = await screen.findByTestId("image-choice-off");
    await waitFor(() => expect(off).not.toBeDisabled());
    expect(off).toHaveAttribute("aria-pressed", "true");

    fireEvent.click(screen.getByTestId("image-choice-gemini-native"));
    await waitFor(() => expect(rpc.settingsSet).toHaveBeenCalledWith({ image_provider: "gemini-native" }));
  });

  it("sends a provider that is not connected to the provider pane", async () => {
    rpc.list.mockResolvedValue(rows("gemini-native"));
    const onOpenProviders = vi.fn();
    render(<ImageGenSection onOpenProviders={onOpenProviders} />);
    fireEvent.click(await screen.findByTestId("image-go-providers"));
    expect(onOpenProviders).toHaveBeenCalled();
    // Nothing to pick or agree to before there is a credential.
    expect(screen.queryByTestId("image-consent")).toBeNull();
    expect(rpc.listModels).not.toHaveBeenCalled();
  });

  it("lists image models and saves the chosen one and the consent", async () => {
    rpc.list.mockResolvedValue(rows("gemini-native", { "gemini-native": { configured: true } }));
    rpc.listModels.mockResolvedValue({ models: ["gemini-3.1-flash-image"] });
    render(<ImageGenSection onOpenProviders={() => {}} />);

    expect(await screen.findByTestId("image-state")).toHaveTextContent("settings.images.state.needsConsent");
    await waitFor(() => expect(rpc.listModels).toHaveBeenCalledWith("gemini-native"));
    const input = screen.getByTestId("image-model-input");
    expect(input).toHaveAttribute("placeholder", "settings.images.model.pick");

    fireEvent.change(input, { target: { value: " gemini-3.1-flash-image " } });
    fireEvent.blur(input);
    await waitFor(() =>
      expect(rpc.settingsSet).toHaveBeenCalledWith({ image_providers: { "gemini-native": { model: "gemini-3.1-flash-image" } } }),
    );

    const consent = screen.getByTestId("image-consent");
    expect(consent.parentElement).toHaveTextContent("settings.images.consent:provider=settings.providers.name.gemini-native");
    fireEvent.click(consent);
    await waitFor(() => {
      const patch = rpc.settingsSet.mock.calls.at(-1)?.[0];
      expect(patch.image_providers["gemini-native"].consented_at).toBeGreaterThan(0);
    });
  });

  it("offers the account default for Codex and shows a list failure without blocking entry", async () => {
    rpc.list.mockResolvedValue(rows("openai-codex", { "openai-codex": { configured: true, consented: true } }));
    rpc.listModels.mockRejectedValue(new Error("boom"));
    render(<ImageGenSection onOpenProviders={() => {}} />);
    expect(await screen.findByTestId("image-state")).toHaveTextContent("settings.images.state.ready");
    expect(screen.getByTestId("image-model-input")).toHaveAttribute("placeholder", "settings.images.model.auto");
    expect(await screen.findByTestId("image-model-error")).toHaveTextContent("settings.images.model.listFailed");
    expect(screen.getByTestId("image-model-input")).not.toBeDisabled();
  });
});
