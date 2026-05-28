# Plan 15 — UX Polish Round (Paper-Cuts Cleanup)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans or subagent-driven-development.

**Goal:** Sweep through 10 small UI inconsistencies accumulated since the MVP and clean them in one batch. No backend changes. One new dependency: `lucide-react`. After this round every sheet shares the same paper tone, every glyph comes from a consistent icon set, every toast goes through one provider, every Cmd+K command belongs to one of 8 fixed sections, and the user can open a "단축키 도움말" modal from the palette.

**Architecture:**
- New: `ToastProvider.tsx`, `lib/icons.ts`, `ShortcutsModal.{tsx,css}`.
- Modified: `App.tsx` (wrap router with ToastProvider), `App.css` (form classes, ai-chip rules, ws-toast lift), `Workspace.tsx` (consume `useToast`, palette sections, register shortcuts command), `Library.tsx` (lucide icons + actions container + toast), `Settings.tsx` (drop inline styles), `VersionSheet.css` + `NotePopover.css` (paper tone rewrite), `ThreadView.css` (light polish), `EntitySheet.tsx` + `ThreadSheet.tsx` + `VersionSheet.tsx` + `ActiveThreadsPanel.tsx` + `NotePopover.tsx` (replace inline glyphs with lucide), `AIMode.tsx` + `AIContextPanel.tsx` + `AIMode.css` (unified `.ai-chip`), ContextPanel scroll behavior.

**Locked decisions:**
1. Install `lucide-react`. Replace React-rendered glyphs only; literal ProseMirror content (editor `☘︎` marker) stays.
2. All sheets unify on warm paper tone (`#faf9f6` family). VersionSheet loses its `#111` dark style.
3. Toast lifts to app-level `ToastProvider` + `useToast()`. Drop all `alert()` calls.
4. New `단축키 도움말` Cmd+K command opens a modal listing every shortcut.

> **Note**: This is the navigational summary. Full task descriptions follow.

Verification: no vitest; each task verifies with `pnpm tsc -b && pnpm build` from `apps/desktop/`.

---

## Tasks

### Task 1 — Toast lift to app-level provider

Create `apps/desktop/src/components/ToastProvider.tsx` with `<ToastProvider>` (renders the bubble JSX) + `useToast()` hook. Wrap `<Routes>` in `App.tsx`. Update `Workspace.tsx` to consume `useToast()` instead of its local `toast` state + `showToast` function + ws-toast JSX. Add import in Workspace.

Commit: `feat(ui): lift toast to app-level ToastProvider with useToast() hook`

### Task 2 — Install lucide-react + replace glyphs

`pnpm --filter linetta-desktop add lucide-react`. Create `apps/desktop/src/lib/icons.ts` re-exporting `MoreHorizontal`, `Settings`, `Plus`, `Upload`, `X`, `Trash2`, `User`, `HelpCircle`.

Replace inline glyphs (`···`, `+`, `×`, `·`) throughout: Library header, EntitySheet/ThreadSheet/VersionSheet/NotePopover close buttons, attr/relation/beat add buttons, ActiveThreadsPanel `+`. The editor `☘︎` marker stays as ProseMirror content.

Add CSS for inline-flex alignment of icon+label buttons.

Commit: `feat(ui): install lucide-react and replace inline glyphs with consistent icons`

### Task 3 — VersionSheet paper tone rewrite

Replace `VersionSheet.css` entirely. Background `#faf9f6`, dividers `#ece9e0`, preview block `#fffefb`. Buttons match EntitySheet family. Thin scrollbar for timeline + preview.

Commit: `style(version-sheet): paper-tone rewrite matching EntitySheet`

### Task 4 — NotePopover paper tone rewrite

Replace `NotePopover.css`. Paper background, EntitySheet-style buttons (`.primary` / `.danger`). The `.note-marker` styling (☘︎ in editor) stays as-is.

Commit: `style(note-popover): paper-tone palette matching EntitySheet`

### Task 5 — ThreadView lightening

Replace `ThreadView.css`. Page background `#faf9f6`. Each thread lane becomes a `#fffefb` card with `#ece9e0` border. Lane track line is `#d8d6cf`. Beat discs get a paper-colored border so they "punch out" of the card. Shadows softened.

Commit: `style(thread-view): lighter card layout with paper-tone lanes`

### Task 6 — Form class consistency

In `App.css`: define `.field` (vertical label+input stack, 0.4rem gap), `.field-row` (flex row variant for inline button next to input), polish existing `.check-row` / `.radio-row`. Standardize `.field input`, `.field textarea`, `.field select` styling.

Modify `Settings.tsx`: drop inline `style={{...}}` hacks added for GitHub 동기화 in Plan 13 — wrap input + button in `<div className="field-row">`. Delete the "엔진 로그 (post-MVP)" section entirely.

Commit: `refactor(settings): use shared .field/.field-row classes and drop inline styles`

### Task 7 — Empty-state copy + post-MVP cleanup

Unify on "아직 X가 없어요" pattern:
- Workspace right rail mention empty → `아직 @멘션이 없어요`
- ActiveThreadsPanel empty → `아직 활성 스토리라인이 없어요`
- ThreadView empty → `아직 스토리라인이 없어요. Cmd+K → "이 씬을 새 Thread로 표시"로 시작하세요.`
- EntitySheet relationships empty → `아직 관계가 없어요`
- ThreadSheet beats empty → `아직 마디가 없어요`

