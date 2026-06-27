# iPad/iOS Product UX (Sub-project A) Design

**Status:** Design approved, pending spec review → writing-plans.

## Context

Linetta already ships a working mobile foundation on `main` (see `docs/superpowers/plans/2026-06-21-mobile-engine-embed.md`):

- The Go engine is embedded in-process via a C ABI (`apps/desktop/src-tauri/src/ffi.rs`), replacing the iOS-forbidden sidecar.
- iOS `xcframework` and Android `.so` build under the `mobile` build tag; `build.rs` is target-aware; `gen/apple` / `gen/android` Tauri projects are generated; the iOS simulator smoke (`make smoke-mobile-ios-sim`) passes.
- A compact responsive pass exists: a single breakpoint `COMPACT_WORKSPACE_QUERY = "(max-width: 860px)"` drives a drawer + bottom-sheet layout for phone-width viewports.
- Desktop-only features are already gated for mobile: `git_sync_available` and `unavailable_providers` flow from engine diagnostics into `routes/Settings.tsx`, which auto-filters unavailable providers.

**Note on the stale branch:** `origin/feat/mobile-engine-embed` is an obsolete parallel branch (only CI fixes remain ahead). The mobile work is merged to `main`; that branch is not the source of truth.

## Goal

Make Linetta a **Universal iPhone + iPad app** suitable for simultaneous App Store launch, with iPad as a **first-class, primary target** for long-form fiction writing. This sub-project covers the **product UX layer only**. Out of scope (separate sub-projects): Apple Pencil/Scribble (B), and signing/provisioning/App Store submission pipeline (C).

The core problem: today **iPad landscape (1024–1366px) gets the desktop multi-pane layout** (not touch-tuned) and **iPad portrait (768–1024px) gets the phone drawer** (wastes the large screen). Neither is right.

### Decisions captured during brainstorming

- Universal app, iPhone + iPad, simultaneous App Store launch.
- A **new iPad-specific layout** (not a scaled-up phone UI, not the raw desktop layout).
- First-class input: **external keyboard shortcuts** and **Apple Pencil/Scribble** (Scribble handled in sub-project B).
- Trackpad/pointer tuning and Split View / Stage Manager are **not** first-class targets this round (must not break, but not design-driven).

## Non-goals

- Native `UIKeyCommand` registration (the iPad "hold ⌘" system HUD). Discoverability uses the existing in-app `ShortcutsModal` instead.
- First-class trackpad/pointer hover affordance tuning.
- First-class Split View / Stage Manager adaptive behavior (the layout must degrade without breaking, but narrow-multitasking polish is deferred).
- Android product UX (Android secrets persistence remains unsupported per the mobile-engine plan).
- Apple Pencil / Scribble integration (sub-project B).
- Any signing, entitlement, provisioning, TestFlight, or App Store Connect work (sub-project C).

## Architecture

The current architecture is **render-once + CSS reshapes**: Workspace components always render, and `@media` rules transform them (drawer, bottom sheets). The only layout JS state is `railCollapsed` (outline) plus per-panel toggles (`companionOpen`, fact book, contextual edit). This design keeps that architecture and adds a third size tier plus thin orchestration.

### Size-class model (intent-based, 3 tiers)

Width alone cannot separate iPad portrait (768px) from a phone, so tiers combine **width + `(pointer: coarse)`** (finger input):

| Tier | Condition | Targets | Layout |
|---|---|---|---|
| **Compact** | `≤ 700px`, or narrow multitasking slot | iPhone, Slide Over | Existing drawer + bottom sheets (unchanged) |
| **iPad** (new) | `701–1180px` **and `(pointer: coarse)`** | iPad portrait + small landscape | New adaptive layout (below) |
| **Desktop** | `≥ 1180px`, or `(pointer: fine)` | Mac, 12.9" landscape, narrow desktop windows | Existing multi-pane (unchanged) |

Rationale for `(pointer: coarse)`: a 1100px iPad needs touch tuning; a 1100px desktop window needs hover affordances. Same width, different input → must split by pointer type. The Magic Keyboard trackpad reports `fine` — an accepted edge case since pointer is not a first-class target this round.

### iPad-tier layout model (the new design)

- **Outline:** an inline collapsible sidebar that **pushes** the editor (not a modal overlay). Default collapsed in portrait, expanded in landscape.
- **Editor (TipTap):** always central and first-class; reading-width max constraint retained.
- **Auxiliary panels (companion / context / fact book / contextual edit):** **one at a time**, as a right-side slide-over **inspector** (~360px) — not a bottom sheet, not a desktop fixed multi-column. Keeps the manuscript visible while writing.
- **Sheets (Entity / Thread / Version):** **centered modals** on iPad (not full-screen bottom sheets).

## Components & Changes

No component-tree rewrite. The work is CSS plus thin JS state generalization.

### 1. Size-class generalization (JS) — `routes/Workspace.tsx`

- Replace the binary `isCompactWorkspace()` (lines ~64–67) with a `useSizeClass(): "compact" | "ipad" | "desktop"` hook that subscribes to three `matchMedia` queries (generalizing the listener at line ~192).
- `railCollapsed` seeding (lines ~125, ~1538): iPad-portrait → collapsed, iPad-landscape → expanded, compact → collapsed.
- New **`useInspector` orchestration**: in the iPad tier only, opening one auxiliary panel closes the others (mutual exclusion over `companionOpen` / fact book / contextual edit). Desktop and compact behavior unchanged (panels independent).

