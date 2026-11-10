import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Project } from "../lib/types";
import { I18nProvider } from "../lib/i18n";
import { EpisodeRuleOffer, needsEpisodeRule } from "./EpisodeRuleOffer";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  projectsUpdate: vi.fn(),
  projectsList: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: { get: mocks.settingsGet },
  projects: { update: mocks.projectsUpdate, list: mocks.projectsList },
}));

const base: Project = {
  id: "w1",
  title: "연재 하나",
  genres: [],
  length_target: "series",
  default_pov: "first",
  style_notes: "",
  outline: "",
  outline_preset: "webnovel",
  episode_char_target: 5000,
  char_count_basis: "with_spaces",
  synopsis: "",
  word_count: 0,
  created_at: 1,
  updated_at: 1,
};

beforeEach(() => {
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
  mocks.projectsUpdate.mockImplementation(async (patch: Partial<Project> & { id: string }) => ({ ...base, ...patch }));
  try {
    localStorage.clear();
  } catch {
    /* no storage in this environment */
  }
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderOffer(project: Project, onProjectChanged = vi.fn()) {
  render(
    <I18nProvider>
      <EpisodeRuleOffer project={project} onProjectChanged={onProjectChanged} />
    </I18nProvider>,
  );
  return onProjectChanged;
}

describe("EpisodeRuleOffer", () => {
  it("is for web novels counted with spaces that have no bounds yet", () => {
    expect(needsEpisodeRule(base)).toBe(true);
    expect(needsEpisodeRule({ ...base, episode_char_min: 5000 })).toBe(false);
    expect(needsEpisodeRule({ ...base, episode_char_max: 6000 })).toBe(false);
    expect(needsEpisodeRule({ ...base, char_count_basis: "without_spaces" })).toBe(false);
    expect(needsEpisodeRule({ ...base, outline_preset: "novel" })).toBe(false);
  });

  it("gives this work the minimum and raises a lower target to the preset's", async () => {
    const changed = renderOffer(base);
    await userEvent.click(screen.getByTestId("episode-rule-apply"));
    await waitFor(() =>
      expect(mocks.projectsUpdate).toHaveBeenCalledWith({ id: "w1", episode_char_min: 5000, episode_char_target: 5300 }),
    );
    expect(changed).toHaveBeenCalled();
  });

  it("keeps a longer target the writer chose", async () => {
    renderOffer({ ...base, episode_char_target: 6000 });
    await userEvent.click(screen.getByTestId("episode-rule-apply"));
    await waitFor(() =>
      expect(mocks.projectsUpdate).toHaveBeenCalledWith({ id: "w1", episode_char_min: 5000, episode_char_target: 6000 }),
    );
  });

  it("applies to every web novel without bounds, skipping ones counted without spaces", async () => {
    mocks.projectsList.mockResolvedValue([
      base,
      { ...base, id: "w2", title: "긴 연재", episode_char_target: 6000 },
      { ...base, id: "w3", title: "공백 제외", char_count_basis: "without_spaces", episode_char_target: 4000 },
      { ...base, id: "w4", title: "이미 기준 있음", episode_char_min: 4500 },
      { ...base, id: "n1", title: "장편", outline_preset: "novel" },
    ]);
    const changed = renderOffer(base);
    await userEvent.click(screen.getByTestId("episode-rule-apply-all"));

    expect(await screen.findByTestId("episode-rule-outcome")).toBeTruthy();
    expect(mocks.projectsUpdate.mock.calls.map(([patch]) => patch)).toEqual([
      { id: "w1", episode_char_min: 5000, episode_char_target: 5300 },
      { id: "w2", episode_char_min: 5000, episode_char_target: 6000 },
    ]);
    expect(screen.getByText(/웹소설 작품 2개에 적용했습니다\. 공백 제외 기준인 작품 1개\(공백 제외\)는 건너뛰었습니다\./)).toBeTruthy();
    // Only the open work reports back; the others reload when opened.
    expect(changed).toHaveBeenCalledTimes(1);
  });

  it("stops asking about a work once dismissed", async () => {
    renderOffer(base);
    await userEvent.click(screen.getByTestId("episode-rule-dismiss"));
    expect(screen.queryByTestId("episode-rule-offer")).toBeNull();
    cleanup();
    renderOffer(base);
    expect(screen.queryByTestId("episode-rule-offer")).toBeNull();
    expect(mocks.projectsUpdate).not.toHaveBeenCalled();
  });
});
