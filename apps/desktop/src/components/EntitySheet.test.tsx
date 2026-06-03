import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Entity } from "../lib/types";
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
}));

vi.mock("../lib/rpc", () => ({
  entities: mocks.entities,
  relationships: mocks.relationships,
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
  });

  it("saves a character role selected from core-role presets", async () => {
    const user = userEvent.setup();
    mocks.entities.get.mockResolvedValue(baseEntity);

    render(<EntitySheet entityId="entity-1" onClose={vi.fn()} />);

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

    render(<EntitySheet entityId="entity-1" onClose={vi.fn()} />);

    expect(await screen.findByDisplayValue("폐쇄 도시")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "메인무대" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "특별한 장소" })).toBeInTheDocument();
  });
});
