import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Entity } from "../lib/types";
import { I18nProvider } from "../lib/i18n";
import { EntitySheet } from "./EntitySheet";

const mocks = vi.hoisted(() => ({
  visuals: {
    getSheet: vi.fn().mockResolvedValue({ entity_id: "entity-1", age_range: "", build: "", hair: "", outfit: "", signature: "", palette: "", notes: "", updated_at: 0 }),
    setSheet: vi.fn().mockResolvedValue({}), listImages: vi.fn().mockResolvedValue([]),
    addImage: vi.fn(), getImage: vi.fn(), deleteImage: vi.fn(),
    getArtStyle: vi.fn().mockResolvedValue({ project_id: "project-1", style: "", negative_prompt: "", updated_at: 0 }),
    setArtStyle: vi.fn(),
  },
  entities: {
    get: vi.fn(),
    update: vi.fn(),
    scenes: vi.fn(),
  },
  relationships: {
    listByEntity: vi.fn(),
    delete: vi.fn(),
  },
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  visuals: mocks.visuals,
  entities: mocks.entities,
  relationships: mocks.relationships,
  settings: {
    get: mocks.settingsGet,
  },
}));

const baseEntity: Entity = {
  id: "entity-1",
  project_id: "project-1",
  kind: "character",
  name: "해진",
  aliases: [],
  role: "",
  summary: "",
  attributes: {},
  created_at: 1,
  updated_at: 1,
};

describe("EntitySheet", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.relationships.listByEntity.mockResolvedValue([]);
    mocks.entities.scenes.mockResolvedValue([]);
    mocks.entities.update.mockImplementation(async (input) => ({ ...baseEntity, ...input }));
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
  });

  const renderSheet = (props: Partial<ComponentProps<typeof EntitySheet>> = {}) => render(
    <I18nProvider>
      <EntitySheet entityId="entity-1" onClose={vi.fn()} {...props} />
    </I18nProvider>,
  );

  it("saves visual fields with the entity", async () => {
    const user = userEvent.setup();
    mocks.entities.get.mockResolvedValue(baseEntity);
    renderSheet();
    await user.type(await screen.findByLabelText("머리"), "검은 머리");
    await user.click(screen.getByRole("button", { name: "저장" }));
    await waitFor(() => expect(mocks.visuals.setSheet).toHaveBeenCalledWith(expect.objectContaining({ entity_id: "entity-1", hair: "검은 머리" })));
  });

  it("leaves an unchanged visual sheet untouched", async () => {
    const user = userEvent.setup();
    mocks.entities.get.mockResolvedValue(baseEntity);
    renderSheet();
    await screen.findByLabelText("머리");
    await user.click(screen.getByRole("button", { name: "저장" }));
    await waitFor(() => expect(mocks.entities.update).toHaveBeenCalled());
    expect(mocks.visuals.setSheet).not.toHaveBeenCalled();
  });

  it("refuses an oversized reference before uploading", async () => {
    const user = userEvent.setup();
    mocks.entities.get.mockResolvedValue(baseEntity);
    renderSheet();
    await user.upload(await screen.findByLabelText("참조 이미지 추가"), new File([new Uint8Array(5 * 1024 * 1024 + 1)], "large.png", { type: "image/png" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("이미지는 5 MB 이하여야 합니다.");
    expect(mocks.visuals.addImage).not.toHaveBeenCalled();
  });

  it("uploads reference bytes as base64", async () => {
    const user = userEvent.setup();
    mocks.entities.get.mockResolvedValue(baseEntity);
    mocks.visuals.addImage.mockResolvedValue({ id: "i1", mime: "image/png", caption: "", byte_size: 3 });
    mocks.visuals.getImage.mockResolvedValue({ mime: "image/png", data_base64: "YWJj" });
    renderSheet();
    const input = await screen.findByLabelText("참조 이미지 추가");
    await user.upload(input, new File(["abc"], "ref.png", { type: "image/png" }));
    await waitFor(() => expect(mocks.visuals.addImage).toHaveBeenCalledWith({ entity_id: "entity-1", mime: "image/png", caption: "", data_base64: "YWJj" }));
  });

  it("saves a character role selected from core-role presets", async () => {
    const user = userEvent.setup();
    mocks.entities.get.mockResolvedValue(baseEntity);

    renderSheet();

    expect(await screen.findByDisplayValue("해진")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "주인공" }));
    await user.click(screen.getByRole("button", { name: "저장" }));

    await waitFor(() => {
      expect(mocks.entities.update).toHaveBeenCalledWith(expect.objectContaining({
        id: "entity-1",
        role: "주인공",
      }));
    });
  });

  it("shows place-stage presets for place entities", async () => {
    const place = { ...baseEntity, kind: "place" as const, name: "폐쇄 도시" };
    mocks.entities.get.mockResolvedValue(place);

    renderSheet();

    expect(await screen.findByDisplayValue("폐쇄 도시")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "인물 비주얼 시트" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "메인무대" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "특별한 장소" })).toBeInTheDocument();
  });

  it("offers item role and attribute presets for worldbuilding props", async () => {
    const user = userEvent.setup();
    const item = { ...baseEntity, kind: "item" as const, name: "검은 단검" };
    mocks.entities.get.mockResolvedValue(item);

    renderSheet();

    expect(await screen.findByDisplayValue("검은 단검")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "핵심 아이템" }));
    await user.click(screen.getByRole("button", { name: "효과" }));
    await user.click(screen.getByRole("button", { name: "저장" }));

    await waitFor(() => {
      expect(mocks.entities.update).toHaveBeenCalledWith(expect.objectContaining({
        id: "entity-1",
        role: "핵심 아이템",
        attributes: { 효과: "" },
      }));
    });
  });

  it("offers skill and magic presets for concept entities", async () => {
    const concept = { ...baseEntity, kind: "concept" as const, name: "빛의 맹약" };
    mocks.entities.get.mockResolvedValue(concept);

    renderSheet();

    expect(await screen.findByDisplayValue("빛의 맹약")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "마법" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "스킬" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "발동 조건" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "비용" })).toBeInTheDocument();
  });

  it("opens contextual change for the current entity", async () => {
    const user = userEvent.setup();
    const onContextChange = vi.fn();
    mocks.entities.get.mockResolvedValue(baseEntity);

    renderSheet({ onContextChange });

    await screen.findByDisplayValue("해진");
    await user.click(screen.getByRole("button", { name: "작품 전체 변경" }));

    expect(onContextChange).toHaveBeenCalledWith("entity-1");
  });
});
