# Linetta — Immersive Rebuild Design

**Date:** 2026-05-25
**Status:** Approved for implementation planning
**Scope:** Greenfield rebuild — replaces all existing Linetta code (Go novel-packet engine + Swift macOS app + tessera dependency).

## 1. Vision and Principles

Linetta becomes an **immersive desktop writing app**. The writer sees only the text they are writing; everything else — outline, characters, threads, AI — appears on demand and recedes afterward.

Four ground rules:

1. **Progressive Disclosure** — Every schema entity (Node, Entity, Thread, Mention) appears only when summoned. At rest, the writer sees only prose.
2. **Command-driven UX** — `Cmd+K` palette is the universal entry point. No persistent visual menus eat screen space.
3. **Modal contexts, not modal dialogs** — Edit / AI / ZEN are *modes of the same canvas*, not separate windows. The writer's hand stays in place.
4. **Keyboard-first** — Writers do not move their hand to a mouse.

The product *itself* changes from the previous Linetta: from "AI drafts a novel for you" to "the writer writes; AI assists inline".

## 2. Architecture

### 2.1 Tiers

```
┌─────────────────────────────────────────────────────┐
│ Tauri App Bundle (linetta.app)                      │
│                                                     │
│  ┌────────────────────┐   stdio JSONRPC ┌─────────┐│
│  │ Tauri Shell (Rust) │ ◄─────────────► │ Engine  ││
│  │  - window mgmt     │   pipe          │ (Go)    ││
│  │  - sidecar spawn   │                 │ binary  ││
│  │  - global shortcuts│                 │ sidecar ││
│  │  - file dialog     │                 │         ││
│  └─────────┬──────────┘                 └────┬────┘│
│            │ Tauri commands/events           │     │
│            ▼                                 ▼     │
│  ┌────────────────────┐                 ┌─────────┐│
│  │ Web UI (React+Vite)│                 │ SQLite  ││
│  │  - Tiptap editor   │                 │ + JSON  ││
│  │  - Library         │                 │ blobs   ││
│  │  - Workspace/ZEN   │                 └─────────┘│
│  │  - Cmd+K palette   │                            │
│  └────────────────────┘                            │
└─────────────────────────────────────────────────────┘
                                ▲
                                │ spawn (PATH or bundled)
                                ▼
                  ┌─────────────────────────────┐
                  │ Codex CLI / Claude Code CLI │
                  │  (user's OAuth, no API key) │
                  └─────────────────────────────┘
```

### 2.2 Boundary responsibilities

- **Tauri Shell (Rust)** — window management, OS-level global shortcuts (ZEN ESC), sidecar lifecycle, OS file dialogs, capture engine stdout/stderr for the log panel. JSONRPC router is a thin pass-through.
- **Web UI (React + Vite)** — screens, in-app keybindings, Tiptap, calling Tauri commands/events. *Owns no persistent state.* The DB is the engine's; the UI re-queries.
- **Go Engine** — DB, schema, markdown ↔ Tiptap JSON, mention parsing, calls to `tars/pkg/llm`, streaming. CLI offers `linetta-engine --stdio` (default, used by Tauri) and `linetta-engine --serve` (for tests / external debugging). **Zero dependency on `tessera`.**

### 2.3 Stack decisions

| Concern | Choice |
|---|---|
| Desktop shell | Tauri 2.x (Rust) |
| Frontend | React 18 + Vite + TypeScript |
| Editor | Tiptap (ProseMirror) |
| Engine language | Go 1.25 |
| LLM library | `github.com/devlikebear/tars/pkg/llm` (provider-normalized) |
| LLM providers (MVP) | Codex CLI **and** Claude Code CLI — both reuse user OAuth, no API key |
| Storage | SQLite (`modernc.org/sqlite`, WAL mode) |
| IPC | stdio JSONRPC between Rust shell and Go engine; Tauri commands/events between Web UI and Rust |

### 2.4 IPC protocols

