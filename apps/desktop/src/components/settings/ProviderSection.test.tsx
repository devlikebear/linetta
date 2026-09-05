import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  providersList: vi.fn(),
  providersListModels: vi.fn(),
  settingsSet: vi.fn(),
  codexLoginStart: vi.fn(),
  codexLoginStatus: vi.fn(),
  codexLogout: vi.fn(),
  openExternalUrl: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  providers: { list: rpc.providersList, listModels: rpc.providersListModels },
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

const PROVIDER_IDS = ["openai-codex", "anthropic", "gemini-native", "openai"] as const;

/** The four rows providers.list returns, with `active` marked and per-id
 *  overrides folded in. Called fresh on every RPC — see the note in
 *  beforeEach on why that matters. */
function rows(active: string, extras: Record<string, Record<string, unknown>> = {}) {
  return PROVIDER_IDS.map((id) => ({
    id,
    auth: id === "openai-codex" ? "oauth" : "api_key",
    active: id === active,
    configured: false,
    consented: false,
    ...extras[id],
  }));
}

/** Let React settle the promises a click just started. Deliberately not
 *  `findBy*`: @testing-library/dom's fake-timer detection looks for a `jest`
 *  global that vitest's `globals: true` never defines, so waitFor takes its
 *  real-timer branch while the timers it needs are faked and unadvanced. */
async function flush(times = 4) {
  await act(async () => {
    for (let i = 0; i < times; i += 1) await Promise.resolve();
  });
}

