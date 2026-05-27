# Plan 11 — Multiple Tone Presets

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans`.

**Goal:** Replace the single `tone_preset: bool` ("내 톤") with a richer single-select tone palette (`my`, `cool`, `sensory`, `dry`, `tense`, `lyrical`, `humor`). The selected tone shapes the AI system prompt: `my` keeps the current style_notes injection; the others append a tone instruction.

**Architecture:** Engine swaps `ai.Options.TonePreset bool` → `ai.Options.Tone string`. `prompts.go::buildSystem` switches on the tone. Frontend swaps the lone checkbox for a 7-chip row in both `AIMode` and `AIContextPanel`. `ai_runs.context_json` now carries `options.tone` instead of `options.tone_preset` — old rows just become legacy data (no schema change; both fields are JSON-only).

**Spec reference:** §5.5 (existing `톤 프리셋: "내 톤"`), §11.2 P2 (multiple tone presets beyond "내 톤").

**Locked decisions:**
1. Single-select string ID. Default = `my` (preserves existing behavior).
2. Presets: `my` / `cool` / `sensory` / `dry` / `tense` / `lyrical` / `humor`.
3. UI: chip row (radio-style — clicking a chip selects it).

---

## Tone preset mapping (locked text)

| ID | Korean label | Prompt fragment when selected |
|---|---|---|
| `my` | 내 톤 | `작가의 스타일 노트(반드시 따를 것):\n{style_notes}\n` (current behavior; only when style_notes non-empty) |
| `cool` | 차갑게 | `이번 출력은 차갑고 거리감 있는 톤으로 유지하라.\n` |
| `sensory` | 감각적 | `이번 출력은 시각·청각·촉각 묘사를 적극 활용한 감각적 톤으로 유지하라.\n` |
| `dry` | 건조하게 | `이번 출력은 형용사를 줄이고 사실 위주의 건조한 톤으로 유지하라.\n` |
| `tense` | 긴장감 | `이번 출력은 짧은 문장과 끊김으로 긴장감을 살린 톤으로 유지하라.\n` |
| `lyrical` | 서정 | `이번 출력은 율격이 살아있는 서정적인 톤으로 유지하라.\n` |
| `humor` | 유머 | `이번 출력은 가볍고 위트 있는 톤으로 유지하라.\n` |
| (empty / unknown) | — | no tone fragment |

---

## Phase A: Engine (1 task)

### Task 1: `ai.Options.Tone` + `buildSystem` switch

Files:
- Modify: `engine/internal/ai/ai.go`
- Modify: `engine/internal/ai/prompts.go`
- Modify: `engine/internal/ai/prompts_test.go`
- Modify: `engine/internal/ai/context_test.go` (if any tests set `TonePreset`)

1. **`ai.go`** — replace the bool field on `Options`:

```go
const (
	TonePresetMy       = "my"
	TonePresetCool     = "cool"
	TonePresetSensory  = "sensory"
	TonePresetDry      = "dry"
	TonePresetTense    = "tense"
	TonePresetLyrical  = "lyrical"
	TonePresetHumor    = "humor"
)

type Options struct {
	Tone      string `json:"tone"`
	ShortForm bool   `json:"short_form"`
}
```

(Remove the old `TonePreset bool` field.)

2. **Failing test in `prompts_test.go`** — extend or add:

```go
func TestBuildSystem_myToneEmphasizesStyleNotes(t *testing.T) {
	c := Context{
		StyleNotes: "단문 위주, 한자어 자제.",
		Options:    Options{Tone: TonePresetMy},
		UserPrompt: "재작성",
	}
	sys := BuildMessages(c)[0].Content
	if !strings.Contains(sys, "단문 위주, 한자어 자제.") {
		t.Errorf("my tone should inject style_notes: %q", sys)
	}
}

func TestBuildSystem_coolToneAppendsPhrase(t *testing.T) {
	c := Context{Options: Options{Tone: TonePresetCool}, UserPrompt: "확장"}
	sys := BuildMessages(c)[0].Content
	if !strings.Contains(sys, "차갑고 거리감") {
		t.Errorf("cool tone phrase missing: %q", sys)
	}
}