- **Synchronous calls** (`createWork`, `listNodes`, `updateNodeContent`, etc.) → JSONRPC request/response over stdio.
- **Streaming (AI)** — `ai.run` returns a `runId` immediately. The engine then pushes `ai.delta` / `ai.done` / `ai.error` JSONRPC notifications. The Rust shell forwards them as Tauri events (`ai-delta` etc.); React listens.
- **Errors** — JSONRPC error objects; the Rust shell translates them into user-friendly messages and posts them to the log panel.

## 3. Data Model

SQLite with JSON columns where convenient. The full schema is built in MVP; UI exposure is staged.

```sql
-- 작품
CREATE TABLE projects (
  id            TEXT PRIMARY KEY,         -- uuid
  title         TEXT NOT NULL,
  genres        TEXT NOT NULL,            -- json array: ["SF","문학"]
  length_target TEXT NOT NULL,            -- flash|short|novella|novel|series
  default_pov   TEXT NOT NULL,            -- first|third_limited|omniscient
  style_notes   TEXT NOT NULL DEFAULT '', -- 작가의 "내 톤" 메모 (AI 자동주입)
  word_count    INTEGER NOT NULL DEFAULT 0, -- 전체 합계 캐시
  last_opened_node_id TEXT,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL,
  archived_at   INTEGER
);

-- 재귀 Node 트리 (자유 깊이)
CREATE TABLE nodes (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  parent_id   TEXT REFERENCES nodes(id) ON DELETE CASCADE,
  ordinal     INTEGER NOT NULL,           -- 형제 안에서 순서
  kind        TEXT NOT NULL,              -- 'container' | 'leaf'
  label       TEXT NOT NULL,              -- '1부', '3장', '씬 2'
  title       TEXT NOT NULL DEFAULT '',   -- '시작', '해변에서'
  content_doc TEXT,                       -- Tiptap JSON (leaf만 NULL 아님)
  status      TEXT NOT NULL DEFAULT 'draft', -- draft|revision|final
  word_count  INTEGER NOT NULL DEFAULT 0, -- 컨테이너는 자식 합산 캐시
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);
CREATE INDEX idx_nodes_project ON nodes(project_id, parent_id, ordinal);

-- Entity (인물·장소·물건·개념)
CREATE TABLE entities (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL,              -- character|place|item|concept
  name        TEXT NOT NULL,              -- @멘션 키 ('해진')
  aliases     TEXT NOT NULL DEFAULT '[]', -- json array
  role        TEXT NOT NULL DEFAULT '',   -- 'POV', '주인공' 같은 태그
  summary     TEXT NOT NULL DEFAULT '',
  attributes  TEXT NOT NULL DEFAULT '{}', -- json: {나이:32, 직업:"사진작가"}
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL,
  UNIQUE(project_id, name)
);

-- Mention: Node ↔ Entity (자동 생성, 작가 의식 안 함)
CREATE TABLE mentions (
  id         TEXT PRIMARY KEY,
  node_id    TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  entity_id  TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  position   INTEGER NOT NULL,            -- Tiptap doc 내 offset
  surface    TEXT NOT NULL                -- 본문에 실제 등장한 표현
);
CREATE INDEX idx_mentions_node ON mentions(node_id);
CREATE INDEX idx_mentions_entity ON mentions(entity_id);

-- Relationship: Entity ↔ Entity (Entity 시트의 "관계" 섹션) — schema only in MVP
CREATE TABLE relationships (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  from_id    TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  to_id      TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  label      TEXT NOT NULL,                -- '옛 친구', '형'
  notes      TEXT NOT NULL DEFAULT ''
);

-- Thread (스토리라인) — schema only in MVP
CREATE TABLE threads (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  color      TEXT NOT NULL DEFAULT '#666',
  summary    TEXT NOT NULL DEFAULT '',
  closed_at  INTEGER
);

-- Beat: Thread의 마디 — schema only in MVP
CREATE TABLE beats (
  id         TEXT PRIMARY KEY,
  thread_id  TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  node_id    TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  ordinal    INTEGER NOT NULL,
  label      TEXT NOT NULL DEFAULT '',
  intensity  INTEGER NOT NULL DEFAULT 1
);

-- Note: 마진 노트 — schema only in MVP
CREATE TABLE notes (
  id         TEXT PRIMARY KEY,
  node_id    TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  anchor     INTEGER NOT NULL,
  body       TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

-- Version snapshot (자동 + 명시적 Cmd+S)
CREATE TABLE node_snapshots (
  id          TEXT PRIMARY KEY,
  node_id     TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  content_doc TEXT NOT NULL,              -- Tiptap JSON at snapshot time
  reason      TEXT NOT NULL,              -- 'autosave' | 'manual' | 'ai-replace'
  created_at  INTEGER NOT NULL
);
CREATE INDEX idx_snapshots_node ON node_snapshots(node_id, created_at);

-- AI 호출 이력
CREATE TABLE ai_runs (
  id           TEXT PRIMARY KEY,
  project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id      TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  provider     TEXT NOT NULL,             -- 'codex-cli' | 'claude-code-cli'
  prompt       TEXT NOT NULL,
  context_json TEXT NOT NULL,             -- 어떤 Entity·Thread·이웃 텍스트가 들어갔는지
  output       TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL,             -- streaming|done|error|cancelled
  error        TEXT,
  started_at   INTEGER NOT NULL,
  ended_at     INTEGER
);

CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at INTEGER NOT NULL
);
```

