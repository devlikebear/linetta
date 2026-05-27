# Plan 12 — Focus Mode (Dim Non-Current Paragraphs)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans or subagent-driven-development.

**Goal:** Add a "focus mode" toggle that dims every paragraph except the one currently containing the cursor. Toggleable via Cmd+K, default state persisted in settings.json alongside `typewriter_default`.

**Architecture:** Small change. Backend: one new field `focus_default bool` on settings.Config + Patch. Frontend: one new Tiptap `Extension` registering a ProseMirror plugin that maintains a `DecorationSet` adding a CSS class to non-current paragraphs; `Workspace` tracks a `focus` boolean (loaded from settings.focus_default on mount, toggleable via Cmd+K), passes to `<TiptapEditor focus={focus} />` which conditionally adds the extension.

**Spec:** §11.2 P3 "Focus mode (dim non-current paragraphs)".

**Locked decisions:** paragraph-level dimming; toggle via Cmd+K + Settings checkbox; default persisted to settings.json.

---

## Tasks

### Task 1: Engine — `focus_default` in settings.Config

**Files:**
- Modify: `engine/internal/settings/settings.go`
- Modify: `engine/internal/settings/settings_test.go`

1. Add `FocusDefault bool` field to `Config` and `Patch`:
   ```go
   type Config struct {
       Provider          string `json:"provider"`
       TypewriterDefault bool   `json:"typewriter_default"`
       FocusDefault      bool   `json:"focus_default"`
       BackupDir         string `json:"backup_dir,omitempty"`
   }
   type Patch struct {
       Provider          *string `json:"provider,omitempty"`
       TypewriterDefault *bool   `json:"typewriter_default,omitempty"`
       FocusDefault      *bool   `json:"focus_default,omitempty"`
   }
   ```

2. In `Set`, mirror the typewriter handling:
   ```go
   if p.FocusDefault != nil {
       next.FocusDefault = *p.FocusDefault
   }
   ```

3. In `persistable` struct that excludes `BackupDir`, include `FocusDefault`:
   ```go
   persistable := Config{
       Provider:          next.Provider,
       TypewriterDefault: next.TypewriterDefault,
       FocusDefault:      next.FocusDefault,
   }
   ```

4. Update existing tests where they set `TypewriterDefault: boolPtr(true)` — they don't need changes. Add one new test:
   ```go
   func TestSet_focusDefault(t *testing.T) {
       s := newStoreOnTemp(t)
       if _, err := s.Set(context.Background(), Patch{FocusDefault: boolPtr(true)}); err != nil {
           t.Fatalf("Set: %v", err)
       }
       got, _ := s.Get(context.Background())
       if !got.FocusDefault {
           t.Errorf("focus_default not persisted: %+v", got)
       }
   }
   ```

5. Verify + commit:
   ```bash
   cd engine && go test ./internal/settings/... -race
   git add engine/internal/settings/
   git commit -m "feat(settings): add focus_default field"
   ```

### Task 2: Frontend — TS Settings type + Settings page UI