### 2. iPad-tier styles (CSS) — `App.css` (+ per-panel `.css`)

- New `@media (min-width: 701px) and (max-width: 1180px) and (pointer: coarse)` block.
- Outline: neutralize the compact `position: fixed` overlay/backdrop rules; render as an inline collapsible sidebar that pushes the editor.
- Auxiliary panels: override the compact bottom-sheet rules → right-side slide-over inspector (fixed ~360px width).
- Sheets: override full-screen bottom-sheet rules → centered modal.
- Touch hit targets ≥ 44pt (`ws-tool` etc.); landscape `env(safe-area-inset-left/right)`; editor max-width retained.

### 3. Touch targets / command bar

- Reuse the existing compact horizontal-scroll command bar in the iPad tier, guaranteeing 44pt targets. Add a command-bar affordance to open `ShortcutsModal` (discoverability — see below).

### 4. Keyboard shortcuts on iPad — mostly inherited, finishing only

- iPadOS WKWebView reports `navigator.platform === "MacIntel"`, so the existing `isMac ? e.metaKey : e.ctrlKey` paths (e.g. `components/ZenMode.tsx:43`, `components/editor/Tiptap.tsx:144`) **resolve to ⌘ on iPad** — correct for Magic Keyboard. The 14 web `keydown` handler files receive hardware-keyboard events in WKWebView.
- System-conflict check: the current set (⌘P/S/J/I/F/. , ⌘⇧F, ⌘Z, ⌘⇧Z; see `components/ShortcutsModal.tsx`) does not collide with reserved iPadOS combos (⌘Space, ⌘Tab, ⌘H). Keep as-is.
- Discoverability: the native "hold ⌘" HUD needs `UIKeyCommand` (non-goal). Use the existing in-app `ShortcutsModal` ⌘ list, reachable from the iPad command bar.
- Hardware ↔ soft keyboard transitions change editor height; respond via `visualViewport` + safe-area handling.

### 5. iOS feature-reduction UX — reuse existing plumbing, finish empty states

- **Providers:** CLI-provider auto-filtering already works (`routes/Settings.tsx:78,150`). Add explicit copy ("this device supports API-key providers only") so providers don't silently vanish; route onboarding's first-provider step through the API-key flow on iOS.
- **Git sync:** when `git_sync_available === false`, render the Settings git-sync section as **disabled with a reason** ("not supported on iOS") rather than a broken/no-op button.
- **Secrets:** iOS Keychain works (darwin build) → API keys persist. The `keychain-access-groups` entitlement is sub-project C.

## Testing & Verification

The existing "responsive test" (`routes/Workspace.responsive.test.ts`) is a **source-string-assertion** vitest, not a render test. This design both follows that convention (for CSS guards) and strengthens it where logic is now extractable.

### Unit tests (vitest)

- `useSizeClass`: tier resolution across boundary widths (700/701, 1180/1181) × `pointer` coarse/fine, via `matchMedia` mock.
- `useInspector`: iPad mutual exclusion (opening one panel closes others); desktop/compact independence preserved.
- CSS guard (matching existing convention): assert the new `@media … (pointer: coarse)` block and its key rules exist.

### Render / layout checks

- Zero horizontal overflow + correct layout mode at representative iPad widths **834 (11" portrait), 1194 (11" landscape), 1024 (12.9" portrait)** with `pointer: coarse` emulation (reuse the existing Chrome-metric approach; promote to a single Playwright case if practical).
- Assert per-width: outline pushes (not overlays), right inspector slides over, sheets render as centered modals.

### iOS feature-reduction checks

- Go test (regression guard on already-gated code): under the `mobile` tag, diagnostics returns CLI providers in `unavailable_providers` and `git_sync_available: false`.
- vitest: Settings renders the "API-key only" copy and the disabled-git-sync reason from those signals.

### Real-device QA checklist (launch gate, not automatable)

On a physical iPad: all 14 shortcuts fire; external↔soft keyboard height transitions; landscape safe areas; inspector + sheet behavior; onboarding spotlight placement. Keep `make smoke-mobile-ios-sim` as a regression gate. (Scribble QA belongs to sub-project B.)

## Decomposition context

This is **sub-project A** of three for the iPhone + iPad launch:

- **A — iPad/iOS product UX** (this spec): the launch critical path; the bulk of new design/build.
- **B — Apple Pencil / Scribble:** spike-gated R&D on the TipTap/ProseMirror editor; depends on A's editor layout; separable from launch if it doesn't land.
- **C — iOS release & App Store submission:** App ID, provisioning, `keychain-access-groups` entitlement, `.ipa` export, TestFlight, App Store Connect (iPhone + iPad screenshots), privacy labels; reuses the existing Mac App Store submission infrastructure. Can proceed in parallel with A.

## Risks

- **TipTap/ProseMirror input handling** is the main interaction risk overall, but the Scribble portion is deferred to B. Within A, the risk is keyboard-event delivery in WKWebView for all 14 handlers — verified on real device in the QA gate.
- `(pointer: coarse)` misclassification when a Magic Keyboard trackpad is attached (reports `fine`); accepted this round since pointer is a non-goal.
- Width-tier boundaries vs. real iPad multitasking slot widths: the layout must degrade gracefully into Compact in narrow slots (covered by the Compact tier) even though Split View is a non-goal.