### 3.1 Key data decisions

- **`content_doc` is Tiptap JSON**, not markdown. `@` mentions must survive as atom nodes so hover/click can resolve them. Markdown is only an export format.
- **`mentions` is derived state**. On every node save, the engine walks the Tiptap doc and rebuilds the node's mentions (delete-all-then-insert). The writer never sees the table.
- **Free recursive tree**. `parent_id=NULL` ⇒ project root. Only `kind='leaf'` carries `content_doc`. Users add nodes through Cmd+K commands ("새 씬 아래에", "새 장 옆에").
- **`word_count` cache**. Leaf computes its own on save; container is propagated upward; project total mirrors on the `projects` row.
- **MVP UI exposes 6 tables**: projects, nodes, entities, mentions, node_snapshots, ai_runs. The other 4 (relationships, threads, beats, notes) are persisted but have no UI yet.

## 4. UI Surface Contract

The reference wireframes (Library / Library New-Work Modal / Workspace Edit·AI·ZEN / Cmd+K / Outline Panel / Entity Sheet / Thread View) are the source of truth. Below is the implementation contract.

### 4.1 Library (`01`)

- Centered layout. Top-left `···` (library menu), top-right `설정`. No other chrome.
- Center: `[ APP NAME / 미니멀 헤드라인 ]` in large serif.
- `+ 새 작품` single outlined button.
- "최근 작품 · 3-5개" label + up to 4 cards in a row (recently opened first). Each card: title placeholder line + meta line `{lengthLabel} · {humanCount}`.
- Bottom `전체 라이브러리 →` link (hidden if ≤ 4 projects).
- Card click → routes to `projects.last_opened_node_id` (or first leaf).

### 4.2 New-Project modal (`02`)

- Fields: 제목 / 장르(multi-select, `+` for custom) / 예상 분량(5 options, single) / 기본 시점(3 options, single).
- Default genre chips: SF · 판타지 · 추리 · 문학.
- On `시작`:
  1. Insert `projects` row.
  2. Insert first leaf node (`label='씬 1'`, `title=''`, `parent_id=NULL`, `ordinal=0`, `content_doc='{...empty paragraph...}'`).
  3. Set `last_opened_node_id`.
- Route to Workspace, editor focused.

### 4.3 Workspace · Edit mode (`03A`)

- Top: `← 작품 · 1부 › 3장 · 씬 2` (each segment opens a sibling dropdown) | center `편집/AI` toggle | right `ZEN`.
- Left edge: hover hint "호버 → 아웃라인"; entering hover area slides in the Outline Panel (`04B`); auto-recedes after 3s idle.
- Main: Tiptap, serif, 65ch, centered.
- Right context panel (200px, always visible): `인물·장소` (mentioned entities in this node) / `활성 Thread` (placeholder in MVP) / `씬 상태`. Header has `···` → opens project settings sheet.
- The placeholder hint line shown in the wireframe is for dev only; remove before shipping.

