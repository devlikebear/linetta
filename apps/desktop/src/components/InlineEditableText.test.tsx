import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { InlineEditableText } from "./InlineEditableText";

describe("InlineEditableText", () => {
  it("commits a trimmed value on blur", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn().mockResolvedValue(undefined);
    render(
      <InlineEditableText
        value="씬 1"
        ariaLabel="씬 이름"
        className="scene-title-input"
        onCommit={onCommit}
      />,
    );

    const input = screen.getByLabelText("씬 이름");
    await user.clear(input);
    await user.type(input, " 첫 만남 ");
    await user.tab();

    await waitFor(() => expect(onCommit).toHaveBeenCalledWith("첫 만남"));
  });

  it("resets empty edits instead of committing them", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(
      <InlineEditableText
        value="작품 제목"
        ariaLabel="소설 제목"
        className="project-title-input"
        onCommit={onCommit}
      />,
    );

    const input = screen.getByLabelText("소설 제목");
    await user.clear(input);
    await user.tab();

    expect(onCommit).not.toHaveBeenCalled();
    expect(input).toHaveValue("작품 제목");
  });

  it("cancels edits with Escape without committing on blur", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const onCancel = vi.fn();
    render(
      <InlineEditableText
        value="이전 제목"
        ariaLabel="표시 제목"
        autoFocus
        onCommit={onCommit}
        onCancel={onCancel}
      />,
    );

    const input = screen.getByLabelText("표시 제목");
    await user.clear(input);
    await user.type(input, "새 제목");
    await user.keyboard("{Escape}");

    expect(onCancel).toHaveBeenCalledOnce();
    expect(onCommit).not.toHaveBeenCalled();
    expect(input).toHaveValue("이전 제목");
  });
});