**Files:**
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/routes/Settings.tsx`

1. In `types.ts`:
   ```ts
   export interface Settings {
     provider: ProviderID;
     typewriter_default: boolean;
     focus_default: boolean;
     backup_dir: string;
   }

   export interface SettingsPatch {
     provider?: ProviderID;
     typewriter_default?: boolean;
     focus_default?: boolean;
   }
   ```

2. In `Settings.tsx`, below the typewriter checkbox add:
   ```tsx
   <label className="check-row">
     <input
       type="checkbox"
       checked={current.focus_default}
       onChange={(e) => apply({ focus_default: e.target.checked })}
       disabled={saving}
     />
     <span>새 씬을 열 때 Focus 모드(현재 단락 외 디밍) 켜기</span>
   </label>
   ```

3. Verify + commit:
   ```bash
   cd apps/desktop && pnpm tsc -b
   git add apps/desktop/src/lib/types.ts apps/desktop/src/routes/Settings.tsx
   git commit -m "feat(settings-ui): focus_default toggle"
   ```

### Task 3: Tiptap Focus extension

**Files:**
- Create: `apps/desktop/src/components/editor/FocusExtension.ts`
- Modify: `apps/desktop/src/components/editor/Tiptap.tsx`
- Modify: `apps/desktop/src/components/editor/Tiptap.css`

1. `FocusExtension.ts`:
   ```ts
   import { Extension } from "@tiptap/core";
   import { Plugin, PluginKey } from "@tiptap/pm/state";
   import { Decoration, DecorationSet } from "@tiptap/pm/view";

   /**
    * FocusExtension: when active, dims every top-level paragraph/heading/
    * blockquote except the one containing the current selection. Adds the
    * CSS class `tiptap-dim` to dimmed blocks.
    */
   const focusKey = new PluginKey("linetta-focus");

   function buildDecorations(state: any): DecorationSet {
     const { doc, selection } = state;
     const decorations: Decoration[] = [];
     // The current paragraph is the deepest block ancestor of the head.
     let currentBlockPos = -1;
     doc.descendants((node: any, pos: number) => {
       if (!node.isBlock || node.isLeaf) return true;
       if (pos <= selection.head && selection.head <= pos + node.nodeSize) {
         currentBlockPos = pos;
       }
       return true;
     });
     doc.descendants((node: any, pos: number) => {
       if (!node.isBlock || node.isLeaf) return true;
       if (pos !== currentBlockPos) {
         decorations.push(
           Decoration.node(pos, pos + node.nodeSize, { class: "tiptap-dim" }),
         );
       }
       return true;
     });
     return DecorationSet.create(doc, decorations);
   }

   export const FocusExtension = Extension.create({
     name: "linettaFocus",
     addProseMirrorPlugins() {
       return [
         new Plugin({
           key: focusKey,
           state: {
             init: (_, state) => buildDecorations(state),
             apply: (_tr, _old, _oldState, newState) => buildDecorations(newState),
           },
           props: {
             decorations(state) {
               return (this as any).getState(state);
             },
           },
         }),
       ];
     },
   });
   ```

2. In `Tiptap.tsx`, accept a `focus?: boolean` prop. In the extensions array passed to `useEditor`, conditionally include `FocusExtension`:
   ```tsx
   interface Props {
     // ... existing
     focus?: boolean;
   }
   ```
   And:
   ```tsx
   const editor = useEditor(
     {
       extensions: [
         StarterKit.configure({}),
         ...(extensions ?? []),
         ...(focus ? [FocusExtension] : []),
       ],
       content: initialDoc,
       autofocus: "end",
       // ...
     },
     [initialKey, focus],  // re-init when focus toggles
   );
   ```
   Add `import { FocusExtension } from "./FocusExtension";` at top.

3. Append to `Tiptap.css`:
   ```css
   .tiptap-wrap .ProseMirror .tiptap-dim {
     opacity: 0.22;
     transition: opacity 0.18s ease-out;
   }
   ```

4. Verify + commit:
   ```bash
   cd apps/desktop && pnpm tsc -b
   git add apps/desktop/src/components/editor/FocusExtension.ts apps/desktop/src/components/editor/Tiptap.tsx apps/desktop/src/components/editor/Tiptap.css
   git commit -m "feat(editor): FocusExtension dims non-current paragraphs"
   ```

### Task 4: Workspace integration + Cmd+K

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx`

1. Add state near typewriter:
   ```ts
   const [focus, setFocus] = useState(false);
   ```

2. Extend the existing settings-loading effect to also set focus:
   ```ts
   useEffect(() => {
     let cancelled = false;
     settingsApi.get()
       .then((s) => {
         if (cancelled) return;
         setTypewriter(s.typewriter_default);
         setFocus(s.focus_default);
       })
       .catch(() => { /* benign */ });
     return () => { cancelled = true; };
   }, []);
   ```

3. Pass `focus={focus}` to `<TiptapEditor>`.

4. Add a Cmd+K command in the `보기` section (or wherever fits — same neighborhood as the outline command):
   ```ts
   cmds.push({
     id: "toggle-focus",
     section: "보기",
     label: focus ? "Focus 모드 끄기" : "Focus 모드 켜기",
     run: () => setFocus((v) => !v),
   });
   ```

5. Verify + commit:
   ```bash
   cd apps/desktop && pnpm tsc -b && pnpm build
   git add apps/desktop/src/routes/Workspace.tsx
   git commit -m "feat(workspace): Focus 모드 토글 + settings load"
   ```

### Task 5: Smoke + tag

1. Rebuild engine + launch:
   ```bash
   ./scripts/build-engine.sh
   LINETTA_HOME=/tmp/linetta-plan12 ./scripts/dev.sh
   ```
2. Create project, write multiple paragraphs.
3. Cmd+K → "Focus 모드 켜기" → other paragraphs dim to ~25% opacity. Click into a different paragraph → it lights up, previous one dims.
4. Cmd+K → "Focus 모드 끄기" → all paragraphs return to full opacity.
5. Settings → toggle `focus_default` on → reopen project → Focus is already active.
6. Tag:
   ```bash
   git tag plan-12-focus-mode-done
   ```

---

## Done conditions

- [ ] `go test ./... -race` green.
- [ ] `pnpm tsc -b && pnpm build` green.
- [ ] Smoke walkthrough passes (visible dimming, follows cursor, persists via settings).
- [ ] `plan-12-focus-mode-done` tag exists.