### 4.4 Workspace · AI mode (`03B`)

- Same chrome; main area replaced.
- Top: PROMPT input.
- Below: streaming output region. A single line above the output shows `생성됨 · 컨텍스트: @해진, @윤서, 동해 해변 · Thread: 잃어버린 시간` — exactly what was attached.
- Action buttons: `커서에 삽입`, `선택 영역 교체`, `다시 생성`, `버리기`.
- Right panel replaced: read-only `AI에게 전달됨` checklist + `옵션` (톤 프리셋 / 길이).
- Insert/replace → applies edit to body, creates `ai-replace` snapshot, auto-returns to Edit mode.

### 4.5 Workspace · ZEN mode (`03C`)

- Pure black background, white serif text only.
- On top-edge hover: thin progress bar appears briefly.
- Bottom faint meta: `ESC로 나가기 · 612자 · 씬 2`.
- `ESC` or `⌘.` exits, cursor position preserved.

### 4.6 Command Palette (`Cmd+K`, `04A`)

- Sections: `이동` (씬으로 / 장으로 / 이전·다음 씬) + `보기` (아웃라인, 캐릭터 시트, 흐름) + node operations (새 씬·새 장·이름 바꾸기·삭제) + project ops (작품 설정, 이 씬의 이전 버전).
- Outline and character sheet entries are enabled in MVP. Thread view appears in the list but is marked "(곧 추가됨)" and disabled.
- "AI 호출 이력" entry is **not** in MVP — see §10. It appears starting post-MVP.

### 4.7 Outline Panel (`04B`)

- Tree with one-step indentation; current scene highlighted.
- Right-click → 여기 아래 새 씬 / 여기 옆에 새 장 / 이름 바꾸기 / 삭제.
- Drag-reorder is **out of MVP**; replaced by Cmd+K → "이 씬 위로/아래로".

### 4.8 Entity Sheet (`04C`)

- Triggered by double-click on a `@`mention or by the hover card's "편집".
- Right slide-in. Fields: avatar (one-letter glyph) · 이름 · kind+role · 속성 (key:value, free) · 요약 · 관계 (post-MVP placeholder).
- Closing returns focus to the body; sheet auto-recedes.

### 4.9 Thread View (`04D`)

- Post-MVP. Cmd+K entry exists but disabled in MVP.

## 5. Editor + Mention + AI Pipelines

### 5.1 Editor model

- Tiptap extensions: `StarterKit` (paragraph, heading, blockquote, hardBreak, history) + custom `Mention` atom + `Snapshot` meta.
- No raw markdown shown to the writer. Input rules: `**bold**`, `_italic_`, `>` blockquote, `---` horizontal rule.
- Body width 65ch. Typewriter scroll is a per-user toggle: when on, the current line is locked to the viewport center.

### 5.2 `@` mention pipeline

1. Writer types `@` → Tiptap `Suggestion` plugin opens menu.
2. Query passes through Tauri → Go `searchEntities(projectId, query)`. Results include "새 인물로 추가: 'X' (Character)".
3. Selection inserts a `mention` atom: `{ type:"mention", attrs:{ entityId, surface } }`. Render as blue underlined `@해진`.
4. On node save, the engine walks the Tiptap doc → recomputes `mentions` (delete-all-then-insert).
5. Hover → React calls `getEntity(entityId)` → small card. Double-click opens the Entity Sheet (`04C`).

### 5.3 Save flow (autosave + snapshot)

- Tiptap `onUpdate` debounced 800ms → `updateNodeContent(nodeId, doc)` JSONRPC (idempotent, always full doc).
- Engine: normalize doc → recompute word_count → UPDATE node → propagate word_count up the parent chain → recompute mentions → if last autosave > 60s ago, insert `node_snapshots(reason='autosave')`.
- `Cmd+S` → immediate `node_snapshots(reason='manual')` + toast "스냅샷 저장됨".
- AI replace → snapshot taken *before* the replacement (`reason='ai-replace'`).

### 5.4 AI mode pipeline

