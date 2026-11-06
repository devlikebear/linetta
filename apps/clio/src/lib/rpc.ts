import { invoke } from "@tauri-apps/api/core";
import type {
  ContextCount,
  ContextProfile,
  EngineStatus,
  ImportParams,
  ImportPreview,
  ImportResult,
  ListPostsParams,
  Post,
  VoiceProfileParams,
} from "./types";

// Tauri commands defined in src-tauri.

export async function enginePing(): Promise<string> {
  return invoke<string>("engine_ping");
}

export async function engineStatus(): Promise<EngineStatus> {
  return invoke<EngineStatus>("engine_status");
}

export async function openExternalUrl(url: string): Promise<void> {
  return invoke<void>("open_external_url", { url });
}

/** A JSON-RPC error from the engine, with its code and the method that failed. */
export class RpcError extends Error {
  readonly code?: number;
  readonly data?: unknown;
  readonly method: string;
  readonly requestId?: number;

  constructor(method: string, message: string, code?: number, data?: unknown, requestId?: number) {
    super(message);
    this.name = "RpcError";
    this.method = method;
    this.code = code;
    this.data = data;
    this.requestId = requestId;
  }
}

function normalizeRpcError(method: string, error: unknown): RpcError {
  if (error && typeof error === "object") {
    const wire = error as { code?: unknown; message?: unknown; data?: unknown; request_id?: unknown };
    if (typeof wire.message === "string") {
      return new RpcError(
        method,
        wire.message,
        typeof wire.code === "number" ? wire.code : undefined,
        wire.data,
        typeof wire.request_id === "number" ? wire.request_id : undefined,
      );
    }
  }
  return new RpcError(method, error instanceof Error ? error.message : String(error));
}

export async function rpcCall<T>(method: string, params?: unknown): Promise<T> {
  try {
    return await invoke<T>("engine_call", { method, params: params ?? null });
  } catch (error) {
    throw normalizeRpcError(method, error);
  }
}

export const posts = {
  importPreview: (params: ImportParams) => rpcCall<ImportPreview>("posts.import_preview", params),
  import: (params: ImportParams) => rpcCall<ImportResult>("posts.import", params),
  // The engine marshals an empty list as null; normalise so callers can map it.
  list: async (params: ListPostsParams = {}) =>
    (await rpcCall<Post[] | null>("posts.list", params)) ?? [],
  counts: async () => (await rpcCall<ContextCount[] | null>("posts.counts")) ?? [],
};

export const voice = {
  profile: (params: VoiceProfileParams) => rpcCall<ContextProfile>("voice.profile", params),
};
