import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const events = vi.hoisted(() => ({ listeners: new Map<string, Array<(event: { payload: unknown }) => void>>() }));
vi.mock("@tauri-apps/api/event", () => ({
  listen: (name: string, listener: (event: { payload: unknown }) => void) => {
    events.listeners.set(name, [...(events.listeners.get(name) ?? []), listener]);
    return Promise.resolve(() => undefined);
  },
}));

const rpc = vi.hoisted(() => ({
  get: vi.fn(),
  approve: vi.fn(),
  reject: vi.fn(),
  providers: vi.fn(),
  node: vi.fn(),
  image: vi.fn(),
  list: vi.fn(),
  suggestPrior: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  illustrationRequests: { get: rpc.get, approve: rpc.approve, reject: rpc.reject, list: vi.fn() },
  imageProviders: { list: rpc.providers },
  nodes: { get: rpc.node },
  illustrations: { get: rpc.image, list: rpc.list, suggestPrior: rpc.suggestPrior },
}));

vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    language: "ko",
    t: (key: string, vars?: Record<string, string | number>) =>
      vars
        ? `${key}:${Object.entries(vars)
            .map(([name, value]) => `${name}=${value}`)
            .join(",")}`
        : key,
  }),
}));

import { IllustrationRequestCard } from "./IllustrationRequestCard";

const request = (extra: Record<string, unknown> = {}) => ({
  id: "r1",
  project_id: "p",
  target_node_id: "s1",
  purpose: "insert",
  prompt: "비 오는 골목",
  character_ids: ["c1"],
  include_references: true,
  aspect: "portrait",
  caption: "우산",
  note: "첫 장면을 그려 봤어요",
  source: "agent",
  status: "pending",
  created_at: 1,
  ...extra,
});

const ready = [{ id: "gemini-native", active: true, configured: true, consented: true, model: "m" }];

describe("IllustrationRequestCard", () => {
  URL.createObjectURL = vi.fn(() => "blob:img");
  URL.revokeObjectURL = vi.fn();
  beforeEach(() => {
    Object.values(rpc).forEach((f) => f.mockReset());
    rpc.suggestPrior.mockResolvedValue([]);
    events.listeners.clear();
    rpc.get.mockResolvedValue(request());
    rpc.providers.mockResolvedValue(ready);
    rpc.node.mockResolvedValue({ id: "s1", label: "씬 1", title: "늦은 병문안" });
    rpc.image.mockResolvedValue({ id: "pic", mime: "image/jpeg", data_base64: btoa("jpg") });
  });

  it("shows what the agent asked for and draws only with the writer's edits", async () => {
    rpc.approve.mockResolvedValue(request({ status: "done", illustration_id: "pic", placed_at: 5 }));
    render(<IllustrationRequestCard requestId="r1" />);
    expect(await screen.findByText("첫 장면을 그려 봤어요")).toBeInTheDocument();
    expect(await screen.findByText(/씬 1 · 늦은 병문안/)).toBeInTheDocument();
    expect(rpc.approve).not.toHaveBeenCalled();

    fireEvent.change(screen.getByTestId("illustration-request-prompt"), { target: { value: "고친 프롬프트" } });
    fireEvent.click(screen.getByText("illustration.aspect.landscape"));
    await waitFor(() => expect(screen.getByTestId("illustration-request-approve")).not.toBeDisabled());
    fireEvent.click(screen.getByTestId("illustration-request-approve"));
    await waitFor(() =>
      expect(rpc.approve).toHaveBeenCalledWith("r1", {
        prompt: "고친 프롬프트",
        aspect: "landscape",
        caption: "우산",
        include_references: true,
        prior_illustration_ids: [],
      }),
    );
    expect(await screen.findByTestId("illustration-request-done")).toHaveTextContent("illustrationRequest.placed.insert");
  });

  it("shows the earlier pictures the agent chose, selected, beside the suggestion", async () => {
    rpc.get.mockResolvedValue(request({ prior_illustration_ids: ["chosen"] }));
    rpc.suggestPrior.mockResolvedValue([{ id: "suggested", caption: "" }]);
    rpc.list.mockResolvedValue({ items: [{ id: "chosen", caption: "1화" }, { id: "other", caption: "" }], count: 2, bytes: 0 });
    rpc.approve.mockResolvedValue(request({ status: "done", illustration_id: "pic", placed_at: 5 }));
    render(<IllustrationRequestCard requestId="r1" />);

    await waitFor(() => expect(screen.getByTestId("illustration-prior-chosen")).toHaveAttribute("aria-pressed", "true"));
    expect(screen.getByTestId("illustration-prior-suggested")).toHaveAttribute("aria-pressed", "false");
    expect(screen.queryByTestId("illustration-prior-other")).toBeNull();
    fireEvent.click(screen.getByTestId("illustration-prior-suggested"));
    await waitFor(() => expect(screen.getByTestId("illustration-request-approve")).not.toBeDisabled());
    fireEvent.click(screen.getByTestId("illustration-request-approve"));
    await waitFor(() =>
      expect(rpc.approve).toHaveBeenCalledWith("r1", expect.objectContaining({ prior_illustration_ids: ["chosen", "suggested"] })),
    );
  });

  it("sends the writer to image settings instead of drawing when no provider is ready", async () => {
    rpc.providers.mockResolvedValue([{ id: "gemini-native", active: true, configured: true, consented: false }]);
    const openSettings = vi.fn();
    render(<IllustrationRequestCard requestId="r1" onOpenSettings={openSettings} />);
    fireEvent.click(await screen.findByText("illustration.openSettings"));
    expect(openSettings).toHaveBeenCalled();
    expect(screen.queryByTestId("illustration-request-approve")).toBeNull();
  });

  it("rejects without drawing", async () => {
    rpc.reject.mockResolvedValue(request({ status: "rejected" }));
    render(<IllustrationRequestCard requestId="r1" />);
    fireEvent.click(await screen.findByTestId("illustration-request-reject"));
    await waitFor(() => expect(rpc.reject).toHaveBeenCalledWith("r1"));
    expect(await screen.findByTestId("illustration-request-status")).toHaveTextContent("illustrationRequest.status.rejected");
    expect(rpc.approve).not.toHaveBeenCalled();
  });

  it("follows a decision made elsewhere through illustrations-changed", async () => {
    render(<IllustrationRequestCard requestId="r1" />);
    await screen.findByTestId("illustration-request-approve");
    rpc.get.mockResolvedValue(request({ status: "failed", error: "provider refused" }));
    await act(async () => {
      for (const listener of events.listeners.get("illustrations-changed") ?? []) {
        listener({ payload: { project_id: "p", request_id: "other", status: "done" } });
      }
    });
    expect(rpc.get).toHaveBeenCalledTimes(1);
    await act(async () => {
      for (const listener of events.listeners.get("illustrations-changed") ?? []) {
        listener({ payload: { project_id: "p", request_id: "r1", status: "failed" } });
      }
    });
    expect(await screen.findByRole("alert")).toHaveTextContent("provider refused");
    expect(screen.getByTestId("illustration-request-approve")).toHaveTextContent("illustration.again");
  });

  it("says a drawing that could not be placed was still kept", async () => {
    rpc.get.mockResolvedValue(request({ status: "done", illustration_id: "pic", error: "drawn but not placed: conflict" }));
    render(<IllustrationRequestCard requestId="r1" />);
    expect(await screen.findByTestId("illustration-request-done")).toHaveTextContent("illustrationRequest.notPlaced");
  });
});