1. Writer enters PROMPT (or clicks a preset chip — see 5.5) → "생성" or `⌘↵`.
2. React → `ai.run` JSONRPC `{ projectId, nodeId, prompt, options:{ tonePreset, length } }`.
3. Engine assembles context:
   - **Current node body** (full)
   - **Previous-scene summary** — MVP: first 300 chars trim of the previous-leaf body. Post-MVP: LLM-summarized + cached.
   - **Mentioned entities** — for each mention in this node: name, kind, role, summary, attributes.
   - **Active threads** — empty array in MVP.
   - **`projects.style_notes`**.
4. System prompt template (`engine/internal/ai/prompts/`) merges the above → `tars/pkg/llm` `Client.Chat(..., stream=true)`.
5. Each delta → `ai.delta` notification → Rust → Tauri event → React appends to the streaming pane.
6. End → `ai.done` (full text, usage). Action buttons activate.
7. `커서에 삽입` / `선택 영역 교체` → Tiptap transaction. Replace takes an `ai-replace` snapshot first. Auto-return to Edit mode.
8. `다시 생성` → new `ai.run` with the same context. `버리기` → discard UI state (`ai_runs` record persists).

### 5.5 Preset chips and options

- Three preset chips above PROMPT: `재작성`, `확장`, `요약`. Clicking fills PROMPT with a template and positions the cursor at the end, so the writer can edit.
  - 재작성: "이 문단을 다른 톤으로 다시 써줘. 톤: {tonePreset}"
  - 확장: "이 장면을 더 감각적으로 확장해줘."
  - 요약: "이 씬을 한 문단으로 요약해줘."
- `톤 프리셋: "내 톤"` checkbox → when on, `style_notes` is emphasized in the system prompt.
- `길이: 한 문단` checkbox → adds "한 문단 이내" constraint.

### 5.6 Cancellation

- React calls `ai.cancel(runId)` → engine propagates context cancel to `tars/pkg/llm` → `ai_runs.status='cancelled'`.

## 6. Library Behavior + Project Metadata

- Boot → spawn engine sidecar → ready handshake → `listProjects(limit=5, orderBy=lastOpenedAt desc)`.
- 0 projects → new-project modal auto-opens.
- 1–5 → card grid.
- 6+ → recent 4–5 cards + `전체 라이브러리 →` link.
- Card meta: `{lengthLabel(length_target)} · {humanCount(word_count)}`. 0 words → "초안 시작 전".
- Project metadata edit lives in a sheet opened from the right context panel's `···` (title · genres · length_target · default_pov · `style_notes`).
- Soft archive only via card right-click. Permanent delete requires the Archived tab + two-step confirmation.
- `projects.last_opened_node_id` is updated (throttled, 5s) as the writer activates leaves; on missing node, falls back to first leaf.

## 7. Persistence, Versioning, Recovery, Export

### 7.1 File layout (macOS)

```
~/Library/Application Support/com.devlikebear.linetta/
  library.db
  library.db-wal
  library.db-shm
  settings.json
  backups/YYYY-MM-DD/library-HHMMSS.db
```

`LINETTA_HOME` env var overrides the base path (used by tests and dev).

### 7.2 SQLite tuning

```
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA foreign_keys=ON;
```

### 7.3 Migrations

- `engine/internal/store/migrations/NNNN_*.sql`, monotonic numbering, recorded in `schema_migrations`.
- Fresh-install applies `0001_init.sql` containing the schema in §3. There are **no migrations from the prior Linetta schema** — this is greenfield.

### 7.4 Snapshot retention

- `manual` / `ai-replace` — kept forever.
- `autosave` — same-minute autosaves overwrite each other; > 24h thinned to 1/hour; > 30 days thinned to 1/day.
- Thin-out runs once at boot and once per day at midnight (engine timer).

### 7.5 Backups

- On first boot of each calendar day, engine runs SQLite `.backup` → `backups/YYYY-MM-DD/library-HHMMSS.db`.
- Files older than 14 days are deleted by the same job.
- External backup folder (iCloud Drive / Dropbox) is **post-MVP**.

### 7.6 Recovery UI

