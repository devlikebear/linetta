import type { SizeClass } from "./useSizeClass";

export interface InspectorState {
  factBook: boolean;
  contextual: boolean;
}

type Key = keyof InspectorState;
const PRIORITY: Key[] = ["factBook", "contextual"];

export function reconcileInspector(
  prev: InspectorState,
  next: InspectorState,
  sizeClass: SizeClass,
): InspectorState {
  if (sizeClass !== "ipad") return next;
  const openCount = PRIORITY.filter((k) => next[k]).length;
  if (openCount <= 1) return next;
  const justOpened = PRIORITY.find((k) => !prev[k] && next[k]);
  const winner = justOpened ?? PRIORITY.find((k) => next[k])!;
  return {
    factBook: winner === "factBook",
    contextual: winner === "contextual",
  };
}
