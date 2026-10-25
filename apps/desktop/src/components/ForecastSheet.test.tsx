import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { ToastProvider } from "./ToastProvider";
import { ForecastSheet } from "./ForecastSheet";
import type { Forecast } from "../lib/types";

const events = vi.hoisted(() => ({ listeners: new Map<string, (event: { payload: unknown }) => void>() }));
vi.mock("@tauri-apps/api/event", () => ({
  listen: (name: string, listener: (event: { payload: unknown }) => void) => {
    events.listeners.set(name, listener);
    return Promise.resolve(() => events.listeners.delete(name));
  },
}));

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  select: vi.fn(),
  clearSelection: vi.fn(),
  delete: vi.fn(),
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: { get: mocks.settingsGet },
  forecast: {
    list: mocks.list,
    select: mocks.select,
    clearSelection: mocks.clearSelection,
    delete: mocks.delete,
  },
}));

function makeForecast(overrides: Partial<Forecast> = {}): Forecast {
  return {
    id: "f-1",
    project_id: "project-1",
    divergence: "다음 회차 갈래?",
    base_node_id: "n-1",
    horizon: 3,
    branches: [
      {
        id: "b1",
        title: "맞선다",
        premise: "주인공이 물러서지 않는다.",
        beats: [{ episode_offset: 1, summary: "첫 비트" }],
        risks: [{ kind: "continuity", description: "이전 화와 어긋날 수 있음" }],
      },
      {
        id: "b2",
        title: "물러선다",
        premise: "주인공이 피한다.",
        beats: [{ episode_offset: 1, summary: "다른 비트" }],
      },
    ],
    source: "agent",
    created_at: 2000,
    stale: false,
    base_episode_label: "3화",
    ...overrides,
  };
}

function renderSheet(props: Partial<React.ComponentProps<typeof ForecastSheet>> = {}) {
  const onClose = vi.fn();
  const onRequestOutlineReflection = vi.fn();
  render(
    <I18nProvider>
      <ToastProvider>
        <ForecastSheet
          projectId="project-1"
          onClose={onClose}
          onRequestOutlineReflection={onRequestOutlineReflection}
          {...props}
        />
      </ToastProvider>
    </I18nProvider>,
  );
  return { onClose, onRequestOutlineReflection };
}

beforeEach(() => {
  vi.clearAllMocks();
  events.listeners.clear();
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
  mocks.list.mockResolvedValue({ forecasts: [makeForecast()] });
  mocks.select.mockResolvedValue({ ok: true, stale: false });
  mocks.clearSelection.mockResolvedValue({ ok: true });
  mocks.delete.mockResolvedValue({ ok: true });
});

afterEach(() => {
  cleanup();
});

describe("ForecastSheet", () => {
  it("renders every branch as its own card", async () => {
    renderSheet();
    await waitFor(() => expect(screen.getAllByTestId("forecast-branch")).toHaveLength(2));
    expect(screen.getByText("맞선다")).toBeInTheDocument();
    expect(screen.getByText("물러선다")).toBeInTheDocument();
    expect(screen.getByText("주인공이 물러서지 않는다.")).toBeInTheDocument();
  });

  it("choosing a branch calls forecast.select directly — no agent/LLM path", async () => {
    const user = userEvent.setup();
    renderSheet();
    await waitFor(() => expect(screen.getAllByTestId("forecast-choose")).toHaveLength(2));

    await act(async () => {
      await user.click(screen.getAllByTestId("forecast-choose")[0]);
    });

    expect(mocks.select).toHaveBeenCalledWith("f-1", "b1");
    // Only the direct RPC fired; nothing resembling an agent run was touched.
    expect(mocks.select).toHaveBeenCalledTimes(1);
  });

  it("shows [선택 해제] and [개요 반영 제안 요청] once a branch is selected", async () => {
    mocks.list.mockResolvedValue({
      forecasts: [makeForecast({ selected_branch: "b1" })],
    });
    renderSheet();
    await waitFor(() => expect(screen.getByTestId("forecast-deselect")).toBeInTheDocument());
    expect(screen.getByTestId("forecast-request-reflection")).toBeInTheDocument();
    // The other, unselected branch still offers [이 갈래 고르기].
    expect(screen.getAllByTestId("forecast-choose")).toHaveLength(1);
  });

  it("deselecting calls forecast.clearSelection directly", async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue({
      forecasts: [makeForecast({ selected_branch: "b1" })],
    });
    renderSheet();
    await waitFor(() => expect(screen.getByTestId("forecast-deselect")).toBeInTheDocument());

    await act(async () => {
      await user.click(screen.getByTestId("forecast-deselect"));
    });

    expect(mocks.clearSelection).toHaveBeenCalledWith("project-1");
  });

  it("requesting outline reflection hands back the templated prompt, naming the branch and forecast", async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue({
      forecasts: [makeForecast({ selected_branch: "b1" })],
    });
    const { onRequestOutlineReflection } = renderSheet();
    await waitFor(() => expect(screen.getByTestId("forecast-request-reflection")).toBeInTheDocument());

    await act(async () => {
      await user.click(screen.getByTestId("forecast-request-reflection"));
    });

    expect(onRequestOutlineReflection).toHaveBeenCalledTimes(1);
    const prompt = onRequestOutlineReflection.mock.calls[0][0] as string;
    expect(prompt).toContain("맞선다");
    expect(prompt).toContain("f-1");
  });

  it("shows the stale banner with reasons, and omits it when fresh", async () => {
    mocks.list.mockResolvedValue({
      forecasts: [
        makeForecast({
          stale: true,
          reasons: [
            { group: "entities", count: 2 },
            { group: "episodes", episode_labels: ["3화"] },
          ],
        }),
      ],
    });
    renderSheet();
    await waitFor(() => expect(screen.getByText(/정사가 바뀜/)).toBeInTheDocument());
    expect(screen.getByText(/인물 2개/)).toBeInTheDocument();
    expect(screen.getByText(/3화 본문/)).toBeInTheDocument();
  });

  it("deleting a forecast calls forecast.delete", async () => {
    const user = userEvent.setup();
    renderSheet();
    await waitFor(() => expect(screen.getByTestId("forecast-delete")).toBeInTheDocument());

    await act(async () => {
      await user.click(screen.getByTestId("forecast-delete"));
    });

    expect(mocks.delete).toHaveBeenCalledWith("f-1");
  });

  it("shows the empty state when there are no forecasts", async () => {
    mocks.list.mockResolvedValue({ forecasts: [] });
    renderSheet();
    await waitFor(() => expect(screen.queryByTestId("forecast-list")).not.toBeInTheDocument());
    expect(screen.getByText(/아직 저장된 갈래 예측이 없습니다/)).toBeInTheDocument();
  });

  it("refetches when mcp-changed fires for linetta_save_forecast", async () => {
    renderSheet();
    await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(1));

    await act(async () => {
      events.listeners.get("mcp-changed")?.({
        payload: { project_id: "project-1", tool: "linetta_save_forecast", source: "agent" },
      });
    });

    await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(2));
  });

  it("does not refetch for an unrelated mcp-changed tool", async () => {
    renderSheet();
    await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(1));

    await act(async () => {
      events.listeners.get("mcp-changed")?.({
        payload: { project_id: "project-1", tool: "linetta_write_scene", source: "agent" },
      });
    });

    await new Promise((r) => setTimeout(r, 10));
    expect(mocks.list).toHaveBeenCalledTimes(1);
  });
});
