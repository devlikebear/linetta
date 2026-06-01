import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CommandPalette, type Command } from "./CommandPalette";

function renderPalette(commands: Command[], onClose = vi.fn()) {
  render(<CommandPalette open onClose={onClose} commands={commands} />);
  return { onClose };
}

describe("CommandPalette", () => {
  it("filters commands and runs the selected command", async () => {
    const user = userEvent.setup();
    const run = vi.fn();
    const other = vi.fn();
    const { onClose } = renderPalette([
      { id: "new", section: "노드", label: "새 씬", run },
      { id: "settings", section: "프로젝트", label: "설정 열기", run: other },
    ]);

    await user.type(screen.getByPlaceholderText("명령 검색…"), "설정");
    await user.keyboard("{Enter}");

    expect(run).not.toHaveBeenCalled();
    expect(other).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("does not run disabled commands", async () => {
    const user = userEvent.setup();
    const run = vi.fn();
    renderPalette([{ id: "missing", section: "이동", label: "이전 씬", disabled: true, run }]);

    await user.keyboard("{Enter}");

    expect(run).not.toHaveBeenCalled();
  });
});
