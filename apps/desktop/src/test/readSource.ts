import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const appRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");

/** Read a file under `src/`, for tests that assert on source text.
 *
 *  Several things in this app cannot be checked by rendering: that a hook has
 *  a mount site, that a CSS rule survived a refactor, that a callback is in an
 *  effect's dependency list. Those tests read the source and assert on it.
 *
 *  The three-line path dance this replaces was copied into every one of them.
 */
export function readSource(path: string): Promise<string> {
  return readFile(resolve(appRoot, "src", path), "utf8");
}

/** Read a file anywhere under `apps/desktop`, for the few assertions that
 *  cross into the Tauri shell. */
export function readAppFile(path: string): Promise<string> {
  return readFile(resolve(appRoot, path), "utf8");
}
