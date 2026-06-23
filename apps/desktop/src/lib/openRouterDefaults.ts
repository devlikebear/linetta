export const OPENROUTER_SMART_DEFAULT_MODEL = "openai/gpt-5.4";
export const OPENROUTER_FAST_MODEL = "google/gemini-3-flash-preview";
export const OPENROUTER_AUTO_MODEL = "openrouter/auto";

export const OPENROUTER_DEFAULT_MODEL_OPTIONS = [
  OPENROUTER_SMART_DEFAULT_MODEL,
  OPENROUTER_FAST_MODEL,
  OPENROUTER_AUTO_MODEL,
];

export type OpenRouterRecommendedModel = {
  id: string;
  titleKey: OpenRouterRecommendedModelTitleKey;
  summaryKey: OpenRouterRecommendedModelSummaryKey;
};

export type OpenRouterRecommendedModelTitleKey =
  | "settings.setup.openrouter.modelPreset.quality"
  | "settings.setup.openrouter.modelPreset.fast"
  | "settings.setup.openrouter.modelPreset.auto";

export type OpenRouterRecommendedModelSummaryKey =
  | "settings.setup.openrouter.modelPreset.qualityHelp"
  | "settings.setup.openrouter.modelPreset.fastHelp"
  | "settings.setup.openrouter.modelPreset.autoHelp";

export const OPENROUTER_RECOMMENDED_MODELS: OpenRouterRecommendedModel[] = [
  {
    id: OPENROUTER_SMART_DEFAULT_MODEL,
    titleKey: "settings.setup.openrouter.modelPreset.quality",
    summaryKey: "settings.setup.openrouter.modelPreset.qualityHelp",
  },
  {
    id: OPENROUTER_FAST_MODEL,
    titleKey: "settings.setup.openrouter.modelPreset.fast",
    summaryKey: "settings.setup.openrouter.modelPreset.fastHelp",
  },
  {
    id: OPENROUTER_AUTO_MODEL,
    titleKey: "settings.setup.openrouter.modelPreset.auto",
    summaryKey: "settings.setup.openrouter.modelPreset.autoHelp",
  },
];

export function organizeOpenRouterModelOptions(models: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  const add = (model: string) => {
    const id = model.trim();
    if (!id || seen.has(id) || isOpenRouterNonTextModel(id)) return;
    seen.add(id);
    out.push(id);
  };
  OPENROUTER_DEFAULT_MODEL_OPTIONS.forEach(add);
  models.forEach(add);
  return out;
}

function isOpenRouterNonTextModel(model: string): boolean {
  const id = model.trim().toLowerCase();
  if (!id) return true;
  return [
    "image",
    "img",
    "tts",
    "audio",
    "speech",
    "music",
    "video",
    "veo",
    "lyria",
    "stable-diffusion",
    "flux",
    "dall-e",
    "midjourney",
  ].some((marker) => id.includes(marker));
}
