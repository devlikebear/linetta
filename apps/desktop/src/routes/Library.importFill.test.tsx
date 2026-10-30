import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "../components/ToastProvider";
import { I18nProvider } from "../lib/i18n";
import { Library } from "./Library";

// story-assist 3.D.3: the "Story World 채우기" dialog only appears after a
// Markdown import when a provider is actually connected, and its [시작]
// button only fills the agent panel's composer — it never sends on its own.

const navigateMock = vi.fn();
vi.mock("react-router-dom", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-router-dom")>()),
  useNavigate: () => navigateMock,
}));

const mocks = vi.hoisted(() => ({
  pickAndReadMarkdown: vi.fn(),
  importsPreview: vi.fn(),
  importsMarkdown: vi.fn(),
  projectsList: vi.fn(),
  settingsGet: vi.fn(),
  diagnosticsGet: vi.fn(),
  providersList: vi.fn(),
  extractionEstimate: vi.fn(),
}));

vi.mock("../lib/importLoad", () => ({
  pickAndReadMarkdown: mocks.pickAndReadMarkdown,
}));

vi.mock("../lib/rpc", () => ({
  projects: { list: mocks.projectsList, create: vi.fn(), archive: vi.fn(), delete: vi.fn() },
  exportApi: { project: vi.fn() },
  imports: { preview: mocks.importsPreview, markdown: mocks.importsMarkdown },
  settings: { get: mocks.settingsGet, set: vi.fn() },
  diagnostics: { get: mocks.diagnosticsGet },
  search: { query: vi.fn() },
  openPath: vi.fn(),
  backupApi: { createRecovery: vi.fn() },
  providers: { list: mocks.providersList },
  proposals: { extractionEstimate: mocks.extractionEstimate },
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

async function runImport(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "가져오기 (.md)" }));
  await screen.findByText("확인 후 가져오기");
  await user.click(screen.getByRole("button", { name: "확인 후 가져오기" }));
}

describe("Library import → Story World 채우기", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    mocks.projectsList.mockResolvedValue([]);
    mocks.settingsGet.mockResolvedValue({
      language: "ko", provider: "openai-codex", typewriter_default: false, focus_default: false,
      git_sync_dir: "", git_sync_commit_template: "", backup_dir: "",
      safety_checklist_dismissed: true, onboarding_tour_enabled: false, onboarding_tour_seen_version: "",
    });
    mocks.diagnosticsGet.mockResolvedValue({
      version: "0.0.1", home: "/tmp/linetta", db_path: "/tmp/linetta/library.db",
      migration_version: 7, migration_count: 7, ops_status: [],
    });
    mocks.pickAndReadMarkdown.mockResolvedValue({ fileName: "story.md", content: "# 1화\n\n본문" });
    mocks.importsPreview.mockResolvedValue({
      title: "story", container_count: 1, leaf_count: 1, warnings: [],
      roots: [{ label: "1화", kind: "leaf" }],
    });
    mocks.importsMarkdown.mockResolvedValue({
      project_id: "new-project", container_count: 1, leaf_count: 1, warnings: [],
    });
    mocks.extractionEstimate.mockResolvedValue({ episodes: 1, chars: 120, input_tokens: 60 });
  });

  it("navigates straight to Workspace when no provider is connected", async () => {
    mocks.providersList.mockResolvedValue([]);
    const user = userEvent.setup();
    renderLibrary();
    await runImport(user);
    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith("/workspace/new-project"));
    expect(screen.queryByText("Story World 채우기")).toBeNull();
  });

  it("shows the dialog with the estimate when a provider is connected", async () => {
    mocks.providersList.mockResolvedValue([
      { id: "anthropic", auth: "api_key", active: true, configured: true, consented: true, model: "claude" },
    ]);
    const user = userEvent.setup();
    renderLibrary();
    await runImport(user);
    expect(await screen.findByText("Story World 채우기")).toBeTruthy();
    expect(await screen.findByText(/1개.*120자.*60/)).toBeTruthy();
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it("'시작' sends the writer to Workspace with the extraction prompt queued, not sent", async () => {
    mocks.providersList.mockResolvedValue([
      { id: "anthropic", auth: "api_key", active: true, configured: true, consented: true, model: "claude" },
    ]);
    const user = userEvent.setup();
    renderLibrary();
    await runImport(user);
    await screen.findByText("Story World 채우기");
    await user.click(screen.getByRole("button", { name: "시작" }));
    expect(navigateMock).toHaveBeenCalledWith("/workspace/new-project", {
      state: { fillStoryWorldPrompt: expect.stringContaining("회차") },
    });
  });

  it("'나중에' sends the writer to Workspace with no prompt queued", async () => {
    mocks.providersList.mockResolvedValue([
      { id: "anthropic", auth: "api_key", active: true, configured: true, consented: true, model: "claude" },
    ]);
    const user = userEvent.setup();
    renderLibrary();
    await runImport(user);
    await screen.findByText("Story World 채우기");
    await user.click(screen.getByRole("button", { name: "나중에" }));
    expect(navigateMock).toHaveBeenCalledWith("/workspace/new-project");
  });

  it("does not offer the dialog when the provider is configured but not consented", async () => {
    mocks.providersList.mockResolvedValue([
      { id: "anthropic", auth: "api_key", active: true, configured: true, consented: false },
    ]);
    const user = userEvent.setup();
    renderLibrary();
    await runImport(user);
    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith("/workspace/new-project"));
    expect(screen.queryByText("Story World 채우기")).toBeNull();
  });
});
