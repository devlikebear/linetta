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
  git_sync_dir: "/tmp/linetta-sync",
  git_sync_commit_template: "Linetta sync {date}",
  backup_dir: "/tmp/linetta/backups",
  safety_checklist_dismissed: false,
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
    mocks.providersListModels.mockResolvedValue({ models: [] });
    mocks.providersDetectCli.mockResolvedValue({ path: "" });
    mocks.providersTest.mockResolvedValue({
      ok: true,
      provider: "anthropic",
      model: "claude-sonnet-4-6",
      message: "연결되었습니다",
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
});
