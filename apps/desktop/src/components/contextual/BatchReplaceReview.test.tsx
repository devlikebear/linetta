import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../lib/i18n";
import type { ReplacePlan } from "../../lib/types";
import { BatchReplaceReview } from "./BatchReplaceReview";

const plan: ReplacePlan = {
  project_id: "project-1",
  query: "민호",
  replacement: "민준",
  candidates: [
    {
      id: "scene-1:1",
      node_id: "scene-1",
      breadcrumb: "1부 / 1장 / 씬 1",
      before: "민호는 고지서를 보았다.",
      after: "민준는 고지서를 보았다.",
      occurrences: 1,
      selected: true,
      preview_version: 1,
    },
    {
      id: "scene-2:1",
      node_id: "scene-2",
      breadcrumb: "1부 / 1장 / 씬 2",
      before: "민호는 현관에 섰다.",
      after: "민준는 현관에 섰다.",
      occurrences: 1,
      selected: true,
      preview_version: 1,
    },
  ],
};

describe("BatchReplaceReview", () => {
  it("applies only the selected candidates", async () => {
    const user = userEvent.setup();
    const onApply = vi.fn();
    render(
      <I18nProvider>
        <BatchReplaceReview plan={plan} onApply={onApply} />
      </I18nProvider>,
    );

    await user.click(screen.getByRole("button", { name: /1부 \/ 1장 \/ 씬 2/ }));
    await user.click(screen.getByRole("button", { name: "선택 적용" }));

    expect(onApply).toHaveBeenCalledWith(["scene-1:1"]);
  });
});
