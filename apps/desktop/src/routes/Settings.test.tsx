import { render, screen, waitFor } from "@testing-library/react";
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
  webSearchTest: vi.fn(),
  diagnosticsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
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
  webSearch: {
    test: mocks.webSearchTest,
  },
  diagnostics: {
    get: mocks.diagnosticsGet,
  },
}));

function renderSettings() {
  render(
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
};

async function clickAdvancedProvider(user: ReturnType<typeof userEvent.setup>, label: RegExp, desc: string) {
  const buttons = await screen.findAllByRole("button", { name: label });
  const match = buttons.find((button) => button.textContent?.includes(desc));
  if (!match) throw new Error(`Provider button not found: ${label}`);
  await user.click(match);
}

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
    });
    mocks.providersListModels.mockResolvedValue({ models: [] });
    mocks.providersDetectCli.mockResolvedValue({ path: "" });
    mocks.providersTest.mockResolvedValue({
      ok: true,
      provider: "anthropic",
      model: "claude-sonnet-4-6",
      message: "연결되었습니다",
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
    expect(screen.getByText("LLM 도구")).toBeInTheDocument();
    expect(screen.getByLabelText("web_search API 키")).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/저장된 검색 API 키 있음/)).toBeInTheDocument();
  });

  it("renders the beginner AI setup guide with subscription policy links", async () => {
    renderSettings();

    expect(await screen.findByText("AI 연결 마법사")).toBeInTheDocument();
    expect(screen.getByText(/Claude와 Gemini 구독 로그인은/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /ChatGPT 구독으로 연결/ })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /OpenAI Codex CLI 안내/ })).toHaveAttribute(
      "href",
      "https://developers.openai.com/codex/cli",
    );
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
    expect(screen.getByText("AI Setup Wizard")).toBeInTheDocument();
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

    expect(await screen.findByRole("link", { name: /OpenAI Codex CLI guide/ })).toBeInTheDocument();
    expect(screen.getByText(/If the selected folder is not a git repository yet/)).toBeInTheDocument();
    expect(screen.getByText(/1 file/)).toBeInTheDocument();
    expect(screen.getByText(/Committed/)).toBeInTheDocument();
    expect(screen.getByText(/Pushed/)).toBeInTheDocument();
    expect(screen.getByText(/New backup created/)).toBeInTheDocument();
  });

  it("selecting the Claude API guide persists the API provider", async () => {
    const user = userEvent.setup();
    renderSettings();

    await user.click(await screen.findByRole("button", { name: /Claude API 키로 연결/ }));
    await user.click(await screen.findByRole("button", { name: "Claude API 선택" }));

    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({ provider: "anthropic" }),
    );
    expect(await screen.findByText(/Claude 구독 하네스는 Linetta에서 지원하지 않습니다/)).toBeInTheDocument();
  });

  it("shows CLI path field only for the legacy claude-code-cli provider", async () => {
    const user = userEvent.setup();
    renderSettings();

    expect(await screen.findByText("AI 연결 마법사")).toBeInTheDocument();
    expect(screen.queryByLabelText("CLI 경로 (선택)")).not.toBeInTheDocument();
    await clickAdvancedProvider(user, /Claude Code CLI/, "기존 설정 유지용");

    expect(await screen.findByLabelText("CLI 경로 (선택)")).toBeInTheDocument();
    expect(screen.queryByLabelText("API 키")).not.toBeInTheDocument();
  });

  it("auto-detect fills the CLI path and persists it for claude-code-cli", async () => {
    mocks.providersDetectCli.mockResolvedValue({ path: "/opt/homebrew/bin/claude" });
    const user = userEvent.setup();
    renderSettings();

    await clickAdvancedProvider(user, /Claude Code CLI/, "기존 설정 유지용");
    await user.click(await screen.findByRole("button", { name: "자동 찾기" }));

    await waitFor(() => expect(mocks.providersDetectCli).toHaveBeenCalled());
    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({
        providers: { "claude-code-cli": { cli_path: "/opt/homebrew/bin/claude" } },
      }),
    );
    expect(await screen.findByDisplayValue("/opt/homebrew/bin/claude")).toBeInTheDocument();
  });

  it("custom Base URL field persists for OpenAI-compatible providers", async () => {
    const user = userEvent.setup();
    renderSettings();

    await clickAdvancedProvider(user, /OpenAI API/, "호환 엔드포인트");
    const baseUrl = await screen.findByLabelText("Base URL (선택)");
    await user.type(baseUrl, "https://api.minimax.io/v1");
    await user.tab();

    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({
        providers: { openai: { base_url: "https://api.minimax.io/v1" } },
      }),
    );
  });

  it("selecting Anthropic reveals API key + model fields and persists the provider", async () => {
    const user = userEvent.setup();
    renderSettings();

    await clickAdvancedProvider(user, /Claude API/, "Anthropic Console API 키로 연결");

    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({ provider: "anthropic" }),
    );
    expect(await screen.findByLabelText("API 키")).toBeInTheDocument();
    expect(screen.getByLabelText("모델")).toBeInTheDocument();
  });

  it("entering a model sends a per-provider patch", async () => {
    const user = userEvent.setup();
    renderSettings();

    await clickAdvancedProvider(user, /OpenAI API/, "호환 엔드포인트");
    const modelInput = await screen.findByLabelText("모델");
    await user.type(modelInput, "gpt-4o");
    await user.tab(); // blur

    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({
        providers: { openai: { model: "gpt-4o" } },
      }),
    );
  });

  it("refresh models fetches the active provider's list and fills the datalist", async () => {
    mocks.providersListModels.mockResolvedValue({
      models: ["claude-haiku-4-5", "claude-sonnet-4-6"],
    });
    const user = userEvent.setup();
    renderSettings();

    await clickAdvancedProvider(user, /Claude API/, "Anthropic Console API 키로 연결");
    await user.click(await screen.findByRole("button", { name: "모델 새로고침" }));

    await waitFor(() =>
      expect(mocks.providersListModels).toHaveBeenCalledWith("anthropic"),
    );
    await waitFor(() =>
      expect(document.querySelector('option[value="claude-sonnet-4-6"]')).not.toBeNull(),
    );
  });

  it("connection test persists unsaved provider drafts before pinging the provider", async () => {
    const user = userEvent.setup();
    renderSettings();

    await clickAdvancedProvider(user, /Claude API/, "Anthropic Console API 키로 연결");
    await user.type(await screen.findByLabelText("API 키"), "sk-ant-test");
    await user.type(screen.getByLabelText("모델"), "claude-sonnet-4-6");
    await user.click(await screen.findByRole("button", { name: "연결 테스트" }));

    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({
        providers: {
          anthropic: expect.objectContaining({
            api_key: "sk-ant-test",
          }),
        },
      }),
    );
    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({
        providers: {
          anthropic: expect.objectContaining({
            model: "claude-sonnet-4-6",
          }),
        },
      }),
    );
    await waitFor(() => expect(mocks.providersTest).toHaveBeenCalledWith("anthropic"));
    expect(await screen.findByText("연결 성공: 연결되었습니다")).toBeInTheDocument();
  });

  it("shows saved provider API key state without echoing the secret", async () => {
    mocks.settingsGet.mockResolvedValue({
      ...baseSettings,
      provider: "anthropic",
      web_search_api_key_set: false,
      providers: { anthropic: { model: "claude-sonnet-4-6", api_key_set: true } },
    });
    renderSettings();

    expect(await screen.findByText("API 키 저장됨")).toBeInTheDocument();
    expect(screen.getByLabelText("API 키")).toHaveValue("");
    expect(screen.getByPlaceholderText(/저장된 API 키 있음/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "키 삭제" })).toBeInTheDocument();
  });

  it("web_search connection test persists an unsaved API key before testing", async () => {
    const user = userEvent.setup();
    renderSettings();

    await user.type(await screen.findByLabelText("web_search API 키"), "BSA-test");
    await user.click(await screen.findByRole("button", { name: "web_search 연결 테스트" }));

    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({ web_search_api_key: "BSA-test" }),
    );
    await waitFor(() => expect(mocks.webSearchTest).toHaveBeenCalled());
    expect(await screen.findByText("web_search 연결 성공: 검색 결과 1건 응답")).toBeInTheDocument();
  });

  it("web_search connection test shows the provider error message", async () => {
    mocks.webSearchTest.mockRejectedValue(new Error("web_search brave api key is required"));
    const user = userEvent.setup();
    renderSettings();

    await user.click(await screen.findByRole("button", { name: "web_search 연결 테스트" }));

    expect(await screen.findByText(/web_search 연결 실패: Error: web_search brave api key is required/)).toBeInTheDocument();
  });

  it("web_search connection test handles a redacted missing API key field", async () => {
    mocks.settingsGet.mockResolvedValue({
      ...baseSettings,
      web_search_api_key: undefined,
      web_search_api_key_set: true,
    });
    const user = userEvent.setup();
    renderSettings();

    await user.click(await screen.findByRole("button", { name: "web_search 연결 테스트" }));

    await waitFor(() => expect(mocks.webSearchTest).toHaveBeenCalled());
    expect(mocks.settingsSet).not.toHaveBeenCalledWith({ web_search_api_key: undefined });
    expect(await screen.findByText("web_search 연결 성공: 검색 결과 1건 응답")).toBeInTheDocument();
  });

  it("hides sandbox-unavailable providers and their setup guides when diagnostics reports them", async () => {
    mocks.diagnosticsGet.mockResolvedValue({
      version: "",
      home: "",
      db_path: "",
      migration_version: 0,
      migration_count: 0,
      ops_status: [],
      unavailable_providers: ["claude-code-cli", "openai-codex"],
    });
    renderSettings();

    // Wait for the settings to load
    await screen.findByText("AI 연결 마법사");

    // claude-code-cli provider toggle must NOT be rendered
    expect(screen.queryByText(/Claude Code CLI/)).not.toBeInTheDocument();
    // openai-codex setup guide button must NOT be rendered
    expect(screen.queryByRole("button", { name: /ChatGPT 구독으로 연결/ })).not.toBeInTheDocument();

    // Other providers must still be visible (Claude API setup guide button)
    expect(screen.getByRole("button", { name: /Claude API 키로 연결/ })).toBeInTheDocument();
  });

  it("resets setup guide to first available when stored provider is hidden in MAS build", async () => {
    // Stored provider is openai-codex, which is in the unavailable list — its guide
    // (chatgpt-subscription) gets filtered out. The component should fall back to
    // the first available guide without crashing.
    mocks.settingsGet.mockResolvedValue({
      ...baseSettings,
      provider: "openai-codex",
    });
    mocks.diagnosticsGet.mockResolvedValue({
      version: "",
      home: "",
      db_path: "",
      migration_version: 0,
      migration_count: 0,
      ops_status: [],
      unavailable_providers: ["claude-code-cli", "openai-codex"],
    });

    renderSettings();

    await screen.findByText("AI 연결 마법사");

    // The chatgpt-subscription guide button must NOT be shown (filtered out)
    expect(screen.queryByRole("button", { name: /ChatGPT 구독으로 연결/ })).not.toBeInTheDocument();
    // The detail panel must show one of the available guides — confirm by checking
    // the action button text rendered inside .setup-guide (not the choice list).
    // openai-api is the first available guide after filtering, so its action button appears.
    const actionBtn = document.querySelector(".setup-guide button");
    expect(actionBtn).not.toBeNull();
    expect(actionBtn!.textContent).toMatch(/선택|API 키로 연결/u);
  });

  it("shows all providers when diagnostics returns an empty unavailable list", async () => {
    mocks.diagnosticsGet.mockResolvedValue({
      version: "",
      home: "",
      db_path: "",
      migration_version: 0,
      migration_count: 0,
      ops_status: [],
      unavailable_providers: [],
    });
    renderSettings();

    await screen.findByText("AI 연결 마법사");

    // Both sandbox-only providers must be visible in non-MAS build
    // (claude-code-cli appears as a provider toggle button; openai-codex as a setup guide button)
    expect(screen.getByRole("button", { name: /Claude Code CLI/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /ChatGPT 구독으로 연결/ })).toBeInTheDocument();
  });
});