- MVP exposes exactly one path: `Cmd+K → "이 씬의 이전 버전"` → right sheet with a timeline (manual on top; autosaves collapsed) → preview + "이 버전으로 복원" (creates a new `manual` snapshot of the current body before replacing).
- Per-node only. Whole-project rollback is post-MVP.

### 7.7 Export / Import

- Export project → single `.md` with H1/H2/H3 from the node tree; `@해진` rendered as plain text; entities listed in an appendix.
- Export single node → just that leaf.
- **No import in MVP.**

### 7.8 Crash recovery

- WAL handles the common case. On shutdown, the Tauri shell asks React to flush before the engine is terminated.
- On boot, if the previous shutdown was unclean and the last save is > 5 minutes old, the user is offered the most recent autosave snapshot.

## 8. Error Handling

| Class | Example | User-facing reaction |
|---|---|---|
| Engine not booting / crash | sidecar exits non-zero | Full-screen dialog "엔진을 시작할 수 없습니다 — 로그 보기 / 다시 시도" |
| JSONRPC timeout | > 5s | Toast "응답이 지연됩니다" + one automatic retry |
| DB locked / write failure | concurrent write | Silent retry × 3 (50/150/500ms); on final failure, toast + Cmd+S failure marker |
| AI provider missing | Codex / Claude Code CLI not on PATH | Settings link; AI toggle disabled with installation guidance |
| AI call failed | provider error | Inline error in the AI result area; "다시 생성" stays enabled |
| AI context exceeded | token limit | Inline notice "내 작품이 너무 길어 직전 씬 요약만 첨부했습니다" + automatic fallback |
| Migration failure | corrupt DB | Auto-backup + safe-mode boot + diagnostic dialog |

## 9. Testing

- **Go engine — TDD required.** New RPC handlers and domain logic follow `superpowers:test-driven-development`. SQLite in-memory for store tests. Pure functions (markdown ↔ Tiptap JSON, mention walker, summary trim) carry the bulk of unit coverage.
- **React** — Vitest + Testing Library for interactive widgets (Tiptap editor, command palette, mention suggestion). Page-level coverage delegated to e2e.
- **E2E** — Playwright via `tauri-driver`. MVP ships with one golden-path test: new project → write → @mention → AI mode → export.
- **CI** — GitHub Actions on macOS: `go test ./...`, `pnpm test`, `cargo check`, `tauri build` on every PR. E2E runs on main pushes only (longer timeout).

## 10. Observability and Privacy

- Engine logs (`stderr`) → Rust shell ring buffer (500 lines) → Settings "엔진 로그" panel.
- `ai_runs` table is the LLM audit trail. MVP exposes it via DB query only; `Cmd+K → "AI 호출 이력"` UI is post-MVP.
- **No crash reporting upload.** Private writing is private.
- AI calls happen only on explicit user action. No background prefetch.
- `ai_runs.context_json` stores exactly what left the machine — by design.
- Tauri CSP locks the WebView from contacting external domains. All external traffic flows through the engine.

## 11. Phasing

### 11.1 MVP — definition of done

When all of the following ship, a writer can complete one work end-to-end:

1. Boot / bundle — Tauri .app on macOS with engine sidecar
2. Library — grid, new-project modal, archive, full-library page
3. Workspace Edit mode — Tiptap + serif + 65ch + typewriter toggle + autosave + Cmd+S + right context panel (인물·장소 / 씬 상태; 활성 Thread placeholder)
4. `@` mention — search, autocomplete, instant entity creation, hover card, double-click sheet
5. Entity Sheet (`04C`) — attributes, summary, relationship section placeholder
6. Workspace AI mode — PROMPT + preset chips + streaming + 4 actions + auto context + options
7. ZEN mode (`03C`)
8. Command Palette — navigation, view (outline; character sheet enabled; thread disabled), node ops, project ops
9. Outline Panel (`04B`)
10. Version restore — Cmd+K → "이 씬의 이전 버전"
11. Export — project / node → markdown
12. Settings — provider selection (Codex CLI / Claude Code CLI), typewriter default, backup path display
13. Auto backup — daily `.backup`, 14-day retention

