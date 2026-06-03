import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { CommandPalette, type Command } from "./CommandPalette";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
}));

beforeEach(() => {
  vi.clearAllMocks();
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
});

function renderPalette(commands: Command[], onClose = vi.fn()) {
  render(
    <I18nProvider>
      <CommandPalette open onClose={onClose} commands={commands} />
    </I18nProvider>,
  );
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
