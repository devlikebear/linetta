import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { invoke } from "@tauri-apps/api/core";
import { Write } from "./Write";
import type { Draft } from "../lib/types";

const draft = (over: Partial<Draft> = {}): Draft => ({
  id: 1,
  body: "처음 글",
  platform: "x",
  kind: "post",
  content_version: 1,
  published_at: 0,
  created_at: 1,
  updated_at: 1,
  ...over,
});

type Call = { method: string; params: Record<string, unknown> };
let calls: Call[] = [];
let handlers: Record<string, (params: Record<string, unknown>) => unknown> = {};

function mock(h: typeof handlers) {
  handlers = h;
  calls = [];
  vi.mocked(invoke).mockImplementation(async (_cmd, args) => {
    const { method, params } = args as { method: string; params: Record<string, unknown> };
    calls.push({ method, params });
    const fn = handlers[method];
    if (!fn) throw new Error(`unexpected ${method}`);
    return fn(params);
  });
}

const last = () => calls[calls.length - 1];
const count = (m: string) => calls.filter((c) => c.method === m).length;

async function renderWrite() {
  render(
    <MemoryRouter>
      <Write />
    </MemoryRouter>,
  );
  await screen.findByLabelText("초안 본문");
}

describe("Write", () => {
  beforeEach(() => {
    vi.mocked(invoke).mockReset();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("loads the newest draft and shows findings with the X counter", async () => {
    mock({
      "drafts.list": () => [draft({ body: "결론적으로 가" })],
      "check.run": () => ({
        context_used: "x/post",
        findings: [{ rule: "ai_phrase", severity: "strong", start: 0, end: 4, message: "AI 표현", evidence: "내 X 글 112편 중 0편" }],
      }),
    });
    await renderWrite();
    expect(await screen.findByText("AI 표현")).toBeInTheDocument();
    expect(screen.getByText("내 X 글 112편 중 0편")).toBeInTheDocument();
    expect(screen.getByText("기준: x/post")).toBeInTheDocument();
    // 결론적으로(5 Hangul) = 10, space = 1, 가 = 2
    expect(screen.getByTestId("x-counter")).toHaveTextContent("13/280");
    expect(screen.getByTestId("hl-mirror").querySelector("[data-severity=strong]")).toHaveTextContent("결론적으");
  });

  it("marks the counter over 280 and hides it outside X", async () => {
    mock({
      "drafts.list": () => [draft({ body: "가".repeat(141) })],
      "check.run": () => ({ findings: [], context_used: "x/post" }),
    });
    await renderWrite();
    const counter = screen.getByTestId("x-counter");
    expect(counter).toHaveTextContent("282/280");
    expect(counter).toHaveClass("over");

    fireEvent.change(screen.getByLabelText("맥락"), { target: { value: "blog/post" } });
    expect(screen.queryByTestId("x-counter")).not.toBeInTheDocument();
  });

  it("checks 300ms and saves 1s after the last input", async () => {
    mock({
      "drafts.list": () => [draft()],
      "check.run": () => ({ findings: [], context_used: "x/post" }),
      "drafts.update": (p) => draft({ content_version: 2, body: p.body as string }),
    });
    await renderWrite();
    await act(async () => {}); // let the initial check settle
    vi.useFakeTimers();
    const checks = count("check.run");

    fireEvent.change(screen.getByLabelText("초안 본문"), { target: { value: "고친 글" } });
    fireEvent.change(screen.getByLabelText("초안 본문"), { target: { value: "고친 글!" } });
    await act(async () => { await vi.advanceTimersByTimeAsync(299); });
    expect(count("check.run")).toBe(checks);
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(count("check.run")).toBe(checks + 1);
    expect(last()?.params).toMatchObject({ body: "고친 글!", platform: "x", kind: "post" });

    expect(count("drafts.update")).toBe(0);
    await act(async () => { await vi.advanceTimersByTimeAsync(700); });
    expect(count("drafts.update")).toBe(1);
    expect(last()?.params).toEqual({ id: 1, content_version: 1, body: "고친 글!", platform: "x", kind: "post" });

    // The next save sends the version the server returned.
    fireEvent.change(screen.getByLabelText("초안 본문"), { target: { value: "또 고침" } });
    await act(async () => { await vi.advanceTimersByTimeAsync(1000); });
    expect(last()?.params).toMatchObject({ content_version: 2, body: "또 고침" });
  });

  it("re-fetches and toasts on a content conflict", async () => {
    mock({
      "drafts.list": () => [draft()],
      "check.run": () => ({ findings: [], context_used: "x/post" }),
      "drafts.update": () => {
        throw { code: -32009, message: "draft content_version conflict" };
      },
      "drafts.get": () => draft({ body: "다른 곳에서 고침", content_version: 5 }),
    });
    await renderWrite();
    await act(async () => {});
    vi.useFakeTimers();

    fireEvent.change(screen.getByLabelText("초안 본문"), { target: { value: "내가 고침" } });
    await act(async () => { await vi.advanceTimersByTimeAsync(1000); });

    expect(count("drafts.get")).toBe(1);
    expect(screen.getByLabelText("초안 본문")).toHaveValue("다른 곳에서 고침");
    expect(screen.getByRole("status")).toHaveTextContent("최신 내용으로 바꿨어요");

    mock({
      "check.run": () => ({ findings: [], context_used: "x/post" }),
      "drafts.update": (p) => draft({ content_version: 6, body: p.body as string }),
    });
    fireEvent.change(screen.getByLabelText("초안 본문"), { target: { value: "이어 씀" } });
    await act(async () => { await vi.advanceTimersByTimeAsync(1000); });
    expect(calls.find((c) => c.method === "drafts.update")?.params).toMatchObject({ content_version: 5 });
  });

  it("creates a draft with the selected context", async () => {
    mock({
      "drafts.list": () => [],
      "drafts.create": (p) => draft({ id: 2, body: "", platform: p.platform as "x", kind: p.kind as "post" }),
      "check.run": () => ({ findings: [], context_used: "x/post" }),
    });
    render(
      <MemoryRouter>
        <Write />
      </MemoryRouter>,
    );
    expect(await screen.findByText(/아직 초안이 없어요/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "새 초안" }));
    await screen.findByLabelText("초안 본문");
    expect(calls.find((c) => c.method === "drafts.create")?.params).toEqual({ platform: "x", kind: "post", body: "" });
  });

  it("deletes after confirmation only", async () => {
    mock({
      "drafts.list": () => [draft()],
      "check.run": () => ({ findings: [], context_used: "x/post" }),
      "drafts.delete": () => ({}),
    });
    const confirm = vi.spyOn(window, "confirm");
    await renderWrite();

    confirm.mockReturnValueOnce(false);
    fireEvent.click(screen.getByRole("button", { name: /^삭제/ }));
    expect(count("drafts.delete")).toBe(0);

    confirm.mockReturnValueOnce(true);
    fireEvent.click(screen.getByRole("button", { name: /^삭제/ }));
    expect(await screen.findByText(/아직 초안이 없어요/)).toBeInTheDocument();
    expect(count("drafts.delete")).toBe(1);
    confirm.mockRestore();
  });
});
