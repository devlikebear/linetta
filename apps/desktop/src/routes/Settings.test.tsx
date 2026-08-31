import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { Settings } from "./Settings";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  settingsSet: vi.fn(),
  gitSyncInit: vi.fn(),
  opsStatusGet: vi.fn(),
  opsStatusClearError: vi.fn(),
  providersListModels: vi.fn(),
  providersDetectCli: vi.fn(),
  providersTest: vi.fn(),
  openRouterKeyInfo: vi.fn(),
  openRouterOAuthStart: vi.fn(),
  openRouterOAuthFinish: vi.fn(),
  webSearchTest: vi.fn(),
  diagnosticsGet: vi.fn(),
  exportCompanionHistory: vi.fn(),
  openExternalUrl: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  // Rejecting hides the background-residence section, matching a shell
  // without those commands; BackgroundSection has its own tests.
  backgroundPrefsGet: () => Promise.reject(new Error("not in this test")),
  backgroundPrefsSet: () => Promise.reject(new Error("not in this test")),
  settings: {
    get: mocks.settingsGet,
    set: mocks.settingsSet,
  },
  gitSync: {
    init: mocks.gitSyncInit,
  },
  opsStatus: {
    get: mocks.opsStatusGet,
    clearError: mocks.opsStatusClearError,
  },
  providers: {
    listModels: mocks.providersListModels,
    detectCli: mocks.providersDetectCli,
    test: mocks.providersTest,
  },
  openRouter: {
    keyInfo: mocks.openRouterKeyInfo,
    oauthStart: mocks.openRouterOAuthStart,
    oauthFinish: mocks.openRouterOAuthFinish,
  },
  webSearch: {
    test: mocks.webSearchTest,
  },
  diagnostics: {
    get: mocks.diagnosticsGet,
  },
  exportApi: {
    companionHistory: mocks.exportCompanionHistory,
  },
  openExternalUrl: mocks.openExternalUrl,
}));

function renderSettings() {
  return render(
    <MemoryRouter>
      <I18nProvider>
        <Settings />
      </I18nProvider>
    </MemoryRouter>,
  );
}

const baseSettings = {
  language: "ko" as const,
  provider: "openai-codex" as const,
  typewriter_default: true,
  focus_default: false,
  theme: "system" as const,
  palette: "hanji" as const,
  editor_font_size: 20,
  editor_line_height: 1.92,
  copy_profile: "plain" as const,
  git_sync_dir: "/tmp/linetta-sync",
  git_sync_commit_template: "Linetta sync {date}",
  backup_dir: "/tmp/linetta/backups",
  safety_checklist_dismissed: false,
  onboarding_tour_enabled: true,
  onboarding_tour_seen_version: "",
  web_search_provider: "brave" as const,
  web_search_api_key: "",
  web_search_api_key_set: true,
  ai_data_sharing_consent_version: 0,
  ai_data_sharing_consented_at: 0,
};


