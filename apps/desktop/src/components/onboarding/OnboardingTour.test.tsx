import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../lib/i18n";
import { OnboardingTour, type OnboardingTourStep } from "./OnboardingTour";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
}));

function renderTour(
  steps: OnboardingTourStep[],
  opts: {
    onFinish?: () => void;
    onSkip?: () => void;
    language?: "ko" | "en" | "ja";
  } = {},
) {
  mocks.settingsGet.mockResolvedValue({ language: opts.language ?? "ko" });
  render(
    <I18nProvider>
      <button type="button" data-tour="first">first target</button>
      <button type="button" data-tour="second">second target</button>
      <OnboardingTour
        open
        steps={steps}
        onFinish={opts.onFinish ?? vi.fn()}
        onSkip={opts.onSkip ?? vi.fn()}
      />
    </I18nProvider>,
  );
}

const steps: OnboardingTourStep[] = [
  { target: "first", title: "첫 안내", body: "첫 설명" },
  { target: "second", title: "두 번째 안내", body: "두 번째 설명" },
];

describe("OnboardingTour", () => {
  const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;

  beforeEach(() => {
    vi.clearAllMocks();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      value: vi.fn(),
      configurable: true,
      writable: true,
    });
  });

  afterEach(() => {
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      value: originalScrollIntoView,
      configurable: true,
      writable: true,
    });
  });

  it("shows the first anchored step and moves next/previous", async () => {
    const user = userEvent.setup();
    renderTour(steps);

    expect(await screen.findByRole("heading", { name: "첫 안내" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "다음" }));
    expect(screen.getByRole("heading", { name: "두 번째 안내" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "이전" }));
    expect(screen.getByRole("heading", { name: "첫 안내" })).toBeInTheDocument();
  });

  it("skips and completes the tour", async () => {
    const user = userEvent.setup();
    const onSkip = vi.fn();
    const onFinish = vi.fn();
    renderTour(steps, { onSkip, onFinish });

    await user.click(await screen.findByRole("button", { name: "건너뛰기" }));
    expect(onSkip).toHaveBeenCalledOnce();
    cleanup();

    renderTour(steps, { onFinish });
    await user.click(await screen.findByRole("button", { name: "다음" }));
    await user.click(screen.getByRole("button", { name: "완료" }));
    expect(onFinish).toHaveBeenCalledOnce();
  });

  it("skips missing anchors instead of showing a blank tour", async () => {
    renderTour([
      { target: "missing", title: "없는 안내", body: "없음" },
      { target: "second", title: "보이는 안내", body: "있음" },
    ]);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "보이는 안내" })).toBeInTheDocument();
    });
    expect(screen.queryByRole("heading", { name: "없는 안내" })).not.toBeInTheDocument();
  });

  it("localizes controls in Japanese", async () => {
    renderTour(steps, { language: "ja" });

    expect(await screen.findByRole("button", { name: "次へ" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "スキップ" })).toBeInTheDocument();
  });

  it("scrolls once per step and only remeasures on viewport changes", async () => {
    const scrollIntoView = HTMLElement.prototype.scrollIntoView as ReturnType<typeof vi.fn>;
    renderTour(steps);

    expect(await screen.findByRole("heading", { name: "첫 안내" })).toBeInTheDocument();
    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledTimes(1);
    });
    expect(scrollIntoView).toHaveBeenCalledWith({ block: "center", inline: "nearest", behavior: "auto" });

    window.dispatchEvent(new Event("resize"));

    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("renders the fixed tour layer through document.body", async () => {
    renderTour(steps);

    expect(await screen.findByRole("dialog", { name: "온보딩 투어" })).toBeInTheDocument();
    expect(document.querySelector(".tour-layer")?.parentElement).toBe(document.body);
  });
});
