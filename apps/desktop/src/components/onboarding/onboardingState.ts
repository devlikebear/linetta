import type { Settings } from "../../lib/types";

export type OnboardingPhase = "library" | "workspace";

export const CURRENT_ONBOARDING_TOUR_VERSION = "library-workspace-v1";
export const MANUAL_PHASE_STORAGE_KEY = "linetta:onboarding:manual-phase";
export const WORKSPACE_PENDING_STORAGE_KEY = "linetta:onboarding:workspace-pending";

function storage(): Storage | null {
  try {
    return window.localStorage ?? null;
  } catch {
    return null;
  }
}

export function readStoredPhase(key: string): OnboardingPhase | null {
  const s = storage();
  if (!s || typeof s.getItem !== "function") return null;
  const value = s.getItem(key);
  return value === "library" || value === "workspace" ? value : null;
}

export function storePhase(key: string, phase: OnboardingPhase) {
  const s = storage();
  if (s && typeof s.setItem === "function") s.setItem(key, phase);
}

export function clearStoredPhase(key: string) {
  const s = storage();
  if (s && typeof s.removeItem === "function") s.removeItem(key);
}

export function shouldAutoStartOnboarding(settings: Settings | null | undefined): boolean {
  if (!settings) return false;
  if (settings.onboarding_tour_enabled === false) return false;
  return settings.onboarding_tour_seen_version !== CURRENT_ONBOARDING_TOUR_VERSION;
}
