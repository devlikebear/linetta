# iPad/iOS Product UX (Sub-project A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Linetta a first-class iPad app (Universal iPhone+iPad) by adding a touch-tuned iPad layout tier and finishing the iOS feature-reduction UX, without rewriting the component tree.

**Architecture:** Keep the existing "render-once + CSS reshapes" model. Add a third size tier (`compact | ipad | desktop`) resolved by width + `(pointer: coarse)`, a thin "one inspector at a time" orchestration for iPad, a new iPad `@media` block layered after the compact block, and corrected iOS feature-gating (CLI providers + git-sync) surfaced as explicit UX rather than silent removal.

**Tech Stack:** React 18 + TypeScript + Vite, Tauri 2 (iOS), vitest, Go 1.26 engine (build tags `mas`/`mobile`), CSS `@media` with `pointer`/`env(safe-area-inset-*)`.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-06-27-ipad-ios-product-ux-design.md`.
- Size tiers (intent-based, evaluated desktop → ipad → compact):
  - **desktop**: `(min-width: 1181px), (pointer: fine)`
  - **ipad**: `(min-width: 701px) and (min-height: 600px) and (pointer: coarse)` — the `min-height: 600px` guard excludes iPhone landscape (height ≤ 430px), which must stay **compact**.
  - **compact**: neither of the above (fallback).
- The existing compact CSS block `@media (max-width: 860px)` stays **untouched**; the new iPad block is layered **after** it so coarse-pointer devices in the 701–860 overlap get iPad rules while fine-pointer desktop windows keep compact rules.
- Out of scope (other sub-projects): Apple Pencil/Scribble (B); signing/provisioning/`keychain-access-groups`/TestFlight/App Store Connect (C); native `UIKeyCommand` HUD; first-class trackpad/pointer and Split View/Stage Manager.
- Engine module path: `github.com/devlikebear/linetta/engine`. Build tags compose: `mas` and `mobile` are orthogonal; never break the `mas` build.
- All new frontend files use the repo's existing import style and 2-space indentation. Run frontend tests with `cd apps/desktop && pnpm test`. Run engine tests with `cd engine && go test ...`.
- TDD: write the failing test first, watch it fail, implement minimally, watch it pass, commit.

---

## Task 1: Gate CLI providers unavailable on `mobile` builds (engine)

The frontend hides providers listed in diagnostics `unavailable_providers`. Today only the `mas` build reports `claude-code-cli`/`openai-codex` as unavailable (`engine/internal/ai/guard_mas.go` is `//go:build mas`). A pure `mobile` iOS build (no `mas` tag) would `!mas` → `guard_enabled.go` → return `nil`, so those exec/sandbox-incompatible providers would appear selectable on iOS and then fail. The exact reasons in `guard_mas.go` (subprocess exec blocked, sandboxed home) apply identically to mobile. Mirror the existing `mas || mobile` gating already used by `gitsync_disabled.go` and `clidetect_mas.go`.

**Files:**
- Modify: `engine/internal/ai/guard_mas.go:1` (build tag)
- Modify: `engine/internal/ai/guard_enabled.go:1` (build tag)
- Test: `engine/internal/ai/guard_mobile_test.go` (create)

**Interfaces:**
- Consumes: nothing new.
- Produces: under `-tags mobile`, `ai.UnavailableProviders()` returns `["claude-code-cli", "openai-codex"]` (sorted); `ai.guardProvider("claude-code-cli")` returns a non-nil error.

- [ ] **Step 1: Write the failing test**

```go
//go:build mobile

// engine/internal/ai/guard_mobile_test.go
package ai

import "testing"

func TestMobileBlocksCLIAndCodex(t *testing.T) {
	got := UnavailableProviders()
	want := map[string]bool{"claude-code-cli": true, "openai-codex": true}
	if len(got) != len(want) {
		t.Fatalf("UnavailableProviders() = %v, want keys %v", got, want)
	}
	for _, p := range got {
		if !want[p] {
			t.Fatalf("unexpected provider %q in %v", p, got)
		}
	}
	if err := guardProvider("claude-code-cli"); err == nil {
		t.Fatal("guardProvider(claude-code-cli) = nil, want error on mobile build")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd engine && go test -tags mobile ./internal/ai/ -run TestMobileBlocksCLIAndCodex -v`
Expected: FAIL — under `mobile` the `guard_enabled.go` variant compiles (`UnavailableProviders` returns nil, `guardProvider` returns nil).

- [ ] **Step 3: Change the build tags**

In `engine/internal/ai/guard_mas.go`, change line 1:

