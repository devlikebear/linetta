import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { FactBookPanel } from "./FactBookPanel";

const companionState = vi.hoisted(() => ({
  value: {
    messages: [] as import("../hooks/useCompanion").ChatMessage[],
    streaming: "",
    thinking: "",
    reasoning: "",
    status: "idle",
    send: vi.fn(),
    cancel: vi.fn(),
    clear: vi.fn(),
    compact: vi.fn(),
  },
}));

const companionHook = vi.hoisted(() => ({
  onApplied: undefined as (() => void) | undefined,
}));

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  factsList: vi.fn(),
  factsCreateFromUrl: vi.fn(),
  factsDelete: vi.fn(),
  companionApplyOps: vi.fn(),
}));

vi.mock("../hooks/useCompanion", () => ({
  useCompanion: (_projectId: string, _nodeIdRef: { current: string | null }, onApplied?: () => void) => {
    companionHook.onApplied = onApplied;
    return companionState.value;
  },
}));

vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
  facts: {
    list: mocks.factsList,
    createFromUrl: mocks.factsCreateFromUrl,
    delete: mocks.factsDelete,
  },
  companion: {
    applyOps: mocks.companionApplyOps,
  },
}));

function renderPanel(props: Partial<ComponentProps<typeof FactBookPanel>> = {}) {
  return render(
    <I18nProvider>
      <FactBookPanel
        projectId="project-1"
        nodeId="node-1"
        sceneLabel="씬 1"
        beforeReview={vi.fn()}
        onClose={vi.fn()}
        onChanged={vi.fn()}
        {...props}
      />
    </I18nProvider>,
  );
}

