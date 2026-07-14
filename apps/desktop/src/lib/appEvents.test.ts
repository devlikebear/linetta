import { describe, expect, it, vi } from "vitest";
import { dispatchAppEvent, subscribeAppEvent } from "./appEvents";
import type { Settings } from "./types";

describe("typed app events", () => {
  it("delivers typed settings payloads and unsubscribes", () => {
    const listener = vi.fn();
    const unsubscribe = subscribeAppEvent("linetta:settings-updated", listener);
    const settings = { language: "ko" } as Settings;

    dispatchAppEvent("linetta:settings-updated", settings);
    unsubscribe();
    dispatchAppEvent("linetta:settings-updated", { ...settings, language: "en" });

    expect(listener).toHaveBeenCalledTimes(1);
    expect(listener).toHaveBeenCalledWith(settings);
  });
});
