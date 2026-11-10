import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Project, SubmissionReport } from "../lib/types";
import { I18nProvider } from "../lib/i18n";
import { SubmissionCheck } from "./SubmissionCheck";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  projectsUpdate: vi.fn(),
  submissionCheck: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: { get: mocks.settingsGet },
  projects: { update: mocks.projectsUpdate, submissionCheck: mocks.submissionCheck },
}));

const base: Project = {
  id: "w1",
  title: "공모전 작품",
  genres: [],
  length_target: "series",
  default_pov: "first",
  style_notes: "",
  outline: "",
  outline_preset: "webnovel",
  episode_char_min: 5000,
  episode_char_target: 5300,
  char_count_basis: "with_spaces",
  synopsis: "",
  word_count: 0,
  created_at: 1,
  updated_at: 1,
};

const contest: Project = { ...base, submission_min_episodes: 7, submission_total_min: 30000, submission_needs_synopsis: true };

function report(overrides: Partial<SubmissionReport> = {}): SubmissionReport {
  return {
    has_rule: true,
    structured: true,
    episodes: 5,
    min_episodes: 7,
    episode_rule: true,
    meeting: 1,
    short: [
      { node_id: "e2", scene_id: "s2", label: "2화", chars: 3925, delta: -1075 },
      { node_id: "e3", scene_id: "s3", label: "3화", chars: 4380, delta: -620 },
      { node_id: "e4", scene_id: "s4", label: "4화", chars: 4900, delta: -100 },
      { node_id: "e5", scene_id: "s5", label: "5화", chars: 4000, delta: -1000 },
    ],
    long: [],
    total_chars: 18420,
    total_min: 30000,
    total_max: 0,
    total_state: "short",
    total_delta: -11580,
    needs_synopsis: true,
    has_synopsis: false,
    ready: false,
    ...overrides,
  };
}

function renderCheck(project: Project, onOpenScene = vi.fn(), onProjectChanged = vi.fn()) {
  render(
    <I18nProvider>
      <SubmissionCheck project={project} onOpenScene={onOpenScene} onProjectChanged={onProjectChanged} />
    </I18nProvider>,
  );
  return { onOpenScene, onProjectChanged };
}

beforeEach(() => {
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
  mocks.projectsUpdate.mockImplementation(async (patch: Partial<Project> & { id: string }) => ({ ...base, ...patch }));
  mocks.submissionCheck.mockResolvedValue(report());
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("SubmissionCheck", () => {
  it("checks nothing for a work with no submission rule", async () => {
    renderCheck(base);
    expect(screen.getByTestId("submission-check")).not.toHaveAttribute("open");
    await waitFor(() => expect(screen.queryByTestId("submission-list")).toBeNull());
    expect(mocks.submissionCheck).not.toHaveBeenCalled();
  });

  it("lists what the work still lacks, line by line", async () => {
    renderCheck(contest);
    expect(await screen.findByTestId("submission-episodes")).toHaveTextContent("화 수: 5 / 7화");
    expect(screen.getByTestId("submission-episodes")).toHaveClass("is-short");
    const length = screen.getByTestId("submission-episode-length");
    expect(length).toHaveTextContent("기준 충족 1화");
    expect(length).toHaveTextContent("미달 4화");
    expect(length).toHaveTextContent("2화 −1,075자");
    expect(length).toHaveTextContent("외 1화");
    expect(screen.getAllByTestId("submission-short-episode")).toHaveLength(3);
    expect(screen.getByTestId("submission-total")).toHaveTextContent("작품 분량: 18,420자 / 최소 30,000자");
    expect(screen.getByTestId("submission-synopsis")).toHaveTextContent("시놉시스: 없음");
    expect(screen.queryByTestId("submission-ready")).toBeNull();
  });

  it("opens a short episode at its first scene", async () => {
    const { onOpenScene } = renderCheck(contest);
    const [first] = await screen.findAllByTestId("submission-short-episode");
    await userEvent.click(first);
    expect(onOpenScene).toHaveBeenCalledWith("s2");
  });

  it("says when every rule is met", async () => {
    mocks.submissionCheck.mockResolvedValue(
      report({ episodes: 7, meeting: 7, short: [], total_chars: 36000, total_state: "ok", total_delta: 0, has_synopsis: true, ready: true }),
    );
    renderCheck(contest);
    expect(await screen.findByTestId("submission-ready")).toBeInTheDocument();
    expect(screen.getByTestId("submission-episode-length")).toHaveClass("is-met");
  });

  it("refuses a work minimum past its maximum before it reaches the engine", async () => {
    renderCheck({ ...contest, submission_total_max: 50000 });
    const input = screen.getByRole("textbox", { name: "작품 최소" });
    await userEvent.clear(input);
    await userEvent.type(input, "60000{Enter}");
    expect(await screen.findByRole("alert")).toHaveTextContent("작품 최소 분량이 최대보다 크지 않게");
    expect(mocks.projectsUpdate).not.toHaveBeenCalled();
  });

  it("saves the rule a field at a time", async () => {
    const { onProjectChanged } = renderCheck(base);
    await userEvent.type(screen.getByRole("textbox", { name: "최소 화 수" }), "7{Enter}");
    await waitFor(() => expect(mocks.projectsUpdate).toHaveBeenCalledWith({ id: "w1", submission_min_episodes: 7 }));
    expect(onProjectChanged).toHaveBeenCalled();
    await userEvent.click(screen.getByTestId("submission-needs-synopsis"));
    await waitFor(() => expect(mocks.projectsUpdate).toHaveBeenCalledWith({ id: "w1", submission_needs_synopsis: true }));
  });
});
