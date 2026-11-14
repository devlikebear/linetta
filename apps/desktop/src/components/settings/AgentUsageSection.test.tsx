import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ usage: vi.fn() }));

vi.mock("../../lib/rpc", () => ({ agent: { usage: rpc.usage } }));
vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    language: "en",
    t: (key: string, vars?: Record<string, string | number>) =>
      vars ? `${key}:${Object.entries(vars).map(([k, v]) => `${k}=${v}`).join(",")}` : key,
  }),
}));

import { AgentUsageSection, cacheHitRate } from "./AgentUsageSection";

const row = (over: Partial<Parameters<typeof cacheHitRate>[0][number]>) => ({
  kind: "turn", provider: "anthropic", model: "claude-opus-5-5",
  calls: 0, input: 0, output: 0, cached: 0, cache_read: 0, cache_write: 0, ...over,
});

describe("AgentUsageSection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("asks for the last 30 days and says when nothing is recorded", async () => {
    rpc.usage.mockResolvedValue({ since: 0, rows: [] });
    render(<AgentUsageSection />);
    expect(await screen.findByTestId("agent-usage-empty")).toBeInTheDocument();
    expect(rpc.usage).toHaveBeenCalledWith(30);
  });

  it("shows each kind on its model and the cache hit rate over all of them", async () => {
    rpc.usage.mockResolvedValue({
      since: 0,
      rows: [
        row({ calls: 12, input: 100, output: 900, cache_read: 8000, cache_write: 1900 }),
        row({ kind: "self_review", model: "claude-haiku-5-5", calls: 3, input: 0, output: 50 }),
      ],
    });
    render(<AgentUsageSection />);
    expect(await screen.findByTestId("agent-usage-total")).toHaveTextContent(
      "settings.agentUsage.total:calls=15,rate=80%",
    );
    const rows = screen.getAllByTestId("agent-usage-row");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("settings.agentUsage.kind.turn · claude-opus-5-5");
    expect(rows[0]).toHaveTextContent("cacheRead=8K");
    expect(rows[1]).toHaveTextContent("settings.agentUsage.kind.self_review · claude-haiku-5-5");
  });
});

describe("cacheHitRate", () => {
  it("counts Anthropic reads on top of input and OpenAI's cached inside it", () => {
    expect(cacheHitRate([row({ input: 100, cache_read: 900 })])).toBeCloseTo(0.9);
    expect(cacheHitRate([row({ input: 1000, cached: 600 })])).toBeCloseTo(0.6);
    expect(cacheHitRate([])).toBeNull();
  });
});
