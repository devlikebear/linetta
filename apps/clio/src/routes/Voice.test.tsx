import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { invoke } from "@tauri-apps/api/core";
import { Voice } from "./Voice";
import { FEATURE_NAMES } from "../lib/types";

const stat = (v: number) => ({ mean: v, p10: v, p50: v, p90: v });
const features = Object.fromEntries(FEATURE_NAMES.map((n) => [n, stat(0.5)]));

function profile(over: Record<string, unknown>) {
  return {
    key: "x/post",
    fallback: "x/post",
    n: 112,
    features: { ...features, length: { mean: 60, p10: 20, p50: 55, p90: 120 } },
    endings: {},
    top: [{ ending: "다", share: 0.4 }, { ending: "요", share: 0.2 }],
    polite_share: 0.25,
    mixed_share: 0.1,
    examples: ["p1", "p2"],
    ...over,
  };
}

function mockEngine(byKind: Record<string, unknown>) {
  vi.mocked(invoke).mockImplementation(async (_cmd, args) => {
    const { method, params } = args as { method: string; params: { kind?: string } };
    if (method === "voice.profile") return byKind[params.kind ?? "*"];
    if (method === "posts.list") return [{ id: "p1", body: "예시 본문입니다", platform: "x", kind: "post" }];
    throw new Error(`unexpected ${method}`);
  });
}

describe("Voice", () => {
  beforeEach(() => { vi.mocked(invoke).mockReset(); });

  it("renders the profile", async () => {
    mockEngine({ post: profile({}) });
    render(<MemoryRouter><Voice /></MemoryRouter>);

    expect(await screen.findByText("25%")).toBeInTheDocument();
    expect(screen.getByText("표본 112편")).toBeInTheDocument();
    const row = screen.getByText("길이").closest("tr") as HTMLElement;
    expect(within(row).getByText("20")).toBeInTheDocument();
    expect(within(row).getByText("55")).toBeInTheDocument();
    expect(within(row).getByText("120")).toBeInTheDocument();
    expect(screen.getByText("40%")).toBeInTheDocument();
    expect(screen.getByText("예시 본문입니다")).toBeInTheDocument();
    expect(screen.getByText(/본문을 불러오지 못한 글 \(p2\)/)).toBeInTheDocument();
    expect(screen.queryByText(/표본이 적어/)).not.toBeInTheDocument();
  });

  it("shows the fallback notice", async () => {
    mockEngine({ post: profile({}), reply: profile({ key: "x/reply", fallback: "x/*", n: 150 }) });
    render(<MemoryRouter><Voice /></MemoryRouter>);
    await screen.findByText("표본 112편");
    await userEvent.setup().click(screen.getByRole("tab", { name: "X 답글" }));
    expect(await screen.findByText(/표본이 적어 X 전체 기준으로 보여 줌/)).toBeInTheDocument();
  });

  it("shows an empty state linking to the import screen", async () => {
    mockEngine({ post: profile({ n: 0, examples: [], top: [], features: {} }) });
    render(<MemoryRouter><Voice /></MemoryRouter>);
    expect(await screen.findByText("아직 가져온 글이 없어요.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "가져오기로 가기" })).toHaveAttribute("href", "/import");
  });
});
