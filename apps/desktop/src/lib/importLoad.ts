import { open } from "@tauri-apps/plugin-dialog";
import { readTextFile } from "@tauri-apps/plugin-fs";

export interface PickedMarkdown {
  fileName: string; // basename without trailing .md/.markdown
  content: string;
}

/** Open the OS file dialog and read the chosen .md file. Returns null when
 *  the user cancels. Throws on read failure. */
export async function pickAndReadMarkdown(): Promise<PickedMarkdown | null> {
  const picked = await open({
    multiple: false,
    directory: false,
    filters: [{ name: "Markdown", extensions: ["md", "markdown"] }],
  });
  if (!picked || typeof picked !== "string") return null;
  const content = await readTextFile(picked);
  const base = picked.split(/[\\/]/).pop() ?? "import";
  const fileName = base.replace(/\.(md|markdown)$/i, "");
  return { fileName, content };
}
