import { save } from "@tauri-apps/plugin-dialog";
import { writeTextFile } from "@tauri-apps/plugin-fs";
import type { ExportPayload } from "./types";

/** Open the OS save dialog seeded with the suggested filename, then write the
 *  markdown to disk. Returns the chosen path, or null if the user cancelled. */
export async function saveExportedMarkdown(payload: ExportPayload): Promise<string | null> {
  const path = await save({
    defaultPath: payload.suggested_filename,
    filters: [{ name: "Markdown", extensions: ["md"] }],
  });
  if (!path) return null;
  await writeTextFile(path, payload.markdown);
  return path;
}
