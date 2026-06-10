import { fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { ZenMode } from "./ZenMode";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
}));

vi.mock("./editor/Tiptap", async () => {
  const react = await import("react");
  return {
    TiptapEditor: react.forwardRef(() => <div data-testid="zen-editor" />),
  };
});

beforeEach(() => {
  vi.clearAllMocks();
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
});

function renderZen(props: Partial<ComponentProps<typeof ZenMode>> = {}) {
  return render(
    <I18nProvider>
      <ZenMode
        initialDoc={{ type: "doc", content: [{ type: "paragraph" }] }}
        charCount={100}
        sceneLabel="씬 1"
        onChange={vi.fn()}
        onCharCount={vi.fn()}
        onManualSave={vi.fn()}
        onExit={vi.fn()}
        {...props}
      />
    </I18nProvider>,
  );
}

describe("ZenMode", () => {
  it("uses episode count and target for web novel progress", async () => {
    const { container } = renderZen({ episodeCharCount: 2500, target: 5000 });

    expect(await screen.findByText("이번 화 2,500 / 5,000자 · 씬 1 · esc로 종료")).toBeInTheDocument();

    const overlay = container.querySelector(".zen-overlay");
    expect(overlay).toBeTruthy();
    fireEvent.pointerMove(overlay!, { clientY: 4 });

    expect(container.querySelector<HTMLElement>(".zen-progress-fill")).toHaveStyle({ width: "50%" });
  });
});
