import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
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

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  factsList: vi.fn(),
  factsDelete: vi.fn(),
}));

vi.mock("../hooks/useCompanion", () => ({
  useCompanion: () => companionState.value,
}));

vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
  facts: {
    list: mocks.factsList,
    delete: mocks.factsDelete,
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
  beforeEach(() => {
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
    mocks.factsDelete.mockResolvedValue({ ok: true });
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

    expect(companionState.value.send).toHaveBeenCalledWith("검색 후 자료집에 저장: 런던 경찰 총기 휴대");
  });

  it("sends a direct reply from the fact book panel", async () => {
    const user = userEvent.setup();
    renderPanel();

    const input = await screen.findByRole("textbox", { name: "자료집 답장 입력" });
    await user.type(input, "https://www.met.police.uk/");
    await user.click(screen.getByRole("button", { name: "전송" }));

    expect(companionState.value.send).toHaveBeenCalledWith("https://www.met.police.uk/");
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
