import { describe, expect, it, vi } from "vitest";

import { SceneSaveQueue } from "./sceneSaveQueue";

describe("SceneSaveQueue", () => {
  it("serializes saves per scene and advances the expected version", async () => {
    const persist = vi.fn()
      .mockResolvedValueOnce({ content_version: 1 })
      .mockResolvedValueOnce({ content_version: 2 });
    const queue = new SceneSaveQueue(persist);
    queue.seed("scene-a", 0);

    const first = queue.save("scene-a", "first");
    const second = queue.save("scene-a", "second");
    await Promise.all([first, second]);

    expect(persist).toHaveBeenNthCalledWith(1, "scene-a", "first", 0);
    expect(persist).toHaveBeenNthCalledWith(2, "scene-a", "second", 1);
  });

  it("does not replace a newer externally seeded version with an older response", async () => {
    let resolveSave!: (value: { content_version: number }) => void;
    const persist = vi.fn()
      .mockImplementationOnce(() => new Promise<{ content_version: number }>((resolve) => {
        resolveSave = resolve;
      }))
      .mockResolvedValueOnce({ content_version: 3 });
    const queue = new SceneSaveQueue(persist);
    queue.seed("scene-a", 0);

    const pending = queue.save("scene-a", "draft");
    await Promise.resolve();
    queue.seed("scene-a", 2);
    resolveSave({ content_version: 1 });
    await pending;

    await queue.save("scene-a", "after restore");
    expect(persist).toHaveBeenLastCalledWith("scene-a", "after restore", 2);
  });
});