describe("Settings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.removeItem("linetta:onboarding:manual-phase");
    window.localStorage.removeItem("linetta:onboarding:workspace-pending");
    // Stateful settings mock mirroring the engine: patches merge onto the live
    // config, and the `providers` map merges per-key.
    let state: Record<string, unknown> = { ...baseSettings };
    mocks.settingsGet.mockImplementation(() => Promise.resolve({ ...state }));
    mocks.opsStatusGet.mockResolvedValue([]);
    mocks.settingsSet.mockImplementation((patch: Record<string, unknown>) => {
      const providerPatch = patch.providers as Record<string, Record<string, unknown>> | undefined;
      const redactedProviderPatch = providerPatch
        ? Object.fromEntries(Object.entries(providerPatch).map(([key, value]) => {
          const next = { ...value };
          if (typeof next.api_key === "string" && next.api_key !== "") {
            delete next.api_key;
            next.api_key_set = true;
          }
          if (next.clear_api_key) {
            delete next.clear_api_key;
            next.api_key_set = false;
          }
          return [key, next];
        }))
        : undefined;
      const providers = {
        ...(state.providers as Record<string, unknown> | undefined),
        ...redactedProviderPatch,
      };
      const nextPatch = { ...patch };
      if (typeof nextPatch.web_search_api_key === "string") {
        nextPatch.web_search_api_key_set = nextPatch.web_search_api_key !== "";
        nextPatch.web_search_api_key = "";
      }
      state = { ...state, ...nextPatch, providers };
      return Promise.resolve({ ...state });
    });
    mocks.diagnosticsGet.mockResolvedValue({
      version: "",
      home: "",
      db_path: "",
      migration_version: 0,
      migration_count: 0,
      ops_status: [],
      unavailable_providers: [],
      git_sync_available: true,
      companion_history_exists: true,
    });
    mocks.exportCompanionHistory.mockResolvedValue({
      markdown: "# 리네타 컴패니언 기록",
      suggested_filename: "linetta-companion-20260825.md",
    });
    mocks.providersListModels.mockResolvedValue({ models: [] });
    mocks.providersDetectCli.mockResolvedValue({ path: "" });
    mocks.providersTest.mockResolvedValue({
      ok: true,
      provider: "anthropic",
      model: "claude-sonnet-4-6",
      message: "연결되었습니다",
    });
    mocks.openRouterKeyInfo.mockResolvedValue({
      ok: true,
      provider: "openrouter",
      label: "Linetta",
      limit: 10,
      limit_remaining: 8,
      usage_monthly: 2,
    });
    mocks.openRouterOAuthStart.mockResolvedValue({
      request_id: "req-1",
      auth_url: "https://openrouter.ai/auth?callback_url=http%3A%2F%2F127.0.0.1%3A1234%2Fcallback",
      callback_url: "http://127.0.0.1:1234/callback",
      expires_at: 1,
    });
    mocks.openRouterOAuthFinish.mockResolvedValue({
      ok: true,
      provider: "openrouter",
      model: "openai/gpt-5.4",
      message: "OpenRouter 연결이 완료되었습니다.",
    });
    mocks.webSearchTest.mockResolvedValue({
      ok: true,
      provider: "brave",
      message: "검색 결과 1건 응답",
    });
  });

  it("renders backup, git sync, and degraded summarizer status", async () => {
    mocks.opsStatusGet.mockResolvedValue([
      {
        job_name: "backup.daily",
        last_started_at: 1780200000000,
        last_finished_at: 1780200001000,
        last_ok: true,
        last_error: "",
        metadata_json: "{\"backup_ran\":true,\"path\":\"/tmp/linetta/backups/library.db\"}",
      },
      {
        job_name: "git_sync",
        last_started_at: 1780200002000,
        last_finished_at: 1780200003000,
        last_ok: false,
        last_error: "git push: fatal: no upstream",
        metadata_json: "{\"files_written\":1,\"committed\":true,\"pushed\":false}",
      },
      {
        job_name: "summarizer",
        last_started_at: 1780200004000,
        last_finished_at: 1780200005000,
        last_ok: false,
        last_error: "provider unavailable",
        metadata_json: "{\"failure_count\":2}",
      },
    ]);

    renderSettings();

    expect(await screen.findByText("최근 백업 상태")).toBeInTheDocument();
    expect(screen.getByText(/백업 성공/)).toBeInTheDocument();
    expect(screen.getByText(/git push: fatal: no upstream/)).toBeInTheDocument();
    expect(screen.getByText("요약기 상태")).toBeInTheDocument();
    expect(screen.getByText(/provider unavailable/)).toBeInTheDocument();
  });

  it("defaults to Korean and switches the settings UI to English", async () => {
    const user = userEvent.setup();
    renderSettings();

    const selector = await screen.findByLabelText("앱 언어");
    expect(selector).toHaveValue("ko");

    await user.selectOptions(selector, "en");

    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({ language: "en" }),
    );
    expect(await screen.findByLabelText("App language")).toHaveValue("en");
    expect(screen.getByRole("heading", { name: "Settings" })).toBeInTheDocument();
  });

  it("persists onboarding tour toggle and queues a manual replay", async () => {
    const user = userEvent.setup();
    renderSettings();

    await user.click(await screen.findByRole("button", { name: /첫 실행 투어 자동 표시/ }));
    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({ onboarding_tour_enabled: false }),
    );

    await user.click(screen.getByRole("button", { name: "투어 다시 보기" }));

    expect(window.localStorage.getItem("linetta:onboarding:manual-phase")).toBe("library");
  });

  it("persists editor theme and typography controls", async () => {
    const user = userEvent.setup();
    renderSettings();

    expect(await screen.findByText("에디터")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "다크" }));
    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({ theme: "dark" }),
    );

    const fontSize = screen.getByLabelText("글자 크기");
    await user.clear(fontSize);
    await user.type(fontSize, "18");
    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({ editor_font_size: 18 }),
    );

    const lineHeight = screen.getByLabelText("줄간격");
    await user.clear(lineHeight);
    await user.type(lineHeight, "2.1");
    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({ editor_line_height: 2.1 }),
    );

    await user.selectOptions(screen.getByLabelText("복사 프로필"), "munpia");
    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({ copy_profile: "munpia" }),
    );
  });

  it("persists the colour palette separately from the light/dark theme", async () => {
    const user = userEvent.setup();
    renderSettings();

    const group = await screen.findByRole("radiogroup", { name: "색 팔레트" });
    const hanji = within(group).getByRole("radio", { name: /한지/ });
    const press = within(group).getByRole("radio", { name: /프레스/ });
    expect(hanji).toHaveAttribute("aria-checked", "true");

    await user.click(press);
    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({ palette: "press" }),
    );
    await waitFor(() => expect(press).toHaveAttribute("aria-checked", "true"));
    expect(mocks.settingsSet).not.toHaveBeenCalledWith(
      expect.objectContaining({ theme: expect.anything() }),
    );
  });

  it("renders setup links and ops metadata in English when selected", async () => {
    mocks.settingsGet.mockResolvedValue({
      ...baseSettings,
      language: "en",
      web_search_api_key_set: false,
    });
    mocks.opsStatusGet.mockResolvedValue([
      {
        job_name: "git_sync",
        last_started_at: 1780200002000,
        last_finished_at: 1780200003000,
        last_ok: true,
        last_error: "",
        metadata_json: "{\"files_written\":1,\"committed\":true,\"pushed\":true}",
      },
      {
        job_name: "backup.daily",
        last_started_at: 1780200004000,
        last_finished_at: 1780200005000,
        last_ok: true,
        last_error: "",
        metadata_json: "{\"backup_ran\":true}",
      },
    ]);

    renderSettings();

    // The awaited element is the load barrier: everything below is a sync get.
    await screen.findByText("Backup");
    expect(screen.getByText(/If the selected folder is not a git repository yet/)).toBeInTheDocument();
    expect(screen.getByText(/1 file/)).toBeInTheDocument();
    expect(screen.getByText(/Committed/)).toBeInTheDocument();
    expect(screen.getByText(/Pushed/)).toBeInTheDocument();
    expect(screen.getByText(/New backup created/)).toBeInTheDocument();
  });

  it("shows Git Sync section as disabled (with note) when git_sync_available is false, and full section when true", async () => {
    // When git_sync_available is false, the section heading IS rendered but only with the unavailable note,
    // not with the full git-sync form (e.g. no folder input).
    mocks.diagnosticsGet.mockResolvedValue({
      version: "",
      home: "",
      db_path: "",
      migration_version: 0,
      migration_count: 0,
      ops_status: [],
      unavailable_providers: [],
      git_sync_available: false,
    });
    const { unmount } = render(
      <MemoryRouter>
        <I18nProvider>
          <Settings />
        </I18nProvider>
      </MemoryRouter>,
    );

    // Wait for settings to load (an element that is always present)
    await screen.findByText("백업");
    // Title is shown in the disabled state section
    expect(screen.getByText("GitHub 동기화")).toBeInTheDocument();
    // The full form (folder input) must NOT appear
    expect(screen.queryByLabelText("git 폴더")).not.toBeInTheDocument();
    unmount();

    // When git_sync_available is true, the full section (with folder input) IS rendered.
    mocks.diagnosticsGet.mockResolvedValue({
      version: "",
      home: "",
      db_path: "",
      migration_version: 0,
      migration_count: 0,
      ops_status: [],
      unavailable_providers: [],
      git_sync_available: true,
    });
    renderSettings();

    await screen.findByText("백업");
    expect(screen.getByText("GitHub 동기화")).toBeInTheDocument();
    expect(screen.getByLabelText("git 폴더")).toBeInTheDocument();
  });

  it("keeps the transcript export out of a library that never used the companion", async () => {
    mocks.diagnosticsGet.mockResolvedValue({
      version: "",
      home: "",
      db_path: "",
      migration_version: 0,
      migration_count: 0,
      ops_status: [],
      unavailable_providers: [],
      git_sync_available: true,
      companion_history_exists: false,
    });
    renderSettings();

    await screen.findByText("백업");
    expect(screen.queryByTestId("legacy-ai-export")).toBeNull();
  });

  it("hands the companion record back before the companion goes away", async () => {
    const user = userEvent.setup();
    renderSettings();

    await user.click(await screen.findByTestId("legacy-ai-export"));
    await waitFor(() => expect(mocks.exportCompanionHistory).toHaveBeenCalledTimes(1));
  });
});
