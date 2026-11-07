import { rpcCall } from "./rpc";
import type { CheckParams, CheckResult, CreateDraftParams, Draft, UpdateDraftParams } from "./types";

/** JSON-RPC code the engine returns for a stale content_version. */
export const CODE_CONTENT_CONFLICT = -32009;

export const drafts = {
  create: (params: CreateDraftParams) => rpcCall<Draft>("drafts.create", params),
  get: (id: number) => rpcCall<Draft>("drafts.get", { id }),
  // The engine may marshal an empty list as null.
  list: async () => (await rpcCall<Draft[] | null>("drafts.list", {})) ?? [],
  update: (params: UpdateDraftParams) => rpcCall<Draft>("drafts.update", params),
  delete: (id: number) => rpcCall<unknown>("drafts.delete", { id }),
};

export const check = {
  run: async (params: CheckParams): Promise<CheckResult> => {
    const res = await rpcCall<CheckResult>("check.run", params);
    return { ...res, findings: res.findings ?? [] };
  },
};
