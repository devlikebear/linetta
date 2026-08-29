import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const repoApp = resolve(here, "../..");

/**
 * The webview can only reach engine methods named in RENDERER_ENGINE_METHODS.
 * Adding an RPC wrapper and forgetting the allowlist is this codebase's most
 * repeated bug: nothing fails to compile, nothing fails to build, the feature
 * just does nothing when a writer clicks it.
 *
 * Two files, one list, no compiler between them — so the check lives here.
 */
async function callers(): Promise<string[]> {
  const src = await readFile(resolve(repoApp, "src/lib/rpc.ts"), "utf8");
  const found = new Set<string>();
  // Every call site is a literal: `rpcCall<T>("some.method", …)`.
  for (const m of src.matchAll(/rpcCall<[^>]*>\(\s*"([a-z0-9_.]+)"/g)) {
    found.add(m[1]);
  }
  return [...found].sort();
}

async function allowed(): Promise<string[]> {
  const src = await readFile(resolve(repoApp, "src-tauri/src/lib.rs"), "utf8");
  const block = src.slice(
    src.indexOf("const RENDERER_ENGINE_METHODS"),
    src.indexOf("];", src.indexOf("const RENDERER_ENGINE_METHODS")),
  );
  return [...block.matchAll(/"([a-z0-9_.]+)"/g)].map((m) => m[1]);
}

describe("renderer engine method allowlist", () => {
  it("finds both sides (guards against the extraction silently breaking)", async () => {
    expect((await callers()).length).toBeGreaterThan(50);
    expect((await allowed()).length).toBeGreaterThan(50);
  });

  it("allows every method the renderer actually calls", async () => {
    const list = await allowed();
    const missing = (await callers()).filter((m) => !list.includes(m));
    expect(missing, "add these to RENDERER_ENGINE_METHODS in src-tauri/src/lib.rs").toEqual([]);
  });

  it("carries no method the renderer no longer calls", async () => {
    // Not fatal at runtime, but a stale entry is a widened surface nobody meant
    // to keep — the comment above the list says new methods get reviewed.
    const called = await callers();
    const stale = (await allowed()).filter((m) => !called.includes(m));
    expect(stale).toEqual([]);
  });

  it("stays sorted, because the lookup is a binary search", async () => {
    // `is_renderer_engine_method` calls `binary_search`. An out-of-order entry
    // is not a lint problem: the method becomes unreachable.
    const list = await allowed();
    expect(list).toEqual([...list].sort());
  });
});
