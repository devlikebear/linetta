import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_AI_CONTEXT_SELECTION } from "../ai/AIContextChecklist";
import { I18nProvider } from "../../lib/i18n";
import type { AIContextPreview, ContextCounts } from "../../lib/types";
import { CompanionPanel } from "./CompanionPanel";

const aiCounts: ContextCounts = {
  nearbyScenes: 0,
  hasOutline: false,
  hasSynopsis: false,
  relatedScenes: 0,
  entities: 0,
  relationships: 0,
  plotBeats: 0,
  notes: 0,
  projectMetaFields: 0,
  hasStyleNotes: false,
};

const aiPreview: AIContextPreview = {
  counts: aiCounts,
  selectedItemCount: 1,
  selectedCharCount: 10,
  selectedTokenEstimate: 4,
  budgetTokenEstimate: 4,
  sections: [
    {
      id: "current_scene",
      label: "현재 씬 본문",
      present: true,
      selected: true,
      count: 1,
      preview: "현재 씬 본문입니다.",
      charCount: 10,
      tokenEstimate: 4,
    },
  ],
};

const companionState = vi.hoisted(() => ({
  value: {
    messages: [] as {
      role: "user" | "assistant";
      content: string;
      proposal?: import("../../hooks/useCompanion").ChatMessage["proposal"];
      choices?: import("../../hooks/useCompanion").ChatMessage["choices"];
      nodeLabel?: string;
      scope?: "scene" | "project" | "global";
      errored?: boolean;
      retryText?: string;
      aiSetupIssue?: import("../../lib/types").AISetupIssue;
      rawError?: string;
    }[],
    streaming: "",
    thinking: "",
    reasoning: "",
    status: "idle",
    send: vi.fn(),
    cancel: vi.fn(),
    clear: vi.fn(),
    compact: vi.fn(),
    lastArgs: [] as unknown[],
  },
}));
const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  settingsSet: vi.fn(),
  companionPreviewContext: vi.fn(),
  companionReferencesList: vi.fn(),
  companionReferencesCreate: vi.fn(),
  companionReferencesUpdate: vi.fn(),
  companionReferencesDelete: vi.fn(),
}));

vi.mock("../../hooks/useCompanion", () => ({
  useCompanion: (...args: unknown[]) => {
    companionState.value.lastArgs = args;
    return companionState.value;
  },
}));

vi.mock("../../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
    set: mocks.settingsSet,
  },
  companion: {
    previewContext: mocks.companionPreviewContext,
    references: {
      list: mocks.companionReferencesList,
      create: mocks.companionReferencesCreate,
      update: mocks.companionReferencesUpdate,
      delete: mocks.companionReferencesDelete,
    },
  },
}));

function renderPanel(props: Partial<ComponentProps<typeof CompanionPanel>> = {}) {
  return render(
    <I18nProvider>
      <CompanionPanel projectId="p1" nodeIdRef={{ current: "n1" }} onClose={vi.fn()} onApplied={vi.fn()} {...props} />
    </I18nProvider>,
  );
}

