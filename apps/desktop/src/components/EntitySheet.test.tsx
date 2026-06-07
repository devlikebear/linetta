import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Entity } from "../lib/types";
import { I18nProvider } from "../lib/i18n";
import { EntitySheet } from "./EntitySheet";

const mocks = vi.hoisted(() => ({
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

  const renderSheet = () => render(
    <I18nProvider>
      <EntitySheet entityId="entity-1" onClose={vi.fn()} />
    </I18nProvider>,
  );

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
});
