import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../lib/i18n";
import { SemanticSearchSection } from "./SemanticSearchSection";

const mocks = vi.hoisted(() => ({ status: vi.fn(), enable: vi.fn(), disable: vi.fn(), settingsGet: vi.fn() }));
vi.mock("../../hooks/useEngineEvent", () => ({ useEngineEvent: () => {} }));
vi.mock("../../lib/rpc", () => ({
  settings: { get: mocks.settingsGet },
  semanticSearch: { status: mocks.status, enable: mocks.enable, disable: mocks.disable },
}));

const model = { installed: true, downloading: false, received: 0, total: 141 * 1024 * 1024 };

describe("SemanticSearchSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
  });

  it("switches meaning search on, which fetches the model", async () => {
    const user = userEvent.setup();
    mocks.status.mockResolvedValue({ supported: true, enabled: false, model: { ...model, installed: false } });
    mocks.enable.mockResolvedValue({ supported: true, enabled: true, model: { ...model, installed: false, downloading: true, received: 70 * 1024 * 1024 } });
    render(<I18nProvider><SemanticSearchSection /></I18nProvider>);
    await user.click(await screen.findByTestId("semantic-enabled"));
    expect(mocks.enable).toHaveBeenCalled();
  });

  it("deletes the model only after asking", async () => {
    const user = userEvent.setup();
    mocks.status.mockResolvedValue({ supported: true, enabled: true, model });
    mocks.disable.mockResolvedValue({ supported: true, enabled: false, model: { ...model, installed: false } });
    render(<I18nProvider><SemanticSearchSection /></I18nProvider>);
    expect(await screen.findByTestId("semantic-state")).toHaveTextContent("141MB");
    await user.click(screen.getByRole("button", { name: "모델 삭제" }));
    expect(mocks.disable).not.toHaveBeenCalled();
    await user.click(screen.getByTestId("semantic-delete-confirm"));
    await waitFor(() => expect(mocks.disable).toHaveBeenCalledWith(true));
  });

  it("shows nothing where the model cannot run", async () => {
    mocks.status.mockResolvedValue({ supported: false, enabled: false, model: { ...model, installed: false } });
    render(<I18nProvider><SemanticSearchSection /></I18nProvider>);
    await waitFor(() => expect(mocks.status).toHaveBeenCalled());
    expect(screen.queryByTestId("semantic-search-section")).toBeNull();
  });
});
