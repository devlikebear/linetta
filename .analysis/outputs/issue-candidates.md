# Issue Candidates

### SEC-001: stale autosave cross-scene overwrite

- Module: `apps/desktop/src/routes/Workspace.tsx`
- Type: `SEC`
- Evidence: delayed doc argument is executed by latest callback capturing a different node ID.
- Suggested action: node/version-bound save queue, cancel/flush, optimistic locking, regression tests.

### TIDY-001: Workspace responsibility concentration

- Module: `apps/desktop/src/routes/Workspace.tsx`
- Type: `TIDY`
- Evidence: UI, outline, save ordering, AI orchestration and render tree live in one 2k-line component.
- Suggested action: after SEC-001 hotfix, extract test seams, outline and persistence hooks incrementally.

### DUP-001: repeated Tiptap tree walkers

- Module: `engine/internal`
- Type: `DUP`
- Evidence: five near-identical document-to-text walkers drift independently.
- Suggested action: share only identical output policy in `internal/tiptapdoc`.

### SEC-002: backup success marker precedes verified backup

- Module: `engine/internal/backup`
- Type: `SEC`
- Evidence: date directory is created before VACUUM and treated as completion.
- Suggested action: validated temp backup plus atomic completion marker.

### TIDY-002: RPC and localization contracts lack executable schemas

- Module: `apps/desktop/src/lib`
- Type: `TIDY`
- Evidence: wire null/optional drift and `MessageKey` begins with `string`.
- Suggested action: fixture contract tests and i18n catalog/placeholder tests.
