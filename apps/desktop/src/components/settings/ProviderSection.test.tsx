import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  providersList: vi.fn(),
  settingsSet: vi.fn(),
  codexLoginStart: vi.fn(),
  codexLoginStatus: vi.fn(),
  codexLogout: vi.fn(),
  openExternalUrl: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  providers: { list: rpc.providersList },
  settings: { set: rpc.settingsSet },
  codex: {
    loginStart: rpc.codexLoginStart,
    loginStatus: rpc.codexLoginStatus,
    logout: rpc.codexLogout,
  },
  openExternalUrl: rpc.openExternalUrl,
}));

vi.mock("../../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
}));

import { ProviderSection } from "./ProviderSection";

function statusRow(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "openai-codex",
    auth: "oauth",
    active: false,
    configured: false,
    consented: false,
    ...overrides,
  };
}

describe("ProviderSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.providersList.mockResolvedValue([
      statusRow({ id: "openai-codex", auth: "oauth", active: true }),
      statusRow({ id: "anthropic", auth: "api_key" }),
      statusRow({ id: "gemini-native", auth: "api_key" }),
      statusRow({ id: "openai", auth: "api_key" }),
    ]);
    rpc.settingsSet.mockResolvedValue({});
    rpc.codexLoginStart.mockResolvedValue({ auth_url: "https://chatgpt.com/auth/start" });
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: false });
    rpc.codexLogout.mockResolvedValue({ ok: true });
    rpc.openExternalUrl.mockResolvedValue(undefined);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders the four providers and marks the active one", async () => {
    render(<ProviderSection />);

    const codex = await screen.findByTestId("provider-choice-openai-codex");
    expect(codex).toHaveAttribute("aria-pressed", "true");

    for (const id of ["anthropic", "gemini-native", "openai"]) {
      const button = screen.getByTestId(`provider-choice-${id}`);
      expect(button).toHaveAttribute("aria-pressed", "false");
    }
  });

  it("saves the chosen provider and does not keep it local", async () => {
    render(<ProviderSection />);
    await screen.findByTestId("provider-choice-anthropic");

    await userEvent.click(screen.getByTestId("provider-choice-anthropic"));

    await waitFor(() => expect(rpc.settingsSet).toHaveBeenCalledWith({ provider: "anthropic" }));
  });

  it("shows configured and consent state for the active provider", async () => {
    rpc.providersList.mockResolvedValue([
      statusRow({ id: "openai-codex", auth: "oauth", active: true, configured: true, consented: true }),
      statusRow({ id: "anthropic", auth: "api_key" }),
      statusRow({ id: "gemini-native", auth: "api_key" }),
      statusRow({ id: "openai", auth: "api_key" }),
    ]);
    render(<ProviderSection />);

    const state = await screen.findByTestId("provider-state");
    expect(state.textContent).toContain("settings.providers.state.configured");
    expect(state.textContent).toContain("settings.providers.state.consented");
  });

  it("renders a reason code as a translated message", async () => {
    rpc.providersList.mockRejectedValue(
      Object.assign(new Error("x"), {
        data: { reason: "provider_not_configured" },
      }),
    );
    render(<ProviderSection />);

    const error = await screen.findByTestId("provider-error");
    expect(error.textContent).toBe("errors.providerNotConfigured");
  });

  it("never puts a stored key back into the input", async () => {
    rpc.providersList.mockResolvedValue([
      statusRow({ id: "openai-codex", auth: "oauth" }),
      statusRow({ id: "anthropic", auth: "api_key", active: true, configured: true }),
      statusRow({ id: "gemini-native", auth: "api_key" }),
      statusRow({ id: "openai", auth: "api_key" }),
    ]);
    render(<ProviderSection />);

    const input = (await screen.findByTestId("provider-key-input")) as HTMLInputElement;
    expect(input.value).toBe("");
    expect(input.placeholder).toBe("settings.providers.apiKey.stored");
  });

  it("shows the placeholder for an unconfigured provider", async () => {
    rpc.providersList
      .mockResolvedValueOnce([
        statusRow({ id: "openai-codex", auth: "oauth", active: true }),
        statusRow({ id: "anthropic", auth: "api_key" }),
        statusRow({ id: "gemini-native", auth: "api_key" }),
        statusRow({ id: "openai", auth: "api_key" }),
      ])
      .mockResolvedValueOnce([
        statusRow({ id: "openai-codex", auth: "oauth" }),
        statusRow({ id: "anthropic", auth: "api_key", active: true }),
        statusRow({ id: "gemini-native", auth: "api_key" }),
        statusRow({ id: "openai", auth: "api_key" }),
      ]);
    render(<ProviderSection />);
    await screen.findByTestId("provider-choice-openai-codex");

    await userEvent.click(screen.getByTestId("provider-choice-anthropic"));
    const input = (await screen.findByTestId("provider-key-input")) as HTMLInputElement;
    expect(input.value).toBe("");
    expect(input.placeholder).toBe("settings.providers.apiKey.placeholder");
  });

  it("disables save until a key is typed, and saves the trimmed draft", async () => {
    rpc.providersList.mockResolvedValue([
      statusRow({ id: "openai-codex", auth: "oauth" }),
      statusRow({ id: "anthropic", auth: "api_key", active: true }),
      statusRow({ id: "gemini-native", auth: "api_key" }),
      statusRow({ id: "openai", auth: "api_key" }),
    ]);
    render(<ProviderSection />);

    const input = await screen.findByTestId("provider-key-input");
    const save = screen.getByTestId("provider-key-save");
    expect(save).toBeDisabled();

    await userEvent.type(input, "  sk-live-abc  ");
    expect(save).toBeEnabled();

    await userEvent.click(save);
    await waitFor(() =>
      expect(rpc.settingsSet).toHaveBeenCalledWith({
        providers: { anthropic: { api_key: "sk-live-abc" } },
      }),
    );
    expect((input as HTMLInputElement).value).toBe("");
  });

  it("clears a key by sending an empty string", async () => {
    rpc.providersList.mockResolvedValue([
      statusRow({ id: "openai-codex", auth: "oauth" }),
      statusRow({ id: "anthropic", auth: "api_key", active: true, configured: true }),
      statusRow({ id: "gemini-native", auth: "api_key" }),
      statusRow({ id: "openai", auth: "api_key" }),
    ]);
    render(<ProviderSection />);

    await userEvent.click(await screen.findByTestId("provider-key-clear"));

    await waitFor(() =>
      expect(rpc.settingsSet).toHaveBeenCalledWith({
        providers: { anthropic: { api_key: "" } },
      }),
    );
  });

  it("offers no clear button for a provider with no stored key", async () => {
    rpc.providersList.mockResolvedValue([
      statusRow({ id: "openai-codex", auth: "oauth" }),
      statusRow({ id: "anthropic", auth: "api_key", active: true }),
      statusRow({ id: "gemini-native", auth: "api_key" }),
      statusRow({ id: "openai", auth: "api_key" }),
    ]);
    render(<ProviderSection />);
    await screen.findByTestId("provider-key-input");
    expect(screen.queryByTestId("provider-key-clear")).toBeNull();
  });

  it("offers base URL only for the openai-compatible provider", async () => {
    rpc.providersList
      .mockResolvedValueOnce([
        statusRow({ id: "openai-codex", auth: "oauth", active: true }),
        statusRow({ id: "anthropic", auth: "api_key" }),
        statusRow({ id: "gemini-native", auth: "api_key" }),
        statusRow({ id: "openai", auth: "api_key" }),
      ])
      .mockResolvedValueOnce([
        statusRow({ id: "openai-codex", auth: "oauth" }),
        statusRow({ id: "anthropic", auth: "api_key" }),
        statusRow({ id: "gemini-native", auth: "api_key" }),
        statusRow({ id: "openai", auth: "api_key", active: true }),
      ]);
    render(<ProviderSection />);

    await screen.findByTestId("provider-choice-openai");
    expect(screen.queryByTestId("provider-base-url")).toBeNull();

    await userEvent.click(screen.getByTestId("provider-choice-openai"));
    expect(await screen.findByTestId("provider-base-url")).toBeInTheDocument();
  });

  it("drops the base URL draft when the provider changes", async () => {
    rpc.providersList
      .mockResolvedValueOnce([
        statusRow({ id: "openai-codex", auth: "oauth" }),
        statusRow({ id: "anthropic", auth: "api_key" }),
        statusRow({ id: "gemini-native", auth: "api_key" }),
        statusRow({
          id: "openai",
          auth: "api_key",
          active: true,
          base_url: "https://openrouter.ai/api/v1",
        }),
      ])
      .mockResolvedValueOnce([
        statusRow({ id: "openai-codex", auth: "oauth" }),
        statusRow({ id: "anthropic", auth: "api_key", active: true }),
        statusRow({ id: "gemini-native", auth: "api_key" }),
        statusRow({ id: "openai", auth: "api_key" }),
      ]);
    render(<ProviderSection />);

    const baseUrlInput = (await screen.findByTestId(
      "provider-base-url-input",
    )) as HTMLInputElement;
    expect(baseUrlInput.value).toBe("https://openrouter.ai/api/v1");

    await userEvent.click(screen.getByTestId("provider-choice-anthropic"));
    const keyInput = await screen.findByTestId("provider-key-input");
    expect(screen.queryByTestId("provider-base-url")).toBeNull();

    await userEvent.type(keyInput, "sk-live-xyz");
    await userEvent.click(screen.getByTestId("provider-key-save"));

    await waitFor(() =>
      expect(rpc.settingsSet).toHaveBeenLastCalledWith({
        providers: { anthropic: { api_key: "sk-live-xyz" } },
      }),
    );
  });

  it("opens the browser and polls until Codex reports signed in", async () => {
    rpc.codexLoginStatus
      .mockResolvedValueOnce({ logged_in: false })
      .mockResolvedValueOnce({ logged_in: true, email: "writer@example.com" });

    render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    vi.useFakeTimers();

    fireEvent.click(loginButton);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(rpc.codexLoginStart).toHaveBeenCalledTimes(1);
    expect(rpc.openExternalUrl).toHaveBeenCalledWith("https://chatgpt.com/auth/start");
    expect(rpc.codexLoginStatus).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    expect(rpc.codexLoginStatus).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("provider-codex-login")).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    expect(rpc.codexLoginStatus).toHaveBeenCalledTimes(2);
    expect(screen.getByTestId("provider-codex-email")).toHaveTextContent("writer@example.com");

    // The poll must have stopped: advancing further does not call again.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(rpc.codexLoginStatus).toHaveBeenCalledTimes(2);
  });

  it("shows a signed-in fallback when Codex reports no email claim", async () => {
    rpc.codexLoginStatus.mockResolvedValueOnce({ logged_in: true });

    render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    vi.useFakeTimers();
    fireEvent.click(loginButton);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });

    expect(screen.getByTestId("provider-codex-email")).toHaveTextContent(
      "settings.providers.codex.signedIn",
    );
  });

  it("stops polling when Codex reports a failed login", async () => {
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: false, login_failed: true });

    render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    vi.useFakeTimers();
    fireEvent.click(loginButton);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    expect(rpc.codexLoginStatus).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("provider-codex-failed")).toBeInTheDocument();

    // A failed login must not leave the poll running forever.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4500);
    });
    expect(rpc.codexLoginStatus).toHaveBeenCalledTimes(1);
  });

  it("clears a stale failure banner as soon as a retry starts", async () => {
    rpc.codexLoginStatus.mockResolvedValueOnce({ logged_in: false, login_failed: true });

    render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    vi.useFakeTimers();
    fireEvent.click(loginButton);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    expect(screen.getByTestId("provider-codex-failed")).toBeInTheDocument();

    // Retrying must not leave the old failure banner up while the fresh
    // poll is still in flight.
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: false });
    fireEvent.click(screen.getByTestId("provider-codex-login"));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(screen.queryByTestId("provider-codex-failed")).toBeNull();
  });

  it("clears the interval on unmount so an abandoned login stops calling the engine", async () => {
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: false });

    const { unmount } = render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    vi.useFakeTimers();
    fireEvent.click(loginButton);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    unmount();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(4500);
    });
    expect(rpc.codexLoginStatus).not.toHaveBeenCalled();
  });

  it("logs out of Codex and returns to the sign-in button", async () => {
    rpc.providersList.mockResolvedValue([
      statusRow({ id: "openai-codex", auth: "oauth", active: true, configured: true }),
      statusRow({ id: "anthropic", auth: "api_key" }),
      statusRow({ id: "gemini-native", auth: "api_key" }),
      statusRow({ id: "openai", auth: "api_key" }),
    ]);
    rpc.codexLoginStatus.mockResolvedValueOnce({ logged_in: true, email: "writer@example.com" });

    render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    vi.useFakeTimers();
    fireEvent.click(loginButton);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    expect(screen.getByTestId("provider-codex-email")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("provider-codex-logout"));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(rpc.codexLogout).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("provider-codex-login")).toBeInTheDocument();
  });
});
