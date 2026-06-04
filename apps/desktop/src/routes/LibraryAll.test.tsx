import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { LibraryAll } from "./LibraryAll";

const mocks = vi.hoisted(() => ({
  projectsList: vi.fn(),
  projectsArchive: vi.fn(),
  projectsRestore: vi.fn(),
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  projects: {
    list: mocks.projectsList,
    archive: mocks.projectsArchive,
    restore: mocks.projectsRestore,
  },
  settings: {
    get: mocks.settingsGet,
  },
}));

function renderLibraryAll(initialEntry = "/library/all") {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <I18nProvider>
        <LibraryAll />
      </I18nProvider>
    </MemoryRouter>,
  );
}

describe("LibraryAll", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.projectsList.mockResolvedValue([
      {
        id: "project-active",
        title: "진행작",
        genres: ["literary"],
        length_target: "short",
        default_pov: "first",
        style_notes: "",
        outline: "",
        synopsis: "",
        word_count: 10,
        last_opened_node_id: "node-active",
        created_at: 1,
        updated_at: 2,
      },
      {
        id: "project-archived",
        title: "보관작",
        genres: ["mystery"],
        length_target: "novella",
        default_pov: "third_limited",
        style_notes: "",
        outline: "",
        synopsis: "",
        word_count: 20,
        last_opened_node_id: "node-archived",
        created_at: 1,
        updated_at: 2,
        archived_at: 3,
      },
    ]);
    mocks.projectsArchive.mockResolvedValue({ ok: true });
    mocks.projectsRestore.mockResolvedValue({ ok: true });
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
  });

  it("restores an archived project from the archived tab card menu", async () => {
    const user = userEvent.setup();
    renderLibraryAll();

    await user.click(await screen.findByRole("button", { name: "보관됨" }));
    await user.click(await screen.findByLabelText("보관작 작품 옵션"));
    await user.click(screen.getByRole("menuitem", { name: "복원" }));

    expect(mocks.projectsRestore).toHaveBeenCalledWith("project-archived");
    expect(mocks.projectsList).toHaveBeenCalledTimes(3);
  });

  it("opens directly to the archived tab from the archive route", async () => {
    renderLibraryAll("/library/all?tab=archived");

    expect(await screen.findByRole("button", { name: "보관작 중편 20자" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "진행작 단편 10자" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "보관됨" })).toHaveClass("accent");
  });
});
