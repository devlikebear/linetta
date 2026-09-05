import type { SizeClass } from "./useSizeClass";

export interface InspectorState {
  factBook: boolean;
  contextual: boolean;
  canon: boolean;
  agent: boolean;
}

type Key = keyof InspectorState;
const PRIORITY: Key[] = ["factBook", "contextual", "canon", "agent"];

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
  // Built from PRIORITY rather than named field-by-field, so a fourth panel is
  // one entry in the list above instead of an edit here that is easy to skip.
  const only = {} as InspectorState;
  for (const k of PRIORITY) only[k] = k === winner;
  return only;
}
