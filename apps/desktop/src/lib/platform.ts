/**
 * Which desktop OS the shell is running on.
 *
 * Several strings differ per platform for reasons that have nothing to do with
 * each other -- where a key is stored, where a CLI is installed -- so the
 * detection lives here once rather than in each consumer.
 */
export type PlatformKind = "macos" | "windows" | "other";

export function platformKind(): PlatformKind {
  const platform = navigator.platform.toLowerCase();
  if (platform.includes("mac")) return "macos";
  if (platform.includes("win")) return "windows";
  return "other";
}

export function isWindows(): boolean {
  return platformKind() === "windows";
}
