import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { DirectionSection } from "./DirectionSection";

const mocks = vi.hoisted(() => ({
  getFocus: vi.fn(),
  setFocus: vi.fn(),
  getSceneIntent: vi.fn(),
  setSceneIntent: vi.fn(),
}));
vi.mock("../lib/rpc", () => ({
  settings: { get: vi.fn().mockResolvedValue({ language: "en" }) },
  direction: {
    getFocus: mocks.getFocus,
    setFocus: mocks.setFocus,
    getSceneIntent: mocks.getSceneIntent,
    setSceneIntent: mocks.setSceneIntent,
  },
}));

const noFocus = { project_id: "p", body: "", set_at: 0, present: false, episodes_since: 0, stale: false };
const freshFocus = {
  project_id: "p", body: "Don't resolve their misunderstanding yet", anchor_node_id: "n1",
  set_at: 1000, present: true, episodes_since: 0, stale: false,
};
const staleFocus = { ...freshFocus, episodes_since: 3, stale: true };
const missingAnchorFocus = { ...freshFocus, anchor_node_id: "", episodes_since: -1, stale: true };

const noIntent = { present: false, intent: { node_id: "n1", project_id: "p", goal: "", keep: [], avoid: [], updated_at: 0 } };
const savedIntent = {
  present: true,
  intent: { node_id: "n1", project_id: "p", goal: "Close the distance", keep: ["silence"], avoid: ["a confession"], updated_at: 1 },
};

function renderSection(props: Partial<Parameters<typeof DirectionSection>[0]> = {}) {
  return render(<I18nProvider><DirectionSection projectId="p" nodeId="n1" {...props} /></I18nProvider>);
}

describe("DirectionSection", () => {
  beforeEach(() => {
    mocks.getFocus.mockReset().mockResolvedValue(noFocus);
    mocks.setFocus.mockReset();
    mocks.getSceneIntent.mockReset().mockResolvedValue(noIntent);
    mocks.setSceneIntent.mockReset();
  });

  it("loads the saved focus and scene intent", async () => {
    mocks.getFocus.mockResolvedValue(freshFocus);
    mocks.getSceneIntent.mockResolvedValue(savedIntent);
    renderSection();

    const focusBox = await screen.findByLabelText(/Current focus/);
    expect(focusBox).toHaveValue("Don't resolve their misunderstanding yet");
    expect(screen.getByLabelText(/Goal, one line/)).toHaveValue("Close the distance");
    expect(screen.getByLabelText(/Keep \(one per line/)).toHaveValue("silence");
    expect(screen.getByLabelText(/Avoid \(one per line/)).toHaveValue("a confession");
  });

  it("saves the focus and the scene intent", async () => {
    renderSection();
    const focusBox = await screen.findByLabelText(/Current focus/);
    fireEvent.change(focusBox, { target: { value: "New focus" } });
    mocks.setFocus.mockResolvedValue({ ...freshFocus, body: "New focus" });
    fireEvent.click(screen.getAllByRole("button", { name: "Save" })[0]);
    await waitFor(() => expect(mocks.setFocus).toHaveBeenCalledWith("p", "New focus"));

    fireEvent.change(screen.getByLabelText(/Goal, one line/), { target: { value: "Close the distance" } });
    fireEvent.change(screen.getByLabelText(/Keep \(one per line/), { target: { value: " silence \n\n" } });
    fireEvent.change(screen.getByLabelText(/Avoid \(one per line/), { target: { value: "a confession" } });
    mocks.setSceneIntent.mockResolvedValue(savedIntent.intent);
    fireEvent.click(screen.getAllByRole("button", { name: "Save" })[1]);

    await waitFor(() => expect(mocks.setSceneIntent).toHaveBeenCalledWith(
      "n1", "Close the distance", ["silence"], ["a confession"],
    ));
  });

  it("blocks saving and shows the reason when the focus is over the limit", async () => {
    renderSection();
    const focusBox = await screen.findByLabelText(/Current focus/);
    fireEvent.change(focusBox, { target: { value: "a".repeat(501) } });

    expect(await screen.findByText("The focus must be 500 characters or fewer.")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Save" })[0]).toBeDisabled();
    expect(mocks.setFocus).not.toHaveBeenCalled();
  });

  it("shows the stale badge and clears it with Keep as is", async () => {
    mocks.getFocus.mockResolvedValue(staleFocus);
    renderSection();

    expect(await screen.findByText(/Set 3 episodes ago/)).toBeInTheDocument();
    mocks.setFocus.mockResolvedValue({ ...freshFocus, episodes_since: 0, stale: false });
    fireEvent.click(screen.getByRole("button", { name: "Keep as is" }));

    await waitFor(() => expect(mocks.setFocus).toHaveBeenCalledWith("p", freshFocus.body));
    await waitFor(() => expect(screen.queryByText(/Set 3 episodes ago/)).not.toBeInTheDocument());
  });

  it("names a missing anchor episode instead of an episode count", async () => {
    mocks.getFocus.mockResolvedValue(missingAnchorFocus);
    renderSection();
    expect(await screen.findByText("Anchor episode not found · still valid?")).toBeInTheDocument();
  });

  it("hides the scene-intent block when there is no open scene", async () => {
    renderSection({ nodeId: undefined });
    await screen.findByLabelText(/Current focus/);
    expect(screen.queryByText("This scene's intent")).not.toBeInTheDocument();
    expect(mocks.getSceneIntent).not.toHaveBeenCalled();
  });
});
