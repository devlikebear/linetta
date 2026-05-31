import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "../components/ToastProvider";
import { Library } from "./Library";

const mocks = vi.hoisted(() => ({
  projectsList: vi.fn(),
  projectsCreate: vi.fn(),
  importsPreview: vi.fn(),
  importsMarkdown: vi.fn(),
  settingsGet: vi.fn(),
  settingsSet: vi.fn(),
  diagnosticsGet: vi.fn(),
  openPath: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  projects: {
    list: mocks.projectsList,
    create: mocks.projectsCreate,
  },
  imports: {
    preview: mocks.importsPreview,
    markdown: mocks.importsMarkdown,
  },
  settings: {
    get: mocks.settingsGet,
    set: mocks.settingsSet,
  },
  diagnostics: {
    get: mocks.diagnosticsGet,
  },
  openPath: mocks.openPath,
}));

function renderLibrary() {
  render(
    <MemoryRouter>
      <ToastProvider>
        <Library />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("Library", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.projectsList.mockResolvedValue([
      {
        id: "project-1",
        title: "Quiet City",
        genres: ["literary"],
        length_target: "short",
        default_pov: "first",
        style_notes: "",
        outline: "",
        word_count: 120,
        last_opened_node_id: "node-1",
        created_at: 1,
        updated_at: 2,
      },
    ]);
    mocks.settingsGet.mockResolvedValue({
      provider: "claude-code-cli",
      typewriter_default: false,
      focus_default: false,
      git_sync_dir: "",
      git_sync_commit_template: "",
      backup_dir: "/tmp/linetta/backups",
      safety_checklist_dismissed: false,
    });
    mocks.settingsSet.mockResolvedValue({
      provider: "claude-code-cli",
      typewriter_default: false,
      focus_default: false,
      git_sync_dir: "",
      git_sync_commit_template: "",
      backup_dir: "/tmp/linetta/backups",
      safety_checklist_dismissed: true,
    });
    mocks.diagnosticsGet.mockResolvedValue({
      version: "0.0.1",
      home: "/tmp/linetta",
      db_path: "/tmp/linetta/library.db",
      migration_version: 7,
      migration_count: 7,
      ops_status: [],
    });
  });

  it("shows and dismisses the first-run writing safety checklist", async () => {
    const user = userEvent.setup();
    renderLibrary();

    expect(await screen.findByText("쓰기 안전 체크리스트")).toBeInTheDocument();
    expect(screen.getByText("/tmp/linetta")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "다시 보지 않기" }));

    expect(mocks.settingsSet).toHaveBeenCalledWith({ safety_checklist_dismissed: true });
  });

  it("opens the data folder from the library menu", async () => {
    const user = userEvent.setup();
    mocks.settingsGet.mockResolvedValue({
      provider: "claude-code-cli",
      typewriter_default: false,
      focus_default: false,
      git_sync_dir: "",
      git_sync_commit_template: "",
      backup_dir: "/tmp/linetta/backups",
      safety_checklist_dismissed: true,
    });
    renderLibrary();

    await user.click(await screen.findByLabelText("라이브러리 옵션"));
    await user.click(screen.getByRole("menuitem", { name: "데이터 폴더 열기" }));

    expect(mocks.openPath).toHaveBeenCalledWith("/tmp/linetta");
  });
});
