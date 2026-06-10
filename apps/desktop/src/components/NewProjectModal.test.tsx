import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { NewProjectModal } from "./NewProjectModal";

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

function renderModal(onSubmit = vi.fn().mockResolvedValue(undefined)) {
  render(
    <I18nProvider>
      <NewProjectModal open onClose={vi.fn()} onSubmit={onSubmit} />
    </I18nProvider>,
  );
  return { onSubmit };
}

describe("NewProjectModal", () => {
  it("defaults to a web novel serial project with web novel genre chips", async () => {
    const user = userEvent.setup();
    const { onSubmit } = renderModal();

    expect(await screen.findByRole("button", { name: "웹소설 연재" })).toHaveClass("on");
    expect(screen.getByRole("button", { name: "현대판타지" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "단편" })).not.toBeInTheDocument();

    await user.type(screen.getByLabelText("제목"), "새 연재작");
    await user.click(screen.getByRole("button", { name: "현대판타지" }));
    await user.click(screen.getByRole("button", { name: "만들기" }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith({
        title: "새 연재작",
        genres: ["현대판타지"],
        length_target: "series",
        default_pov: "first",
        outline_preset: "webnovel",
      });
    });
  });

  it("switches back to the regular novel flow", async () => {
    const user = userEvent.setup();
    const { onSubmit } = renderModal();

    await user.click(await screen.findByRole("button", { name: "일반 소설" }));

    expect(screen.getByRole("button", { name: "일반 소설" })).toHaveClass("on");
    expect(screen.getByRole("button", { name: "단편" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "현대판타지" })).not.toBeInTheDocument();

    await user.type(screen.getByLabelText("제목"), "장편 원고");
    await user.click(screen.getByRole("button", { name: "장편" }));
    await user.click(screen.getByRole("button", { name: "판타지" }));
    await user.click(screen.getByRole("button", { name: "만들기" }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith({
        title: "장편 원고",
        genres: ["판타지"],
        length_target: "novel",
        default_pov: "first",
        outline_preset: "novel",
      });
    });
  });
});
