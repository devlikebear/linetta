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
  providersTest: vi.fn(),
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
  // Mirrors ../lib/rpc exactly. `openRouter`, `webSearch` and
  // `providers.detectCli` used to be mocked here and no longer exist on the
  // real module — a mock for a method nothing can call tests nothing, and the
  // 1.0 leftovers had already drifted (an OpenRouter success message returning
  // provider "openai-codex" after a mechanical id rename).
  providers: {
    listModels: mocks.providersListModels,
    test: mocks.providersTest,
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
    // The sidebar remembers the last category per tab; tests must each start
    // from the default ("general") or they inherit the previous test's spot.
    window.sessionStorage.removeItem("linetta.settings.category");
    // Stateful settings mock mirroring the engine: patches merge onto the live
    // config, and the `providers` map merges per-key.
    let state: Record<string, unknown> = { ...baseSettings };
    mocks.settingsGet.mockImplementation(() => Promise.resolve({ ...state }));
    mocks.opsStatusGet.mockResolvedValue([]);
    mocks.settingsSet.mockImplementation((patch: Record<string, unknown>) => {
      // settings.go merges a provider patch onto the *existing* entry field by
      // field — a patch carrying only `model` must not wipe a stored
      // `consented_at`. Replacing the entry wholesale (what this mock used to
      // do) hides exactly the bug the engine's merge loop exists to avoid.
      const providerPatch = patch.providers as
        | Record<string, Record<string, unknown>>
        | undefined;
      const existing = (state.providers ?? {}) as Record<string, Record<string, unknown>>;
      const providers = { ...existing };
      for (const [id, entry] of Object.entries(providerPatch ?? {})) {
        const next = { ...(existing[id] ?? {}), ...entry };
        // The engine trims every string field it stores, deletes a stored key
        // when a patch carries an empty api_key, and redacts (never echoes) a
        // non-empty one.
        if (typeof next.model === "string") next.model = next.model.trim();
        if (typeof next.base_url === "string") next.base_url = next.base_url.trim();
        if (typeof next.api_key === "string") {
          next.api_key_set = next.api_key.trim() !== "";
          delete next.api_key;
        }
        providers[id] = next;
      }
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
    // The engine answers providers.test with `{ok: true}` and nothing else —
    // a failure is an RPC error carrying a reason code, not a payload field.
    // #94 builds the provider pane against this shape.
    mocks.providersTest.mockResolvedValue({ ok: true });
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

    const user = userEvent.setup();
    renderSettings();

    await user.click(await screen.findByTestId("settings-nav-backup"));
    expect(await screen.findByText("최근 백업 상태")).toBeInTheDocument();
    expect(screen.getByText(/백업 성공/)).toBeInTheDocument();
    // The degraded summarizer banner is global: visible from any category.
    expect(screen.getByText("요약기 상태")).toBeInTheDocument();
    expect(screen.getByText(/provider unavailable/)).toBeInTheDocument();

    await user.click(screen.getByTestId("settings-nav-sync"));
    expect(await screen.findByText(/git push: fatal: no upstream/)).toBeInTheDocument();
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

    await user.click(await screen.findByTestId("settings-nav-editor"));
    expect(await screen.findByRole("heading", { name: "에디터" })).toBeInTheDocument();

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

    await user.click(await screen.findByTestId("settings-nav-editor"));
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

    const user = userEvent.setup();
    renderSettings();

    await user.click(await screen.findByTestId("settings-nav-sync"));
    expect(await screen.findByText(/If the selected folder is not a git repository yet/)).toBeInTheDocument();
    expect(screen.getByText(/1 file/)).toBeInTheDocument();
    expect(screen.getByText(/Committed/)).toBeInTheDocument();
    expect(screen.getByText(/Pushed/)).toBeInTheDocument();

    await user.click(screen.getByTestId("settings-nav-backup"));
    expect(await screen.findByText(/New backup created/)).toBeInTheDocument();
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
    const user = userEvent.setup();
    const { unmount } = render(
      <MemoryRouter>
        <I18nProvider>
          <Settings />
        </I18nProvider>
      </MemoryRouter>,
    );

    await user.click(await screen.findByTestId("settings-nav-sync"));
    // Title is shown in the disabled state section
    expect(await screen.findByText("GitHub 동기화")).toBeInTheDocument();
    // The full form (folder input) must NOT appear
    expect(screen.queryByLabelText("git 폴더")).not.toBeInTheDocument();
    unmount();
    window.sessionStorage.removeItem("linetta.settings.category");

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

    await user.click(await screen.findByTestId("settings-nav-sync"));
    expect(await screen.findByText("GitHub 동기화")).toBeInTheDocument();
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
    const user = userEvent.setup();
    renderSettings();

    await user.click(await screen.findByTestId("settings-nav-backup"));
    await screen.findByText("최근 백업 상태");
    expect(screen.queryByTestId("legacy-ai-export")).toBeNull();
  });

  it("hands the companion record back before the companion goes away", async () => {
    const user = userEvent.setup();
    renderSettings();

    await user.click(await screen.findByTestId("settings-nav-backup"));
    await user.click(await screen.findByTestId("legacy-ai-export"));
    await waitFor(() => expect(mocks.exportCompanionHistory).toHaveBeenCalledTimes(1));
  });
});
