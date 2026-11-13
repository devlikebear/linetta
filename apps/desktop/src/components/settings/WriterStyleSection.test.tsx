import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ get: vi.fn(), set: vi.fn() }));

vi.mock("../../lib/rpc", () => ({
  style: { getWriterRules: rpc.get, setWriterRules: rpc.set },
}));
vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));
vi.mock("../../lib/rpcMessage", () => ({
  rpcErrorMessage: (error: { data?: { reason?: string } }) => `error:${error.data?.reason ?? "unknown"}`,
}));

import { WriterStyleSection } from "./WriterStyleSection";

describe("WriterStyleSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.get.mockResolvedValue({ avoid_phrases: ["that was"], max_sentence_chars: 80, updated_at: 1 });
  });

  it("shows the writer's rules and saves what was typed", async () => {
    rpc.set.mockResolvedValue({ avoid_phrases: ["that was", "suddenly"], max_sentence_chars: 0, updated_at: 2 });
    render(<WriterStyleSection />);
    const box = await screen.findByLabelText("style.avoidPhrases");
    expect(box).toHaveValue("that was");
    expect(screen.getByLabelText("style.maxSentence")).toHaveValue(80);

    fireEvent.change(box, { target: { value: "that was\n  suddenly \n\n" } });
    fireEvent.change(screen.getByLabelText("style.maxSentence"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "style.save" }));

    await waitFor(() => expect(rpc.set).toHaveBeenCalledWith({ avoid_phrases: ["that was", "suddenly"], max_sentence_chars: 0 }));
    expect(await screen.findByRole("status")).toHaveTextContent("settings.writerStyle.saved");
  });

  it("says why a save was refused", async () => {
    rpc.set.mockRejectedValue({ data: { reason: "style_phrase_too_long" } });
    render(<WriterStyleSection />);
    fireEvent.click(await screen.findByRole("button", { name: "style.save" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("error:style_phrase_too_long");
  });
});
