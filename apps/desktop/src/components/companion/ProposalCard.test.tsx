import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../lib/i18n";
import { ProposalCard } from "./ProposalCard";

const mocks = vi.hoisted(() => ({
  applyOps: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  settings: {
    get: vi.fn().mockResolvedValue({ language: "ko" }),
  },
  companion: {
    applyOps: mocks.applyOps,
  },
}));

describe("ProposalCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.applyOps.mockResolvedValue({ applied: 1, failures: [] });
  });

  it("labels scene text replacement proposals without exposing the raw op", () => {
    render(
      <I18nProvider>
        <ProposalCard
          proposal={{
            run_id: "r1",
            valid: true,
            summary: "퇴고 제안",
            ops: [{ op: "set_scene_text", text: "새 본문" }],
          }}
          projectId="project-1"
          nodeIdRef={{ current: "node-1" }}
          onApplied={vi.fn()}
        />
      </I18nProvider>,
    );

    expect(screen.getByText("현재 씬 본문 교체")).toBeInTheDocument();
    expect(screen.queryByText("set_scene_text")).not.toBeInTheDocument();
  });

  it("does not report applied when the engine applies zero proposal ops", async () => {
    const user = userEvent.setup();
    const onApplied = vi.fn();
    mocks.applyOps.mockResolvedValue({
      applied: 0,
      failures: [{ index: 0, op: "set_scene_text", error: "본문 변경 실패" }],
    });

    render(
      <I18nProvider>
        <ProposalCard
          proposal={{
            run_id: "r1",
            valid: true,
            summary: "퇴고 제안",
            ops: [{ op: "set_scene_text", text: "새 본문" }],
          }}
          projectId="project-1"
          nodeIdRef={{ current: "node-1" }}
          onApplied={onApplied}
        />
      </I18nProvider>,
    );

    await user.click(screen.getByRole("button", { name: "적용" }));

    await waitFor(() => expect(screen.getByText(/실패 1건/)).toBeInTheDocument());
    expect(onApplied).not.toHaveBeenCalled();
  });
});
