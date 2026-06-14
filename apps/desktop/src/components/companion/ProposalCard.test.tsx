import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../lib/i18n";
import { ProposalCard } from "./ProposalCard";

vi.mock("../../lib/rpc", () => ({
  settings: {
    get: vi.fn().mockResolvedValue({ language: "ko" }),
  },
  companion: {
    applyOps: vi.fn(),
  },
}));

describe("ProposalCard", () => {
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
});
