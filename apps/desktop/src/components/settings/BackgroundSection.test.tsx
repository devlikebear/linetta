import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  get: vi.fn(),
  set: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  backgroundPrefsGet: rpc.get,
  backgroundPrefsSet: rpc.set,
}));

vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    language: "ko",
    t: (key: string) => key,
  }),
}));

import { BackgroundSection } from "./BackgroundSection";

describe("BackgroundSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.get.mockResolvedValue({ close_to_tray: false, autostart: false });
    rpc.set.mockImplementation((patch: Record<string, unknown>) =>
      Promise.resolve({
        close_to_tray: patch.closeToTray ?? false,
        autostart: patch.autostart ?? false,
      }),
    );
  });

  it("renders the toggles once the shell answers and flips close-to-tray", async () => {
    const user = userEvent.setup();
    render(<BackgroundSection />);

    await waitFor(() => expect(screen.getByTestId("background-section")).toBeInTheDocument());

    await user.click(screen.getByTestId("background-close-to-tray"));
    await waitFor(() =>
      expect(rpc.set).toHaveBeenCalledWith(expect.objectContaining({ closeToTray: true })),
    );
  });

  it("flips autostart through the shell", async () => {
    const user = userEvent.setup();
    render(<BackgroundSection />);
    await waitFor(() => expect(screen.getByTestId("background-autostart")).toBeInTheDocument());

    await user.click(screen.getByTestId("background-autostart"));
    await waitFor(() =>
      expect(rpc.set).toHaveBeenCalledWith(expect.objectContaining({ autostart: true })),
    );
  });

  it("stays hidden when the shell has no background commands (mobile)", async () => {
    rpc.get.mockRejectedValue(new Error("unknown command"));
    render(<BackgroundSection />);
    await waitFor(() => expect(rpc.get).toHaveBeenCalled());
    expect(screen.queryByTestId("background-section")).not.toBeInTheDocument();
  });
});
