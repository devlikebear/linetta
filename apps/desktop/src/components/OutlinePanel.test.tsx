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

  it("opens part and container maintenance actions from the popup menu", async () => {
    const user = userEvent.setup();
    const onCreatePart = vi.fn();
    const onMoveNodeUp = vi.fn();
    const onMoveNodeDown = vi.fn();
    const onDeleteNode = vi.fn();

    renderOutline({
      onCreatePart,
      onMoveNodeUp,
      onMoveNodeDown,
      onDeleteNode,
    });

    fireEvent.contextMenu(screen.getByText("1장"));
    await user.click(screen.getByRole("menuitem", { name: "새 부" }));
    expect(onCreatePart).toHaveBeenCalledWith(chapter);

    fireEvent.contextMenu(screen.getByText("1장"));
    await user.click(screen.getByRole("menuitem", { name: "위로 이동" }));
    expect(onMoveNodeUp).toHaveBeenCalledWith(chapter);

    fireEvent.contextMenu(screen.getByText("1장"));
    await user.click(screen.getByRole("menuitem", { name: "아래로 이동" }));
    expect(onMoveNodeDown).toHaveBeenCalledWith(chapter);

    fireEvent.contextMenu(screen.getByText("1장"));
    await user.click(screen.getByRole("menuitem", { name: "삭제" }));
    expect(onDeleteNode).toHaveBeenCalledWith(chapter);
  });

  it("checks outline issues and offers automatic repair", async () => {
    const user = userEvent.setup();
    const onRepairOutline = vi.fn();
    const onUndoRepairOutline = vi.fn();
    const duplicate: TreeNode = { ...scene, id: "scene-2", ordinal: 1, label: "씬 1" };
    const emptyChapter: TreeNode = { ...chapter, id: "chapter-empty", ordinal: 1, label: "1장", children: [] };
    const directScene: TreeNode = { ...scene, id: "scene-direct", parent_id: "part-1", ordinal: 0, label: "씬 2" };
    const part: TreeNode = { ...chapter, id: "part-1", label: "1부", children: [directScene] };

    renderOutline({
      tree: [{ ...chapter, children: [scene, duplicate] }, emptyChapter, part],
      onRepairOutline,
      onUndoRepairOutline,
      canUndoRepair: true,
    });

    await user.click(screen.getByRole("button", { name: "아웃라인 점검" }));

    expect(screen.getByText("점검 결과")).toBeInTheDocument();
    expect(screen.getAllByText(/같은 단계에 중복된 이름/).length).toBeGreaterThan(0);
    expect(screen.getByText(/빈 장 또는 부/)).toBeInTheDocument();
    expect(screen.getAllByText(/장 밖에 있는 씬/).length).toBeGreaterThan(0);

    await user.click(screen.getByRole("button", { name: "자동 정리" }));
    expect(onRepairOutline).toHaveBeenCalledOnce();

    await user.click(screen.getByRole("button", { name: "마지막 정리 되돌리기" }));
    expect(onUndoRepairOutline).toHaveBeenCalledOnce();
  });

  it("does not flag a top-level part title containing 장 as a root chapter but suggests label cleanup", async () => {
    const user = userEvent.setup();
    const expansionScene: TreeNode = { ...scene, id: "scene-expansion", parent_id: "chapter-expansion" };
    const expansionChapter: TreeNode = {
      ...chapter,
      id: "chapter-expansion",
      parent_id: "part-expansion",
      label: "1장 - 경계의 틈",
      children: [expansionScene],
    };
    const expansionPart: TreeNode = {
      ...chapter,
      id: "part-expansion",
      parent_id: undefined,
      label: "확장된 동화성",
      title: "두 번째 부",
      children: [expansionChapter],
    };

    renderOutline({ tree: [expansionPart], onRepairOutline: vi.fn() });

    await user.click(screen.getByRole("button", { name: "아웃라인 점검" }));

    expect(screen.queryByText(/부 밖에 있는 장: 확장된 동화성/)).not.toBeInTheDocument();
    expect(screen.getByText(/번호\/표시제목 정리 필요: 확장된 동화성/)).toBeInTheDocument();
    expect(screen.queryByText("문제 없음")).not.toBeInTheDocument();
  });

  it("flags a part-like container nested under another part", async () => {
    const user = userEvent.setup();
    const nestedChapter: TreeNode = {
      ...chapter,
      id: "chapter-nested",
      parent_id: "part-nested",
      label: "1장 - 멈춰버린 약속",
      children: [],
    };
    const nestedPart: TreeNode = {
      ...chapter,
      id: "part-nested",
      parent_id: "part-root",
      label: "개별성의 경계선 2차 - 2027, AGI와 자아의 재정의",
      children: [nestedChapter],
    };
    const rootPart: TreeNode = {
      ...chapter,
      id: "part-root",
      label: "개별성의 경계선",
      children: [nestedPart],
    };

    renderOutline({ tree: [rootPart], onRepairOutline: vi.fn() });

    await user.click(screen.getByRole("button", { name: "아웃라인 점검" }));

    expect(screen.getByText(/다른 부\/장 안에 들어간 부/)).toBeInTheDocument();
  });

  it("flags chapter containers nested inside another chapter", async () => {
    const user = userEvent.setup();
    const nestedChapter: TreeNode = {
      ...chapter,
      id: "chapter-nested",
      parent_id: "chapter-wrapper",
      label: "1장 - 경계의 틈",
      children: [],
    };
    const wrapper: TreeNode = {
      ...chapter,
      id: "chapter-wrapper",
      parent_id: "part-root",
      label: "1장",
      children: [nestedChapter],
    };
    const rootPart: TreeNode = {
      ...chapter,
      id: "part-root",
      label: "개별성의 경계선",
      children: [wrapper],
    };

    renderOutline({ tree: [rootPart], onRepairOutline: vi.fn() });

    await user.click(screen.getByRole("button", { name: "아웃라인 점검" }));

    expect(screen.getByText(/장 안에 들어간 장\/부/)).toBeInTheDocument();
  });

  it("flags structural markers saved as empty scene rows", async () => {
    const user = userEvent.setup();
    const chapterMarker: TreeNode = {
      ...scene,
      id: "chapter-marker",
      parent_id: "chapter-wrapper",
      label: "1장 - 경계의 틈",
      title: "",
      word_count: 0,
    };
    const partMarker: TreeNode = {
      ...scene,
      id: "part-marker",
      parent_id: "chapter-wrapper",
      label: "개별성의 경계선 2차 - 2027, AGI와 자아의 재정의",
      title: "",
      word_count: 0,
    };
    const nextChapterMarker: TreeNode = {
      ...scene,
      id: "next-chapter-marker",
      parent_id: "chapter-wrapper",
      label: "1장 - 멈춰버린 약속",
      title: "",
      word_count: 0,
    };
    const wrapper: TreeNode = {
      ...chapter,
      id: "chapter-wrapper",
      parent_id: "part-root",
      label: "1장",
      children: [scene, chapterMarker, partMarker, nextChapterMarker],
    };
    const rootPart: TreeNode = {
      ...chapter,
      id: "part-root",
      label: "개별성의 경계선",
      children: [wrapper],
    };

    renderOutline({ tree: [rootPart], onRepairOutline: vi.fn() });

    await user.click(screen.getByRole("button", { name: "아웃라인 점검" }));

    expect(screen.getAllByText(/씬으로 저장된 장/).length).toBeGreaterThan(0);
    expect(screen.getByText(/씬으로 저장된 부 후보/)).toBeInTheDocument();
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
