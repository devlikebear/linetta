import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  connectStatus: vi.fn(),
  workStatus: vi.fn(),
  upsertWork: vi.fn(),
  publishEpisode: vi.fn(),
  setEpisodeVisibility: vi.fn(),
  openExternalUrl: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  publishApi: {
    connectStatus: rpc.connectStatus,
    workStatus: rpc.workStatus,
    upsertWork: rpc.upsertWork,
    publishEpisode: rpc.publishEpisode,
    setEpisodeVisibility: rpc.setEpisodeVisibility,
  },
  openExternalUrl: rpc.openExternalUrl,
}));

vi.mock("../lib/i18n", () => ({
  useI18n: () => ({
    language: "ko",
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
}));

import { PublishDialog } from "./PublishDialog";

const SITE = "https://works.marvin-42.com";
const CONNECTED = { state: "connected", site_url: SITE, pen_name: "마곰", handle: "devlikebear" };
const URL3 = `${SITE}/@devlikebear/k-office-god/3`;

function episode(overrides: Record<string, unknown> = {}) {
  return {
    node_id: "ep-3",
    label: "3화",
    title: "첫 출근",
    ordinal: 3,
    leaf_ids: ["scene-a", "scene-b"],
    char_count: 5120,
    state: "unpublished",
    ...overrides,
  };
}

function status(overrides: Record<string, unknown> = {}) {
  return {
    project_id: "p1",
    connected: true,
    registered: true,
    work: { url: `${SITE}/@devlikebear/k-office-god`, slug: "k-office-god", blurb: "소개글", serial_status: "ongoing" },
    episodes: [episode()],
    site_only: [],
    ...overrides,
  };
}

function renderDialog(nodeId = "scene-a") {
  const onOpenSettings = vi.fn();
  const onChanged = vi.fn();
  render(
    <PublishDialog
      open
      projectId="p1"
      nodeId={nodeId}
      onClose={() => {}}
      onOpenSettings={onOpenSettings}
      onChanged={onChanged}
    />,
  );
  return { onOpenSettings, onChanged };
}

describe("PublishDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.connectStatus.mockResolvedValue(CONNECTED);
    rpc.workStatus.mockResolvedValue(status());
    rpc.openExternalUrl.mockResolvedValue(undefined);
  });

  it("not connected: points to Settings and asks the site nothing", async () => {
    rpc.connectStatus.mockResolvedValue({ state: "disconnected", site_url: SITE });
    const { onOpenSettings } = renderDialog();
    expect(await screen.findByTestId("publish-not-connected")).toHaveTextContent("publish.notConnected");
    expect(rpc.workStatus).not.toHaveBeenCalled();
    await userEvent.click(screen.getByTestId("publish-open-settings"));
    expect(onOpenSettings).toHaveBeenCalled();
  });

  it("first publish of a work: needs a blurb, registers the work, then shows the address", async () => {
    rpc.workStatus.mockResolvedValueOnce(status({ registered: false, work: undefined }));
    rpc.upsertWork.mockResolvedValue({ url: "", slug: "k-office-god", blurb: "새 소개", serial_status: "completed" });
    rpc.publishEpisode.mockResolvedValue({ created: true, episode: episode({ state: "public", number: 3, url: URL3 }) });
    rpc.workStatus.mockResolvedValue(status({ episodes: [episode({ state: "public", number: 3, url: URL3 })] }));
    const { onChanged } = renderDialog();

    expect(await screen.findByTestId("publish-work-new")).toBeInTheDocument();
    // The scene resolves to its episode by the engine's own list.
    expect(screen.getByTestId("publish-episode")).toHaveTextContent("3화 · 첫 출근");
    const submit = screen.getByTestId("publish-submit");
    expect(submit).toBeDisabled();

    await userEvent.type(screen.getByTestId("publish-blurb"), "새 소개");
    await userEvent.selectOptions(screen.getByTestId("publish-serial"), "completed");
    await userEvent.type(screen.getByTestId("publish-slug"), "K-Office-God");
    await userEvent.click(submit);

    await waitFor(() => expect(rpc.publishEpisode).toHaveBeenCalledWith("scene-a"));
    expect(rpc.upsertWork).toHaveBeenCalledWith({
      project_id: "p1",
      blurb: "새 소개",
      serial_status: "completed",
      slug: "k-office-god",
    });
    expect(rpc.upsertWork.mock.invocationCallOrder[0]).toBeLessThan(rpc.publishEpisode.mock.invocationCallOrder[0]);
    expect(await screen.findByTestId("publish-url")).toHaveTextContent(URL3);
    expect(screen.getByTestId("publish-state")).toHaveTextContent("publish.state.public");
    expect(onChanged).toHaveBeenCalled();

    await userEvent.click(screen.getByTestId("publish-open"));
    expect(rpc.openExternalUrl).toHaveBeenCalledWith(URL3);
  });

  it("republish: an edited episode reads as modified, keeps its address, and does not re-register", async () => {
    rpc.workStatus.mockResolvedValue(status({ episodes: [episode({ state: "modified", modified: true, number: 3, url: URL3 })] }));
    rpc.publishEpisode.mockResolvedValue({ created: false, episode: episode({ state: "public", number: 3, url: URL3 }) });
    renderDialog("ep-3");

    expect(await screen.findByTestId("publish-state")).toHaveTextContent("publish.state.modified");
    expect(screen.getByTestId("publish-work-edit")).toBeInTheDocument();
    const submit = screen.getByTestId("publish-submit");
    expect(submit).toHaveTextContent("publish.republish");
    await userEvent.click(submit);
    await waitFor(() => expect(screen.getByTestId("publish-state")).toHaveTextContent("publish.state.public"));
    expect(rpc.upsertWork).not.toHaveBeenCalled();
    expect(screen.getByTestId("publish-url")).toHaveTextContent(URL3);
  });

  it("error: keeps everything, explains in the reader's language, and offers a retry", async () => {
    rpc.publishEpisode.mockRejectedValueOnce({ data: { reason: "publish_unreachable" } });
    renderDialog();
    await userEvent.click(await screen.findByTestId("publish-submit"));
    expect(await screen.findByTestId("publish-error")).toHaveTextContent("errors.publishUnreachable");
    expect(screen.getByTestId("publish-submit")).toHaveTextContent("publish.retry");

    rpc.publishEpisode.mockResolvedValue({ created: true, episode: episode({ state: "public", number: 1, url: URL3 }) });
    await userEvent.click(screen.getByTestId("publish-submit"));
    await waitFor(() => expect(screen.queryByTestId("publish-error")).not.toBeInTheDocument());
    expect(rpc.publishEpisode).toHaveBeenCalledTimes(2);
  });

  it("a revoked token turns the dialog into a reconnect prompt", async () => {
    rpc.publishEpisode.mockRejectedValue({ data: { reason: "publish_unauthorized" } });
    renderDialog();
    await userEvent.click(await screen.findByTestId("publish-submit"));
    rpc.connectStatus.mockResolvedValue({ state: "failed", site_url: SITE, failure_reason: "revoked" });
    expect(await screen.findByTestId("publish-error")).toHaveTextContent("errors.publishUnauthorized");
    expect(screen.getByTestId("publish-open-settings")).toBeInTheDocument();
  });

  it("rejected: shows the reason summary and how to ask for another review", async () => {
    rpc.workStatus.mockResolvedValue(
      status({ episodes: [episode({ state: "rejected", number: 3, url: URL3, moderation_reason: "과도한 폭력 묘사" })] }),
    );
    renderDialog();
    const review = await screen.findByTestId("publish-review");
    expect(review).toHaveTextContent("publish.review.rejected");
    expect(within(review).getByTestId("publish-review-reason")).toHaveTextContent("과도한 폭력 묘사");
    expect(review).toHaveTextContent("publish.review.appeal");
  });

  it("pending and needs_review read as in review", async () => {
    for (const state of ["pending", "needs_review"]) {
      rpc.workStatus.mockResolvedValue(status({ episodes: [episode({ state, number: 3, url: URL3 })] }));
      const view = render(
        <PublishDialog open projectId="p1" nodeId="ep-3" onClose={() => {}} onOpenSettings={() => {}} />,
      );
      expect(await view.findByTestId("publish-review")).toHaveTextContent(
        state === "pending" ? "publish.review.pending" : "publish.review.needsReview",
      );
      view.unmount();
    }
  });

  it("makes a published episode private, and lists site-only episodes with the same choice", async () => {
    rpc.workStatus.mockResolvedValue(
      status({
        episodes: [episode({ state: "public", number: 3, url: URL3 })],
        site_only: [{ node_id: "gone", title: "지운 회차", char_count: 10, state: "site_only", number: 2, remote_status: "public" }],
      }),
    );
    rpc.setEpisodeVisibility.mockResolvedValue({ ...episode({ state: "private", number: 3, url: URL3 }) });
    renderDialog();
    expect(await screen.findByTestId("publish-site-only")).toHaveTextContent("지운 회차");

    await userEvent.click(screen.getByTestId("publish-make-private"));
    await waitFor(() => expect(rpc.setEpisodeVisibility).toHaveBeenCalledWith("p1", "ep-3", "private"));
    await userEvent.click(screen.getByTestId("publish-site-only-private-gone"));
    await waitFor(() => expect(rpc.setEpisodeVisibility).toHaveBeenCalledWith("p1", "gone", "private"));
  });

  it("offline: says the status is the last one seen", async () => {
    rpc.workStatus.mockResolvedValue(status({ offline: true }));
    renderDialog();
    expect(await screen.findByTestId("publish-offline")).toHaveTextContent("publish.offline");
  });

  it("a node outside any one episode cannot be published", async () => {
    renderDialog("arc-1");
    expect(await screen.findByTestId("publish-no-episode")).toBeInTheDocument();
    expect(screen.getByTestId("publish-submit")).toBeDisabled();
  });
});
