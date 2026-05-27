# Plan 14 — Markdown Import (Create Project from .md)

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:executing-plans` or `superpowers:subagent-driven-development`.

**Goal:** From the Library screen, the user picks a `.md` file on disk and Linetta creates a brand-new project whose 부/장/씬 tree mirrors the markdown's heading hierarchy and whose 씬 bodies are Tiptap-shaped JSON parsed from the markdown blocks. Inverse of Plan 6's export. Round-trip parity is NOT required — we map a sensible subset.

**Architecture:**
- Backend: new `engine/internal/importmd/` package — `markdown.go` (vendored markdown subset parser → Tiptap blocks), `tree.go` (heading-hierarchy Outline), `builder.go` (writes project + nodes). One new RPC `imports.markdown`.
- Frontend: `Library.tsx` gains a "가져오기 (.md)" button. Reads the file via `@tauri-apps/plugin-fs::readTextFile`, sends `{file_name, content}` to engine, navigates to the new project.

**Locked decisions:**
1. H1=title (fallback to file basename), H2=부, H3=장, H4=씬. Body under a container with no deeper heading turns that container into a leaf. Body alongside deeper children inserts a synthetic `씬 1` leaf carrying the body first.
2. Single .md → 1 project. No bulk import.
3. Vendored parser, no external markdown dep.
4. Subset: ATX `#`–`####`, paragraph, `**bold**`, `_italic_`, `>` blockquote, hardBreak (two trailing spaces).
5. Mentions stay as plain text — no entity creation.
6. Engine never reads files; the renderer passes content as a string.

> **Note**: This file is a navigational summary. The implementer should read the FULL task descriptions from the prior conversation turn (the Plan agent's output) before starting each task. All code blocks shown to the implementer there are the source of truth for verbatim implementation.

---

## Tasks

### Task 1: Vendored markdown parser
- Create: `engine/internal/importmd/markdown.go` (ParseInlines, ParseBlocks; Tiptap-shaped types)
- Create: `engine/internal/importmd/markdown_test.go` (8 cases: bold/italic, hardBreak, unmatched mark, paragraph split on blank lines, blockquote, empty, json marshal sanity)
- TDD. Commit: `feat(importmd): vendored markdown parser for paragraph/bold/italic/blockquote/hardBreak`

### Task 2: Outline tree from markdown
- Create: `engine/internal/importmd/tree.go` (Outline, OutlineNode, ParseOutline with level-clamped 5+→4, H1-as-title, body-before-first-heading dropped)
- Create: `engine/internal/importmd/tree_test.go` (8 cases incl. H3-with-body-no-H4, malformed, orphan body)
- TDD. Commit: `feat(importmd): ParseOutline maps H1/H2/H3/H4 to title/부/장/씬`

### Task 3: Builder + RPC handler
- Create: `engine/internal/importmd/builder.go` — `BuildProject(ctx, pr, nr, now, outline, fallbackTitle) (project.Project, error)`. Inserts root nodes as siblings of the auto-seeded `씬 1` (then deletes the seed). For containers with both Body and Children, inserts a synthetic `씬 1` leaf first with the body. Default `NewInput`: `Genres: []`, `LengthTarget: "short"`, `DefaultPOV: "first"`. Empty doc fallback: `{"type":"doc","content":[{"type":"paragraph"}]}`.
- Create: `engine/internal/importmd/builder_test.go` (4 cases: empty→keeps seed, full tree, H3-body-no-H4 leaf, H2-with-body-and-children synthetic scene)
- Create: `engine/internal/rpc/handlers/imports.go` — `ImportMarkdown(pr, nr, now Clock) rpc.Handler`. Params `{file_name, content}` → result `{project_id}`. Strips `.md`/`.markdown` from file_name for fallback title.
- Create: `engine/internal/rpc/handlers/imports_test.go` (2 cases: creates from content; fallback title from file name)
- Modify: `engine/cmd/linetta-engine/main.go` — register `s.Handle("imports.markdown", handlers.ImportMarkdown(projects, nodes, clock))` near `export.*` block.
- Commit: `feat(importmd): BuildProject + imports.markdown RPC create project from .md`

### Task 4: Frontend "가져오기" button
- Modify: `apps/desktop/src-tauri/capabilities/default.json` — add `"fs:allow-read-text-file"`.
- Modify: `apps/desktop/src/lib/types.ts` — append `ImportMarkdownResult { project_id: string }`.
- Modify: `apps/desktop/src/lib/rpc.ts` — add `imports = { markdown(fileName, content) }` namespace.
- Create: `apps/desktop/src/lib/importLoad.ts` — `pickAndReadMarkdown()` helper using `@tauri-apps/plugin-dialog::open` + `@tauri-apps/plugin-fs::readTextFile`. Returns `{fileName, content} | null` (null on cancel).
- Modify: `apps/desktop/src/routes/Library.tsx` — add `가져오기 (.md)` button below `+ 새 작품`. On click → `pickAndReadMarkdown()` → `imports.markdown(...)` → `navigate('/workspace/' + project_id)`. Alert on failure.
- Verify: `pnpm tsc -b && pnpm build`.
- Commit: `feat(library): 가져오기 button imports a .md file as a new project`

### Task 5: Smoke + tag
1. Export an existing project to .md (Plan 6).
2. Fresh `LINETTA_HOME=/tmp/linetta-plan14`, launch dev.
3. Library → "가져오기 (.md)" → pick the file.
4. Verify Outline shows 부/장/씬 tree; open a leaf; verify body text + bold/italic.
5. H1-only .md: project with the H1 as title, auto-seeded 씬 1 (body before first H2 is dropped — acceptable degradation).
6. Malformed .md (no headings): project with file-basename title, single 씬 1 leaf.
7. No-H1 with structure: file-basename title, tree shape preserved.
8. Tag: `git tag plan-14-markdown-import-done`.

## Done conditions
- [ ] `go test ./engine/... -race` green.
- [ ] `pnpm tsc -b && pnpm build` green.
- [ ] All 4 smoke fixtures behave correctly.
- [ ] `plan-14-markdown-import-done` tag exists.

## Out of scope
- Body between H1 and first H2 (dropped on import).
- Mention entity creation from `@해진` text.
- Bulk directory import.
- Round-trip parity guarantee.
- Headings deeper than H4 (collapsed).
- Multi-line spanning marks.
- Images, code blocks, lists (pass-through as plain text with raw chars).