describe("ProviderSection", () => {
  // Which provider the engine currently considers active, and any extra
  // fields on a given row. Tests set these before render.
  let activeId: string;
  let rowExtras: Record<string, Record<string, unknown>>;

  beforeEach(() => {
    vi.clearAllMocks();
    activeId = "openai-codex";
    rowExtras = {};
    // The engine's bookkeeping in miniature. Two things matter here:
    // settings.set({provider}) moves the active row, so a test can switch
    // providers and back; and every providers.list resolves a *fresh* array,
    // the way a real RPC that parses new JSON does. A mock that hands back
    // one shared reference makes React bail out of setList, which silently
    // hides every bug in an effect keyed on the list.
    rpc.providersList.mockImplementation(() => Promise.resolve(rows(activeId, rowExtras)));
    rpc.providersListModels.mockImplementation(() => Promise.resolve({ models: [] }));
    rpc.settingsSet.mockImplementation((patch: { provider?: string }) => {
      if (patch.provider) activeId = patch.provider;
      return Promise.resolve({});
    });
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
    rowExtras = { "openai-codex": { configured: true, consented: true } };
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
    activeId = "anthropic";
    rowExtras = { anthropic: { configured: true } };
    render(<ProviderSection />);

    const input = (await screen.findByTestId("provider-key-input")) as HTMLInputElement;
    expect(input.value).toBe("");
    expect(input.placeholder).toBe("settings.providers.apiKey.stored");
  });

  it("shows the placeholder for an unconfigured provider", async () => {
    render(<ProviderSection />);
    await screen.findByTestId("provider-choice-openai-codex");

    await userEvent.click(screen.getByTestId("provider-choice-anthropic"));
    const input = (await screen.findByTestId("provider-key-input")) as HTMLInputElement;
    expect(input.value).toBe("");
    expect(input.placeholder).toBe("settings.providers.apiKey.placeholder");
  });

  it("disables save until a key is typed, and saves the trimmed draft", async () => {
    activeId = "anthropic";
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
    activeId = "anthropic";
    rowExtras = { anthropic: { configured: true } };
    render(<ProviderSection />);

    await userEvent.click(await screen.findByTestId("provider-key-clear"));

    await waitFor(() =>
      expect(rpc.settingsSet).toHaveBeenCalledWith({
        providers: { anthropic: { api_key: "" } },
      }),
    );
  });

  it("offers no clear button for a provider with no stored key", async () => {
    activeId = "anthropic";
    render(<ProviderSection />);
    await screen.findByTestId("provider-key-input");
    expect(screen.queryByTestId("provider-key-clear")).toBeNull();
  });

  it("offers base URL only for the openai-compatible provider", async () => {
    render(<ProviderSection />);

    await screen.findByTestId("provider-choice-openai");
    expect(screen.queryByTestId("provider-base-url")).toBeNull();

    await userEvent.click(screen.getByTestId("provider-choice-openai"));
    expect(await screen.findByTestId("provider-base-url")).toBeInTheDocument();
  });

  it("never lets an openai base URL ride into another provider's key patch", async () => {
    activeId = "openai";
    rowExtras = { openai: { base_url: "https://openrouter.ai/api/v1" } };
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

    // toHaveBeenLastCalledWith is an exact shape match: no base_url key at
    // all, not even an undefined one. settings.set is all-or-nothing and the
    // engine rejects base_url on any id but openai.
    await waitFor(() =>
      expect(rpc.settingsSet).toHaveBeenLastCalledWith({
        providers: { anthropic: { api_key: "sk-live-xyz" } },
      }),
    );
  });

  it("drops an unsaved base URL draft when the writer changes provider", async () => {
    activeId = "openai";
    render(<ProviderSection />);

    const baseUrlInput = (await screen.findByTestId(
      "provider-base-url-input",
    )) as HTMLInputElement;
    fireEvent.change(baseUrlInput, { target: { value: "https://typed.example/v1" } });
    expect(baseUrlInput.value).toBe("https://typed.example/v1");

    await userEvent.click(screen.getByTestId("provider-choice-anthropic"));
    await screen.findByTestId("provider-key-input");

    await userEvent.click(screen.getByTestId("provider-choice-openai"));
    const back = (await screen.findByTestId("provider-base-url-input")) as HTMLInputElement;
    expect(back.value).toBe("");
  });

  it("keeps an unsaved key draft through a background providers.list reload", async () => {
    activeId = "openai";
    rowExtras = { openai: { base_url: "https://openrouter.ai/api/v1" } };
    render(<ProviderSection />);

    const keyInput = (await screen.findByTestId("provider-key-input")) as HTMLInputElement;
    fireEvent.change(keyInput, { target: { value: "sk-live-typed" } });

    // Tab on to the base URL and edit it: the blur saves, and the save
    // reloads providers.list. That reload must not touch the key draft.
    const baseUrl = screen.getByTestId("provider-base-url-input");
    fireEvent.change(baseUrl, { target: { value: "https://ollama.local/v1" } });
    fireEvent.focusOut(baseUrl);

    await waitFor(() =>
      expect(rpc.settingsSet).toHaveBeenCalledWith({
        providers: { openai: { base_url: "https://ollama.local/v1" } },
      }),
    );

    expect((screen.getByTestId("provider-key-input") as HTMLInputElement).value).toBe(
      "sk-live-typed",
    );
    await waitFor(() => expect(screen.getByTestId("provider-key-save")).toBeEnabled());

    await userEvent.click(screen.getByTestId("provider-key-save"));
    await waitFor(() =>
      expect(rpc.settingsSet).toHaveBeenLastCalledWith({
        providers: { openai: { api_key: "sk-live-typed" } },
      }),
    );
  });

  it("does not save the base URL on a blur that changed nothing", async () => {
    activeId = "openai";
    rowExtras = { openai: { base_url: "https://openrouter.ai/api/v1" } };
    render(<ProviderSection />);

    const baseUrl = await screen.findByTestId("provider-base-url-input");
    fireEvent.focus(baseUrl);
    fireEvent.focusOut(baseUrl);

    await flush();
    expect(rpc.settingsSet).not.toHaveBeenCalled();
    expect(rpc.providersList).toHaveBeenCalledTimes(1);
  });

  it("disables the model refresh button until a credential is stored", async () => {
    activeId = "anthropic";
    render(<ProviderSection />);

    const refreshButton = await screen.findByTestId("provider-model-refresh");
    expect(refreshButton).toBeDisabled();
  });

  it("enables the model refresh button once a credential is stored", async () => {
    activeId = "anthropic";
    rowExtras = { anthropic: { configured: true } };
    render(<ProviderSection />);

    const refreshButton = await screen.findByTestId("provider-model-refresh");
    expect(refreshButton).toBeEnabled();
  });

  it("populates the model datalist on refresh", async () => {
    activeId = "anthropic";
    rowExtras = { anthropic: { configured: true } };
    // Fresh array every call — see the note in beforeEach on why a shared
    // literal would hide bugs in an effect keyed on this list's identity.
    rpc.providersListModels.mockImplementation(() =>
      Promise.resolve({ models: ["claude-opus", "claude-haiku"] }),
    );
    render(<ProviderSection />);

    await userEvent.click(await screen.findByTestId("provider-model-refresh"));

    await waitFor(() => expect(rpc.providersListModels).toHaveBeenCalledWith("anthropic"));
    const datalist = document.getElementById("provider-model-list");
    expect(datalist?.querySelectorAll("option")).toHaveLength(2);
  });

  it("keeps the model input usable when the list fails to load", async () => {
    activeId = "anthropic";
    rowExtras = { anthropic: { configured: true } };
    render(<ProviderSection />);
    await screen.findByTestId("provider-model-refresh");

    // A model list is a convenience, not the control — its failure must land
    // on its own line and never in `error`, which drives the section-level
    // alert. Otherwise a writer whose network hiccuped cannot type a model
    // name they already know.
    rpc.providersListModels.mockImplementation(() =>
      Promise.reject(
        Object.assign(new Error("x"), { data: { reason: "provider_unreachable" } }),
      ),
    );

    await userEvent.click(screen.getByTestId("provider-model-refresh"));

    const modelError = await screen.findByTestId("provider-model-error");
    expect(modelError.textContent).toBe("errors.providerUnreachable");
    expect(screen.queryByTestId("provider-error")).toBeNull();

    const input = screen.getByTestId("provider-model-input") as HTMLInputElement;
    expect(input).toBeEnabled();
    await userEvent.type(input, "gpt-5-custom");
    expect(input.value).toBe("gpt-5-custom");
  });

  it("saves an empty model as the provider default", async () => {
    activeId = "anthropic";
    render(<ProviderSection />);

    const input = (await screen.findByTestId("provider-model-input")) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "claude-x" } });
    fireEvent.change(input, { target: { value: "" } });
    fireEvent.focusOut(input);

    await waitFor(() =>
      expect(rpc.settingsSet).toHaveBeenCalledWith({
        providers: { anthropic: { model: "" } },
      }),
    );
  });

  it("saves the trimmed model draft on blur", async () => {
    activeId = "anthropic";
    render(<ProviderSection />);

    const input = await screen.findByTestId("provider-model-input");
    fireEvent.change(input, { target: { value: "  claude-sonnet-5  " } });
    fireEvent.focusOut(input);

    await waitFor(() =>
      expect(rpc.settingsSet).toHaveBeenCalledWith({
        providers: { anthropic: { model: "claude-sonnet-5" } },
      }),
    );
  });

  it("drops an unsaved model draft when the writer changes provider", async () => {
    activeId = "anthropic";
    render(<ProviderSection />);

    const input = (await screen.findByTestId("provider-model-input")) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "claude-typed" } });
    expect(input.value).toBe("claude-typed");

    await userEvent.click(screen.getByTestId("provider-choice-openai"));
    await screen.findByTestId("provider-base-url-input");

    await userEvent.click(screen.getByTestId("provider-choice-anthropic"));
    const back = (await screen.findByTestId("provider-model-input")) as HTMLInputElement;
    expect(back.value).toBe("");
  });

  it("keeps an unsaved model draft through a background providers.list reload", async () => {
    activeId = "openai";
    rowExtras = { openai: { base_url: "https://openrouter.ai/api/v1" } };
    render(<ProviderSection />);

    const modelInput = (await screen.findByTestId("provider-model-input")) as HTMLInputElement;
    fireEvent.change(modelInput, { target: { value: "gpt-5-typed" } });

    // Tab on to the base URL and edit it: the blur saves, and the save
    // reloads providers.list. That reload must not touch the model draft.
    const baseUrl = screen.getByTestId("provider-base-url-input");
    fireEvent.change(baseUrl, { target: { value: "https://ollama.local/v1" } });
    fireEvent.focusOut(baseUrl);

    await waitFor(() =>
      expect(rpc.settingsSet).toHaveBeenCalledWith({
        providers: { openai: { base_url: "https://ollama.local/v1" } },
      }),
    );

    expect((screen.getByTestId("provider-model-input") as HTMLInputElement).value).toBe(
      "gpt-5-typed",
    );
  });

  it("does not erase a key typed on another provider when an abandoned Codex poll returns", async () => {
    render(<ProviderSection />);
    await screen.findByTestId("provider-codex-login");
    await flush();
    rpc.codexLoginStatus.mockClear();
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: false, login_failed: true });

    vi.useFakeTimers();
    fireEvent.click(screen.getByTestId("provider-codex-login"));
    await flush();

    // The writer abandons the browser and switches to Anthropic to paste a
    // key instead. The orphaned poll is still running.
    fireEvent.click(screen.getByTestId("provider-choice-anthropic"));
    await flush(8);
    fireEvent.change(screen.getByTestId("provider-key-input"), {
      target: { value: "sk-ant-typed" },
    });

    // ~1.5s later the poll reports the abandoned login failed and reloads.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    await flush();

    expect(rpc.codexLoginStatus).toHaveBeenCalledTimes(1);
    expect((screen.getByTestId("provider-key-input") as HTMLInputElement).value).toBe(
      "sk-ant-typed",
    );
  });

  it("does not resurrect a stale Codex failure when the writer returns before the orphaned poll resolves", async () => {
    render(<ProviderSection />);
    await screen.findByTestId("provider-codex-login");
    await flush();
    rpc.codexLoginStatus.mockClear();

    vi.useFakeTimers();
    fireEvent.click(screen.getByTestId("provider-codex-login"));
    await flush();

    // The writer abandons the browser, switches to Anthropic, then comes
    // straight back to Codex — all before the poll's first tick fires.
    fireEvent.click(screen.getByTestId("provider-choice-anthropic"));
    await flush(8);

    // The fresh fetch this return triggers reports the truth: no attempt in
    // flight from here.
    rpc.codexLoginStatus.mockResolvedValueOnce({ logged_in: false });
    fireEvent.click(screen.getByTestId("provider-choice-openai-codex"));
    await flush(8);

    expect(screen.queryByTestId("provider-codex-failed")).toBeNull();

    // ~1.5s after the original login click, the orphaned poll's tick finally
    // resolves — reporting the abandoned attempt failed. It must not
    // overwrite the fresh, correct status above with a stale failure.
    rpc.codexLoginStatus.mockResolvedValueOnce({ logged_in: false, login_failed: true });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    await flush();

    expect(screen.queryByTestId("provider-codex-failed")).toBeNull();
    expect(screen.getByTestId("provider-codex-login")).toBeInTheDocument();
  });

  it("does not let an abandoned login's late tick fail the retry that replaced it", async () => {
    render(<ProviderSection />);
    await screen.findByTestId("provider-codex-login");
    await flush();
    rpc.codexLoginStatus.mockClear();

    vi.useFakeTimers();

    // Attempt #1. Its first tick's login_status is slow — still in flight
    // when everything below happens. clearInterval cannot call it back.
    let settleAbandoned: (s: unknown) => void = () => {};
    rpc.codexLoginStatus.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          settleAbandoned = resolve;
        }),
    );
    fireEvent.click(screen.getByTestId("provider-codex-login"));
    await flush();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    await flush();
    expect(rpc.codexLoginStatus).toHaveBeenCalledTimes(1);

    // The writer abandons that browser tab, switches to Anthropic, and comes
    // back. The fresh fetch on the way back reports a clean slate.
    fireEvent.click(screen.getByTestId("provider-choice-anthropic"));
    await flush(8);
    fireEvent.click(screen.getByTestId("provider-choice-openai-codex"));
    await flush(8);
    expect(screen.queryByTestId("provider-codex-failed")).toBeNull();

    // Attempt #2: a real retry, whose poll the writer is now waiting on.
    fireEvent.click(screen.getByTestId("provider-codex-login"));
    await flush();
    rpc.codexLoginStatus.mockClear();

    // Only now does attempt #1's tick resolve, reporting that *it* failed.
    await act(async () => {
      settleAbandoned({ logged_in: false, login_failed: true });
    });
    await flush();

    // It must not be read as attempt #2 failing...
    expect(screen.queryByTestId("provider-codex-failed")).toBeNull();

    // ...and, because login_failed is terminal, it must not have stopped
    // attempt #2's own poll on the way past.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    await flush();
    expect(rpc.codexLoginStatus).toHaveBeenCalledTimes(1);
  });

  it("polls a retry after a failed attempt exactly like the first one", async () => {
    render(<ProviderSection />);
    await screen.findByTestId("provider-codex-login");
    await flush();
    rpc.codexLoginStatus.mockClear();
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: false, login_failed: true });

    vi.useFakeTimers();
    fireEvent.click(screen.getByTestId("provider-codex-login"));
    await flush();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    await flush();
    expect(screen.getByTestId("provider-codex-failed")).toBeInTheDocument();

    // Second click, no provider switch in between: the poll must run again
    // and report the retry's own result.
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: true, email: "writer@example.com" });
    fireEvent.click(screen.getByTestId("provider-codex-login"));
    await flush();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    await flush();

    expect(screen.getByTestId("provider-codex-email")).toHaveTextContent("writer@example.com");
    expect(screen.queryByTestId("provider-codex-failed")).toBeNull();
  });

  it("shows an already signed-in Codex account without waiting for a fresh login", async () => {
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: true, email: "writer@example.com" });
    render(<ProviderSection />);

    expect(await screen.findByTestId("provider-codex-email")).toHaveTextContent(
      "writer@example.com",
    );
    expect(screen.getByTestId("provider-codex-logout")).toBeInTheDocument();
    expect(screen.queryByTestId("provider-codex-login")).toBeNull();
  });

  it("falls back to a signed-in label when the email claim is an empty string", async () => {
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: true, email: "" });
    render(<ProviderSection />);

    expect(await screen.findByTestId("provider-codex-email")).toHaveTextContent(
      "settings.providers.codex.signedIn",
    );
  });

  it("drops a stale Codex failure when the writer leaves the provider and comes back", async () => {
    render(<ProviderSection />);
    await screen.findByTestId("provider-codex-login");
    await flush();
    rpc.codexLoginStatus.mockClear();
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: false, login_failed: true });

    vi.useFakeTimers();
    fireEvent.click(screen.getByTestId("provider-codex-login"));
    await flush();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    await flush();
    expect(screen.getByTestId("provider-codex-failed")).toBeInTheDocument();

    // The engine forgets the failure at the next login_start; the pane must
    // not remember it either.
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: false });
    fireEvent.click(screen.getByTestId("provider-choice-anthropic"));
    await flush(8);
    fireEvent.click(screen.getByTestId("provider-choice-openai-codex"));
    await flush(8);

    expect(screen.getByTestId("provider-codex-login")).toBeInTheDocument();
    expect(screen.queryByTestId("provider-codex-failed")).toBeNull();
  });

  it("opens the browser and polls until Codex reports signed in", async () => {
    render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    await flush();
    rpc.codexLoginStatus.mockClear();
    rpc.codexLoginStatus
      .mockResolvedValueOnce({ logged_in: false })
      .mockResolvedValueOnce({ logged_in: true, email: "writer@example.com" });

    vi.useFakeTimers();
    fireEvent.click(loginButton);
    await flush(2);
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
    render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    await flush();
    rpc.codexLoginStatus.mockClear();
    rpc.codexLoginStatus.mockResolvedValueOnce({ logged_in: true });

    vi.useFakeTimers();
    fireEvent.click(loginButton);
    await flush(2);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });

    expect(screen.getByTestId("provider-codex-email")).toHaveTextContent(
      "settings.providers.codex.signedIn",
    );
  });

  it("stops polling when Codex reports a failed login", async () => {
    render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    await flush();
    rpc.codexLoginStatus.mockClear();
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: false, login_failed: true });

    vi.useFakeTimers();
    fireEvent.click(loginButton);
    await flush(2);

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

  it("stops the poll and says why when login_status itself fails", async () => {
    render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    await flush();
    rpc.codexLoginStatus.mockClear();
    rpc.codexLoginStatus.mockRejectedValue(
      Object.assign(new Error("x"), { data: { reason: "provider_not_configured" } }),
    );

    vi.useFakeTimers();
    fireEvent.click(loginButton);
    await flush(2);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    await flush();

    expect(screen.getByTestId("provider-error").textContent).toBe("errors.providerNotConfigured");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4500);
    });
    expect(rpc.codexLoginStatus).toHaveBeenCalledTimes(1);
  });

  it("reports a reload that fails after a successful login", async () => {
    render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    await flush();
    rpc.codexLoginStatus.mockClear();
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: true, email: "writer@example.com" });
    // The engine socket drops between login_status and providers.list.
    rpc.providersList.mockImplementation(() =>
      Promise.reject(Object.assign(new Error("x"), { data: { reason: "provider_not_configured" } })),
    );

    vi.useFakeTimers();
    fireEvent.click(loginButton);
    await flush(2);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    await flush();

    expect(screen.getByTestId("provider-error").textContent).toBe("errors.providerNotConfigured");
  });

  it("clears a stale failure banner before the retry's first round trip", async () => {
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: false, login_failed: true });
    render(<ProviderSection />);
    expect(await screen.findByTestId("provider-codex-failed")).toBeInTheDocument();

    let release: (v: { auth_url: string }) => void = () => {};
    rpc.codexLoginStart.mockImplementationOnce(
      () =>
        new Promise<{ auth_url: string }>((resolve) => {
          release = resolve;
        }),
    );

    vi.useFakeTimers();
    fireEvent.click(screen.getByTestId("provider-codex-login"));
    await flush();

    // Gone before login_start has even returned: the banner must not hang
    // over the whole browser-opening round trip.
    expect(screen.queryByTestId("provider-codex-failed")).toBeNull();

    await act(async () => {
      release({ auth_url: "https://chatgpt.com/auth/start" });
      await Promise.resolve();
    });
  });

  it("clears the interval on unmount so an abandoned login stops calling the engine", async () => {
    const { unmount } = render(<ProviderSection />);
    const loginButton = await screen.findByTestId("provider-codex-login");
    await flush();
    rpc.codexLoginStatus.mockClear();

    vi.useFakeTimers();
    fireEvent.click(loginButton);
    await flush(2);

    unmount();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(4500);
    });
    expect(rpc.codexLoginStatus).not.toHaveBeenCalled();
  });

  it("logs out of Codex and returns to the sign-in button", async () => {
    rpc.codexLoginStatus.mockResolvedValue({ logged_in: true, email: "writer@example.com" });
    render(<ProviderSection />);

    await userEvent.click(await screen.findByTestId("provider-codex-logout"));

    await waitFor(() => expect(rpc.codexLogout).toHaveBeenCalledTimes(1));
    expect(await screen.findByTestId("provider-codex-login")).toBeInTheDocument();
  });
});