describe("CompanionPanel", () => {
  beforeEach(() => {
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
    mocks.settingsSet.mockImplementation((patch: unknown) => Promise.resolve(patch));
    mocks.companionPreviewContext.mockResolvedValue({
      counts: {
        nearbyScenes: 1,
        hasOutline: true,
        hasSynopsis: false,
        relatedScenes: 0,
        entities: 0,
        relationships: 0,
        plotBeats: 0,
        notes: 0,
        projectMetaFields: 0,
        hasStyleNotes: false,
      },
      sections: [
        {
          id: "current_scene",
          label: "작성된 본문 발췌",
          present: true,
          selected: true,
          count: 1,
          preview: "씬 1\n인간의 개별성은 무엇일까?",
          charCount: 18,
          tokenEstimate: 6,
        },
        {
          id: "overview",
          label: "작품 개요",
          present: true,
          selected: true,
          count: 1,
          preview: "자의식을 다루는 소설",
          charCount: 11,
          tokenEstimate: 4,
        },
      ],
      selectedItemCount: 2,
      selectedCharCount: 29,
      selectedTokenEstimate: 10,
      budgetTokenEstimate: 10,
    });
    mocks.companionReferencesList.mockResolvedValue([]);
    mocks.companionReferencesCreate.mockResolvedValue({
      id: "r1",
      project_id: "p1",
      node_id: "n1",
      source_type: "clipboard",
      purpose: "style",
      title: "클립보드 레퍼런스",
      content: "담담한 문체",
      summary: "",
      char_count: 6,
      token_estimate: 2,
      status: "active",
      created_at: 1,
      updated_at: 1,
    });
    mocks.companionReferencesUpdate.mockResolvedValue({});
    mocks.companionReferencesDelete.mockResolvedValue({ ok: true });
    companionState.value = {
      messages: [],
      streaming: "",
      thinking: "",
      reasoning: "",
      status: "idle",
      send: vi.fn(),
      cancel: vi.fn(),
      clear: vi.fn(),
      compact: vi.fn(),
      lastArgs: [],
    };
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: vi.fn().mockResolvedValue(undefined),
        readText: vi.fn().mockResolvedValue("담담하고 절제된 문체를 참고해줘."),
      },
    });
  });

  it("shows preparation steps while streaming even before any prose", () => {
    companionState.value = { ...companionState.value, status: "streaming", streaming: "", thinking: "" };
    const { container } = renderPanel();

    expect(screen.getByText("응답 준비 중…")).toBeInTheDocument();
    expect(screen.getByText("요청 의도 확인")).toBeInTheDocument();
    expect(screen.getByText("현재 씬 맥락 정리")).toBeInTheDocument();
    expect(screen.getByText("초안 문장 구성")).toBeInTheDocument();
    expect(container.querySelector(".msg-bubble")).toBeNull();
  });

  it("shows writer actions in the empty state and copies one into the draft", async () => {
    const user = userEvent.setup();
    renderPanel();

    expect(screen.getByText("무엇부터 맡길까요?")).toBeInTheDocument();
    expect(screen.getByText("작가 액션")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /다음 문장 이어쓰기/ })).toBeInTheDocument();

    const action = screen.getByRole("button", {
      name: /다음 문장 이어쓰기/,
    });
    await user.click(action);

    expect((screen.getByPlaceholderText(/메시지/) as HTMLTextAreaElement).value).toContain("다음 3~5문장");
    expect(companionState.value.send).not.toHaveBeenCalled();
  });

  it("switches companion history scope between current scene and whole work", async () => {
    const user = userEvent.setup();
    renderPanel();

    expect(screen.getByRole("button", { name: "현재 씬" })).toHaveAttribute("aria-pressed", "true");
    expect(companionState.value.lastArgs[5]).toBe("scene");

    await user.click(screen.getByRole("button", { name: "작품 전체" }));

    expect(screen.getByRole("button", { name: "작품 전체" })).toHaveAttribute("aria-pressed", "true");
    expect(companionState.value.lastArgs[5]).toBe("project");
  });

  it("uses an explicit current node id for history even before the mutable ref catches up", () => {
    renderPanel({
      currentNodeId: "n1",
      nodeIdRef: { current: null },
    });

    expect(companionState.value.lastArgs[1]).toBe("n1");
    expect(companionState.value.lastArgs[5]).toBe("scene");
    expect(screen.getByRole("button", { name: "현재 씬" })).not.toBeDisabled();
  });

  it("shows scene chips for project-wide transcript messages", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [
        { role: "assistant", content: "현재 씬 본문을 반영했습니다.", nodeLabel: "식탁 위 고지서", scope: "scene" },
      ],
    };
    renderPanel();

    expect(screen.queryByText("식탁 위 고지서")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "작품 전체" }));

    expect(screen.getByText("식탁 위 고지서")).toBeInTheDocument();
  });

  it("renders AI draft controls inside the companion panel", async () => {
    const user = userEvent.setup();
    const onRun = vi.fn();
    renderPanel({
      aiDraft: {
        mode: "replace",
        canChooseMode: false,
        options: { tone: "my", short_form: true, context: DEFAULT_AI_CONTEXT_SELECTION },
        contextItemCount: 1,
        contextPreview: aiPreview,
        contextSelection: DEFAULT_AI_CONTEXT_SELECTION,
        variations: [],
        currentIdx: 0,
        status: { kind: "idle" },
        onModeChange: vi.fn(),
        onOptionsChange: vi.fn(),
        onContextSelectionChange: vi.fn(),
        onRun,
        onSwitch: vi.fn(),
        onAccept: vi.fn(),
        onCancel: vi.fn(),
        onContextClick: vi.fn(),
        showChecklist: false,
      },
    });

    expect(screen.getByText("AI 생성")).toBeInTheDocument();
    expect(screen.queryByText("무엇부터 맡길까요?")).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/메시지/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "전송" })).not.toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("프롬프트를 입력하세요…"), "문장을 더 선명하게");
    await user.click(screen.getByRole("button", { name: "생성 ⌘↵" }));

    expect(onRun).toHaveBeenCalledWith("문장을 더 선명하게", false);
    expect(companionState.value.send).not.toHaveBeenCalled();
  });

  it("prefills a companion rewrite prompt for selected editor text", () => {
    renderPanel({
      selectionRewriteRequest: {
        id: "sel-1",
        text: "@브란이 말 없이 지도를 가장자리로 당겼다.",
      },
    });

    const input = screen.getByPlaceholderText(/메시지/) as HTMLTextAreaElement;
    expect(input.value).toContain("선택한 문장을");
    expect(input.value).toContain("@브란이 말 없이 지도를 가장자리로 당겼다.");
    expect(input.value).toContain("set_scene_text");
    expect(companionState.value.send).not.toHaveBeenCalled();
  });

  it("prefills and sends a proofread prompt for selected editor text", async () => {
    const user = userEvent.setup();
    renderPanel({
      selectionRewriteRequest: {
        id: "sel-proofread-1",
        kind: "proofread",
        text: "나는 그말을 믿을수 없엇다.",
      },
    });

    const input = screen.getByPlaceholderText(/메시지/) as HTMLTextAreaElement;
    expect(input.value).toContain("맞춤법");
    expect(input.value).toContain("고유명사");
    expect(input.value).toContain("변경 목록");
    expect(input.value).toContain("나는 그말을 믿을수 없엇다.");

    await user.click(screen.getByRole("button", { name: "전송" }));

    await waitFor(() => {
      expect(companionState.value.send).toHaveBeenCalledWith(expect.stringContaining("맞춤법"));
    });
  });

  it("grows the draft textarea when a picked example wraps to multiple lines", async () => {
    const user = userEvent.setup();
    Object.defineProperty(HTMLTextAreaElement.prototype, "scrollHeight", {
      configurable: true,
      get() {
        return this.value.includes("현재 씬") ? 72 : 32;
      },
    });

    renderPanel();

    const input = screen.getByPlaceholderText(/메시지/) as HTMLTextAreaElement;
    expect(input.style.height).toBe("32px");

    await user.click(screen.getByRole("button", { name: /장면 긴장 강화/ }));

    expect(input.value).toContain("현재 씬");
    expect(input.style.height).toBe("72px");
  });

  it("toggles built-in tool help from the companion header", async () => {
    const user = userEvent.setup();
    renderPanel();

    expect(screen.queryByText("컴패니언이 사용할 수 있는 도구")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "컴패니언 도움말" }));

    expect(screen.getByText("컴패니언이 사용할 수 있는 도구")).toBeInTheDocument();
    expect(screen.getByText("web_search · 최신 자료나 장르 레퍼런스 찾기")).toBeInTheDocument();
    expect(screen.getByText("web_fetch · 특정 URL 본문 확인")).toBeInTheDocument();
    expect(screen.getByText("linetta_apply_ops · 개요, 스토리라인, 비트, 세계관 요소, 관계, 씬, 기억 갱신")).toBeInTheDocument();
  });

  it("lets writers inspect and disable companion context injection", async () => {
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("button", { name: "컨텍스트 확인" }));

    await waitFor(() => {
      expect(mocks.companionPreviewContext).toHaveBeenCalledWith("p1", "n1", {
        context: DEFAULT_AI_CONTEXT_SELECTION,
      });
    });

    expect(screen.getByText("작성된 본문 발췌")).toBeInTheDocument();
    expect(screen.getByText("작품 개요")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "작성된 본문 발췌 미리보기" }));
    expect(screen.getByText(/인간의 개별성/)).toBeInTheDocument();

    await user.click(screen.getByRole("checkbox", { name: "작성된 본문 발췌" }));

    await waitFor(() => {
      expect(mocks.companionPreviewContext).toHaveBeenLastCalledWith("p1", "n1", {
        context: {
          ...DEFAULT_AI_CONTEXT_SELECTION,
          current_scene: false,
        },
      });
    });
  });

  it("shows token estimates in the context panel and chip", async () => {
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("button", { name: "컨텍스트 확인" }));

    expect(await screen.findByText("선택 컨텍스트 ~10")).toBeInTheDocument();
    expect(screen.getByText("2개 항목")).toBeInTheDocument();
    expect(screen.getByText("~6")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "컨텍스트 확인" })).toHaveTextContent("ctx ~10");
  });

  it("offers quick actions when the selected context is large", async () => {
    const user = userEvent.setup();
    mocks.companionPreviewContext.mockResolvedValueOnce({
      counts: {
        nearbyScenes: 1,
        hasOutline: true,
        hasSynopsis: false,
        relatedScenes: 0,
        entities: 0,
        relationships: 0,
        plotBeats: 0,
        notes: 0,
        projectMetaFields: 0,
        hasStyleNotes: false,
      },
      sections: [{
        id: "current_scene",
        label: "작성된 본문 발췌",
        present: true,
        selected: true,
        count: 1,
        preview: "긴 본문",
        charCount: 39000,
        tokenEstimate: 13000,
      }],
      selectedItemCount: 1,
      selectedCharCount: 39000,
      selectedTokenEstimate: 13000,
      budgetTokenEstimate: 13000,
    });
    renderPanel();

    await user.click(screen.getByRole("button", { name: "컨텍스트 확인" }));

    expect(await screen.findByText("현재 씬만")).toBeInTheDocument();
    expect(screen.getByText("레퍼런스 요약")).toBeInTheDocument();
    expect(screen.getByText("대화 요약")).toBeInTheDocument();
  });

  it("adds a clipboard reference for the current scene", async () => {
    const user = userEvent.setup();
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: vi.fn().mockResolvedValue(undefined),
        readText: vi.fn().mockResolvedValue("담담하고 절제된 문체를 참고해줘."),
      },
    });
    renderPanel();

    await user.click(screen.getByRole("button", { name: "컨텍스트 확인" }));
    await user.click(await screen.findByRole("button", { name: "클립보드" }));

    expect(await screen.findByLabelText("레퍼런스 내용")).toHaveValue("담담하고 절제된 문체를 참고해줘.");
    expect(screen.getByLabelText("레퍼런스 목적")).toHaveValue("style");

    await user.click(screen.getByRole("button", { name: "레퍼런스 추가" }));

    await waitFor(() => {
      expect(mocks.companionReferencesCreate).toHaveBeenCalledWith(expect.objectContaining({
        project_id: "p1",
        node_id: "n1",
        source_type: "clipboard",
        purpose: "style",
        content: "담담하고 절제된 문체를 참고해줘.",
      }));
    });
  });

  it("renders provider reasoning in a collapsible block while streaming", () => {
    companionState.value = {
      ...companionState.value,
      status: "streaming",
      streaming: "초안",
      reasoning: "주인공의 동기를 먼저 정한다",
    };
    renderPanel();
    expect(screen.getByText("추론 중…")).toBeInTheDocument();
    expect(screen.getByText("주인공의 동기를 먼저 정한다")).toBeInTheDocument();
  });

  it("sends non-empty messages", async () => {
    const user = userEvent.setup();
    renderPanel();

    await user.type(screen.getByPlaceholderText(/메시지/), "도와줘");
    await user.click(screen.getByRole("button", { name: "전송" }));

    expect(companionState.value.send).toHaveBeenCalledWith("도와줘");
  });

  it("renders AI setup provider errors as a rescue card instead of a raw error bubble", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "api key is required for auth mode api-key",
        rawError: "api key is required for auth mode api-key",
        errored: true,
        aiSetupIssue: "missing_key",
        retryText: "현재 씬 본문 써줘",
      }],
    };
    const { container } = renderPanel();

    expect(screen.getByText("AI 연결이 필요해요")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "가장 쉬운 방법으로 연결" })).toBeInTheDocument();
    expect(container.querySelector(".msg-bubble.errored")).toBeNull();

    await user.click(screen.getByRole("button", { name: "방금 질문 다시 보내기" }));

    expect(companionState.value.send).toHaveBeenCalledWith("현재 씬 본문 써줘");
  });

  it("opens the shared AI setup modal from a rescue card", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "401 unauthorized",
        rawError: "401 unauthorized",
        errored: true,
        aiSetupIssue: "auth_required",
      }],
    };
    renderPanel();

    await user.click(screen.getByRole("button", { name: "가장 쉬운 방법으로 연결" }));

    expect(screen.getByRole("dialog", { name: "AI 연결 마법사" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /ChatGPT 구독으로 연결/ })).toBeInTheDocument();
  });

  it("keeps non-setup companion errors in the existing errored bubble", () => {
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "본문 변경이 만들어지지 않았습니다.",
        errored: true,
        retryText: "현재 씬 본문 써줘",
      }],
    };
    const { container } = renderPanel();

    expect(screen.getByText("본문 변경이 만들어지지 않았습니다.")).toBeInTheDocument();
    expect(screen.queryByText("AI 연결이 필요해요")).not.toBeInTheDocument();
    expect(container.querySelector(".msg-bubble.errored")).not.toBeNull();
  });

  it("attaches an image from the file picker and sends it with the prompt", async () => {
    const user = userEvent.setup();
    renderPanel();

    const file = new File([new Uint8Array([1, 2, 3])], "scene.png", { type: "image/png" });
    await user.upload(screen.getByLabelText("이미지 첨부"), file);

    expect(await screen.findByText("scene.png")).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText(/메시지/), "이 이미지 참고해줘");
    await user.click(screen.getByRole("button", { name: "전송" }));

    await waitFor(() => {
      expect(companionState.value.send).toHaveBeenCalledWith("이 이미지 참고해줘", [
        expect.objectContaining({
          name: "scene.png",
          media_type: "image/png",
          data: expect.any(String),
          size: 3,
        }),
      ]);
    });
  });

  it("attaches a pasted clipboard image before sending", async () => {
    const user = userEvent.setup();
    renderPanel();

    const input = screen.getByPlaceholderText(/메시지/);
    const file = new File([new Uint8Array([9, 8])], "pasted.png", { type: "image/png" });
    fireEvent.paste(input, {
      clipboardData: {
        items: [{ kind: "file", type: "image/png", getAsFile: () => file }],
        files: [file],
      },
    });

    expect(await screen.findByText("pasted.png")).toBeInTheDocument();

    await user.type(input, "붙여넣은 이미지 봐줘");
    await user.click(screen.getByRole("button", { name: "전송" }));

    await waitFor(() => {
      expect(companionState.value.send).toHaveBeenCalledWith("붙여넣은 이미지 봐줘", [
        expect.objectContaining({
          name: "pasted.png",
          media_type: "image/png",
          data: expect.any(String),
          size: 2,
        }),
      ]);
    });
  });

  it("waits for the editor flush before sending messages", async () => {
    const user = userEvent.setup();
    let resolveFlush!: () => void;
    const beforeSend = vi.fn(() => new Promise<void>((resolve) => { resolveFlush = resolve; }));
    renderPanel({ beforeSend });

    await user.type(screen.getByPlaceholderText(/메시지/), "본문 보고 분석해줘");
    await user.click(screen.getByRole("button", { name: "전송" }));

    await waitFor(() => expect(beforeSend).toHaveBeenCalledOnce());
    expect(companionState.value.send).not.toHaveBeenCalled();

    resolveFlush();
    await waitFor(() => expect(companionState.value.send).toHaveBeenCalledWith("본문 보고 분석해줘"));
  });

  it("shows thinking state and hides query/proposal fences from live prose", () => {
    companionState.value = {
      ...companionState.value,
      status: "streaming",
      thinking: "씬 조회 중",
      streaming: "확인 중\n```linetta-query\n{}\n```\n숨겨질 내용",
    };

    renderPanel();

    expect(screen.getByText(/씬 조회 중/)).toBeInTheDocument();
    expect(screen.getByText("확인 중")).toBeInTheDocument();
    expect(screen.queryByText(/linetta-query/)).not.toBeInTheDocument();
  });

  it("renders proposal cards from assistant messages", () => {
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "이렇게 해볼게요",
        proposal: {
          run_id: "r1",
          valid: true,
          summary: "제안",
          ops: [{ op: "create_thread", name: "추적자" }],
        },
      }],
    };

    renderPanel();

    expect(screen.getByText("제안")).toBeInTheDocument();
    expect(screen.getByText("스토리라인 생성: 추적자")).toBeInTheDocument();
  });

  it("renders fact-card proposal operations", () => {
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "자료집에 저장할게요.",
        proposal: {
          run_id: "r1",
          valid: true,
          summary: "자료집 저장",
          ops: [{
            op: "create_fact_card",
            claim: "런던 일반 경찰은 항상 총기를 휴대한다",
            result: "일반 경찰은 통상 비무장이다.",
            status: "verified",
            sources: [{ url: "https://www.met.police.uk/" }],
          }],
        },
      }],
    };

    renderPanel();

    expect(screen.getByText("자료집 저장")).toBeInTheDocument();
    expect(screen.getByText("자료집 카드 생성: 런던 일반 경찰은 항상 총기를 휴대한다")).toBeInTheDocument();
  });

  it("renders choice buttons and sends the picked option on click", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "어떤 제목으로 할까요?",
        choices: {
          run_id: "r1",
          prompt: "새 제목?",
          options: ["「부엌」", "「온기」"],
          allow_custom: true,
        },
      }],
    };

    renderPanel();

    expect(screen.getByText("새 제목?")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "직접 입력" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "「온기」" }));
    expect(companionState.value.send).toHaveBeenCalledWith("「온기」");
  });

  it("copies visible chat messages to the clipboard", async () => {
    const user = userEvent.setup();
    const writeText = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
    companionState.value = {
      ...companionState.value,
      messages: [
        { role: "user", content: "이 장면 이상해?" },
        { role: "assistant", content: "동기가 조금 더 필요해요." },
      ],
    };

    renderPanel();

    await user.click(screen.getByRole("button", { name: "대화 복사" }));

    expect(writeText).toHaveBeenCalledWith(expect.stringContaining("나:\n이 장면 이상해?"));
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining("컴패니언:\n동기가 조금 더 필요해요."));
  });

  it("copies an individual message bubble to the clipboard", async () => {
    const user = userEvent.setup();
    const writeText = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
    companionState.value = {
      ...companionState.value,
      messages: [
        { role: "user", content: "개별성의 비트를 잡아줘" },
        { role: "assistant", content: "자아 인식의 균열을 첫 장면에 배치해보세요." },
      ],
    };

    renderPanel();

    await user.click(screen.getByRole("button", { name: "메시지 복사: 자아 인식의 균열을 첫 장면에 배치해보세요." }));

    expect(writeText).toHaveBeenCalledWith("자아 인식의 균열을 첫 장면에 배치해보세요.");
  });

  it("compacts and clears chat through panel actions", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [
        { role: "user", content: "요약해줘" },
        { role: "assistant", content: "요약했습니다." },
      ],
    };

    renderPanel();

    await user.click(screen.getByRole("button", { name: "대화 압축" }));
    await user.click(screen.getByRole("button", { name: "대화 클리어" }));

    expect(companionState.value.compact).toHaveBeenCalledOnce();
    expect(companionState.value.clear).toHaveBeenCalledOnce();
  });

  it("renders companion controls in English when selected", async () => {
    mocks.settingsGet.mockResolvedValue({ language: "en" });

    renderPanel();

    expect(await screen.findByText("Writing companion")).toBeInTheDocument();
    expect(screen.getByText("Writer actions")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Continue the scene/ })).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Message... (Enter to send, Shift+Enter for line break)")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send" })).toBeInTheDocument();
  });
});
