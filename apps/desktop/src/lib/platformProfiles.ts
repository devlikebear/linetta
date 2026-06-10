import type { PlatformProfileId } from "./types";

export const PLATFORM_PROFILE_IDS = ["plain", "munpia", "series", "joara"] as const;

export function normalizePlatformProfile(profile: string | null | undefined): PlatformProfileId {
  return PLATFORM_PROFILE_IDS.includes(profile as PlatformProfileId)
    ? profile as PlatformProfileId
    : "plain";
}

export function transformPlatformText(text: string, profile: PlatformProfileId): string {
  switch (normalizePlatformProfile(profile)) {
    case "munpia":
      return normalizeFullwidthTilde(normalizeEllipsis(collapseBlankLines(trimLineEndSpaces(normalizeNewlines(text)))));
    case "series":
    case "joara":
      return normalizeEllipsis(collapseBlankLines(trimLineEndSpaces(normalizeNewlines(text))));
    case "plain":
    default:
      return text;
  }
}

function normalizeNewlines(text: string): string {
  return text.replace(/\r\n?/g, "\n");
}

function trimLineEndSpaces(text: string): string {
  return text.replace(/[ \t]+$/gm, "");
}

function collapseBlankLines(text: string): string {
  return text.replace(/\n{3,}/g, "\n\n");
}

function normalizeEllipsis(text: string): string {
  return text.replace(/(?:\.{3,}|…)+/g, "…");
}

function normalizeFullwidthTilde(text: string): string {
  return text.replace(/[～〜]/g, "~");
}
