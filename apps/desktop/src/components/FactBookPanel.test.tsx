import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { FactBookPanel } from "./FactBookPanel";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  factsList: vi.fn(),
  factsCreateFromUrl: vi.fn(),
  factsDelete: vi.fn(),
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
}));

function renderPanel(props: Partial<ComponentProps<typeof FactBookPanel>> = {}) {
  return render(
    <I18nProvider>
      <FactBookPanel
        projectId="project-1"
        nodeId="node-1"
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
    // Call counts leak between cases otherwise, and two of these assert that a
    // save never happened.
    vi.clearAllMocks();
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

  it("opens impact check for a saved fact card", async () => {
    const user = userEvent.setup();
    const onImpactCheck = vi.fn();
    renderPanel({ onImpactCheck });

    await screen.findByText("런던 일반 경찰은 항상 총기를 휴대한다");
    await user.click(screen.getByRole("button", { name: "영향 확인" }));

    expect(onImpactCheck).toHaveBeenCalledWith(expect.stringContaining("런던 일반 경찰은 항상 총기를 휴대한다"));
    expect(onImpactCheck).toHaveBeenCalledWith(expect.stringContaining("일반 경찰은 통상 비무장 근무"));
  });

  it("saves a claim with its source and reloads the list", async () => {
    const user = userEvent.setup();
    const onChanged = vi.fn();
    renderPanel({ onChanged });

    await screen.findByText("런던 일반 경찰은 항상 총기를 휴대한다");
    await user.type(screen.getByLabelText("확인할 내용"), "런던 경찰 총기 휴대");
    await user.type(screen.getByLabelText("출처 주소"), "https://www.met.police.uk/");
    await user.click(screen.getByRole("button", { name: "자료집에 저장" }));

    // The card's default sentence comes from the UI, not the engine: only the
    // UI knows what language the writer reads (#45).
    await waitFor(() => expect(mocks.factsCreateFromUrl).toHaveBeenCalledWith({
      project_id: "project-1",
      node_id: "node-1",
      claim: "런던 경찰 총기 휴대",
      url: "https://www.met.police.uk/",
      result: "직접 입력한 출처 URL에서 확인했습니다.",
    }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
  });

  it("refuses to save a claim with no source", async () => {
    const user = userEvent.setup();
    renderPanel();

    await screen.findByText("런던 일반 경찰은 항상 총기를 휴대한다");
    await user.type(screen.getByLabelText("확인할 내용"), "출처 없는 주장");
    await user.click(screen.getByRole("button", { name: "자료집에 저장" }));

    // A card with no source is the one thing this panel must not produce: the
    // whole point of the dossier is where the writer checked something.
    expect(mocks.factsCreateFromUrl).not.toHaveBeenCalled();
    expect(await screen.findByText("출처 주소를 넣어 주세요.")).toBeInTheDocument();
  });

  it("fills in a claim selected from the editor instead of asking anyone", async () => {
    renderPanel({ selectedClaimRequest: { id: "sel-1", claim: "1923년 경성에는 전차가 다녔다" } });

    // The selection menu used to hand this to the companion. It now seeds the
    // form, leaving the source — the part that makes the card worth having —
    // to the writer.
    await waitFor(() =>
      expect(screen.getByLabelText("확인할 내용")).toHaveValue("1923년 경성에는 전차가 다녔다"),
    );
    expect(mocks.factsCreateFromUrl).not.toHaveBeenCalled();
  });
});
