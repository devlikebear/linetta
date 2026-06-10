import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { ShortcutsModal } from "./ShortcutsModal";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
}));

describe("ShortcutsModal", () => {
  it("lists companion, AI generation, and global search shortcuts", async () => {
    mocks.settingsGet.mockResolvedValue({ language: "ko" });

    render(
      <I18nProvider>
        <ShortcutsModal open onClose={vi.fn()} />
      </I18nProvider>,
    );

    expect(await screen.findByText("단축키")).toBeInTheDocument();
    expect(screen.getByText("글쓰기 동료 열기")).toBeInTheDocument();
    expect(screen.getByText("AI 생성 열기")).toBeInTheDocument();
    expect(screen.getByText("전체 검색 열기")).toBeInTheDocument();
    expect(screen.getByText("⌘J")).toBeInTheDocument();
    expect(screen.getByText("⌘I")).toBeInTheDocument();
    expect(screen.getByText("⌘F")).toBeInTheDocument();
  });
});