### 11.2 Post-MVP roadmap

- **P1** Thread + Beat (`04D`), AI "활성 Thread" wiring, Beat-Node binding
- **P1** Relationship (Entity Sheet "관계", auto bidirectional sync)
- **P2** Margin Note (hover-revealed inline notes)
- **P2** LLM-summarized previous-scene cache (replace the MVP trim)
- **P2** Multiple tone presets beyond "내 톤"
- **P3** Focus mode (dim non-current paragraphs)
- **P3** Ambient sound
- **P3** External backup location
- **P4** Windows / Linux builds (Tauri allows; provider detection needs hardening)
- **P4** Import (markdown / Scrivener)

## 12. Repo Structure (Greenfield)

```
linetta/
├── apps/
│   └── desktop/                  # Tauri shell + React frontend
│       ├── src-tauri/            # Rust shell
│       │   ├── src/main.rs       # window, sidecar spawn, JSONRPC bridge
│       │   ├── src/jsonrpc.rs    # stdio framing
│       │   ├── tauri.conf.json
│       │   └── Cargo.toml
│       ├── src/                  # React UI
│       │   ├── main.tsx
│       │   ├── App.tsx
│       │   ├── routes/library/
│       │   ├── routes/workspace/EditMode.tsx
│       │   ├── routes/workspace/AIMode.tsx
│       │   ├── routes/workspace/ZenMode.tsx
│       │   ├── components/editor/Tiptap.tsx
│       │   ├── components/editor/MentionExtension.ts
│       │   ├── components/CommandPalette.tsx
│       │   ├── components/OutlinePanel.tsx
│       │   ├── components/EntitySheet.tsx
│       │   ├── components/ContextPanel.tsx
│       │   ├── lib/rpc.ts        # Tauri command/event wrappers
│       │   └── lib/types.ts
│       ├── index.html
│       ├── package.json
│       └── vite.config.ts
├── engine/                       # Go engine (linetta-engine binary)
│   ├── cmd/linetta-engine/main.go
│   ├── internal/
│   │   ├── store/                # SQLite + migrations
│   │   ├── store/migrations/0001_init.sql
│   │   ├── rpc/                  # JSONRPC handlers
│   │   ├── project/
│   │   ├── node/
│   │   ├── entity/
│   │   ├── mention/              # Tiptap doc walker
│   │   ├── ai/                   # tars/pkg/llm wrapper, prompt templates
│   │   ├── ai/prompts/
│   │   ├── export/               # node/project → markdown
│   │   └── backup/
│   ├── go.mod                    # tars/pkg/llm only; tessera removed
│   └── go.sum
├── scripts/
│   ├── build-engine.sh           # Builds engine into apps/desktop/src-tauri/binaries/
│   └── dev.sh                    # Concurrent vite + tauri dev
├── docs/superpowers/specs/
│   └── 2026-05-25-linetta-immersive-rebuild-design.md  (this doc)
└── README.md
```

**To delete (greenfield):** `bin/`, `cmd/`, `examples/`, `internal/`, `macos/`, `docs/plan/`, `Makefile`, existing `go.mod` / `go.sum`, `.linetta/` local DB. Remove the `github.com/devlikebear/tessera` module dependency entirely.

## 13. Build and Dev Workflow

- `scripts/build-engine.sh` → `go build -o apps/desktop/src-tauri/binaries/linetta-engine-{target-triple}`.
- Tauri sidecar config registers that path → `tauri build` packages the binary inside the `.app`.
- `scripts/dev.sh` → build engine once → `cd apps/desktop && pnpm tauri dev` (React hot reload; engine restarts on rebuild).

## 14. Open Items (Tracked Post-Approval)

- Confirm the exact Codex CLI invocation surface in `tars/pkg/llm` matches what we need for streaming.
- Confirm Korean IME behavior in Tiptap mention suggestion across macOS input methods (한국어/2벌식, 3벌식, Google IME).
- Decide app icon, dock badge, and `.app` bundle identifier (`com.devlikebear.linetta`).

These are noted for implementation planning rather than this design.
