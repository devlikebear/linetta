import type { CompanionHistoryScope } from "./types";

const STORAGE_PREFIX = "linetta:companion:scope:";

function storage(): Storage | null {
  try {
    return window.localStorage ?? null;
  } catch {
    return null;
  }
}

export function companionScopeStorageKey(projectId: string): string {
  return `${STORAGE_PREFIX}${projectId}`;
}

// The last companion scope is remembered per project, so reopening the panel
// keeps the writer where they left off while switching projects never inherits
// another project's scope.
export function readStoredCompanionScope(projectId: string): CompanionHistoryScope | null {
  if (!projectId) return null;
  const s = storage();
  if (!s || typeof s.getItem !== "function") return null;
  try {
    const value = s.getItem(companionScopeStorageKey(projectId));
    return value === "scene" || value === "project" ? value : null;
  } catch {
    return null;
  }
}

export function storeCompanionScope(projectId: string, scope: CompanionHistoryScope): void {
  if (!projectId) return;
  const s = storage();
  if (!s || typeof s.setItem !== "function") return;
  try {
    s.setItem(companionScopeStorageKey(projectId), scope);
  } catch {
    // Storage can be unavailable (private mode, quota). Scope memory is a
    // convenience, so a failed write must never break sending a message.
  }
}
