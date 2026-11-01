import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ pull: vi.fn() }));

vi.mock("../lib/rpc", () => ({ vaultApi: { pull: rpc.pull } }));
vi.mock("../lib/i18n", () => ({ useI18n: () => ({ t: (key: string) => key }) }));

import type { SyncWork } from "../lib/types";
import { WorkSyncBanner } from "./WorkSyncBanner";

const props = () => ({
  projectId: "p1",
  flush: vi.fn().mockResolvedValue(undefined),
  onReopen: vi.fn(),
  onOpenDialog: vi.fn(),
});
const work = (state: SyncWork["state"]) => ({ project_id: "p1", title: "작품", state });

describe("WorkSyncBanner", () => {
  beforeEach(() => vi.clearAllMocks());

  it("shows nothing for a work in step", () => {
    const { container } = render(<WorkSyncBanner {...props()} work={work("synced")} />);
    expect(container).toBeEmptyDOMElement();
    const again = render(<WorkSyncBanner {...props()} work={undefined} />);
    expect(again.container).toBeEmptyDOMElement();
  });

  it("takes a newer version with one press, saving first", async () => {
    rpc.pull.mockResolvedValue({});
    const p = props();
    render(<WorkSyncBanner {...p} work={work("remote_changes")} />);
    await userEvent.click(screen.getByTestId("work-sync-take"));
    await waitFor(() => expect(p.onReopen).toHaveBeenCalled());
    expect(p.flush.mock.invocationCallOrder[0]).toBeLessThan(rpc.pull.mock.invocationCallOrder[0]);
    expect(rpc.pull).toHaveBeenCalledWith({ project_id: "p1" });
  });

  it("keeps the work open when the pull fails", async () => {
    rpc.pull.mockRejectedValue(new Error("boom"));
    const p = props();
    render(<WorkSyncBanner {...p} work={work("remote_changes")} />);
    await userEvent.click(screen.getByTestId("work-sync-take"));
    expect(await screen.findByTestId("work-sync-error")).toBeInTheDocument();
    expect(p.onReopen).not.toHaveBeenCalled();
  });

  it("sends a conflict to the dialog", async () => {
    const p = props();
    render(<WorkSyncBanner {...p} work={work("conflict")} />);
    await userEvent.click(screen.getByTestId("work-sync-choose"));
    expect(p.onOpenDialog).toHaveBeenCalled();
    expect(rpc.pull).not.toHaveBeenCalled();
  });
});
