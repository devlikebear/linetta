import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Settings } from "./Settings";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  settingsSet: vi.fn(),
  gitSyncInit: vi.fn(),
  opsStatusGet: vi.fn(),
  opsStatusClearError: vi.fn(),
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
}));

function renderSettings() {
  render(
    <MemoryRouter>
      <Settings />
    </MemoryRouter>,
  );
}

describe("Settings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.settingsGet.mockResolvedValue({
      provider: "claude-code-cli",
      typewriter_default: true,
      focus_default: false,
      git_sync_dir: "/tmp/linetta-sync",
      git_sync_commit_template: "Linetta sync {date}",
      backup_dir: "/tmp/linetta/backups",
      safety_checklist_dismissed: false,
      web_search_provider: "brave",
      web_search_api_key: "test-key",
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
  });
});
