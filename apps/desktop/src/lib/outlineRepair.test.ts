import { describe, expect, it, vi } from "vitest";
import type { TreeNode } from "../hooks/useFirstLeaf";
import { OUTLINE_PRESETS, repairOutlineTree, type OutlineRepairRPC } from "./outlineRepair";

function node(input: Partial<TreeNode> & Pick<TreeNode, "id" | "kind" | "label">): TreeNode {
  return {
    project_id: "project-1",
    ordinal: 0,
    title: "",
    status: "draft",
    word_count: 0,
    created_at: 1,
    updated_at: 1,
    children: [],
    ...input,
  };
}

const t = (key: string, values?: Record<string, unknown>) => {
  if (key === "workspace.partNumber") return `${values?.number}부`;
  if (key === "workspace.chapterNumber") return `${values?.number}장`;
  if (key === "workspace.sceneNumber") return `씬 ${values?.number}`;
  if (key === "workspace.webNovelPartNumber") return `${values?.number}권`;
  if (key === "workspace.webNovelChapterNumber") return `${values?.number}화`;
  return key;
};

describe("repairOutlineTree", () => {
  it("moves misplaced scenes and containers, then renumbers labels while preserving titles", async () => {
    const rootScene = node({ id: "root-scene", kind: "leaf", label: "프롤로그", title: "삭제 버튼 앞에서", ordinal: 0 });
    const directScene = node({ id: "direct-scene", kind: "leaf", label: "씬 2 - 기록소 제안", parent_id: "part", ordinal: 0 });
    const deepScene = node({ id: "deep-scene", kind: "leaf", label: "씬 9 - 시드에서 신경이 피어나다", parent_id: "nested", ordinal: 0 });
    const nested = node({ id: "nested", kind: "container", label: "1장 - 경계의 틈", parent_id: "chapter", ordinal: 0, children: [deepScene] });
    const chapter = node({ id: "chapter", kind: "container", label: "2장 - 확장된 동화성", parent_id: "part", ordinal: 1, children: [nested] });
    const part = node({ id: "part", kind: "container", label: "개별성의 경계선", title: "개별성의 경계선 2차", ordinal: 1, children: [directScene, chapter] });
    const calls: string[] = [];
    const rpc: OutlineRepairRPC = {
      createChild: vi.fn(),
      createSibling: vi.fn(),
      moveToParent: vi.fn(async (id, parentID) => {
        calls.push(`move:${id}->${parentID}`);
        return { ok: true as const };
      }),
      moveToRoot: vi.fn(async (id) => {
        calls.push(`root:${id}`);
        return { ok: true as const };
      }),
      convertToContainer: vi.fn(async (id) => {
        calls.push(`convert:${id}`);
        return { ok: true as const };
      }),
      delete: vi.fn(async (id) => {
        calls.push(`delete:${id}`);
        return { ok: true as const };
      }),
      rename: vi.fn(async (id, label, title) => {
        calls.push(`rename:${id}:${label}:${title}`);
        return { ok: true as const };
      }),
    };

    await repairOutlineTree([rootScene, part], rpc, t);

    expect(calls).toContain("move:root-scene->chapter");
    expect(calls).toContain("move:direct-scene->chapter");
    expect(calls).toContain("move:nested->part");
    expect(calls).toContain("rename:part:1부:개별성의 경계선 2차");
    expect(calls).toContain("rename:chapter:1장:확장된 동화성");
    expect(calls).toContain("rename:nested:2장:경계의 틈");
    expect(calls).toContain("rename:root-scene:씬 1:삭제 버튼 앞에서");
    expect(calls).toContain("rename:direct-scene:씬 2:기록소 제안");
    expect(calls).toContain("rename:deep-scene:씬 1:시드에서 신경이 피어나다");
    expect(calls.some((call) => call.startsWith("delete:"))).toBe(false);
  });

  it("moves structural root chapters under an existing part instead of renaming them as parts", async () => {
    const scene = node({ id: "scene", kind: "leaf", label: "씬", parent_id: "root-chapter", ordinal: 0 });
    const rootChapter = node({ id: "root-chapter", kind: "container", label: "1장 - 경계의 틈", ordinal: 0, children: [scene] });
    const part = node({ id: "part", kind: "container", label: "개별성의 경계선", ordinal: 1, children: [] });
    const calls: string[] = [];
    const rpc: OutlineRepairRPC = {
      createChild: vi.fn(async (parentId, kind, label) => node({ id: "created-chapter", parent_id: parentId, kind, label }) as never),
      createSibling: vi.fn(),
      moveToParent: vi.fn(async (id, parentID) => {
        calls.push(`move:${id}->${parentID}`);
        return { ok: true as const };
      }),
      moveToRoot: vi.fn(async (id) => {
        calls.push(`root:${id}`);
        return { ok: true as const };
      }),
      convertToContainer: vi.fn(async (id) => {
        calls.push(`convert:${id}`);
        return { ok: true as const };
      }),
      delete: vi.fn(async (id) => {
        calls.push(`delete:${id}`);
        return { ok: true as const };
      }),
      rename: vi.fn(async (id, label, title) => {
        calls.push(`rename:${id}:${label}:${title}`);
        return { ok: true as const };
      }),
    };

    await repairOutlineTree([rootChapter, part], rpc, t);

    expect(calls).toContain("move:root-chapter->part");
    expect(calls).not.toContain("rename:root-chapter:1부:경계의 틈");
    expect(calls).toContain("rename:part:1부:개별성의 경계선");
    expect(calls).toContain("rename:root-chapter:1장:경계의 틈");
    expect(calls).toContain("rename:scene:씬 1:씬");
  });

  it("does not mistake a part title containing 장 for a chapter while renumbering parts", async () => {
    const scene = node({ id: "scene", kind: "leaf", label: "씬 1", parent_id: "chapter", ordinal: 0 });
    const chapter = node({ id: "chapter", kind: "container", label: "1장 - 경계의 틈", parent_id: "expansion", ordinal: 0, children: [scene] });
    const firstPart = node({ id: "first-part", kind: "container", label: "개별성의 경계선", ordinal: 0, children: [] });
    const expansion = node({ id: "expansion", kind: "container", label: "확장된 동화성", ordinal: 1, children: [chapter] });
    const calls: string[] = [];
    const rpc: OutlineRepairRPC = {
      createChild: vi.fn(),
      createSibling: vi.fn(),
      moveToParent: vi.fn(async (id, parentID) => {
        calls.push(`move:${id}->${parentID}`);
        return { ok: true as const };
      }),
      moveToRoot: vi.fn(async (id) => {
        calls.push(`root:${id}`);
        return { ok: true as const };
      }),
      convertToContainer: vi.fn(async (id) => {
        calls.push(`convert:${id}`);
        return { ok: true as const };
      }),
      delete: vi.fn(async (id) => {
        calls.push(`delete:${id}`);
        return { ok: true as const };
      }),
      rename: vi.fn(async (id, label, title) => {
        calls.push(`rename:${id}:${label}:${title}`);
        return { ok: true as const };
      }),
    };

    await repairOutlineTree([firstPart, expansion], rpc, t);

    expect(calls).not.toContain("move:expansion->first-part");
    expect(calls).toContain("rename:first-part:1부:개별성의 경계선");
    expect(calls).toContain("rename:expansion:2부:확장된 동화성");
    expect(calls).toContain("rename:chapter:1장:경계의 틈");
  });

  it("promotes nested part-like containers to root instead of leaving every chapter under one part", async () => {
    const firstScene = node({ id: "first-scene", kind: "leaf", label: "씬 1", parent_id: "first-chapter", ordinal: 0 });
    const firstChapter = node({ id: "first-chapter", kind: "container", label: "1장", parent_id: "part-1", ordinal: 0, children: [firstScene] });
    const part2Scene = node({ id: "part2-scene", kind: "leaf", label: "씬 1 - 2026년의 침묵", parent_id: "part2-chapter", ordinal: 0 });
    const part2Chapter = node({ id: "part2-chapter", kind: "container", label: "1장 - 멈춰버린 약속", parent_id: "part-2", ordinal: 0, children: [part2Scene] });
    const part2 = node({ id: "part-2", kind: "container", label: "개별성의 경계선 2차 - 2027, AGI와 자아의 재정의", parent_id: "part-1", ordinal: 2, children: [part2Chapter] });
    const part1 = node({ id: "part-1", kind: "container", label: "개별성의 경계선", ordinal: 0, children: [firstChapter, part2] });
    const calls: string[] = [];
    const rpc: OutlineRepairRPC = {
      createChild: vi.fn(),
      createSibling: vi.fn(),
      moveToParent: vi.fn(async (id, parentID) => {
        calls.push(`move:${id}->${parentID}`);
        return { ok: true as const };
      }),
      moveToRoot: vi.fn(async (id) => {
        calls.push(`root:${id}`);
        return { ok: true as const };
      }),
      convertToContainer: vi.fn(async (id) => {
        calls.push(`convert:${id}`);
        return { ok: true as const };
      }),
      delete: vi.fn(async (id) => {
        calls.push(`delete:${id}`);
        return { ok: true as const };
      }),
      rename: vi.fn(async (id, label, title) => {
        calls.push(`rename:${id}:${label}:${title}`);
        return { ok: true as const };
      }),
    };

    await repairOutlineTree([part1], rpc, t);

    expect(calls).toContain("root:part-2");
    expect(calls).toContain("rename:part-1:1부:개별성의 경계선");
    expect(calls).toContain("rename:part-2:2부:개별성의 경계선 2차 - 2027, AGI와 자아의 재정의");
    expect(calls).toContain("rename:part2-chapter:1장:멈춰버린 약속");
    expect(calls).toContain("rename:part2-scene:씬 1:2026년의 침묵");
    expect(calls.some((call) => call.startsWith("delete:"))).toBe(false);
  });

  it("splits a chapter wrapper that contains chapter and part markers", async () => {
    const wrapperScene = node({ id: "wrapper-scene", kind: "leaf", label: "씬 1", parent_id: "wrapper", ordinal: 0 });
    const morning = node({ id: "morning", kind: "leaf", label: "씬 1 - 조간난 아침", parent_id: "wrapper", ordinal: 2 });
    const gapChapter = node({ id: "gap-chapter", kind: "leaf", label: "1장 - 경계의 틈", parent_id: "wrapper", ordinal: 1 });
    const part2 = node({ id: "part-2", kind: "leaf", label: "개별성의 경계선 2차 - 2027, AGI와 자아의 재정의", parent_id: "wrapper", ordinal: 3 });
    const part2Chapter = node({ id: "part2-chapter", kind: "leaf", label: "1장 - 멈춰버린 약속", parent_id: "wrapper", ordinal: 4 });
    const part2Scene = node({ id: "part2-scene", kind: "leaf", label: "씬 1 - 2026년의 침묵", parent_id: "wrapper", ordinal: 5 });
    const wrapper = node({ id: "wrapper", kind: "container", label: "1장", parent_id: "part-1", ordinal: 0, children: [wrapperScene, gapChapter, morning, part2, part2Chapter, part2Scene] });
    const part1 = node({ id: "part-1", kind: "container", label: "개별성의 경계선 - 개별성의 경계선", ordinal: 0, children: [wrapper] });
    const calls: string[] = [];
    const rpc: OutlineRepairRPC = {
      createChild: vi.fn(),
      createSibling: vi.fn(),
      moveToParent: vi.fn(async (id, parentID) => {
        calls.push(`move:${id}->${parentID}`);
        return { ok: true as const };
      }),
      moveToRoot: vi.fn(async (id) => {
        calls.push(`root:${id}`);
        return { ok: true as const };
      }),
      convertToContainer: vi.fn(async (id) => {
        calls.push(`convert:${id}`);
        return { ok: true as const };
      }),
      delete: vi.fn(async (id) => {
        calls.push(`delete:${id}`);
        return { ok: true as const };
      }),
      rename: vi.fn(async (id, label, title) => {
        calls.push(`rename:${id}:${label}:${title}`);
        return { ok: true as const };
      }),
    };

    await repairOutlineTree([part1], rpc, t);

    expect(calls).toContain("convert:gap-chapter");
    expect(calls).toContain("convert:part-2");
    expect(calls).toContain("convert:part2-chapter");
    expect(calls).toContain("move:morning->gap-chapter");
    expect(calls).toContain("move:gap-chapter->part-1");
    expect(calls).toContain("root:part-2");
    expect(calls).toContain("move:part2-chapter->part-2");
    expect(calls).toContain("move:part2-scene->part2-chapter");
    expect(calls).toContain("rename:part-1:1부:개별성의 경계선");
    expect(calls).toContain("rename:gap-chapter:2장:경계의 틈");
    expect(calls).toContain("rename:morning:씬 1:조간난 아침");
    expect(calls).toContain("rename:part-2:2부:개별성의 경계선 2차 - 2027, AGI와 자아의 재정의");
    expect(calls).toContain("rename:part2-chapter:1장:멈춰버린 약속");
    expect(calls).toContain("rename:part2-scene:씬 1:2026년의 침묵");
    expect(calls.some((call) => call.startsWith("delete:"))).toBe(false);
  });

  it("uses the web novel preset to normalize arcs and episodes without losing titles", async () => {
    const firstScene = node({ id: "scene", kind: "leaf", label: "씬 7 - 조각난 아침", parent_id: "episode", ordinal: 0 });
    const episode = node({ id: "episode", kind: "container", label: "1장 - 경계의 틈", ordinal: 0, children: [firstScene] });
    const arc = node({ id: "arc", kind: "container", label: "개별성의 경계선", title: "1막", ordinal: 1, children: [] });
    const calls: string[] = [];
    const rpc: OutlineRepairRPC = {
      createChild: vi.fn(async (parentId, kind, label) => node({ id: "created-episode", parent_id: parentId, kind, label }) as never),
      createSibling: vi.fn(),
      moveToParent: vi.fn(async (id, parentID) => {
        calls.push(`move:${id}->${parentID}`);
        return { ok: true as const };
      }),
      moveToRoot: vi.fn(async (id) => {
        calls.push(`root:${id}`);
        return { ok: true as const };
      }),
      convertToContainer: vi.fn(async (id) => {
        calls.push(`convert:${id}`);
        return { ok: true as const };
      }),
      delete: vi.fn(async (id) => {
        calls.push(`delete:${id}`);
        return { ok: true as const };
      }),
      rename: vi.fn(async (id, label, title) => {
        calls.push(`rename:${id}:${label}:${title}`);
        return { ok: true as const };
      }),
    };

    await repairOutlineTree([episode, arc], rpc, t, OUTLINE_PRESETS.webnovel);

    expect(calls).toContain("move:episode->arc");
    expect(calls).toContain("rename:arc:1권:1막");
    expect(calls).toContain("rename:episode:1화:경계의 틈");
    expect(calls).toContain("rename:scene:씬 1:조각난 아침");
    expect(calls.some((call) => call.startsWith("delete:"))).toBe(false);
  });

  it("leaves direct web novel leaf episodes with body text under their arc", async () => {
    const leafEpisode = node({
      id: "leaf-episode",
      kind: "leaf",
      label: "1화",
      title: "경계의 틈",
      parent_id: "arc",
      ordinal: 0,
      word_count: 1800,
    });
    const containerEpisodeScene = node({
      id: "container-scene",
      kind: "leaf",
      label: "씬 1",
      parent_id: "container-episode",
      ordinal: 0,
    });
    const containerEpisode = node({
      id: "container-episode",
      kind: "container",
      label: "2화",
      parent_id: "arc",
      ordinal: 1,
      children: [containerEpisodeScene],
    });
    const arc = node({
      id: "arc",
      kind: "container",
      label: "1권",
      ordinal: 0,
      children: [leafEpisode, containerEpisode],
    });
    const calls: string[] = [];
    const rpc: OutlineRepairRPC = {
      createChild: vi.fn(),
      createSibling: vi.fn(),
      moveToParent: vi.fn(async (id, parentID) => {
        calls.push(`move:${id}->${parentID}`);
        return { ok: true as const };
      }),
      moveToRoot: vi.fn(async (id) => {
        calls.push(`root:${id}`);
        return { ok: true as const };
      }),
      convertToContainer: vi.fn(async (id) => {
        calls.push(`convert:${id}`);
        return { ok: true as const };
      }),
      delete: vi.fn(async (id) => {
        calls.push(`delete:${id}`);
        return { ok: true as const };
      }),
      rename: vi.fn(async (id, label, title) => {
        calls.push(`rename:${id}:${label}:${title}`);
        return { ok: true as const };
      }),
    };

    await repairOutlineTree([arc], rpc, t, OUTLINE_PRESETS.webnovel);

    expect(calls).not.toContain("move:leaf-episode->container-episode");
    expect(calls).not.toContain("convert:leaf-episode");
    expect(calls).not.toContain("rename:container-episode:1화:");
    expect(calls.some((call) => call.startsWith("delete:"))).toBe(false);
  });
});
