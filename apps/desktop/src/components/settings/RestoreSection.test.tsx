import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  list: vi.fn(),
  peek: vi.fn(),
  restoreProject: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  backupApi: {
    list: rpc.list,
    peek: rpc.peek,
    restoreProject: rpc.restoreProject,
  },
}));

vi.mock("../../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    language: "ko",
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
  localeForLanguage: () => "ko-KR",
}));

import { RestoreSection } from "./RestoreSection";

const BACKUP = {
  path: "C:/home/backups/2026-08-31/library-075141.db",
  kind: "daily" as const,
  size_bytes: 4096,
  created_at: 1_700_000_000_000,
};

describe("RestoreSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.list.mockResolvedValue({ backups: [BACKUP] });
    rpc.peek.mockResolvedValue({
      projects: [{ id: "work-1", title: "잃어버린 도시", word_count: 1200, updated_at: 1 }],
    });
    rpc.restoreProject.mockResolvedValue({ project_id: "new-1", title: "잃어버린 도시 (복원)" });
  });

  it("walks list → peek → restore and reports the new work", async () => {
    const user = userEvent.setup();
    render(<RestoreSection />);

    await user.click(screen.getByTestId("restore-show-backups"));
    await waitFor(() => expect(screen.getByTestId("restore-backup-list")).toBeInTheDocument());
    expect(rpc.list).toHaveBeenCalledTimes(1);

    await user.click(screen.getByText(/settings\.restore\.kind\.daily/));
    await waitFor(() => expect(screen.getByTestId("restore-work-work-1")).toBeInTheDocument());
    expect(rpc.peek).toHaveBeenCalledWith(BACKUP.path);
    expect(screen.getByText("잃어버린 도시")).toBeInTheDocument();

    await user.click(screen.getByTestId("restore-work-work-1"));
    await waitFor(() => expect(screen.getByTestId("restore-done")).toBeInTheDocument());
    // The translated suffix rides along so the engine names the copy visibly.
    expect(rpc.restoreProject).toHaveBeenCalledWith(
      BACKUP.path,
      "work-1",
      "settings.restore.suffix",
    );
    expect(screen.getByTestId("restore-done").textContent).toContain("잃어버린 도시 (복원)");
  });

  it("shows the empty state when no backups exist", async () => {
    rpc.list.mockResolvedValue({ backups: [] });
    const user = userEvent.setup();
    render(<RestoreSection />);

    await user.click(screen.getByTestId("restore-show-backups"));
    await waitFor(() =>
      expect(screen.getByText("settings.restore.empty")).toBeInTheDocument(),
    );
  });

  it("surfaces a restore failure without losing the pane", async () => {
    rpc.restoreProject.mockRejectedValue(new Error("merge failed"));
    const user = userEvent.setup();
    render(<RestoreSection />);

    await user.click(screen.getByTestId("restore-show-backups"));
    await waitFor(() => expect(screen.getByTestId("restore-backup-list")).toBeInTheDocument());
    await user.click(screen.getByText(/settings\.restore\.kind\.daily/));
    await waitFor(() => expect(screen.getByTestId("restore-work-work-1")).toBeInTheDocument());
    await user.click(screen.getByTestId("restore-work-work-1"));

    await waitFor(() =>
      expect(screen.getByText(/merge failed/)).toBeInTheDocument(),
    );
    // The list is still there for another attempt.
    expect(screen.getByTestId("restore-work-work-1")).toBeInTheDocument();
  });
});
