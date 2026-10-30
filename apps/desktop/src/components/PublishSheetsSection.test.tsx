import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import type { PublishSheetsStatus } from "../lib/types";
import { PublishSheetsSection } from "./PublishSheetsSection";

const mocks = vi.hoisted(() => ({
  illustrations: { list: vi.fn(), get: vi.fn() },
  publishApi: { setCover: vi.fn(), sheetsSync: vi.fn() },
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  illustrations: mocks.illustrations,
  publishApi: mocks.publishApi,
  settings: { get: mocks.settingsGet },
}));

const meta = (id: string, caption: string) => ({
  id, project_id: "p1", caption, prompt: "", provider: "", model: "", mime: "image/png", width: 8, height: 8, byte_size: 10, created_at: 1,
});

const status = (over: Partial<PublishSheetsStatus> = {}): PublishSheetsStatus => ({ chosen: 0, remote: true, sheets: [], ...over });

function renderSection(sheets: PublishSheetsStatus, onSynced = vi.fn().mockResolvedValue(undefined)) {
  const run = async (fn: () => Promise<void>) => {
    await fn();
  };
  render(
    <I18nProvider>
      <PublishSheetsSection projectId="p1" sheets={sheets} busy={false} run={run} onSynced={onSynced} />
    </I18nProvider>,
  );
  return { onSynced };
}

describe("PublishSheetsSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
    mocks.illustrations.list.mockResolvedValue({ items: [meta("ill-1", "비 오는 거리"), meta("ill-2", "")], count: 2, bytes: 20 });
    mocks.illustrations.get.mockResolvedValue({ ...meta("ill-1", ""), data_base64: "QUJD" });
    mocks.publishApi.setCover.mockResolvedValue({ cover_id: "ill-1" });
    mocks.publishApi.sheetsSync.mockResolvedValue(status());
  });

  it("says how many sheets go up and shows each on the site with its state", async () => {
    renderSection(status({
      chosen: 2,
      sheets: [
        { entity_id: "e1", kind: "character", name: "강도윤", state: "public" },
        { entity_id: "e2", kind: "place", name: "회사", state: "rejected", moderation_reason: "사유" },
      ],
    }));
    expect(screen.getByTestId("publish-sheets")).toHaveTextContent("시트 2개와 표지가 함께 올라가고");
    const list = screen.getByTestId("publish-sheet-list");
    expect(list).toHaveTextContent("강도윤");
    expect(list).toHaveTextContent("반려");
    expect(list).toHaveTextContent("사유: 사유");
  });

  it("chooses a gallery picture as the cover and previews it", async () => {
    const user = userEvent.setup();
    renderSection(status());
    const select = await screen.findByTestId("publish-cover");
    await waitFor(() => expect(select).toHaveTextContent("그림 2"));
    await user.selectOptions(select, "ill-1");
    expect(mocks.publishApi.setCover).toHaveBeenCalledWith("p1", "ill-1");
    expect(await screen.findByTestId("publish-cover-preview")).toHaveAttribute("src", "data:image/png;base64,QUJD");
  });

  it("uploads the sheets and cover, then refreshes", async () => {
    const user = userEvent.setup();
    const { onSynced } = renderSection(status({ chosen: 1 }));
    await user.click(screen.getByTestId("publish-sheets-sync"));
    expect(mocks.publishApi.sheetsSync).toHaveBeenCalledWith("p1");
    expect(onSynced).toHaveBeenCalled();
    expect(screen.getByTestId("publish-sheets-sync")).toHaveTextContent("올렸습니다");
  });
});