```go
//go:build mas || mobile
```

In `engine/internal/ai/guard_enabled.go`, change line 1:

```go
//go:build !mas && !mobile
```

(Optionally update the `guard_mas.go` doc comment wording from "App Store (sandboxed) build" to "App Store and mobile (sandboxed) builds"; behavior is unchanged for `mas`.)

- [ ] **Step 4: Run tests to verify pass + no regressions across tag profiles**

Run: `cd engine && go test -tags mobile ./internal/ai/ -run TestMobileBlocksCLIAndCodex -v && go test ./internal/ai/ && go test -tags mas ./internal/ai/ && go test -tags mobile ./internal/ai/`
Expected: PASS for all (default build still returns no unavailable providers; `mas` and `mobile` both block the two providers).

- [ ] **Step 5: Commit**

```bash
git add engine/internal/ai/guard_mas.go engine/internal/ai/guard_enabled.go engine/internal/ai/guard_mobile_test.go
git commit -m "feat(engine): report CLI/codex providers unavailable on mobile builds"
```

---

## Task 2: `useSizeClass` hook

**Files:**
- Create: `apps/desktop/src/hooks/useSizeClass.ts`
- Test: `apps/desktop/src/hooks/useSizeClass.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type SizeClass = "compact" | "ipad" | "desktop"`
  - `const DESKTOP_QUERY = "(min-width: 1181px), (pointer: fine)"`
  - `const IPAD_QUERY = "(min-width: 701px) and (min-height: 600px) and (pointer: coarse)"`
  - `function resolveSizeClass(matches: { desktop: boolean; ipad: boolean }): SizeClass`
  - `function useSizeClass(): SizeClass`

- [ ] **Step 1: Write the failing test**

```ts
// apps/desktop/src/hooks/useSizeClass.test.ts
import { describe, expect, it } from "vitest";
import { resolveSizeClass } from "./useSizeClass";

describe("resolveSizeClass", () => {
  it("prefers desktop when desktop query matches", () => {
    expect(resolveSizeClass({ desktop: true, ipad: false })).toBe("desktop");
    // a 12.9" iPad in landscape matches BOTH (min-width:1181) and coarse → desktop wins
    expect(resolveSizeClass({ desktop: true, ipad: true })).toBe("desktop");
  });

  it("returns ipad when only the ipad query matches", () => {
    expect(resolveSizeClass({ desktop: false, ipad: true })).toBe("ipad");
  });

  it("falls back to compact when neither matches", () => {
    expect(resolveSizeClass({ desktop: false, ipad: false })).toBe("compact");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/desktop && pnpm test -- useSizeClass`
Expected: FAIL — `resolveSizeClass` not exported / module missing.

- [ ] **Step 3: Write minimal implementation**

```ts
// apps/desktop/src/hooks/useSizeClass.ts
import { useEffect, useState } from "react";

export type SizeClass = "compact" | "ipad" | "desktop";

export const DESKTOP_QUERY = "(min-width: 1181px), (pointer: fine)";
export const IPAD_QUERY =
  "(min-width: 701px) and (min-height: 600px) and (pointer: coarse)";

export function resolveSizeClass(matches: {
  desktop: boolean;
  ipad: boolean;
}): SizeClass {
  if (matches.desktop) return "desktop";
  if (matches.ipad) return "ipad";
  return "compact";
}

function readSizeClass(): SizeClass {
  if (typeof window === "undefined" || !window.matchMedia) return "desktop";
  return resolveSizeClass({
    desktop: window.matchMedia(DESKTOP_QUERY).matches,
    ipad: window.matchMedia(IPAD_QUERY).matches,
  });
}

export function useSizeClass(): SizeClass {
  const [cls, setCls] = useState<SizeClass>(readSizeClass);
  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const desktop = window.matchMedia(DESKTOP_QUERY);
    const ipad = window.matchMedia(IPAD_QUERY);
    const update = () =>
      setCls(resolveSizeClass({ desktop: desktop.matches, ipad: ipad.matches }));
    desktop.addEventListener("change", update);
    ipad.addEventListener("change", update);
    update();
    return () => {
      desktop.removeEventListener("change", update);
      ipad.removeEventListener("change", update);
    };
  }, []);
  return cls;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/desktop && pnpm test -- useSizeClass`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src/hooks/useSizeClass.ts apps/desktop/src/hooks/useSizeClass.test.ts
