import { invoke } from "@tauri-apps/api/core";
import type {
  AIOptions,
  Beat,
  Entity,
  ExportPayload,
  ListProjectsParams,
  NewBeatInput,
  NewEntityInput,
  NewProjectInput,
  NewRelationshipInput,
  NewRelationshipPairInput,
  NewThreadInput,
  NodeRow,
  Project,
  Relationship,
  Settings,
  SettingsPatch,
  Snapshot,
  SnapshotEntry,
  Thread,
  UpdateBeatInput,
  UpdateEntityInput,
  UpdateRelationshipInput,
  UpdateThreadInput,
} from "./types";

// Tauri commands defined in src-tauri.

export async function enginePing(): Promise<string> {
  return invoke<string>("engine_ping");
}

export async function rpcCall<T>(method: string, params?: unknown): Promise<T> {
  return invoke<T>("engine_call", { method, params: params ?? null });
}

export const projects = {
  create: (input: NewProjectInput) => rpcCall<Project>("projects.create", input),
  list: (params: ListProjectsParams = {}) => rpcCall<Project[]>("projects.list", params),
  get: (id: string) => rpcCall<Project>("projects.get", { id }),
  archive: (id: string) => rpcCall<{ ok: true }>("projects.archive", { id }),
};

export const nodes = {
  get: (id: string) => rpcCall<NodeRow>("nodes.get", { id }),
  updateContent: (id: string, doc: string) =>
    rpcCall<NodeRow>("nodes.update_content", { id, doc }),
  setLastOpened: (projectId: string, nodeId: string) =>
    rpcCall<{ ok: true }>("nodes.set_last_opened", { project_id: projectId, node_id: nodeId }),
  listTree: (projectId: string) =>
    rpcCall<NodeRow[]>("nodes.list_tree", { project_id: projectId }),
  createSibling: (referenceId: string, kind: "leaf" | "container", label: string, title: string) =>
    rpcCall<NodeRow>("nodes.create_sibling", { reference_id: referenceId, kind, label, title }),
  createChild: (parentId: string, kind: "leaf" | "container", label: string, title: string) =>
    rpcCall<NodeRow>("nodes.create_child", { parent_id: parentId, kind, label, title }),
  rename: (id: string, label: string, title: string) =>
    rpcCall<{ ok: true }>("nodes.rename", { id, label, title }),
  delete: (id: string) => rpcCall<{ ok: true }>("nodes.delete", { id }),
  moveUp: (id: string) => rpcCall<{ ok: true }>("nodes.move_up", { id }),
  moveDown: (id: string) => rpcCall<{ ok: true }>("nodes.move_down", { id }),
};

export const snapshots = {
  createManual: (nodeId: string, doc: string) =>
    rpcCall<Snapshot>("snapshots.create_manual", { node_id: nodeId, doc }),
  listForNode: (nodeId: string) =>
    rpcCall<SnapshotEntry[]>("snapshots.list_for_node", { node_id: nodeId }),
  restore: (snapshotId: string) =>
    rpcCall<NodeRow>("snapshots.restore", { snapshot_id: snapshotId }),
};

export const settings = {
  get: () => rpcCall<Settings>("settings.get"),
  set: (patch: SettingsPatch) => rpcCall<Settings>("settings.set", patch),
};

export const exportApi = {
  project: (projectId: string) =>
    rpcCall<ExportPayload>("export.project", { project_id: projectId }),
  node: (nodeId: string) =>
    rpcCall<ExportPayload>("export.node", { node_id: nodeId }),
};

export const entities = {
  search: (projectId: string, query: string, limit = 20) =>
    rpcCall<Entity[]>("entities.search", { project_id: projectId, query, limit }),
  get: (id: string) => rpcCall<Entity>("entities.get", { id }),
  create: (input: NewEntityInput) => rpcCall<Entity>("entities.create", input),
  update: (input: UpdateEntityInput) => rpcCall<Entity>("entities.update", input),
};

export const mentions = {
  listForNode: (nodeId: string) =>
    rpcCall<Entity[]>("mentions.list_for_node", { node_id: nodeId }),
};

export const ai = {
  run: (nodeId: string, prompt: string, options: AIOptions) =>
    rpcCall<{ run_id: string }>("ai.run", { node_id: nodeId, prompt, options }),
  cancel: (runId: string) => rpcCall<{ ok: true }>("ai.cancel", { run_id: runId }),
};

export const threads = {
  create: (input: NewThreadInput) => rpcCall<Thread>("threads.create", input),
  list: (projectId: string, includeClosed = false) =>
    rpcCall<Thread[]>("threads.list", { project_id: projectId, include_closed: includeClosed }),
  get: (id: string) => rpcCall<Thread>("threads.get", { id }),
  update: (input: UpdateThreadInput) => rpcCall<Thread>("threads.update", input),
  close: (id: string) => rpcCall<Thread>("threads.close", { id }),
  reopen: (id: string) => rpcCall<Thread>("threads.reopen", { id }),
};

export const beats = {
  create: (input: NewBeatInput) => rpcCall<Beat>("beats.create", input),
  listByThread: (threadId: string) =>
    rpcCall<Beat[]>("beats.list_by_thread", { thread_id: threadId }),
  listByNode: (nodeId: string) =>
    rpcCall<Beat[]>("beats.list_by_node", { node_id: nodeId }),
  update: (input: UpdateBeatInput) => rpcCall<Beat>("beats.update", input),
  reorder: (threadId: string, ids: string[]) =>
    rpcCall<{ ok: true }>("beats.reorder", { thread_id: threadId, ids }),
  delete: (id: string) => rpcCall<{ ok: true }>("beats.delete", { id }),
};

export const relationships = {
  createOne: (input: NewRelationshipInput) =>
    rpcCall<Relationship>("relationships.create_one", input),
  createPair: (input: NewRelationshipPairInput) =>
    rpcCall<Relationship[]>("relationships.create_pair", input),
  listByEntity: (entityId: string) =>
    rpcCall<Relationship[]>("relationships.list_by_entity", { entity_id: entityId }),
  update: (input: UpdateRelationshipInput) =>
    rpcCall<Relationship>("relationships.update", input),
  delete: (id: string) => rpcCall<{ ok: true }>("relationships.delete", { id }),
};
