import type { ToneID } from "./types";

// Chip presets shown in AIMode and AIContextPanel. Single-select; clicking a
// chip sets aiOptions.tone. Default is "my" (preserves the pre-Plan-11
// `tone_preset: true` behavior — emit project.style_notes into the system prompt).
export const TONE_PRESETS: ReadonlyArray<{ id: ToneID; label: string }> = [
  { id: "my", label: "내 톤" },
  { id: "cool", label: "차갑게" },
  { id: "sensory", label: "감각적" },
  { id: "dry", label: "건조하게" },
  { id: "tense", label: "긴장감" },
  { id: "lyrical", label: "서정" },
  { id: "humor", label: "유머" },
];
