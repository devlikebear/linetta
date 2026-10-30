import type { AnyExtension } from "@tiptap/core";
import { NoteMarkerExtension } from "./NoteMarkerExtension";

/** The extensions every scene editor mounts on top of Tiptap.tsx's own
 *  defaults. The workspace editor and Zen mode both take this one list: an
 *  editor missing one of these nodes opens a scene that uses it as an empty
 *  doc, and its next save overwrites the scene with that empty doc. */
export function sceneEditorExtensions(mention: AnyExtension): AnyExtension[] {
  return [mention, NoteMarkerExtension];
}
