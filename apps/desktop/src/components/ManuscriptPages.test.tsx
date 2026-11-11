import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { ManuscriptPages } from "./ManuscriptPages";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  manuscriptPages: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: { get: mocks.settingsGet },
  projects: { manuscriptPages: mocks.manuscriptPages },
}));

beforeEach(() => {
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderPages(refreshKey: string | null = null) {
  return render(
    <I18nProvider>
      <ManuscriptPages projectId="w1" refreshKey={refreshKey} />
    </I18nProvider>,
  );
}

describe("ManuscriptPages", () => {
  it("shows the work in manuscript sheets and says it is a rough count", async () => {
    mocks.manuscriptPages.mockResolvedValue({ lines: 1032, pages: 104 });
    renderPages();
    const line = await screen.findByTestId("manuscript-pages");
    expect(line.textContent).toBe("200자 원고지 약 104매");
    expect(line.getAttribute("title")).toContain("간이 환산");
    expect(mocks.manuscriptPages).toHaveBeenCalledWith("w1");
  });

  it("shows nothing for an empty work", async () => {
    mocks.manuscriptPages.mockResolvedValue({ lines: 0, pages: 0 });
    renderPages();
    await waitFor(() => expect(mocks.manuscriptPages).toHaveBeenCalled());
    expect(screen.queryByTestId("manuscript-pages")).toBeNull();
  });

  it("shows nothing when the count cannot be read", async () => {
    mocks.manuscriptPages.mockRejectedValue(new Error("engine down"));
    renderPages();
    await waitFor(() => expect(mocks.manuscriptPages).toHaveBeenCalled());
    expect(screen.queryByTestId("manuscript-pages")).toBeNull();
  });

  it("counts again when a scene is saved", async () => {
    mocks.manuscriptPages.mockResolvedValueOnce({ lines: 10, pages: 1 }).mockResolvedValueOnce({ lines: 11, pages: 2 });
    const { rerender } = renderPages("1");
    expect((await screen.findByTestId("manuscript-pages")).textContent).toBe("200자 원고지 약 1매");
    rerender(
      <I18nProvider>
        <ManuscriptPages projectId="w1" refreshKey="2" />
      </I18nProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("manuscript-pages").textContent).toBe("200자 원고지 약 2매"));
    expect(mocks.manuscriptPages).toHaveBeenCalledTimes(2);
  });
});
