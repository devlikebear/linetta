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

const openModal = (agentAvailable = true) =>
  render(
    <I18nProvider>
      <ShortcutsModal open onClose={vi.fn()} agentAvailable={agentAvailable} />
    </I18nProvider>,
  );

describe("ShortcutsModal", () => {
  it("lists shortcuts that are actually bound", async () => {
    mocks.settingsGet.mockResolvedValue({ language: "ko" });

    openModal();

    expect(await screen.findByText("단축키")).toBeInTheDocument();
    expect(screen.getByText("명령 팔레트 열기")).toBeInTheDocument();
    expect(screen.getByText("전체 검색 열기")).toBeInTheDocument();
    expect(screen.getByText("⌘P")).toBeInTheDocument();
    expect(screen.getByText("⌘F")).toBeInTheDocument();
  });

  it("labels ⌘⇧F with what it actually does", async () => {
    // Cmd+Shift+F opens Contextual Edit (Workspace.tsx). Focus mode has no
    // shortcut at all, so the modal must not offer one.
    mocks.settingsGet.mockResolvedValue({ language: "ko" });

    openModal();

    await screen.findByText("단축키");
    expect(screen.getByText("맥락 편집 열기")).toBeInTheDocument();
    expect(screen.queryByText(/Focus 모드/)).not.toBeInTheDocument();
  });

  it("does not advertise the AI draft key, which is still unbound", async () => {
    // Cmd+I went away with the companion and Workspace still leaves it
    // unbound. Listing it would tell the writer about a shortcut that does
    // nothing.
    mocks.settingsGet.mockResolvedValue({ language: "ko" });

    openModal();

    await screen.findByText("단축키");
    expect(screen.queryByText("⌘I")).not.toBeInTheDocument();
    expect(screen.queryByText(/글쓰기 동료/)).not.toBeInTheDocument();
    expect(screen.queryByText(/AI 생성/)).not.toBeInTheDocument();
  });

  it("advertises the agent panel now that Cmd+J opens it", async () => {
    mocks.settingsGet.mockResolvedValue({ language: "ko" });

    openModal();

    await screen.findByText("단축키");
    expect(screen.getByText("⌘J")).toBeInTheDocument();
    expect(screen.getByText("에이전트 패널 열기")).toBeInTheDocument();
  });

  it("hides the agent panel shortcut when agent_available is false", async () => {
    // No provider plumbed in at all (e.g. an iPad build) means Cmd+J does
    // nothing — advertising it would send the writer to open a panel that
    // can't open (#95).
    mocks.settingsGet.mockResolvedValue({ language: "ko" });

    openModal(false);

    await screen.findByText("단축키");
    expect(screen.queryByText("⌘J")).not.toBeInTheDocument();
    expect(screen.queryByText("에이전트 패널 열기")).not.toBeInTheDocument();
  });
});