func TestBuildSystem_emptyToneNoExtra(t *testing.T) {
	c := Context{Options: Options{Tone: ""}, UserPrompt: "재작성"}
	sys := BuildMessages(c)[0].Content
	if strings.Contains(sys, "톤으로 유지하라") || strings.Contains(sys, "스타일 노트") {
		t.Errorf("empty tone leaked fragment: %q", sys)
	}
}

func TestBuildSystem_shortFormStillWorks(t *testing.T) {
	c := Context{Options: Options{Tone: TonePresetSensory, ShortForm: true}, UserPrompt: "확장"}
	sys := BuildMessages(c)[0].Content
	if !strings.Contains(sys, "한 문단 이내") {
		t.Errorf("short_form clause dropped: %q", sys)
	}
	if !strings.Contains(sys, "감각적 톤") {
		t.Errorf("tone fragment missing alongside short_form: %q", sys)
	}
}
```

The existing `TestBuildMessages_shapesSystemAndUser` test uses `Options{TonePreset: true, ShortForm: true}` — update that call site to `Options{Tone: TonePresetMy, ShortForm: true}`.

3. **`prompts.go::buildSystem`** — rewrite the tone branch. Replace the existing `if c.Options.TonePreset && strings.TrimSpace(c.StyleNotes) != "" { ... }` block with a switch:

```go
switch c.Options.Tone {
case TonePresetMy:
	if strings.TrimSpace(c.StyleNotes) != "" {
		b.WriteString("작가의 스타일 노트(반드시 따를 것):\n")
		b.WriteString(c.StyleNotes)
		b.WriteString("\n\n")
	}
case TonePresetCool:
	b.WriteString("이번 출력은 차갑고 거리감 있는 톤으로 유지하라.\n\n")
case TonePresetSensory:
	b.WriteString("이번 출력은 시각·청각·촉각 묘사를 적극 활용한 감각적 톤으로 유지하라.\n\n")
case TonePresetDry:
	b.WriteString("이번 출력은 형용사를 줄이고 사실 위주의 건조한 톤으로 유지하라.\n\n")
case TonePresetTense:
	b.WriteString("이번 출력은 짧은 문장과 끊김으로 긴장감을 살린 톤으로 유지하라.\n\n")
case TonePresetLyrical:
	b.WriteString("이번 출력은 율격이 살아있는 서정적인 톤으로 유지하라.\n\n")
case TonePresetHumor:
	b.WriteString("이번 출력은 가볍고 위트 있는 톤으로 유지하라.\n\n")
}
```

Also, in `buildUser`, the existing `if !c.Options.TonePreset && strings.TrimSpace(c.StyleNotes) != "" { ... }` block that emits `## 작가 메모` when tone_preset is OFF — change the condition to `if c.Options.Tone != TonePresetMy && ...`. (When tone is anything other than `my`, the style_notes show up in the user message as `## 작가 메모`, not in the system prompt.)

4. Verify:

```bash
cd engine && go test ./... -race && go build ./...
```

5. Commit:

```bash
git add engine/internal/ai/ai.go engine/internal/ai/prompts.go engine/internal/ai/prompts_test.go engine/internal/ai/context_test.go
git commit -m "feat(ai): replace tone_preset bool with multi-value Tone string"
```

---

## Phase B: Frontend (1 task)

### Task 2: AIOptions type + AIMode UI + AIContextPanel display

Files:
- Modify: `apps/desktop/src/lib/types.ts`
- Create: `apps/desktop/src/lib/tonePresets.ts`
- Modify: `apps/desktop/src/components/ai/AIMode.tsx`
- Modify: `apps/desktop/src/components/ai/AIContextPanel.tsx`
- Modify: `apps/desktop/src/routes/Workspace.tsx` (initial `aiOptions` state)

1. **`types.ts`** — replace `tone_preset` field:

```ts
export type ToneID =
  | "my"
  | "cool"
  | "sensory"
  | "dry"
  | "tense"
  | "lyrical"
  | "humor";

export interface AIOptions {
  tone: ToneID;
  short_form: boolean;
}
```

2. **Create `tonePresets.ts`**:

