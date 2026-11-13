import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { PublishApprovalRequest } from "../lib/types";

const rpc = vi.hoisted(() => ({ pending: vi.fn(), answer: vi.fn(), image: vi.fn() }));
const events = vi.hoisted(() => ({ handlers: new Map<string, (payload: unknown) => void>() }));

vi.mock("../lib/rpc", () => ({
  publishApi: { approvalsPending: rpc.pending, answerApproval: rpc.answer },
  illustrations: { get: rpc.image },
}));
vi.mock("../hooks/useEngineEvent", () => ({
  useEngineEvent: (name: string, handler: (payload: unknown) => void) => {
    events.handlers.set(name, handler);
  },
}));
vi.mock("../lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string | number>) => (vars ? `${key}:${Object.values(vars).join(",")}` : key),
  }),
}));
vi.mock("../lib/rpcMessage", () => ({
  rpcErrorMessage: (error: { reason?: string }) => `error:${error.reason ?? "unknown"}`,
}));

import { PublishApprovalDialog } from "./PublishApprovalDialog";

function request(over: Partial<PublishApprovalRequest> = {}): PublishApprovalRequest {
  return {
    id: "a1",
    kind: "publish_episode",
    project_id: "p1",
    work_title: "검은 탑",
    node_id: "n1",
    episode_title: "3화 문",
    source: "external",
    client: "claude-code",
    expires_at: Date.now() + 120_000,
    ...over,
  };
}

function announce(pending: PublishApprovalRequest[]) {
  act(() => events.handlers.get("publish-approvals-changed")?.({ pending }));
}

describe("PublishApprovalDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    events.handlers.clear();
    rpc.pending.mockResolvedValue({ pending: [] });
    rpc.answer.mockResolvedValue({ ok: true });
  });

  it("shows nothing while no agent is waiting", async () => {
    const { container } = render(<PublishApprovalDialog />);
    await waitFor(() => expect(rpc.pending).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });

  it("shows a request that was already waiting when the window opened", async () => {
    rpc.pending.mockResolvedValue({ pending: [request()] });
    render(<PublishApprovalDialog />);
    const dialog = await screen.findByTestId("publish-approval");
    expect(dialog).toHaveTextContent("publishApproval.titleEpisode");
    expect(dialog).toHaveTextContent("publishApproval.work:검은 탑");
    expect(dialog).toHaveTextContent("publishApproval.episode:3화 문");
    expect(dialog).toHaveTextContent("publishApproval.fromClient:claude-code");
  });

  it("publishes only when the writer presses upload", async () => {
    render(<PublishApprovalDialog />);
    announce([request()]);
    fireEvent.click(screen.getByRole("button", { name: "publishApproval.approve" }));
    await waitFor(() => expect(rpc.answer).toHaveBeenCalledWith("a1", true));
    await waitFor(() => expect(screen.queryByTestId("publish-approval")).not.toBeInTheDocument());
  });

  it("starts on decline, and Escape declines", async () => {
    render(<PublishApprovalDialog />);
    announce([request()]);
    expect(screen.getByRole("button", { name: "publishApproval.decline" })).toHaveFocus();
    fireEvent.keyDown(screen.getByTestId("publish-approval"), { key: "Escape" });
    await waitFor(() => expect(rpc.answer).toHaveBeenCalledWith("a1", false));
  });

  it("warns before replacing a published episode's text", () => {
    render(<PublishApprovalDialog />);
    announce([request({ republish: true })]);
    const dialog = screen.getByTestId("publish-approval");
    expect(dialog).toHaveTextContent("publishApproval.titleRepublish");
    expect(dialog).toHaveTextContent("publishApproval.republish");
  });

  it("shows the blurb a registration would send and who asked", () => {
    render(<PublishApprovalDialog />);
    announce([request({ kind: "register_work", node_id: undefined, episode_title: undefined, blurb: "탑에 갇힌 검사", serial_status: "ongoing", source: "agent", client: undefined })]);
    const dialog = screen.getByTestId("publish-approval");
    expect(dialog).toHaveTextContent("publishApproval.titleRegister");
    expect(dialog).toHaveTextContent("탑에 갇힌 검사");
    expect(dialog).toHaveTextContent("publishApproval.fromAgent");
  });

  it("closes when the engine says the request ended, and moves to the next one", () => {
    render(<PublishApprovalDialog />);
    announce([request(), request({ id: "a2", episode_title: "4화 바람" })]);
    expect(screen.getByTestId("publish-approval")).toHaveTextContent("publishApproval.more:1");
    announce([request({ id: "a2", episode_title: "4화 바람" })]);
    expect(screen.getByTestId("publish-approval")).toHaveTextContent("4화 바람");
    announce([]);
    expect(screen.queryByTestId("publish-approval")).not.toBeInTheDocument();
  });

  it("says so when the request ended before the answer arrived", async () => {
    rpc.answer.mockRejectedValue({ reason: "publish_approval_gone" });
    render(<PublishApprovalDialog />);
    announce([request(), request({ id: "a2" })]);
    rpc.pending.mockResolvedValue({ pending: [request({ id: "a2" })] });
    fireEvent.click(screen.getByRole("button", { name: "publishApproval.approve" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("error:publish_approval_gone");
  });
});
