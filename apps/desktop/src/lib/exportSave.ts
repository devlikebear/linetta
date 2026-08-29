import { save } from "@tauri-apps/plugin-dialog";
import { writeTextFile } from "@tauri-apps/plugin-fs";
import type { ExportPayload } from "./types";
import type { useI18n } from "./i18n";

/** Open the OS save dialog seeded with the suggested filename, then write the
 *  markdown to disk. Returns the chosen path, or null if the user cancelled.
 *
 *  Only a filename is suggested, never a directory. That is deliberate — the
 *  writer's own last-used folder is a better guess than anything Linetta could
 *  pick — but it means the folder comes from the OS panel's memory, not from
 *  Linetta. A first export lands wherever the panel last was, which on macOS
 *  can be an iCloud Drive folder the writer does not expect. Hence
 *  `describeSaveLocation`: the app cannot choose the destination for them, so
 *  it says where the file went. */
export async function saveExportedMarkdown(payload: ExportPayload): Promise<string | null> {
  const path = await save({
    defaultPath: payload.suggested_filename,
    filters: [{ name: "Markdown", extensions: ["md"] }],
  });
  if (!path) return null;
  await writeTextFile(path, payload.markdown);
  return path;
}

/** Folder names the major consumer sync clients create. Matching on the folder
 *  rather than a full path keeps this working for a relocated iCloud home, a
 *  Dropbox on another volume, and the localized names Windows uses. */
const CLOUD_FOLDER_PATTERNS = [
  /(^|[/\\])Mobile ?Documents([/\\]|$)/i, // iCloud Drive's on-disk name
  /(^|[/\\])iCloud ?Drive([/\\]|$)/i,
  /(^|[/\\])Dropbox([/\\]|$)/i,
  /(^|[/\\])OneDrive[^/\\]*([/\\]|$)/i,
  /(^|[/\\])Google ?Drive([/\\]|$)/i,
  /(^|[/\\])My ?Drive([/\\]|$)/i,
];

export type SaveLocation = {
  /** The directory the file landed in, for showing the writer. */
  folder: string;
  /** True when that directory is inside a folder a sync client watches. */
  synced: boolean;
};

/** Describe where an export actually went.
 *
 *  A local-first app that quietly drops a manuscript into a synced folder has
 *  broken its own promise, and the writer cannot tell from the save dialog
 *  alone. This does not block anything — it is their disk — but it makes the
 *  destination something they saw rather than something they assumed. */
export function describeSaveLocation(path: string): SaveLocation {
  const cut = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  const folder = cut > 0 ? path.slice(0, cut) : path;
  return { folder, synced: CLOUD_FOLDER_PATTERNS.some((re) => re.test(folder)) };
}

type Translate = ReturnType<typeof useI18n>["t"];

/** The message shown after a successful export.
 *
 *  It names the folder rather than saying "done", because "done" is exactly
 *  what left issue #34 open: the writer had no way to tell whether the
 *  manuscript had just been handed to a sync client.
 */
export function exportDestinationMessage(t: Translate, path: string): string {
  const { folder, synced } = describeSaveLocation(path);
  return t(synced ? "export.savedToSynced" : "export.savedTo", { folder });
}