```ts
import type { ToneID } from "./types";

export const TONE_PRESETS: ReadonlyArray<{ id: ToneID; label: string }> = [
  { id: "my", label: "내 톤" },
  { id: "cool", label: "차갑게" },
  { id: "sensory", label: "감각적" },
  { id: "dry", label: "건조하게" },
  { id: "tense", label: "긴장감" },
  { id: "lyrical", label: "서정" },
  { id: "humor", label: "유머" },
];
```

3. **`AIMode.tsx`** — replace the `tone_preset` checkbox with a chip row. Find the section that renders the tone_preset checkbox (search for `tone_preset`) and replace with:

```tsx
<div className="ai-tone-row">
  <span className="ai-tone-label">톤</span>
  <div className="ai-tone-chips">
    {TONE_PRESETS.map((t) => (
      <button
        type="button"
        key={t.id}
        className={`ai-tone-chip${options.tone === t.id ? " active" : ""}`}
        onClick={() => onOptionsChange({ ...options, tone: t.id })}
      >
        {t.label}
      </button>
    ))}
  </div>
</div>
```

Add import:

```ts
import { TONE_PRESETS } from "../../lib/tonePresets";
```

Append CSS (to `AIMode.css` or wherever AIMode styles live — locate first):

```css
.ai-tone-row { display: flex; align-items: center; gap: 0.6rem; margin: 0.4rem 0; }
.ai-tone-label { font-size: 0.85rem; color: #6b6b6b; }
.ai-tone-chips { display: flex; flex-wrap: wrap; gap: 0.3rem; }
.ai-tone-chip {
  font: inherit;
  font-size: 0.82rem;
  padding: 0.2rem 0.7rem;
  border: 1px solid #d8d6cf;
  border-radius: 999px;
  background: white;
  color: #333;
  cursor: pointer;
}
.ai-tone-chip:hover { background: #f3f1ea; }
.ai-tone-chip.active { background: #1a1a1a; color: #faf9f6; border-color: #1a1a1a; }
```

4. **`AIContextPanel.tsx`** — replicate the same chip row (read+set, same shape). The panel and AIMode both let the writer toggle tone.

5. **`Workspace.tsx`** — find the initial `aiOptions` state:

```ts
const [aiOptions, setAiOptions] = useState<AIOptions>({ tone_preset: false, short_form: false });
```

Change to:

```ts
const [aiOptions, setAiOptions] = useState<AIOptions>({ tone: "my", short_form: false });
```

6. Verify:

```bash
cd apps/desktop && pnpm tsc -b && pnpm build
```

7. Commit:

```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/tonePresets.ts apps/desktop/src/components/ai/AIMode.tsx apps/desktop/src/components/ai/AIContextPanel.tsx apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(ai-mode): replace 내 톤 checkbox with multi-tone chip row"
```

(If there's an `AIMode.css` or styles inside `AIMode.tsx`, include those too.)

---

## Phase C: Smoke + tag (1 task)

### Task 3: Manual smoke + tag

1. Rebuild engine + launch:
   ```bash
   ./scripts/build-engine.sh
   LINETTA_HOME=/tmp/linetta-plan11 ./scripts/dev.sh
   ```
2. Create a project, write 100+ chars into a scene.
3. Open AI mode. Confirm the chip row shows 7 chips, `내 톤` selected by default.
4. Click `차갑게` → run AI generation with prompt "확장". Inspect the prompt that went out:
   ```bash
   sqlite3 /tmp/linetta-plan11/library.db \
     "SELECT json_extract(context_json,'\$.options.tone') FROM ai_runs ORDER BY started_at DESC LIMIT 1"
   ```
   Should return `cool`.
5. Output (when it lands) should noticeably reflect cool tone.
6. Switch to `감각적`, generate again. Output reflects sensory tone.
7. Switch back to `내 톤`. With non-empty project style_notes (edit via project settings if needed), generation respects that note.
8. Tag:
   ```bash
   git tag plan-11-tone-presets-done
   ```

---

## Done conditions

- [ ] `go test ./... -race` green.
- [ ] `pnpm tsc -b && pnpm build` green.
- [ ] Smoke checklist passes (tone in ai_runs, observable output differences).
- [ ] `plan-11-tone-presets-done` tag exists.
