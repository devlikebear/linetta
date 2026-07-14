import type { Settings } from "./types";
import type { Editor } from "@tiptap/react";

export interface LinettaEventMap {
  "linetta:settings-updated": Settings;
  "linetta:note-hover": { noteId: string; target: HTMLElement };
  "linetta:note-hover-end": { noteId: string };
  "linetta:note-click": { noteId: string; target: HTMLElement };
  "linetta:mention-pick-new": {
    query: string;
    range: { from: number; to: number };
    editor: Editor;
  };
}

export function dispatchAppEvent<Name extends keyof LinettaEventMap>(
  name: Name,
  detail: LinettaEventMap[Name],
): void {
  window.dispatchEvent(new CustomEvent(name, { detail }));
}

export function subscribeAppEvent<Name extends keyof LinettaEventMap>(
  name: Name,
  listener: (detail: LinettaEventMap[Name]) => void,
): () => void {
  const handler = (event: Event) => listener((event as CustomEvent<LinettaEventMap[Name]>).detail);
  window.addEventListener(name, handler);
  return () => window.removeEventListener(name, handler);
}
