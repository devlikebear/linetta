import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ get: vi.fn(), set: vi.fn(), usage: vi.fn() }));

vi.mock("../../lib/rpc", () => ({
  settings: { get: rpc.get, set: rpc.set },
  summarizer: { usage: rpc.usage },
}));
vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string | number>) =>
      vars ? `${key}:${Object.entries(vars).map(([k, v]) => `${k}=${v}`).join(",")}` : key,
  }),
}));
vi.mock("../../lib/rpcMessage", () => ({
  rpcErrorMessage: (error: { reason?: string }) => `error:${error.reason ?? "unknown"}`,
}));

import { SummarizerSection } from "./SummarizerSection";

describe("SummarizerSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.get.mockResolvedValue({});
    rpc.usage.mockResolvedValue({ calls: 0, input: 0, output: 0 });
  });

  // #135: a settings payload without the key means off, the default.
  it("starts off and shows no usage before the first call", async () => {
    render(<SummarizerSection />);
    const toggle = await screen.findByTestId("summarizer-toggle");
    expect(toggle).toHaveAttribute("aria-checked", "false");
    expect(screen.queryByTestId("summarizer-usage")).toBeNull();
  });

  it("switches on through settings.set", async () => {
    rpc.set.mockResolvedValue({ summarizer_provider_enabled: true });
    render(<SummarizerSection />);
    fireEvent.click(await screen.findByTestId("summarizer-toggle"));
    await waitFor(() => expect(rpc.set).toHaveBeenCalledWith({ summarizer_provider_enabled: true }));
    await waitFor(() => expect(screen.getByTestId("summarizer-toggle")).toHaveAttribute("aria-checked", "true"));
  });

  it("shows what the provider calls have cost, summed", async () => {
    rpc.get.mockResolvedValue({ summarizer_provider_enabled: true });
    rpc.usage.mockResolvedValue({ calls: 3, input: 1200, output: 240 });
    render(<SummarizerSection />);
    expect(await screen.findByTestId("summarizer-usage")).toHaveTextContent(
      "settings.summarizer.usage:calls=3,input=1200,output=240",
    );
  });

  it("says why a switch did not take", async () => {
    rpc.set.mockRejectedValue({ reason: "boom" });
    render(<SummarizerSection />);
    fireEvent.click(await screen.findByTestId("summarizer-toggle"));
    expect(await screen.findByRole("alert")).toHaveTextContent("error:boom");
  });
});
