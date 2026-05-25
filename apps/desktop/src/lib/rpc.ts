import { invoke } from "@tauri-apps/api/core";
import type {
  Entity,
  ListProjectsParams,
  NewEntityInput,
  NewProjectInput,
  NodeRow,
  Project,
  Snapshot,
  UpdateEntityInput,
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
