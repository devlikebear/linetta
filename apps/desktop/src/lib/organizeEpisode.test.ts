import { describe, expect, it, vi } from "vitest";
import { organizeEpisode, type OrganizeDeps } from "./organizeEpisode";

const result = (over: Partial<{ started: boolean; scenes: number; unchanged: number; short: number }>) => ({
  started: false, episode_label: "12화", scenes: 0, unchanged: 0, short: 0, ...over,
});

function deps(over: Partial<OrganizeDeps> = {}): OrganizeDeps & { toast: ReturnType<typeof vi.fn> } {
  return {
    organize: vi.fn(),
    flush: vi.fn().mockResolvedValue(undefined),
    confirm: vi.fn().mockResolvedValue(true),
    toast: vi.fn(),
    t: (key, vars) => `${key}:${Object.values(vars).join(",")}`,
    errorMessage: (e) => `error:${String(e)}`,
    ...over,
  } as OrganizeDeps & { toast: ReturnType<typeof vi.fn> };
}

describe("organizeEpisode", () => {
  it("saves first, then starts and says how many scenes it reads", async () => {
    const order: string[] = [];
    const d = deps({
      flush: vi.fn(async () => { order.push("flush"); }),
      organize: vi.fn(async () => { order.push("organize"); return result({ started: true, scenes: 3 }); }),
    });
    await organizeEpisode("p1", "s1", d);
    expect(order).toEqual(["flush", "organize"]);
    expect(d.toast).toHaveBeenCalledWith("workspace.organize.started:12화,3");
  });

  it("asks before reading unchanged scenes again, and reads them on yes", async () => {
    const organize = vi.fn()
      .mockResolvedValueOnce(result({ unchanged: 2 }))
      .mockResolvedValueOnce(result({ started: true, scenes: 2 }));
    const d = deps({ organize });
    await organizeEpisode("p1", "s1", d);
    expect(d.confirm).toHaveBeenCalledWith("workspace.organize.unchanged:12화");
    expect(organize).toHaveBeenLastCalledWith("p1", "s1", true);
    expect(d.toast).toHaveBeenCalledWith("workspace.organize.started:12화,2");
  });

  it("does nothing more when the writer declines", async () => {
    const organize = vi.fn().mockResolvedValue(result({ unchanged: 2 }));
    const d = deps({ organize, confirm: vi.fn().mockResolvedValue(false) });
    await organizeEpisode("p1", "s1", d);
    expect(organize).toHaveBeenCalledTimes(1);
    expect(d.toast).not.toHaveBeenCalled();
  });

  it("says when there is nothing long enough, and reports a refusal", async () => {
    const d = deps({ organize: vi.fn().mockResolvedValue(result({ short: 2 })) });
    await organizeEpisode("p1", "s1", d);
    expect(d.toast).toHaveBeenCalledWith("workspace.organize.nothing:12화");
    const failing = deps({ organize: vi.fn().mockRejectedValue("trial") });
    await organizeEpisode("p1", "s1", failing);
    expect(failing.toast).toHaveBeenCalledWith("error:trial");
  });
});