Grep for `post-MVP` / `곧 추가됨` / `곧 지원됨` → delete or replace with concrete copy.

Commit: `refactor(empty-states): unify on "아직 X가 없어요" voice and remove post-MVP placeholders`

### Task 8 — Drop alert() in favor of useToast()

In `Library.tsx`, replace `alert(`가져오기 실패: ${err}`)` with `showToast(...)`.

Sanity grep `apps/desktop/src` for `alert(` — should be 0 after. Console logs in CommandPalette etc. stay (dev signals).

Inline `<p className="error">` stays as field-level errors; `.error` class in App.css from Task 6 gives them unified styling.

Commit: `refactor(toast): replace alert() with useToast() across renderer`

### Task 9 — Unified AI chip family

In `AIMode.css`: remove `.aimode-tone-*` rules, replace with `.ai-chip-row`, `.ai-chip-label`, `.ai-chip` family. One shared chip style.

In `AIMode.tsx`: tone chips + 길이 chip (toggling `short_form`) in one horizontal `.ai-chip-row`. Drop the toolbar `<label className="aimode-check">` for length.

Same shape in `AIContextPanel.tsx`. Section heading becomes "톤 · 길이".

Grep `aimode-check` / `aimode-tone-chip` — should be 0 after removal.

Commit: `refactor(ai): unify tone+length into single .ai-chip row in AIMode and AIContextPanel`

### Task 10 — Right-panel scroll + spacing

Update `.ctx-panel` in `App.css`: padding `1.25rem 1rem`, section gap `1.25rem`, `scrollbar-gutter: stable` to prevent horizontal shift, thin scrollbar.

Commit: `style(context-panel): consistent gap and stable scrollbar gutter`

### Task 11 — Cmd+K section label audit

Fixed 8-section vocabulary: `이동` / `보기` / `노드` / `엔티티` / `프로젝트` / `AI` / `내보내기` / `도움말`.

Walk every `cmds.push({...})` in `Workspace.tsx` (~21 sites at lines 468–714). Adjust `section:` to match exactly. Move entity-related commands from `노드` to `엔티티`. Move AI-related to `AI`.

Hint policy: only set `hint` when meaningful (shortcut key, brief status note). Otherwise omit.

Commit: `refactor(palette): constrain Cmd+K section labels to fixed 8-item vocabulary`

### Task 12 — ShortcutsModal + 단축키 도움말 command

Create `ShortcutsModal.tsx` + `.css`. Centered modal listing ≥9 shortcuts:
- Cmd+K = 명령 팔레트 열기
- Cmd+S = 수동 스냅샷 저장
- Cmd+. = ZEN 종료 / 다이얼로그 취소
- ESC = 다이얼로그 닫기 · ZEN 종료 · 선택 해제
- Cmd+Shift+F = Focus 모드 토글
- Cmd+Z / Cmd+Shift+Z = 본문 undo/redo
- @ = 엔티티 멘션 검색
- ESC (✱ 위) = 노트 popover 닫기

Paper tone, lucide `<X />` close, backdrop + ESC dismiss.

In `Workspace.tsx`: import + state `[shortcutsOpen, setShortcutsOpen]`, add `cmds.push({id:"show-shortcuts", section:"도움말", label:"단축키 도움말", run: () => setShortcutsOpen(true)})`, render `<ShortcutsModal open={shortcutsOpen} onClose={...} />`.

Commit: `feat(palette): add 단축키 도움말 command with shortcuts modal`

### Task 13 — Library actions container

In `App.css`: add `.library-actions { display:flex; flex-direction:column; align-items:center; gap:0.5rem; margin:1rem 0 }` + `.library-actions .new-button { min-width:220px; justify-content:center }`.

In `Library.tsx`: wrap both new-button + import-button in `<div className="library-actions">`. Drop `style={{marginTop:"0.4rem"}}` hack. Use `<Plus />` + label / `<Upload />` + label.

Commit: `style(library): unify action buttons in .library-actions container`

### Task 14 — Smoke walkthrough + tag

Run `pnpm tauri dev`, manually verify the 9 polish points listed in Done conditions. Then:

```bash
git tag plan-15-ux-polish-done
```

---

## Done conditions

- [ ] `pnpm tsc -b && pnpm build` green.
- [ ] `grep -rn "alert(" apps/desktop/src/` = 0 matches.
- [ ] `grep -rn "post-MVP" apps/desktop/src/` = 0 matches.
- [ ] `grep -rn "aimode-tone-chip\|aimode-check" apps/desktop/src/` = 0 matches.
- [ ] `grep -rn "style={{" apps/desktop/src/routes/Library.tsx apps/desktop/src/routes/Settings.tsx` = 0 matches.
- [ ] Cmd+K palette section headings ∈ {이동, 보기, 노드, 엔티티, 프로젝트, AI, 내보내기, 도움말}.
- [ ] `ShortcutsModal.tsx` exists, renders ≥9 shortcuts, ESC closes.
- [ ] VersionSheet / NotePopover / ThreadView / EntitySheet / ThreadSheet / AIContextPanel all paper-toned.
- [ ] `lucide-react` in `apps/desktop/package.json` dependencies.
- [ ] `plan-15-ux-polish-done` tag exists.

## Out of scope
- Backend changes.
- New icon system beyond `lucide-react`.
- Theme switcher.
- New keyboard shortcuts (modal documents existing only).
- Cmd+K palette internals rewrite.
- Editor `☘︎` glyph change.
- i18n infrastructure.
- Migration of inline `<p className="error">` to toast.
