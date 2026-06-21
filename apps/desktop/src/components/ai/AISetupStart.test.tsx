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
});