git commit -m "feat(workspace): add useSizeClass tier hook"
```

---

## Task 3: `reconcileInspector` (one-inspector-at-a-time on iPad)

**Files:**
- Create: `apps/desktop/src/hooks/inspector.ts`
- Test: `apps/desktop/src/hooks/inspector.test.ts`

**Interfaces:**
- Consumes: `SizeClass` from Task 2.
- Produces:
  - `interface InspectorState { companion: boolean; factBook: boolean; contextual: boolean }`
  - `function reconcileInspector(prev: InspectorState, next: InspectorState, sizeClass: SizeClass): InspectorState` — on `ipad`, allows at most one panel open: the one that just transitioned closed→open wins; if none just opened but multiple are open, the first of `companion → factBook → contextual` wins. On `compact`/`desktop`, returns `next` unchanged. Idempotent (re-running on its own output is a no-op).

- [ ] **Step 1: Write the failing test**

```ts
// apps/desktop/src/hooks/inspector.test.ts
import { describe, expect, it } from "vitest";
import { reconcileInspector, type InspectorState } from "./inspector";

const S = (c: boolean, f: boolean, x: boolean): InspectorState => ({
  companion: c,
  factBook: f,
  contextual: x,
});

describe("reconcileInspector", () => {
  it("leaves non-ipad tiers untouched", () => {
    const next = S(true, true, false);
    expect(reconcileInspector(S(false, false, false), next, "desktop")).toEqual(next);
    expect(reconcileInspector(S(false, false, false), next, "compact")).toEqual(next);
  });

  it("on ipad keeps the panel that just opened, closing the rest", () => {
    // companion was open; factBook just opened → keep factBook only
    const out = reconcileInspector(S(true, false, false), S(true, true, false), "ipad");
    expect(out).toEqual(S(false, true, false));
  });

  it("on ipad keeps a single open panel as-is", () => {
    const out = reconcileInspector(S(false, false, false), S(false, false, true), "ipad");
    expect(out).toEqual(S(false, false, true));
  });

  it("is idempotent on its own output", () => {
    const once = reconcileInspector(S(true, false, false), S(true, true, false), "ipad");
    const twice = reconcileInspector(S(true, false, false), once, "ipad");
    expect(twice).toEqual(once);
  });

  it("falls back to priority order when multiple are open with no fresh opener", () => {
    const out = reconcileInspector(S(false, true, true), S(false, true, true), "ipad");
    expect(out).toEqual(S(false, true, false)); // factBook before contextual
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/desktop && pnpm test -- inspector`
Expected: FAIL — module/function missing.

- [ ] **Step 3: Write minimal implementation**

```ts
// apps/desktop/src/hooks/inspector.ts
import type { SizeClass } from "./useSizeClass";

export interface InspectorState {
  companion: boolean;
  factBook: boolean;
  contextual: boolean;
}

type Key = keyof InspectorState;
const PRIORITY: Key[] = ["companion", "factBook", "contextual"];

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
    companion: winner === "companion",
    factBook: winner === "factBook",
    contextual: winner === "contextual",
  };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/desktop && pnpm test -- inspector`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src/hooks/inspector.ts apps/desktop/src/hooks/inspector.test.ts
git commit -m "feat(workspace): add reconcileInspector for ipad single-inspector rule"
```

---

## Task 4: Wire `Workspace.tsx` to the size tier + inspector rule

Replace the binary `isCompactWorkspace()`/`COMPACT_WORKSPACE_QUERY` rail seeding with the tier hook, seed the outline rail by iPad orientation (portrait collapsed, landscape expanded), and apply `reconcileInspector` in an effect. Then update the existing source-assertion responsive test so it reflects the new code.

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx` (lines ~62–68 query/helper; ~125 rail state; ~141–147 panel state; ~190–199 matchMedia effect)
- Modify: `apps/desktop/src/routes/Workspace.responsive.test.ts` (lines ~13–19 — the compact-layout source assertions)

**Interfaces:**
- Consumes: `useSizeClass`, `SizeClass` (Task 2); `reconcileInspector`, `InspectorState` (Task 3).
- Produces: a `sizeClass` value in `Workspace` scope used by later CSS-class wiring (no exported API).

- [ ] **Step 1: Update the source-assertion test to the new contract (failing)**

In `apps/desktop/src/routes/Workspace.responsive.test.ts`, replace the first `it(...)` block's assertions (the ones referencing `COMPACT_WORKSPACE_QUERY`/`isCompactWorkspace`) with:

```ts
  it("derives the size tier and seeds the outline rail per tier", async () => {
    const workspace = await readSource("routes/Workspace.tsx");

    expect(workspace).toContain('import { useSizeClass } from "../hooks/useSizeClass"');
    expect(workspace).toContain("const sizeClass = useSizeClass()");
    expect(workspace).toContain('import { reconcileInspector } from "../hooks/inspector"');
    expect(workspace).toContain("reconcileInspector(");
    expect(workspace).toContain('className="ws-tool icon-only mobile-outline-toggle"');
  });
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd apps/desktop && pnpm test -- Workspace.responsive`
Expected: FAIL — the new imports/symbols are not in `Workspace.tsx` yet.

- [ ] **Step 3: Wire the hook + rail seeding**

In `apps/desktop/src/routes/Workspace.tsx`:

1. Add imports near the other route imports (top of file):

```ts
import { useSizeClass } from "../hooks/useSizeClass";
import { reconcileInspector, type InspectorState } from "../hooks/inspector";
```

2. Delete the now-unused `COMPACT_WORKSPACE_QUERY` constant (line ~62) and the `isCompactWorkspace` helper (lines ~64–68).

3. Replace the `railCollapsed` initializer (line ~125) with a tier-aware seed. First add the hook at the top of the component body (just after `const { projectId } = useParams();`):

```ts
  const sizeClass = useSizeClass();
```

   Then change line ~125 to:

```ts
  const [railCollapsed, setRailCollapsed] = useState(() => seedRailCollapsed());
```

   And add this module-scope helper (replacing the deleted `isCompactWorkspace`):

```ts
function seedRailCollapsed(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  // Collapsed on phone-compact and on iPad portrait; expanded on iPad landscape + desktop.
  const compactish = window.matchMedia("(max-width: 700px)").matches;
  const ipadPortrait =
    window.matchMedia("(min-width: 701px) and (max-width: 1180px) and (pointer: coarse)")
      .matches && window.matchMedia("(orientation: portrait)").matches;
  return compactish || ipadPortrait;
}
```

4. Replace the matchMedia effect (lines ~190–199) so it re-collapses on entering a compact/iPad-portrait layout:

```ts
  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const compact = window.matchMedia("(max-width: 700px)");
    const ipadPortrait = window.matchMedia(
      "(min-width: 701px) and (max-width: 1180px) and (pointer: coarse) and (orientation: portrait)",
    );
    const onChange = () => {
      if (compact.matches || ipadPortrait.matches) setRailCollapsed(true);
    };
    compact.addEventListener("change", onChange);
    ipadPortrait.addEventListener("change", onChange);
    onChange();
    return () => {
      compact.removeEventListener("change", onChange);
      ipadPortrait.removeEventListener("change", onChange);
    };
  }, []);
```

5. Update the click-to-collapse-after-outline-select line (was `if (isCompactWorkspace()) setRailCollapsed(true)`, ~line 1538) to:

```ts
    if (sizeClass !== "desktop") setRailCollapsed(true);
```

- [ ] **Step 4: Apply the inspector rule (effect)**

Add this effect after the panel state declarations (`companionOpen`/`factBookOpen`/`contextualEditOpen`, ~line 147):

```ts
  const prevInspectorRef = useRef<InspectorState>({
    companion: false,
    factBook: false,
    contextual: false,
  });
  useEffect(() => {
    const next: InspectorState = {
      companion: companionOpen,
      factBook: factBookOpen,
      contextual: contextualEditOpen,
    };
    const corrected = reconcileInspector(prevInspectorRef.current, next, sizeClass);
    if (corrected.companion !== next.companion) setCompanionOpen(corrected.companion);
    if (corrected.factBook !== next.factBook) setFactBookOpen(corrected.factBook);
    if (corrected.contextual !== next.contextual) setContextualEditOpen(corrected.contextual);
    prevInspectorRef.current = corrected;
  }, [sizeClass, companionOpen, factBookOpen, contextualEditOpen]);
```

(`useRef` is already imported in this file; if not, add it to the existing `react` import.)

- [ ] **Step 5: Run the responsive test + full frontend suite**

Run: `cd apps/desktop && pnpm test -- Workspace.responsive && pnpm test`
Expected: PASS, and no other test regresses (the deleted `isCompactWorkspace` had no other importers — confirm with `grep -rn isCompactWorkspace src/` returning nothing).

- [ ] **Step 6: Commit**

```bash
git add apps/desktop/src/routes/Workspace.tsx apps/desktop/src/routes/Workspace.responsive.test.ts
git commit -m "feat(workspace): drive layout from size tier and ipad inspector rule"
```

---

## Task 5: iPad-tier CSS (App.css)

Add a new `@media` block **after** the existing `@media (max-width: 860px)` block (which ends at `App.css:1629`). It overrides the compact drawer/bottom-sheet rules for coarse-pointer iPad viewports: inline pushing outline sidebar, right-side slide-over inspector for `.workspace .panel`, landscape safe areas, 44pt touch targets. Sheets keep the centered-modal `.modal/.dialog` sizing already defined.

**Files:**
- Modify: `apps/desktop/src/App.css` (append after line 1629)
- Modify: `apps/desktop/src/routes/Workspace.responsive.test.ts` (add a CSS-guard `it(...)`)

**Interfaces:**
- Consumes: existing class names `.ws-body`, `.workspace .rail`, `.mobile-rail-backdrop`, `.workspace .panel`, `.ws-top`, `.ws-tool`.
- Produces: a new `@media` block whose presence/rules the guard test asserts.

- [ ] **Step 1: Add the CSS-guard test (failing)**

Append to `apps/desktop/src/routes/Workspace.responsive.test.ts`:

```ts
  it("adds an ipad-tier layout block layered after the compact block", async () => {
    const css = await readSource("App.css");

    const ipadAt =
      "@media (min-width: 701px) and (max-width: 1180px) and (min-height: 600px) and (pointer: coarse)";
    expect(css).toContain(ipadAt);
    // ipad block must come AFTER the compact block so it overrides it
    expect(css.indexOf(ipadAt)).toBeGreaterThan(css.indexOf("@media (max-width: 860px)"));
    // inline pushing sidebar (no modal backdrop), right inspector
    expect(css).toContain(".mobile-rail-backdrop {\n    display: none;");
    expect(css).toContain("/* ipad: right-side slide-over inspector */");
  });
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd apps/desktop && pnpm test -- Workspace.responsive`
Expected: FAIL — the iPad block is not present.

- [ ] **Step 3: Append the iPad media block**

Add to the end of `apps/desktop/src/App.css` (after line 1629):

```css
/* iPad tier: touch, 701–1180px wide, ≥600px tall. Layered AFTER the compact
   (max-width: 860px) block so coarse-pointer devices in the 701–860 overlap get
   these rules, while fine-pointer desktop windows keep the compact rules. */
@media (min-width: 701px) and (max-width: 1180px) and (min-height: 600px) and (pointer: coarse) {
  /* Outline: inline collapsible sidebar that pushes the editor (no modal). */
  .mobile-rail-backdrop {
    display: none;
  }

  .ws-body,
  .ws-body.rail-collapsed,
  .ws-body.right-wide,
  .ws-body.right-xwide,
  .ws-body.right-history {
    grid-template-columns: var(--rail) minmax(0, 1fr) var(--right);
    --rail: 264px;
    --right: 0px;
  }

  .ws-body.rail-collapsed {
    --rail: 0px;
  }

  .workspace .rail {
    position: relative;
    top: auto;
    bottom: auto;
    left: auto;
    z-index: auto;
    width: 264px;
    box-shadow: none;
    transform: none;
    transition: width 0.18s ease;
  }

  .ws-body.rail-collapsed .rail {
    width: 0;
    overflow: hidden;
    transform: none;
  }

  /* ipad: right-side slide-over inspector */
  .workspace .panel {
    left: auto;
    top: calc(51px + env(safe-area-inset-top));
    right: 0;
    bottom: 0;
    width: min(380px, 60vw);
    height: auto;
    border-left: 1px solid var(--line);
    border-top: 0;
    border-radius: 0;
    padding-right: env(safe-area-inset-right);
  }

  /* Landscape safe areas + touch targets */
  .ws-top {
    padding-left: calc(10px + env(safe-area-inset-left));
    padding-right: calc(10px + env(safe-area-inset-right));
  }

  .ws-tool {
    min-height: 44px;
    min-width: 44px;
  }
}
```

- [ ] **Step 4: Run the guard test + full suite**

Run: `cd apps/desktop && pnpm test -- Workspace.responsive && pnpm test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src/App.css apps/desktop/src/routes/Workspace.responsive.test.ts
git commit -m "feat(workspace): add ipad-tier layout css (inline outline + right inspector)"
```

---

## Task 6: iOS feature-reduction UX in Settings

Today the entire git-sync section is hidden when `gitSyncAvailable` is false (`routes/Settings.tsx:955` — `{gitSyncAvailable && (`), so it vanishes silently on iOS. Replace silent removal with an explicit disabled note, and add a "this device supports API-key providers only" note when any providers are filtered.

**Files:**
- Modify: `apps/desktop/src/routes/Settings.tsx:955` (git section gate) and the provider section (~line 546, where `unavailableProviders` is passed)
- Modify: `apps/desktop/src/lib/i18n.tsx` (add keys to both the `ko` and `en` maps)
- Test: `apps/desktop/src/routes/Settings.iosReduction.test.ts` (create — source-assertion guard, matching the repo's responsive-test convention)

**Interfaces:**
- Consumes: existing `gitSyncAvailable` state, `unavailableProviders` state, `t(...)`.
- Produces: new i18n keys `settings.git.unavailableNote` and `settings.provider.restrictedNote`.

- [ ] **Step 1: Write the guard test (failing)**

```ts
// apps/desktop/src/routes/Settings.iosReduction.test.ts
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "..");
const read = (p: string) => readFile(resolve(srcRoot, p), "utf8");

describe("iOS feature-reduction UX", () => {
  it("shows a disabled git-sync note instead of hiding the section", async () => {
    const settings = await read("routes/Settings.tsx");
    expect(settings).toContain("!gitSyncAvailable && (");
    expect(settings).toContain('t("settings.git.unavailableNote")');
  });

  it("notes API-key-only providers when some are filtered", async () => {
    const settings = await read("routes/Settings.tsx");
    expect(settings).toContain("unavailableProviders.length > 0");
    expect(settings).toContain('t("settings.provider.restrictedNote")');
  });

  it("defines the new i18n keys in both languages", async () => {
    const i18n = await read("lib/i18n.tsx");
    expect((i18n.match(/"settings\.git\.unavailableNote"/g) ?? []).length).toBeGreaterThanOrEqual(2);
    expect((i18n.match(/"settings\.provider\.restrictedNote"/g) ?? []).length).toBeGreaterThanOrEqual(2);
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd apps/desktop && pnpm test -- Settings.iosReduction`
Expected: FAIL — branches and keys not present.

- [ ] **Step 3: Add the disabled git-sync note**

In `apps/desktop/src/routes/Settings.tsx`, immediately before the existing `{gitSyncAvailable && (` at line 955, add:

```tsx
            {!gitSyncAvailable && (
              <section className="set-block">
                <h3>{t("settings.git.title")}</h3>
                <p className="set-note">{t("settings.git.unavailableNote")}</p>
              </section>
            )}
```

(Leave the existing `{gitSyncAvailable && ( ... )}` block unchanged. Use whatever the existing wrapper element/class for sections is — match the neighboring `<section className="set-block">`; if the local class differs, mirror the sibling git section's wrapper.)

- [ ] **Step 4: Add the API-key-only provider note**

In the provider section near where `unavailableProviders` is passed to the picker (~line 546), add a note above or below the picker:

```tsx
              {unavailableProviders.length > 0 && (
                <p className="set-note">{t("settings.provider.restrictedNote")}</p>
              )}
```

- [ ] **Step 5: Add i18n keys (both maps)**

In `apps/desktop/src/lib/i18n.tsx`, add to the **ko** map (near the existing `"settings.git.title"` entry):

```ts
    "settings.git.unavailableNote": "Git 동기화는 이 기기(iOS)에서는 지원되지 않습니다. 데스크톱 앱에서 사용하세요.",
    "settings.provider.restrictedNote": "이 기기에서는 API 키 방식 제공자만 지원됩니다. (CLI/로컬 인증 제공자는 제외)",
```

And the matching **en** map entries (near its `"settings.git.title"`):

```ts
    "settings.git.unavailableNote": "Git sync isn't supported on this device (iOS). Use the desktop app for Git sync.",
    "settings.provider.restrictedNote": "This device supports API-key providers only. (CLI/local-auth providers are excluded.)",
```

- [ ] **Step 6: Run the guard test + full suite**

Run: `cd apps/desktop && pnpm test -- Settings && pnpm test`
Expected: PASS (Settings.iosReduction + existing Settings.test).

- [ ] **Step 7: Commit**

```bash
git add apps/desktop/src/routes/Settings.tsx apps/desktop/src/lib/i18n.tsx apps/desktop/src/routes/Settings.iosReduction.test.ts
git commit -m "feat(settings): explicit iOS feature-reduction notes for git-sync and providers"
```

---

## Task 7: iPad shortcut discoverability + soft-keyboard editor height

External keyboard shortcuts already work in WKWebView (the `isMac` paths resolve to ⌘ on iPad). Two iPad finishing touches: a command-bar entry that opens the existing `ShortcutsModal` (since the native ⌘-hold HUD is a non-goal), and a `visualViewport` effect that exposes the soft-keyboard inset as a CSS variable so the editor footer isn't covered when the on-screen keyboard appears.

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx` (command-bar buttons region ~line 1560–1620; effects region)
- Modify: `apps/desktop/src/App.css` (consume `--kbd-inset` inside the iPad media block)
- Test: `apps/desktop/src/routes/Workspace.ipadInput.test.ts` (create — source-assertion guard)

**Interfaces:**
- Consumes: existing `setShortcutsOpen` state setter; `sizeClass` (Task 4).
- Produces: a `--kbd-inset` CSS custom property set on `document.documentElement`.

- [ ] **Step 1: Write the guard test (failing)**

```ts
// apps/desktop/src/routes/Workspace.ipadInput.test.ts
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const read = (p: string) => readFile(resolve(here, "..", p), "utf8");

describe("iPad input affordances", () => {
  it("adds a shortcuts command-bar button on the ipad tier", async () => {
    const ws = await read("routes/Workspace.tsx");
    expect(ws).toContain('sizeClass === "ipad"');
    expect(ws).toContain("setShortcutsOpen(true)");
    expect(ws).toContain("ipad-shortcuts-toggle");
  });

  it("tracks the soft-keyboard inset via visualViewport", async () => {
    const ws = await read("routes/Workspace.tsx");
    expect(ws).toContain("window.visualViewport");
    expect(ws).toContain("--kbd-inset");
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd apps/desktop && pnpm test -- Workspace.ipadInput`
Expected: FAIL.

- [ ] **Step 3: Add the iPad shortcuts button**

In the `ws-top-actions` command-bar JSX (near the other `ws-tool icon-only` buttons, ~line 1560–1620), add an iPad-only button:

```tsx
            {sizeClass === "ipad" && (
              <button
                type="button"
                className="ws-tool icon-only ipad-shortcuts-toggle"
                aria-label={t("shortcuts.helpLabel")}
                onClick={() => setShortcutsOpen(true)}
              >
                <Keyboard size={18} />
              </button>
            )}
```

(Import `Keyboard` from the existing lucide-react import group at the top of the file. If a keyboard glyph is already imported, reuse it.)

- [ ] **Step 4: Add the visualViewport effect**

Add this effect in the effects region of `Workspace` (e.g. near the matchMedia effect from Task 4):

```ts
  useEffect(() => {
    const vv = typeof window !== "undefined" ? window.visualViewport : null;
    if (!vv) return;
    const root = document.documentElement;
    const update = () => {
      // How much of the layout viewport the soft keyboard covers at the bottom.
      const inset = Math.max(0, window.innerHeight - vv.height - vv.offsetTop);
      root.style.setProperty("--kbd-inset", `${Math.round(inset)}px`);
    };
    vv.addEventListener("resize", update);
    vv.addEventListener("scroll", update);
    update();
    return () => {
      vv.removeEventListener("resize", update);
      vv.removeEventListener("scroll", update);
      root.style.removeProperty("--kbd-inset");
    };
  }, []);
```

- [ ] **Step 5: Consume `--kbd-inset` in the iPad CSS block**

Inside the iPad `@media` block added in Task 5 (in `App.css`), add a rule so the editor scroll area reserves space for the keyboard:

```css
  .ws-editor {
    padding-bottom: var(--kbd-inset, 0px);
  }
```

- [ ] **Step 6: Run the guard test + full suite**

Run: `cd apps/desktop && pnpm test -- Workspace.ipadInput && pnpm test`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/desktop/src/routes/Workspace.tsx apps/desktop/src/App.css apps/desktop/src/routes/Workspace.ipadInput.test.ts
git commit -m "feat(workspace): ipad shortcut discoverability and soft-keyboard inset"
```

---

## Task 8: iPad-width layout regression checks

Add deterministic width-based assertions that the iPad block engages at representative iPad widths and that the compact block still owns phone widths. This stays within the repo's source/metric test style (no device farm).

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.responsive.test.ts` (add a tier-coverage `it(...)`)

**Interfaces:**
- Consumes: `resolveSizeClass` (Task 2).
- Produces: nothing.

- [ ] **Step 1: Write the failing test**

Append to `apps/desktop/src/routes/Workspace.responsive.test.ts`:

```ts
  it("maps representative viewports to the right tier", async () => {
    const { resolveSizeClass } = await import("../hooks/useSizeClass");

    // iPhone portrait (≤700, coarse): compact
    expect(resolveSizeClass({ desktop: false, ipad: false })).toBe("compact");
    // iPhone landscape (height < 600): excluded from ipad by min-height → compact
    expect(resolveSizeClass({ desktop: false, ipad: false })).toBe("compact");
    // iPad 11" portrait 834x1194 coarse: ipad
    expect(resolveSizeClass({ desktop: false, ipad: true })).toBe("ipad");
    // iPad 12.9" landscape 1366 (≥1181): desktop wins even though coarse
    expect(resolveSizeClass({ desktop: true, ipad: true })).toBe("desktop");
    // Mac/desktop window (pointer: fine): desktop
    expect(resolveSizeClass({ desktop: true, ipad: false })).toBe("desktop");
  });
```

- [ ] **Step 2: Run it to verify it fails (or passes if Task 2 merged)**

Run: `cd apps/desktop && pnpm test -- Workspace.responsive`
Expected: PASS once Task 2 is in place (this task documents/locks tier coverage; if it already passes, that is the intended green state — proceed to commit). If `resolveSizeClass` import fails, Task 2 was not completed — stop and finish Task 2 first.

- [ ] **Step 3: Run the full frontend + engine suites (final gate)**

Run: `cd apps/desktop && pnpm test && cd ../../engine && go test ./... && go test -tags mobile ./...`
Expected: PASS across desktop default and mobile tag profiles.

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src/routes/Workspace.responsive.test.ts
git commit -m "test(workspace): lock size-tier coverage for representative viewports"
```

---

## Manual QA checklist (launch gate — not automatable here)

Run on a physical iPad (and an iPhone for regression) before sub-project C submission:

- [ ] All 14 keyboard shortcuts fire with a Magic Keyboard (⌘P/S/J/I/F, ⌘⇧F, ⌘Z/⌘⇧Z, esc, @).
- [ ] External ↔ on-screen keyboard transitions: editor footer stays visible (`--kbd-inset` works); no content jump.
- [ ] Landscape safe areas: no content under the camera/home indicator; `ws-top` respects left/right insets.
- [ ] Outline sidebar pushes the editor (does not overlay); collapses in portrait, expands in landscape.
- [ ] Opening companion/fact-book/contextual on iPad shows one right inspector at a time; manuscript stays visible.
- [ ] Entity/Thread/Version sheets render as centered modals (not full-screen bottom sheets).
- [ ] Settings on a `mobile` build: git-sync shows the disabled note (not blank); provider list shows the API-key-only note and excludes `claude-code-cli`/`openai-codex`.
- [ ] `make smoke-mobile-ios-sim` still passes (engine-link + `library.db` regression gate).

(Apple Pencil/Scribble QA belongs to sub-project B.)

---

## Self-Review

**Spec coverage:**
- 3-tier size model (width + `pointer: coarse`) → Tasks 2, 4, 8 (+ `min-height` guard for iPhone-landscape, added in Global Constraints).
- New iPad layout: inline pushing outline + right slide-over inspector + centered-modal sheets → Task 5; one-inspector-at-a-time → Tasks 3, 4.
- Keyboard shortcuts inherited + iPad discoverability + soft/hard keyboard height → Task 7 (+ manual QA).
- iOS feature-reduction UX (providers + git-sync, explicit not silent) → Task 6; correct engine gating so the data is actually right on iOS → Task 1.
- Testing strategy: hook unit tests (Tasks 2, 3), CSS guards + tier coverage (Tasks 5, 8), engine regression guard (Task 1), feature-reduction guards (Task 6), manual device gate (checklist).

**Placeholder scan:** No "TBD/TODO/handle edge cases". The two "match the sibling wrapper class" notes in Task 6 (Steps 3) and the lucide icon-reuse note in Task 7 depend on reading current source and give the concrete fallback to copy; not placeholders for logic.

**Type consistency:** `SizeClass` (`compact|ipad|desktop`) consistent across Tasks 2/3/4/8. `InspectorState` (`companion|factBook|contextual` booleans) consistent across Tasks 3/4. `reconcileInspector(prev, next, sizeClass)` signature identical in Tasks 3 and 4. i18n keys `settings.git.unavailableNote` / `settings.provider.restrictedNote` identical across Task 6 steps and test. CSS query string `(min-width: 701px) and (max-width: 1180px) and (min-height: 600px) and (pointer: coarse)` identical in Tasks 5 (CSS + guard) and matches the JS queries' intent in Tasks 2/4.

**Biggest residual risk:** The source-assertion guard tests (Tasks 4–7) verify code shape, not rendered behavior — real layout/keyboard correctness is covered only by the manual QA gate on a physical iPad. That is an accepted constraint of the repo's current (non-browser) test setup, mirrored from the existing `Workspace.responsive.test.ts`.
