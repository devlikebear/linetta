import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps, RefObject } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../lib/i18n";
import type { ManuscriptSearchHit, ReplacePlan } from "../../lib/types";
import type { TiptapHandle } from "../editor/Tiptap";
import { ContextualEditPanel } from "./ContextualEditPanel";

const mocks = vi.hoisted(() => ({
  manuscriptSearch: vi.fn(),
  manuscriptReplacePreview: vi.fn(),
  manuscriptReplaceApply: vi.fn(),
  settingsGet: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
  manuscript: {
    search: mocks.manuscriptSearch,
    replacePreview: mocks.manuscriptReplacePreview,
    replaceApply: mocks.manuscriptReplaceApply,
  },
}));

function editorRef(overrides: Partial<TiptapHandle> = {}): RefObject<TiptapHandle> {
  return {
    current: {
      focus: vi.fn(),
      getDoc: vi.fn(() => ({})),
      getSelection: vi.fn(() => null),
      setSelection: vi.fn(),
      findText: vi.fn(() => ({ count: 2, activeIndex: 0 })),
      nextMatch: vi.fn(),
      prevMatch: vi.fn(),
      replaceActiveMatch: vi.fn(() => ({})),
      replaceAllMatches: vi.fn(() => ({})),
      addNoteMarker: vi.fn(),
      removeNoteMarker: vi.fn(),
      editor: null,
      ...overrides,
    } as TiptapHandle,
  };
}

function renderPanel(props: Partial<ComponentProps<typeof ContextualEditPanel>> = {}) {
  const ref = props.editorRef ?? editorRef();
  const onNavigateNode = props.onNavigateNode ?? vi.fn();
  const onBatchApplied = props.onBatchApplied ?? vi.fn();
  render(
    <I18nProvider>
      <ContextualEditPanel
        open
        projectId="project-1"
        currentNodeId="scene-1"
        editorRef={ref}
        onNavigateNode={onNavigateNode}
        onBatchApplied={onBatchApplied}
        onClose={vi.fn()}
        {...props}
      />
    </I18nProvider>,
  );
  return { ref, onNavigateNode };
}

describe("ContextualEditPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
    mocks.manuscriptSearch.mockResolvedValue([
      {
        node_id: "scene-2",
        breadcrumb: "1부 / 2장 / 씬 2",
        snippet: "문 뒤에서 열쇠가 굴러 나왔다.",
        updated_at: 10,
      } satisfies ManuscriptSearchHit,
    ]);
    mocks.manuscriptReplacePreview.mockResolvedValue({
      project_id: "project-1",
      query: "열쇠",
      replacement: "반지",
      candidates: [
        {
          id: "scene-2:1",
          node_id: "scene-2",
          breadcrumb: "1부 / 2장 / 씬 2",
          before: "문 뒤에서 열쇠가 굴러 나왔다.",
          after: "문 뒤에서 반지가 굴러 나왔다.",
          occurrences: 1,
          selected: true,
          preview_version: 1,
        },
      ],
    } satisfies ReplacePlan);
    mocks.manuscriptReplaceApply.mockResolvedValue({
      applied: 1,
      skipped: 0,
      failures: [],
      changed_node_ids: ["scene-2"],
    });
  });

  it("uses the current scene editor API for find and replace", async () => {
    const user = userEvent.setup();
    const ref = editorRef();
    renderPanel({ editorRef: ref });

    await user.type(screen.getByLabelText("찾을 단어"), "민호");
    await waitFor(() => expect(ref.current?.findText).toHaveBeenLastCalledWith("민호", { select: false }));
    expect(screen.getByText("1 / 2")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "다음" }));
    expect(ref.current?.findText).toHaveBeenLastCalledWith("민호", { select: true });
    expect(ref.current?.nextMatch).toHaveBeenCalled();

    await user.type(screen.getByLabelText("바꿀 단어"), "민준");
    await user.click(screen.getByRole("button", { name: "현재 항목 바꾸기" }));
    expect(ref.current?.replaceActiveMatch).toHaveBeenCalledWith("민준");

    await user.click(screen.getByRole("button", { name: "이 씬 전체 바꾸기" }));
    expect(ref.current?.replaceAllMatches).toHaveBeenCalledWith("민호", "민준");
  });

  it("keeps focus in the scene query input while passively counting matches", async () => {
    const user = userEvent.setup();
    const editorFocusTarget = document.createElement("button");
    document.body.appendChild(editorFocusTarget);
    const ref = editorRef({
      findText: vi.fn((_query, options) => {
        if (options?.select !== false) editorFocusTarget.focus();
        return { count: 2, activeIndex: 0 };
      }),
    });
    renderPanel({ editorRef: ref });

    const input = screen.getByLabelText("찾을 단어");
    await user.click(input);
    await user.type(input, "저");

    await waitFor(() => expect(ref.current?.findText).toHaveBeenLastCalledWith("저", { select: false }));
    expect(input).toHaveFocus();

    await user.keyboard("{Enter}");
    await waitFor(() => expect(ref.current?.findText).toHaveBeenLastCalledWith("저", { select: true }));
    expect(editorFocusTarget).toHaveFocus();

    editorFocusTarget.remove();
  });

  it("searches the whole manuscript and navigates to a result", async () => {
    const user = userEvent.setup();
    const onNavigateNode = vi.fn();
    renderPanel({ onNavigateNode });

    await user.click(screen.getByRole("tab", { name: "작품 전체" }));
    await user.type(screen.getByLabelText("작품 전체에서 찾을 단어"), "열쇠");

    await waitFor(() => expect(mocks.manuscriptSearch).toHaveBeenCalledWith("project-1", "열쇠", 20));
    await user.click(await screen.findByRole("button", { name: /1부 \/ 2장 \/ 씬 2/ }));

    expect(onNavigateNode).toHaveBeenCalledWith("scene-2");
  });

  it("previews and applies whole-work replace candidates", async () => {
    const user = userEvent.setup();
    const onBatchApplied = vi.fn();
    renderPanel({ onBatchApplied });

    await user.click(screen.getByRole("tab", { name: "작품 전체" }));
    await user.type(screen.getByLabelText("작품 전체에서 찾을 단어"), "열쇠");
    await user.type(screen.getByLabelText("바꿀 단어"), "반지");
    await user.click(screen.getByRole("button", { name: "미리보기" }));

    await waitFor(() => expect(mocks.manuscriptReplacePreview).toHaveBeenCalledWith("project-1", "열쇠", "반지"));
    expect(await screen.findByText("1부 / 2장 / 씬 2")).toBeInTheDocument();
    expect(screen.getByText("문 뒤에서 반지가 굴러 나왔다.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "선택 적용" }));
    await waitFor(() => expect(mocks.manuscriptReplaceApply).toHaveBeenCalledWith(
      expect.objectContaining({ query: "열쇠", replacement: "반지" }),
      ["scene-2:1"],
    ));
    expect(onBatchApplied).toHaveBeenCalledWith(["scene-2"]);
    expect(await screen.findByText("1개 씬에 적용했습니다.")).toBeInTheDocument();
  });
});
