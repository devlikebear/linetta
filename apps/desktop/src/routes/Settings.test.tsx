import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Settings } from "./Settings";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  settingsSet: vi.fn(),
  gitSyncInit: vi.fn(),
  opsStatusGet: vi.fn(),
  opsStatusClearError: vi.fn(),
  providersListModels: vi.fn(),
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
  },
}));

function renderSettings() {
  render(
    <MemoryRouter>
      <Settings />
    </MemoryRouter>,
  );
}

const baseSettings = {
  provider: "claude-code-cli" as const,
  typewriter_default: true,
  focus_default: false,
  git_sync_dir: "/tmp/linetta-sync",
  git_sync_commit_template: "Linetta sync {date}",
  backup_dir: "/tmp/linetta/backups",
  safety_checklist_dismissed: false,
  web_search_provider: "brave" as const,
  web_search_api_key: "test-key",
};

describe("Settings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Stateful settings mock mirroring the engine: patches merge onto the live
    // config, and the `providers` map merges per-key.
    let state: Record<string, unknown> = { ...baseSettings };
    mocks.settingsGet.mockImplementation(() => Promise.resolve({ ...state }));
    mocks.opsStatusGet.mockResolvedValue([]);
    mocks.settingsSet.mockImplementation((patch: Record<string, unknown>) => {
      const providers = {
        ...(state.providers as Record<string, unknown> | undefined),
        ...(patch.providers as Record<string, unknown> | undefined),
      };
      state = { ...state, ...patch, providers };
      return Promise.resolve({ ...state });
    });
    mocks.providersListModels.mockResolvedValue({ models: [] });
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
  });

  it("shows CLI path field for claude-code-cli and no API key field", async () => {
    renderSettings();
    expect(await screen.findByLabelText("CLI 경로 (선택)")).toBeInTheDocument();
    expect(screen.queryByLabelText("API 키")).not.toBeInTheDocument();
  });

  it("selecting Anthropic reveals API key + model fields and persists the provider", async () => {
    const user = userEvent.setup();
    renderSettings();

    await user.click(await screen.findByRole("button", { name: /Anthropic API/ }));

    await waitFor(() =>
      expect(mocks.settingsSet).toHaveBeenCalledWith({ provider: "anthropic" }),
    );
    expect(await screen.findByLabelText("API 키")).toBeInTheDocument();
    expect(screen.getByLabelText("모델")).toBeInTheDocument();
  });

  it("entering a model sends a per-provider patch", async () => {
    const user = userEvent.setup();
    renderSettings();

    await user.click(await screen.findByRole("button", { name: /OpenAI API/ }));
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

    await user.click(await screen.findByRole("button", { name: /Anthropic API/ }));
    await user.click(await screen.findByRole("button", { name: "모델 새로고침" }));

    await waitFor(() =>
      expect(mocks.providersListModels).toHaveBeenCalledWith("anthropic"),
    );
    await waitFor(() =>
      expect(document.querySelector('option[value="claude-sonnet-4-6"]')).not.toBeNull(),
    );
  });
});
