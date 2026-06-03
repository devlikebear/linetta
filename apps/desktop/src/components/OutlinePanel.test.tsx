import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TreeNode } from "../hooks/useFirstLeaf";
import { I18nProvider } from "../lib/i18n";
import { OutlinePanel } from "./OutlinePanel";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
}));

const scene: TreeNode = {
  id: "scene-1",
  project_id: "project-1",
  parent_id: "chapter-1",
  ordinal: 0,
  kind: "leaf",
  label: "씬 1",
  title: "첫 만남",
  status: "draft",
  word_count: 10,
  created_at: 1,
  updated_at: 1,
  children: [],
};

const chapter: TreeNode = {
  id: "chapter-1",
  project_id: "project-1",
  ordinal: 0,
  kind: "container",
  label: "1장",
  title: "",
  status: "draft",
  word_count: 0,
  created_at: 1,
  updated_at: 1,
  children: [scene],
};

beforeEach(() => {
  vi.clearAllMocks();
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
});

function renderOutline(props: Partial<ComponentProps<typeof OutlinePanel>> = {}) {
  return render(
    <I18nProvider>
      <OutlinePanel
        tree={[chapter]}
        currentId="scene-1"
        collapsed={false}
        onToggleCollapse={vi.fn()}
        onSelect={vi.fn()}
        {...props}
      />
    </I18nProvider>,
  );
}

describe("OutlinePanel", () => {
  it("opens a popup menu for scene rename and creation actions", async () => {
    const user = userEvent.setup();
    const onRename = vi.fn();
    const onCreateScene = vi.fn();
    const onCreateChapter = vi.fn();

    renderOutline({
      onRename,
      onCreateScene,
      onCreateChapter,
    });

    fireEvent.contextMenu(screen.getByRole("button", { name: /씬 1/ }));
    await user.click(screen.getByRole("menuitem", { name: "이름 변경" }));
    expect(onRename).toHaveBeenCalledWith(scene);

    fireEvent.contextMenu(screen.getByRole("button", { name: /씬 1/ }));
    await user.click(screen.getByRole("menuitem", { name: "새 씬" }));
    expect(onCreateScene).toHaveBeenCalledWith(scene);

    fireEvent.contextMenu(screen.getByRole("button", { name: /씬 1/ }));
    await user.click(screen.getByRole("menuitem", { name: "새 장" }));
    expect(onCreateChapter).toHaveBeenCalledWith(scene);
  });

  it("opens scene move and delete actions from the popup menu", async () => {
    const user = userEvent.setup();
    const onMoveSceneUp = vi.fn();
    const onMoveSceneDown = vi.fn();
    const onDeleteScene = vi.fn();

    renderOutline({
      onMoveSceneUp,
      onMoveSceneDown,
      onDeleteScene,
    });

    fireEvent.contextMenu(screen.getByRole("button", { name: /씬 1/ }));
    await user.click(screen.getByRole("menuitem", { name: "위로 이동" }));
    expect(onMoveSceneUp).toHaveBeenCalledWith(scene);

    fireEvent.contextMenu(screen.getByRole("button", { name: /씬 1/ }));
    await user.click(screen.getByRole("menuitem", { name: "아래로 이동" }));
    expect(onMoveSceneDown).toHaveBeenCalledWith(scene);

    fireEvent.contextMenu(screen.getByRole("button", { name: /씬 1/ }));
    await user.click(screen.getByRole("menuitem", { name: "삭제" }));
    expect(onDeleteScene).toHaveBeenCalledWith(scene);
  });

  it("renders the outline chrome and menu actions in English when selected", async () => {
    const user = userEvent.setup();
    const onRename = vi.fn();
    mocks.settingsGet.mockResolvedValue({ language: "en" });

    renderOutline({ onRename });

    expect(await screen.findByText("Outline")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Scene 1/ })).toBeInTheDocument();
    expect(screen.getByText("Chapter 1")).toBeInTheDocument();
    fireEvent.contextMenu(screen.getByRole("button", { name: /Scene 1/ }));
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }));

    expect(onRename).toHaveBeenCalledWith(scene);
  });
});