describe("FactBookPanel", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
    mocks.factsDelete.mockResolvedValue({ ok: true });
    mocks.factsCreateFromUrl.mockResolvedValue({
      id: "fact-url",
      project_id: "project-1",
      node_id: "node-1",
      claim: "런던 경찰 총기 휴대",
      result: "직접 입력한 출처 URL에서 확인했습니다.",
      status: "uncertain",
      category: "",
      sources: [{ id: "src-url", card_id: "fact-url", url: "https://www.met.police.uk/", title: "Met Police", snippet: "official", accessed_at: 200 }],
      created_at: 200,
      updated_at: 200,
    });
    mocks.companionApplyOps.mockResolvedValue({ applied: 1, failures: [] });
    mocks.factsList.mockResolvedValue([
      {
        id: "fact-1",
        project_id: "project-1",
        node_id: "node-1",
        claim: "런던 일반 경찰은 항상 총기를 휴대한다",
        result: "일반 경찰은 통상 비무장 근무이며 무장 경찰은 별도 단위다.",
        status: "verified",
        category: "police",
        sources: [{ id: "src-1", card_id: "fact-1", url: "https://www.met.police.uk/", title: "Met Police", snippet: "", accessed_at: 100 }],
        created_at: 100,
        updated_at: 100,
      },
    ]);
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
    };
    companionHook.onApplied = undefined;
  });

  it("renders saved fact cards with source links", async () => {
    renderPanel();

    expect(await screen.findByText("런던 일반 경찰은 항상 총기를 휴대한다")).toBeInTheDocument();
    expect(screen.getByText("검증됨")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Met Police" })).toHaveAttribute("href", "https://www.met.police.uk/");
    expect(mocks.factsList).toHaveBeenCalledWith("project-1", "node-1");
  });

  it("deletes a fact card and reloads the panel", async () => {
    const user = userEvent.setup();
    renderPanel();

    await screen.findByText("런던 일반 경찰은 항상 총기를 휴대한다");
    await user.click(screen.getByRole("button", { name: "자료 삭제" }));

    expect(mocks.factsDelete).toHaveBeenCalledWith("fact-1");
    await waitFor(() => expect(mocks.factsList.mock.calls.length).toBeGreaterThanOrEqual(2));
  });

  it("flushes the editor and asks companion for choice-only fact candidates", async () => {
    const user = userEvent.setup();
    const beforeReview = vi.fn().mockResolvedValue(undefined);
    renderPanel({ beforeReview });

    await user.click(await screen.findByRole("button", { name: "현재 씬 검토" }));

    await waitFor(() => expect(beforeReview).toHaveBeenCalledOnce());
    expect(companionState.value.send).toHaveBeenCalledWith(expect.stringContaining("linetta-choices"));
    expect(companionState.value.send).toHaveBeenCalledWith(expect.stringContaining("검색 후 자료집에 저장"));
    expect(companionState.value.send).toHaveBeenCalledWith(expect.stringContaining("web_search"));
  });

  it("runs an editor-selected claim request when opened from the selection menu", async () => {
    const beforeReview = vi.fn().mockResolvedValue(undefined);
    renderPanel({
      beforeReview,
      selectedClaimRequest: { id: "selection-1", claim: "비 온 뒤 흙냄새가 지오스민 때문인지 확인" },
    });

    await waitFor(() => expect(beforeReview).toHaveBeenCalledOnce());
    expect(companionState.value.send).toHaveBeenCalledWith(expect.stringContaining("선택한 주장: 비 온 뒤 흙냄새가 지오스민 때문인지 확인"));
    expect(companionState.value.send).toHaveBeenCalledWith(expect.stringContaining("web_search"));
    expect(companionState.value.send).toHaveBeenCalledWith(expect.stringContaining("create_fact_card"));
  });

  it("shows companion choices and sends the picked candidate", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "후보를 골라주세요.",
        choices: {
          run_id: "run-1",
          prompt: "검토할 후보",
          options: ["검색 후 자료집에 저장: 런던 경찰 총기 휴대"],
          allow_custom: false,
        },
      }],
    };
    renderPanel();

    await user.click(await screen.findByRole("button", { name: "검색 후 자료집에 저장: 런던 경찰 총기 휴대" }));

    expect(companionState.value.send).toHaveBeenCalledWith(expect.stringContaining("런던 경찰 총기 휴대"));
    expect(companionState.value.send).toHaveBeenCalledWith(expect.stringContaining("web_search"));
    expect(companionState.value.send).toHaveBeenCalledWith(expect.stringContaining("create_fact_card"));
  });

  it("keeps remaining fact candidates active after one candidate is saved", async () => {
    const user = userEvent.setup();
    const firstClaim = "비 온 뒤 흙냄새의 주된 원인이 토양 미생물 유래 지오스민인지 확인";
    const secondClaim = "뇌우·번개 뒤 공기에서 오존 냄새가 실제로 감지될 수 있는지 확인";
    mocks.factsList
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([{
        id: "fact-saved",
        project_id: "project-1",
        node_id: "node-1",
        claim: firstClaim,
        result: "저장됨",
        status: "verified",
        category: "",
        sources: [{ id: "src-saved", card_id: "fact-saved", url: "https://example.com/fact", title: "Example", snippet: "", accessed_at: 100 }],
        created_at: 100,
        updated_at: 100,
      }]);
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "후보를 골라주세요.",
        choices: {
          run_id: "run-1",
          prompt: "검토할 후보",
          options: [
            `검색 후 자료집에 저장: ${firstClaim}`,
            `검색 후 자료집에 저장: ${secondClaim}`,
          ],
          allow_custom: true,
        },
      }],
    };
    renderPanel();

    await user.click(await screen.findByRole("button", { name: `검색 후 자료집에 저장: ${firstClaim}` }));
    await act(async () => {
      companionHook.onApplied?.();
    });

    await waitFor(() => expect(screen.queryByRole("button", { name: `검색 후 자료집에 저장: ${firstClaim}` })).not.toBeInTheDocument());
    const remaining = screen.getByRole("button", { name: `검색 후 자료집에 저장: ${secondClaim}` });
    expect(remaining).toBeEnabled();

    await user.click(remaining);

    expect(companionState.value.send).toHaveBeenCalledTimes(2);
    expect(companionState.value.send).toHaveBeenLastCalledWith(expect.stringContaining(secondClaim));
  });

  it("renders assistant errors even when no choices are present", async () => {
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "web_search 실행 실패: api key is required",
        errored: true,
      }],
    };
    renderPanel();

    expect(await screen.findByText("web_search 실행 실패: api key is required")).toBeInTheDocument();
  });

  it("hides raw apply-ops JSON from streaming fact-book feedback", async () => {
    companionState.value = {
      ...companionState.value,
      status: "streaming",
      streaming: '{"summary":"현실 팩트카드 저장","ops_json":"[{\\"op\\":\\"create_fact_card\\"}]"}확인된 출처를 기준으로 처리했어요.',
    };
    renderPanel();

    expect(await screen.findByText("확인된 출처를 기준으로 처리했어요.")).toBeInTheDocument();
    expect(screen.queryByText(/ops_json/)).not.toBeInTheDocument();
    expect(screen.queryByText(/create_fact_card/)).not.toBeInTheDocument();
  });

  it("hides raw apply-ops JSON from completed fact-book feedback", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "후보를 골라주세요.",
        choices: {
          run_id: "run-1",
          prompt: "검토할 후보",
          options: ["검색 후 자료집에 저장: 운하 갑문 구조"],
          allow_custom: true,
        },
      }],
    };
    const view = renderPanel();

    await user.click(await screen.findByRole("button", { name: "검색 후 자료집에 저장: 운하 갑문 구조" }));
    companionState.value = {
      ...companionState.value,
      status: "idle",
      messages: [
        companionState.value.messages[0],
        { role: "user", content: "선택한 주장: 운하 갑문 구조" },
        {
          role: "assistant",
          content: '{"summary":"현실 팩트카드 저장","ops_json":"[{\\"op\\":\\"create_fact_card\\"}]"}이번 턴에는 저장 이벤트가 확인되지 않았어요.',
        },
      ],
    };
    view.rerender(
      <I18nProvider>
        <FactBookPanel
          projectId="project-1"
          nodeId="node-1"
          sceneLabel="씬 1"
          beforeReview={vi.fn()}
          onClose={vi.fn()}
          onChanged={vi.fn()}
        />
      </I18nProvider>,
    );

    expect(await screen.findByText("이번 턴에는 저장 이벤트가 확인되지 않았어요.")).toBeInTheDocument();
    expect(screen.queryByText(/ops_json/)).not.toBeInTheDocument();
    expect(screen.queryByText(/create_fact_card/)).not.toBeInTheDocument();
  });

  it("turns raw apply-ops JSON into an applyable fact-card proposal", async () => {
    const user = userEvent.setup();
    const inlineOps = [{
      op: "create_fact_card",
      claim: "운하 갑문 구조",
      result: "갑문은 수위를 맞춰 선박을 이동시키는 구조물이다.",
      status: "verified",
      sources: [{ url: "https://example.com/lock-gate", title: "Lock gate", snippet: "official", accessed_at: 200 }],
    }];
    const inlineArgs = JSON.stringify({
      summary: "현실 팩트카드 저장",
      ops_json: JSON.stringify(inlineOps),
    });
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "후보를 골라주세요.",
        choices: {
          run_id: "run-1",
          prompt: "검토할 후보",
          options: ["검색 후 자료집에 저장: 운하 갑문 구조"],
          allow_custom: true,
        },
      }],
    };
    const view = renderPanel();

    await user.click(await screen.findByRole("button", { name: "검색 후 자료집에 저장: 운하 갑문 구조" }));
    companionState.value = {
      ...companionState.value,
      status: "idle",
      messages: [
        companionState.value.messages[0],
        { role: "user", content: "선택한 주장: 운하 갑문 구조" },
        {
          role: "assistant",
          content: `${inlineArgs}저장 제안을 확인해 주세요.`,
        },
      ],
    };
    view.rerender(
      <I18nProvider>
        <FactBookPanel
          projectId="project-1"
          nodeId="node-1"
          sceneLabel="씬 1"
          beforeReview={vi.fn()}
          onClose={vi.fn()}
          onChanged={vi.fn()}
        />
      </I18nProvider>,
    );

    expect(await screen.findByText("저장 제안을 확인해 주세요.")).toBeInTheDocument();
    expect(screen.getByText("현실 팩트카드 저장")).toBeInTheDocument();
    expect(screen.getByText("자료집 카드 생성: 운하 갑문 구조")).toBeInTheDocument();
    expect(screen.queryByText(/ops_json/)).not.toBeInTheDocument();
    expect(screen.queryByText(/create_fact_card/)).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "적용" }));

    await waitFor(() => expect(mocks.companionApplyOps).toHaveBeenCalledWith(
      "project-1",
      "node-1",
      "",
      [expect.objectContaining({ op: "create_fact_card", claim: "운하 갑문 구조" })],
    ));
    await waitFor(() => expect(mocks.factsList.mock.calls.length).toBeGreaterThanOrEqual(2));
  });

  it("renders fact card proposals inside the panel", async () => {
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "자료집에 저장할 수 있어요.",
        proposal: {
          run_id: "run-1",
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

    expect(await screen.findByText("자료집 저장")).toBeInTheDocument();
    expect(screen.getByText("자료집 카드 생성: 런던 일반 경찰은 항상 총기를 휴대한다")).toBeInTheDocument();
  });

  it("shows refresh feedback after companion applies a fact card", async () => {
    renderPanel();

    await act(async () => {
      companionHook.onApplied?.();
    });

    expect(await screen.findByText("자료집을 새로고침했습니다.")).toBeInTheDocument();
    await waitFor(() => expect(mocks.factsList.mock.calls.length).toBeGreaterThanOrEqual(2));
  });

  it("shows feedback when a picked fact candidate finishes without saving", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "후보를 골라주세요.",
        choices: {
          run_id: "run-1",
          prompt: "검토할 후보",
          options: ["검색 후 자료집에 저장: 런던 경찰 총기 휴대"],
          allow_custom: true,
        },
      }],
    };
    const view = renderPanel();

    await user.click(await screen.findByRole("button", { name: "검색 후 자료집에 저장: 런던 경찰 총기 휴대" }));
    companionState.value = {
      ...companionState.value,
      status: "idle",
      messages: [
        companionState.value.messages[0],
        { role: "user", content: "선택한 주장: 런던 경찰 총기 휴대" },
        { role: "assistant", content: "방금 자료집에 저장 처리했어요." },
      ],
    };
    view.rerender(
      <I18nProvider>
        <FactBookPanel
          projectId="project-1"
          nodeId="node-1"
          sceneLabel="씬 1"
          beforeReview={vi.fn()}
          onClose={vi.fn()}
          onChanged={vi.fn()}
        />
      </I18nProvider>,
    );

    expect(await screen.findByText("아직 자료집 저장 이벤트가 확인되지 않았습니다. 출처 URL을 입력하면 바로 저장할 수 있습니다.")).toBeInTheDocument();
  });

  it("auto-saves a source URL from the assistant response when a picked candidate did not apply", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "후보를 골라주세요.",
        choices: {
          run_id: "run-1",
          prompt: "검토할 후보",
          options: ["검색 후 자료집에 저장: 비 온 뒤 흙냄새"],
          allow_custom: true,
        },
      }],
    };
    const view = renderPanel();

    await user.click(await screen.findByRole("button", { name: "검색 후 자료집에 저장: 비 온 뒤 흙냄새" }));
    companionState.value = {
      ...companionState.value,
      status: "idle",
      messages: [
        companionState.value.messages[0],
        { role: "user", content: "선택한 주장: 비 온 뒤 흙냄새" },
        { role: "assistant", content: "저장 가능한 본문을 확인했어요. 출처 URL: https://biopathogenix.com/petrichor-and-actinomycetes-the-smell-of-rain-explained/" },
      ],
    };
    view.rerender(
      <I18nProvider>
        <FactBookPanel
          projectId="project-1"
          nodeId="node-1"
          sceneLabel="씬 1"
          beforeReview={vi.fn()}
          onClose={vi.fn()}
          onChanged={vi.fn()}
        />
      </I18nProvider>,
    );

    await waitFor(() => expect(mocks.factsCreateFromUrl).toHaveBeenCalledWith({
      project_id: "project-1",
      node_id: "node-1",
      claim: "비 온 뒤 흙냄새",
      url: "https://biopathogenix.com/petrichor-and-actinomycetes-the-smell-of-rain-explained/",
    }));
    expect(await screen.findByText("출처 URL을 확인하고 자료집에 저장했습니다.")).toBeInTheDocument();
    await waitFor(() => expect(mocks.factsList.mock.calls.length).toBeGreaterThanOrEqual(2));
  });

  it("does not auto-save URLs described as inaccessible or insufficient", async () => {
    mocks.factsCreateFromUrl.mockClear();
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "후보를 골라주세요.",
        choices: {
          run_id: "run-1",
          prompt: "검토할 후보",
          options: ["검색 후 자료집에 저장: 비 온 뒤 흙냄새"],
          allow_custom: true,
        },
      }],
    };
    const view = renderPanel();

    await user.click(await screen.findByRole("button", { name: "검색 후 자료집에 저장: 비 온 뒤 흙냄새" }));
    companionState.value = {
      ...companionState.value,
      status: "idle",
      messages: [
        companionState.value.messages[0],
        { role: "user", content: "선택한 주장: 비 온 뒤 흙냄새" },
        { role: "assistant", content: "web_search는 됐지만 https://www.sciencefocus.com/planet-earth/why-does-the-ground-smell-after-it-rains 는 상태 접근은 되지만 본문 텍스트가 충분하지 않아 저장하지 못했어요." },
      ],
    };
    view.rerender(
      <I18nProvider>
        <FactBookPanel
          projectId="project-1"
          nodeId="node-1"
          sceneLabel="씬 1"
          beforeReview={vi.fn()}
          onClose={vi.fn()}
          onChanged={vi.fn()}
        />
      </I18nProvider>,
    );

    await waitFor(() => expect(screen.getByText("아직 자료집 저장 이벤트가 확인되지 않았습니다. 출처 URL을 입력하면 바로 저장할 수 있습니다.")).toBeInTheDocument());
    expect(mocks.factsCreateFromUrl).not.toHaveBeenCalled();
  });

  it("offers an alternative-source retry after direct source URL saving fails", async () => {
    mocks.factsList.mockResolvedValue([]);
    mocks.factsCreateFromUrl.mockRejectedValueOnce(new Error("web_fetch status 404"));
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "후보를 골라주세요.",
        choices: {
          run_id: "run-1",
          prompt: "검토할 후보",
          options: ["검색 후 자료집에 저장: 대장장이가 농기구와 무기를 수리했다"],
          allow_custom: true,
        },
      }],
    };
    renderPanel();

    await user.click(await screen.findByRole("button", { name: "검색 후 자료집에 저장: 대장장이가 농기구와 무기를 수리했다" }));
    const input = await screen.findByRole("textbox", { name: "자료집 답장 입력" });
    await user.type(input, "https://www.britannica.com/topic/blacksmith");
    await user.click(screen.getByRole("button", { name: "전송" }));

    expect(await screen.findByText(/출처 URL 저장 실패: Error: web_fetch status 404/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "다른 출처 찾기" }));

    expect(companionState.value.send).toHaveBeenCalledTimes(2);
    expect(companionState.value.send).toHaveBeenLastCalledWith(expect.stringContaining("대장장이가 농기구와 무기를 수리했다"));
    expect(companionState.value.send).toHaveBeenLastCalledWith(expect.stringContaining("https://www.britannica.com/topic/blacksmith"));
    expect(companionState.value.send).toHaveBeenLastCalledWith(expect.stringContaining("저장 후보에서 제외"));
    expect(companionState.value.send).toHaveBeenLastCalledWith(expect.stringContaining("create_fact_card"));
  });

  it("saves a direct source URL for the picked claim without companion", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "후보를 골라주세요.",
        choices: {
          run_id: "run-1",
          prompt: "검토할 후보",
          options: ["검색 후 자료집에 저장: 런던 경찰 총기 휴대"],
          allow_custom: true,
        },
      }],
    };
    renderPanel();

    await user.click(await screen.findByRole("button", { name: "검색 후 자료집에 저장: 런던 경찰 총기 휴대" }));
    const input = await screen.findByRole("textbox", { name: "자료집 답장 입력" });
    await user.type(input, "https://www.met.police.uk/");
    await user.click(screen.getByRole("button", { name: "전송" }));

    expect(mocks.factsCreateFromUrl).toHaveBeenCalledWith({
      project_id: "project-1",
      node_id: "node-1",
      claim: "런던 경찰 총기 휴대",
      url: "https://www.met.police.uk/",
    });
    expect(companionState.value.send).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(mocks.factsList.mock.calls.length).toBeGreaterThanOrEqual(2));
    expect(await screen.findByText("출처 URL을 확인하고 자료집에 저장했습니다.")).toBeInTheDocument();
    expect(input).toHaveValue("");
  });

  it("keeps non-URL direct replies in the companion path", async () => {
    mocks.factsCreateFromUrl.mockClear();
    const user = userEvent.setup();
    renderPanel();

    const input = await screen.findByRole("textbox", { name: "자료집 답장 입력" });
    await user.type(input, "이 주장도 이어서 봐줘");
    await user.click(screen.getByRole("button", { name: "전송" }));

    expect(companionState.value.send).toHaveBeenCalledWith("이 주장도 이어서 봐줘");
    expect(mocks.factsCreateFromUrl).not.toHaveBeenCalled();
    expect(input).toHaveValue("");
  });

  it("focuses the direct reply input from custom choices", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "출처 URL을 직접 입력해 주세요.",
        choices: {
          run_id: "run-1",
          prompt: "출처 입력",
          options: ["검색 후 자료집에 저장: 런던 경찰 총기 휴대"],
          allow_custom: true,
        },
      }],
    };
    renderPanel();

    await user.click(await screen.findByRole("button", { name: "직접 입력" }));

    expect(screen.getByRole("textbox", { name: "자료집 답장 입력" })).toHaveFocus();
  });
});
