import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import type { Project, WritingStatsDay } from "../lib/types";
import { buildStatsRange, formatStatsDay, statLevel, StatsSection } from "./StatsSection";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  statsRange: vi.fn(),
  statsSummary: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
  stats: {
    range: mocks.statsRange,
    summary: mocks.statsSummary,
  },
}));

const project: Project = {
  id: "project-1",
  title: "연재작",
  genres: ["판타지"],
  length_target: "series",
  default_pov: "third_limited",
  style_notes: "",
  outline: "",
  outline_preset: "webnovel",
  episode_char_target: 5000,
  synopsis: "",
  word_count: 0,
  created_at: 1,
  updated_at: 1,
};

function makeDays(): WritingStatsDay[] {
  const { fromDay } = buildStatsRange();
  const start = new Date(`${fromDay}T12:00:00`);
  return Array.from({ length: 84 }, (_, index) => {
    const day = new Date(start);
    day.setDate(day.getDate() + index);
    return {
      day: formatStatsDay(day),
      chars_added: index === 82 ? 1000 : index === 83 ? 2400 : 0,
    };
  });
}

function renderStatsSection(overrides: Partial<Project> = {}) {
  return render(
    <I18nProvider>
      <StatsSection project={{ ...project, ...overrides }} episodeCharTarget={5000} />
    </I18nProvider>,
  );
}

beforeEach(() => {
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
  mocks.statsRange.mockResolvedValue(makeDays());
  mocks.statsSummary.mockResolvedValue({ today: 2400, week_avg: 500, total_days: 3 });
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("StatsSection", () => {
  it("renders a recent writing heatmap and webnovel weekly pace", async () => {
    const { fromDay, toDay } = buildStatsRange();
    renderStatsSection();

    expect(await screen.findByText("집필 기록")).toBeInTheDocument();
    await waitFor(() => {
      expect(mocks.statsRange).toHaveBeenCalledWith("project-1", fromDay, toDay);
    });
    expect(screen.getByText("7일 평균 500자 · 집필 3일")).toBeInTheDocument();
    expect(screen.getByText("현재 속도로 주당 약 0.7화")).toBeInTheDocument();
    expect(screen.getByTitle(`${toDay}: 2,400자`)).toBeInTheDocument();
  });

  it("renders an empty state when no writing days exist", async () => {
    mocks.statsRange.mockResolvedValue(makeDays().map((day) => ({ ...day, chars_added: 0 })));
    mocks.statsSummary.mockResolvedValue({ today: 0, week_avg: 0, total_days: 0 });

    renderStatsSection();

    expect(await screen.findByText("아직 집필 기록이 없습니다")).toBeInTheDocument();
  });

  it("calculates heatmap levels from the current maximum", () => {
    expect(statLevel(0, 100)).toBe(0);
    expect(statLevel(10, 100)).toBe(1);
    expect(statLevel(40, 100)).toBe(2);
    expect(statLevel(70, 100)).toBe(3);
    expect(statLevel(90, 100)).toBe(4);
  });
});
