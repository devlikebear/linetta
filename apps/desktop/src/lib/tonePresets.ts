import type { ToneID } from "./types";

// Chip presets shown in AIMode and AIContextPanel. Single-select; clicking a
// chip sets aiOptions.tone. Default is "my" (preserves the pre-Plan-11
// `tone_preset: true` behavior — emit project.style_notes into the system prompt).
export const TONE_PRESETS: ReadonlyArray<{ id: ToneID }> = [
  { id: "my" },
  { id: "cool" },
  { id: "sensory" },
  { id: "dry" },
  { id: "tense" },
  { id: "lyrical" },
  { id: "humor" },
];
