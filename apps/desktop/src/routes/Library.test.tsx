import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "../components/ToastProvider";
import { CURRENT_ONBOARDING_TOUR_VERSION } from "../components/onboarding/onboardingState";
import { I18nProvider } from "../lib/i18n";
import { Library } from "./Library";

const mocks = vi.hoisted(() => ({
  projectsList: vi.fn(),
  projectsCreate: vi.fn(),
  projectsArchive: vi.fn(),
  projectsDelete: vi.fn(),
  exportProject: vi.fn(),
  saveExportedMarkdown: vi.fn(),
  importsPreview: vi.fn(),
  importsMarkdown: vi.fn(),
  settingsGet: vi.fn(),
  settingsSet: vi.fn(),
  diagnosticsGet: vi.fn(),
  searchQuery: vi.fn(),
  openPath: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  projects: {
    list: mocks.projectsList,
    create: mocks.projectsCreate,
    archive: mocks.projectsArchive,
    delete: mocks.projectsDelete,
  },
  exportApi: {
    project: mocks.exportProject,
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
  search: {
    query: mocks.searchQuery,
  },
  openPath: mocks.openPath,
}));

vi.mock("../lib/exportSave", () => ({
  saveExportedMarkdown: mocks.saveExportedMarkdown,
}));

function renderLibrary() {
  render(
    <MemoryRouter>
      <ToastProvider>
        <I18nProvider>
          <Library />
        </I18nProvider>
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("Library", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    mocks.projectsList.mockResolvedValue([
      {
        id: "project-1",
        title: "Quiet City",
        genres: ["literary"],
        length_target: "short",
        default_pov: "first",
        style_notes: "",
        outline: "",
        synopsis: "",
        word_count: 120,
        last_opened_node_id: "node-1",
        created_at: 1,
        updated_at: 2,
      },
    ]);
    mocks.projectsArchive.mockResolvedValue({ ok: true });
    mocks.projectsDelete.mockResolvedValue({ ok: true });
    mocks.exportProject.mockResolvedValue({
      suggested_filename: "quiet-city.md",
      markdown: "# Quiet City\n",
    });
    mocks.saveExportedMarkdown.mockResolvedValue("/tmp/quiet-city.md");
    mocks.settingsGet.mockResolvedValue({
      language: "ko",
      provider: "claude-code-cli",
      typewriter_default: false,
      focus_default: false,
      git_sync_dir: "",
      git_sync_commit_template: "",
      backup_dir: "/tmp/linetta/backups",
      safety_checklist_dismissed: false,
      onboarding_tour_enabled: true,
      onboarding_tour_seen_version: "",
    });
    mocks.settingsSet.mockResolvedValue({
      language: "ko",
      provider: "claude-code-cli",
      typewriter_default: false,
      focus_default: false,
      git_sync_dir: "",
      git_sync_commit_template: "",
      backup_dir: "/tmp/linetta/backups",
      safety_checklist_dismissed: true,
      onboarding_tour_enabled: true,
      onboarding_tour_seen_version: "",
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

  it("waits for first-run modals before starting the onboarding tour", async () => {
    const user = userEvent.setup();
    renderLibrary();

    expect(await screen.findByText("쓰기 안전 체크리스트")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Linetta 둘러보기" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "다시 보지 않기" }));

    expect(await screen.findByRole("heading", { name: "Linetta 둘러보기" })).toBeInTheDocument();
  });

  it("does not reopen the onboarding tour while skip persistence is pending", async () => {
    const user = userEvent.setup();
    let resolveSettings: (value: unknown) => void = () => {};
    mocks.settingsGet.mockResolvedValue({
      language: "ko",
      provider: "claude-code-cli",
      typewriter_default: false,
      focus_default: false,
      git_sync_dir: "",
      git_sync_commit_template: "",
      backup_dir: "/tmp/linetta/backups",
      safety_checklist_dismissed: true,
      onboarding_tour_enabled: true,
      onboarding_tour_seen_version: "",
    });
    mocks.settingsSet.mockImplementation(
      () => new Promise((resolve) => {
        resolveSettings = resolve;
      }),
    );
    renderLibrary();

    expect(await screen.findByRole("heading", { name: "Linetta 둘러보기" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "건너뛰기" }));

    expect(mocks.settingsSet).toHaveBeenCalledWith({ onboarding_tour_seen_version: CURRENT_ONBOARDING_TOUR_VERSION });
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "온보딩 투어" })).not.toBeInTheDocument();
    });
    expect(screen.queryByRole("heading", { name: "Linetta 둘러보기" })).not.toBeInTheDocument();

    await act(async () => {
      resolveSettings({
        language: "ko",
        provider: "claude-code-cli",
        typewriter_default: false,
        focus_default: false,
        git_sync_dir: "",
        git_sync_commit_template: "",
        backup_dir: "/tmp/linetta/backups",
        safety_checklist_dismissed: true,
        onboarding_tour_enabled: true,
        onboarding_tour_seen_version: CURRENT_ONBOARDING_TOUR_VERSION,
      });
    });
  });

  it("opens the data folder from the library menu", async () => {
    const user = userEvent.setup();
    mocks.settingsGet.mockResolvedValue({
      language: "ko",
      provider: "claude-code-cli",
      typewriter_default: false,
      focus_default: false,
      git_sync_dir: "",
      git_sync_commit_template: "",
      backup_dir: "/tmp/linetta/backups",
      safety_checklist_dismissed: true,
      onboarding_tour_enabled: true,
      onboarding_tour_seen_version: "library-workspace-v1",
    });
    renderLibrary();

    await user.click(await screen.findByLabelText("라이브러리 옵션"));
    await user.click(screen.getByRole("menuitem", { name: "데이터 폴더 열기" }));

    expect(mocks.openPath).toHaveBeenCalledWith("/tmp/linetta");
  });

  it("always exposes the archive from the home shelf", async () => {
    mocks.projectsList.mockResolvedValue([]);
    mocks.settingsGet.mockResolvedValue({
      language: "ko",
      provider: "claude-code-cli",
      typewriter_default: false,
      focus_default: false,
      git_sync_dir: "",
      git_sync_commit_template: "",
      backup_dir: "/tmp/linetta/backups",
      safety_checklist_dismissed: true,
      onboarding_tour_enabled: true,
      onboarding_tour_seen_version: "library-workspace-v1",
    });
    renderLibrary();

    const archiveLink = await screen.findByRole("link", { name: "보관함" });

    expect(archiveLink).toHaveAttribute("href", "/library/all?tab=archived");
    expect(screen.getByRole("link", { name: "전체 라이브러리 →" })).toBeInTheDocument();
  });

  it("backs up a recent project from its card action menu", async () => {
    const user = userEvent.setup();
    renderLibrary();

    await user.click(await screen.findByLabelText("Quiet City 작품 옵션"));
    await user.click(screen.getByRole("menuitem", { name: "작품 백업 (.md)" }));

    expect(mocks.exportProject).toHaveBeenCalledWith("project-1");
    expect(mocks.saveExportedMarkdown).toHaveBeenCalledWith({
      suggested_filename: "quiet-city.md",
      markdown: "# Quiet City\n",
    });
  });

  it("deletes a recent project after confirmation", async () => {
    const user = userEvent.setup();
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderLibrary();

    await user.pointer({ keys: "[MouseRight]", target: await screen.findByRole("button", { name: "Quiet City 단편 120자" }) });
    await user.click(screen.getByRole("menuitem", { name: "작품 삭제" }));

    expect(confirmSpy).toHaveBeenCalledWith("\"Quiet City\" 작품을 영구 삭제하시겠습니까?");
    expect(mocks.projectsDelete).toHaveBeenCalledWith("project-1");
    expect(mocks.projectsList).toHaveBeenCalledTimes(2);
    confirmSpy.mockRestore();
  });
});
