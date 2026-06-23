import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../lib/i18n";
import { AISetupStart, type GuideID } from "./AISetupStart";

function renderSetup(props: Partial<Parameters<typeof AISetupStart>[0]> = {}) {
  const onGuideIdChange = vi.fn();
  const onSelectProvider = vi.fn();
  function Harness() {
    const [guideId, setGuideId] = useState<GuideID>("chatgpt-subscription");
    return (
      <AISetupStart
        currentProvider="openai-codex"
        currentProviderLabel="ChatGPT 계정 (OpenAI Codex)"
        credentialState="Codex 로그인 필요"
        unavailableProviders={[]}
        selectedGuideId={guideId}
        onGuideIdChange={(id) => {
          onGuideIdChange(id);
          setGuideId(id);
        }}
        onSelectProvider={onSelectProvider}
        saving={false}
        {...props}
      />
    );
  }
  render(
    <I18nProvider>
      <Harness />
    </I18nProvider>,
  );
  return { onGuideIdChange, onSelectProvider };
}

describe("AISetupStart", () => {
  it("renders beginner AI connection choices in Korean", () => {
    renderSetup();

    expect(screen.getByText("AI 연결 마법사")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /가장 쉬운 시작/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /ChatGPT 구독으로 연결/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /OpenAI API 키로 연결/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Claude API 키로 연결/ })).toBeInTheDocument();
  });

  it("selects a direct-key provider from the shared guide", async () => {
    const user = userEvent.setup();
    const { onGuideIdChange, onSelectProvider } = renderSetup();

    await user.click(screen.getByRole("button", { name: /Claude API 키로 연결/ }));
    expect(onGuideIdChange).toHaveBeenCalledWith("claude-api");

    await user.click(screen.getByRole("button", { name: "Claude API 선택" }));
    expect(onSelectProvider).toHaveBeenCalledWith("anthropic");
  });

  it("selects OpenRouter as the recommended easiest path", async () => {
    const user = userEvent.setup();
    const { onSelectProvider } = renderSetup({ selectedGuideId: "openrouter-safe" });

    await user.click(screen.getByRole("button", { name: "OpenRouter 선택" }));

    expect(onSelectProvider).toHaveBeenCalledWith("openrouter");
  });

  it("shows OpenRouter credit state without exposing a key", () => {
    renderSetup({
      selectedGuideId: "openrouter-safe",
      currentProvider: "openrouter",
      currentProviderLabel: "OpenRouter",
      openRouterKeyInfo: {
        ok: true,
        provider: "openrouter",
        label: "Linetta",
        limit: 10,
        limit_remaining: 7.5,
        usage_monthly: 2.5,
      },
    });

    expect(screen.getByText("OpenRouter 한도 상태")).toBeInTheDocument();
    expect(screen.getByText("남은 크레딧")).toBeInTheDocument();
    expect(screen.getByText("$7.50")).toBeInTheDocument();
    expect(screen.queryByText(/or-/)).not.toBeInTheDocument();
  });

  it("starts OpenRouter OAuth from the beginner guide", async () => {
    const user = userEvent.setup();
    const onConnectOpenRouterOAuth = vi.fn();
    renderSetup({
      selectedGuideId: "openrouter-safe",
      onConnectOpenRouterOAuth,
    });

    await user.click(screen.getByRole("button", { name: "OpenRouter로 연결" }));

    expect(onConnectOpenRouterOAuth).toHaveBeenCalled();
  });

  it("uses compact tabs and collapsed steps in modal variant", () => {
    renderSetup({
      variant: "modal",
      selectedGuideId: "openrouter-safe",
      currentProvider: "openrouter",
      currentProviderLabel: "OpenRouter",
    });

    expect(screen.getByRole("tablist", { name: "AI 연결 방식" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /가장 쉬운 시작/ })).toHaveAttribute("aria-selected", "true");
    expect(document.querySelector(".setup-choice")).toBeNull();
    const steps = document.querySelector(".setup-steps-disclosure") as HTMLDetailsElement | null;
    expect(steps).not.toBeNull();
    expect(steps?.open).toBe(false);
  });

  it("lets the OpenRouter guide save a pasted key, refresh models, and run a test", async () => {
    const user = userEvent.setup();
    const onOpenRouterAPIKeyChange = vi.fn();
    const onSaveOpenRouter = vi.fn();
    const onRefreshOpenRouterModels = vi.fn();
    const onTestOpenRouter = vi.fn();
    function Harness() {
      const [apiKeyDraft, setAPIKeyDraft] = useState("");
      return (
        <AISetupStart
          currentProvider="openrouter"
          currentProviderLabel="OpenRouter"
          credentialState="연결 필요"
          unavailableProviders={[]}
          selectedGuideId="openrouter-safe"
          onGuideIdChange={vi.fn()}
          onSelectProvider={vi.fn()}
          saving={false}
          openRouterAPIKeyDraft={apiKeyDraft}
          openRouterModelDraft="openai/gpt-5.4"
          openRouterModelOptions={["openai/gpt-5.4", "openrouter/auto", "anthropic/claude-sonnet-4"]}
          onOpenRouterAPIKeyChange={(value) => {
            onOpenRouterAPIKeyChange(value);
            setAPIKeyDraft(value);
          }}
          onOpenRouterModelChange={vi.fn()}
          onSaveOpenRouter={onSaveOpenRouter}
          onRefreshOpenRouterModels={onRefreshOpenRouterModels}
          onTestOpenRouter={onTestOpenRouter}
        />
      );
    }
    render(
      <I18nProvider>
        <Harness />
      </I18nProvider>,
    );

    await user.type(screen.getByLabelText("OpenRouter API 키"), "or-test");
    await user.click(screen.getByRole("button", { name: "키 저장" }));
    await user.click(screen.getByRole("button", { name: "모델 가져오기" }));
    await user.click(screen.getByRole("button", { name: "연결 테스트" }));

    expect(onOpenRouterAPIKeyChange).toHaveBeenLastCalledWith("or-test");
    expect(onSaveOpenRouter).toHaveBeenCalled();
    expect(onRefreshOpenRouterModels).toHaveBeenCalled();
    expect(onTestOpenRouter).toHaveBeenCalled();
    expect(screen.getByLabelText("OpenRouter 모델")).toHaveValue("openai/gpt-5.4");
    expect(document.querySelector('option[value="openai/gpt-5.4"]')).not.toBeNull();
    expect(document.querySelector('option[value="anthropic/claude-sonnet-4"]')).not.toBeNull();
  });

  it("shows curated OpenRouter writing model presets and hides media-only fetched models", async () => {
    const user = userEvent.setup();
    const onOpenRouterModelChange = vi.fn();
    function Harness() {
      const [modelDraft, setModelDraft] = useState("openai/gpt-5.4");
      return (
        <AISetupStart
          currentProvider="openrouter"
          currentProviderLabel="OpenRouter"
          credentialState="연결 필요"
          unavailableProviders={[]}
          selectedGuideId="openrouter-safe"
          onGuideIdChange={vi.fn()}
          onSelectProvider={vi.fn()}
          saving={false}
          openRouterAPIKeyDraft=""
          openRouterModelDraft={modelDraft}
          openRouterModelOptions={[
            "openai/gpt-5.4",
            "google/gemini-3-flash-preview",
            "google/gemini-3.1-flash-image",
            "google/gemini-3.1-flash-tts-preview",
          ]}
          onOpenRouterAPIKeyChange={vi.fn()}
          onOpenRouterModelChange={(value) => {
            onOpenRouterModelChange(value);
            setModelDraft(value);
          }}
        />
      );
    }
    render(
      <I18nProvider>
        <Harness />
      </I18nProvider>,
    );

    expect(screen.getByText("추천 모델")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /빠르고 비용 효율/ }));

    expect(onOpenRouterModelChange).toHaveBeenCalledWith("google/gemini-3-flash-preview");
    expect(screen.getByLabelText("OpenRouter 모델")).toHaveValue("google/gemini-3-flash-preview");
    expect(document.querySelector('option[value="google/gemini-3.1-flash-image"]')).toBeNull();
    expect(document.querySelector('option[value="google/gemini-3.1-flash-tts-preview"]')).toBeNull();
  });
});
