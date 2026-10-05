import { act, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { ArtStyleSection } from "./ArtStyleSection";

const mocks = vi.hoisted(() => ({
  listeners: new Map<string, (e: { payload: unknown }) => void>(),
  getArtStyle: vi.fn().mockResolvedValue({ project_id: "p", style: "ink", negative_prompt: "", updated_at: 1 }),
}));
vi.mock("@tauri-apps/api/event", () => ({ listen: (name: string, cb: (e: { payload: unknown }) => void) => {
  mocks.listeners.set(name, cb); return Promise.resolve(() => mocks.listeners.delete(name));
} }));
vi.mock("../lib/rpc", () => ({
  settings: { get: vi.fn().mockResolvedValue({ language: "en" }) },
  visuals: { getArtStyle: mocks.getArtStyle },
}));

describe("ArtStyleSection refresh", () => {
  it("ignores other projects and reloads after changes and undo", async () => {
    render(<I18nProvider><ArtStyleSection projectId="p" /></I18nProvider>);
    await screen.findByDisplayValue("ink");
    act(() => mocks.listeners.get("mcp-changed")?.({ payload: { project_id: "other", tool: "linetta_set_art_style" } }));
    expect(mocks.getArtStyle).toHaveBeenCalledTimes(1);
    for (const tool of ["linetta_set_art_style", "linetta_undo_last_change"]) {
      mocks.getArtStyle.mockResolvedValueOnce({ project_id: "p", style: tool, negative_prompt: "", updated_at: 2 });
      act(() => mocks.listeners.get("mcp-changed")?.({ payload: { project_id: "p", tool } }));
      await waitFor(() => expect(screen.getByDisplayValue(tool)).toBeInTheDocument());
    }
  });
});
