
# Plan 6 — MVP Completion (ZEN + Restore + Export + Settings + Backups)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the five remaining MVP DoD items — Settings (#12), Markdown export (#11), Auto backup (#13), Version restore (#10), and ZEN mode (#7) — so a writer can complete one work end-to-end on Linetta: create a project, write with @mentions, ask AI to rewrite, slip into ZEN, restore an earlier version, export to markdown, and tweak provider/typewriter defaults in Settings, all while a daily backup quietly accumulates in the background.

**Architecture:** Three new engine packages (`settings`, `export`, `backup`) plus a `snapshot/retention.go` helper. The `settings` package owns the JSON config and is consulted by `ai.Runner` on each `Start` so provider changes take effect on the next AI run without a restart. `export` reuses the existing Tiptap doc walker pattern from `engine/internal/ai/context.go` but emits markdown instead of plaintext. `backup` runs `VACUUM INTO` on a goroutine driven by an `time.AfterFunc` chain anchored at the next local midnight; the snapshot retention thinning piggybacks on the same schedule. Two new Tauri 2 plugins (`tauri-plugin-dialog`, `tauri-plugin-fs`) handle the OS save-file dialog and write for markdown export — capability JSON is added because the project does not currently have one. The frontend grows three new screens/sheets: real Settings page, a right-side `VersionSheet` (mirroring `EntitySheet`), and a `ZenMode` component that reuses the existing Tiptap editor instance (so cursor position survives the mode flip).

**Tech Stack additions:**
- Rust: `tauri-plugin-dialog = "2"`, `tauri-plugin-fs = "2"`.
- Frontend: `@tauri-apps/plugin-dialog ^2`, `@tauri-apps/plugin-fs ^2`.
- Go: no new modules.

**Spec reference:** `docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md` — §4.5 (ZEN), §4.6 (Cmd+K), §5.3 (save flow), §6 (Library), §7.1–7.7 (Persistence/Versioning/Export/Backup), §8 (Error handling), §10 (Observability), §11.1 (MVP DoD items 7, 10, 11, 12, 13).

---

## Pre-flight

- [ ] Plan 5 AI mode tests pass (`cd engine && go test ./internal/ai/... ./internal/rpc/...`). The `plan-5-ai-mode-done` tag was deferred to Task 15 of this plan, where it is backfilled alongside `plan-6-mvp-completion-done` once the full MVP smoke (which exercises AI mode) succeeds.
- [ ] `git status --short` is empty.
- [ ] `cd engine && go test ./... -race` green.
- [ ] `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- [ ] `cd apps/desktop/src-tauri && cargo check` green.

---

## File Structure (created or modified)

```
engine/internal/settings/
  settings.go          (new — Config struct, Load/Save, Patch)
  settings_test.go     (new)

engine/internal/export/
  markdown.go          (new — Tiptap JSON → markdown serializer)
  markdown_test.go     (new)
  project.go           (new — walks the node tree → single .md)
  project_test.go      (new)

engine/internal/backup/
  backup.go            (new — RunDailyIfNeeded + Prune)
  backup_test.go       (new)
  scheduler.go         (new — Start kicks off the midnight chain)

engine/internal/snapshot/
  retention.go         (new — single SQL pass thinning autosaves)
  retention_test.go    (new)
  repo.go              (modified — ListForNode, GetByID, Update via Restore helpers)
  repo_test.go         (modified — ListForNode + restore round-trip)

engine/internal/rpc/handlers/
  settings.go          (new — settings.get / settings.set)
  settings_test.go     (new)
  export.go            (new — export.project / export.node)
  export_test.go       (new)
  snapshots.go         (modified — list_for_node + restore handlers)
  snapshots_test.go    (modified)

engine/internal/ai/
  runner.go            (modified — provider read each Start via ProviderSource)
  runner_test.go       (modified — fake settings source)

engine/cmd/linetta-engine/main.go  (modified — settings, backup scheduler, retention, new handlers)

apps/desktop/src-tauri/
  Cargo.toml                       (modified — add tauri-plugin-dialog, tauri-plugin-fs)
  src/lib.rs                       (modified — register plugins)
  capabilities/default.json        (new — dialog/fs allowlist)
  tauri.conf.json                  (modified — reference capabilities/default.json)

apps/desktop/src/
  lib/types.ts                     (modified — Settings, SnapshotEntry, ExportPayload)
  lib/rpc.ts                       (modified — settings, export, snapshots.listForNode, snapshots.restore)
  lib/exportSave.ts                (new — wraps dialog + fs plugin write)
  routes/Settings.tsx              (rewritten — real form)
  routes/Settings.test.tsx         (new — smoke render)
  routes/Workspace.tsx             (modified — ZEN mode, version sheet, export commands, typewriter default)
  components/VersionSheet.tsx      (new)
  components/VersionSheet.css      (new)
  components/ZenMode.tsx           (new)
  components/ZenMode.css           (new)
  App.css                          (APPEND minor — ZEN button on top bar)
  package.json                     (modified — add plugin-dialog + plugin-fs)
```

---

## Phase A: Settings backend

### Task 1: `engine/internal/settings` package (TDD)

The `settings` package owns the JSON file at `$LINETTA_HOME/settings.json`. Fields are `provider` (string), `typewriter_default` (bool), plus a computed read-only `backup_dir`. Concurrent access goes through a sync.RWMutex; `Load` is forgiving of missing/corrupt files (returns defaults), `Save` writes atomically via temp file + rename.

**Files:**
- Create: `engine/internal/settings/settings.go`
- Create: `engine/internal/settings/settings_test.go`

- [ ] **Step 1: Failing test**

`engine/internal/settings/settings_test.go`:

```go
package settings

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func newStoreOnTemp(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestLoad_missingFileReturnsDefaults(t *testing.T) {
	s := newStoreOnTemp(t)
	got, err := s.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Provider != "claude-code-cli" {
		t.Errorf("provider = %q, want claude-code-cli", got.Provider)
	}
	if got.TypewriterDefault != false {
		t.Errorf("typewriter_default = %v", got.TypewriterDefault)
	}
	if got.BackupDir == "" {
		t.Error("backup_dir empty")
	}
}

func TestSet_partialPatchPreservesUntouchedFields(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{TypewriterDefault: boolPtr(true)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ := s.Get(context.Background())
	if got.Provider != "claude-code-cli" {
		t.Errorf("provider mutated to %q", got.Provider)
	}
	if !got.TypewriterDefault {
		t.Errorf("typewriter_default not persisted")
	}
}

func TestSet_persistsToDisk(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{Provider: strPtr("openai-codex")}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	s2, err := New()
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	got, _ := s2.Get(context.Background())
	if got.Provider != "openai-codex" {
		t.Errorf("provider not persisted across reload: %q", got.Provider)
	}
}

func TestSet_rejectsUnknownProvider(t *testing.T) {
	s := newStoreOnTemp(t)
	_, err := s.Set(context.Background(), Patch{Provider: strPtr("bad-provider")})
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestLoad_corruptFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("not-json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s, err := New()
	if err != nil {
		t.Fatalf("New on corrupt: %v", err)
	}
	got, _ := s.Get(context.Background())
	if got.Provider != "claude-code-cli" {
		t.Errorf("did not fall back to defaults: %+v", got)
	}
}

func boolPtr(v bool) *bool   { return &v }
func strPtr(v string) *string { return &v }
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/settings/...
```

- [ ] **Step 3: Implement**

`engine/internal/settings/settings.go`:

```go
// Package settings persists user-controlled preferences in $LINETTA_HOME/settings.json.
// The struct is intentionally tiny in MVP: provider choice + typewriter default,
// plus a read-only backup_dir field surfaced for the UI.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/paths"
)

// Allowed provider IDs (must match tars/pkg/llm provider names + UI labels).
const (
	ProviderClaudeCodeCLI = "claude-code-cli"
	ProviderOpenAICodex   = "openai-codex"
)

func validProviders() []string { return []string{ProviderClaudeCodeCLI, ProviderOpenAICodex} }

// Config is the on-disk JSON. backup_dir is computed at Load time and not
// persisted (the field is omitted from JSON marshalling on write).
type Config struct {
	Provider          string `json:"provider"`
	TypewriterDefault bool   `json:"typewriter_default"`
	BackupDir         string `json:"backup_dir,omitempty"`
}

// Patch holds optional updates. Nil pointers mean "leave the field alone".
type Patch struct {
	Provider          *string `json:"provider,omitempty"`
	TypewriterDefault *bool   `json:"typewriter_default,omitempty"`
}

// Store reads and writes the settings file with internal locking.
type Store struct {
	mu  sync.RWMutex
	cfg Config
	dir string
}

// New constructs a Store, ensuring $LINETTA_HOME exists and loading the file.
// Missing or corrupt files yield defaults (and a quiet rewrite on next Set).
func New() (*Store, error) {
	if err := paths.EnsureHome(); err != nil {
		return nil, err
	}
	home, err := paths.Home()
	if err != nil {
		return nil, err
	}
	s := &Store{dir: home, cfg: defaults(home)}
	_ = s.load() // benign: defaults already set
	return s, nil
}

func defaults(home string) Config {
	return Config{
		Provider:          ProviderClaudeCodeCLI,
		TypewriterDefault: false,
		BackupDir:         filepath.Join(home, "backups"),
	}
}

func (s *Store) load() error {
	path := filepath.Join(s.dir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // keep defaults
		}
		return err
	}
	var disk Config
	if err := json.Unmarshal(data, &disk); err != nil {
		return nil // ignore corrupt file; defaults stand
	}
	s.mu.Lock()
	if disk.Provider != "" {
		s.cfg.Provider = disk.Provider
	}
	s.cfg.TypewriterDefault = disk.TypewriterDefault
	s.mu.Unlock()
	return nil
}

// Get returns a copy of the current Config (with backup_dir filled in).
func (s *Store) Get(ctx context.Context) (Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.cfg
	c.BackupDir = filepath.Join(s.dir, "backups")
	return c, nil
}

// Provider returns the active provider id (cheap, lock-protected).
// ai.Runner calls this on every Start so provider changes take effect at once.
func (s *Store) Provider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Provider
}

// Set applies a partial patch, validates, persists atomically, returns the new Config.
func (s *Store) Set(ctx context.Context, p Patch) (Config, error) {
	s.mu.Lock()
	next := s.cfg
	if p.Provider != nil {
		if !contains(validProviders(), *p.Provider) {
			s.mu.Unlock()
			return Config{}, fmt.Errorf("settings: unknown provider %q", *p.Provider)
		}
		next.Provider = *p.Provider
	}
	if p.TypewriterDefault != nil {
		next.TypewriterDefault = *p.TypewriterDefault
	}
	s.cfg = next
	s.mu.Unlock()

	// Persist (no backup_dir on disk).
	persistable := Config{Provider: next.Provider, TypewriterDefault: next.TypewriterDefault}
	body, err := json.MarshalIndent(persistable, "", "  ")
	if err != nil {
		return Config{}, err
	}
	tmp := filepath.Join(s.dir, "settings.json.tmp")
	target := filepath.Join(s.dir, "settings.json")
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return Config{}, err
	}
	if err := os.Rename(tmp, target); err != nil {
		return Config{}, err
	}
	return s.Get(ctx)
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run — green**

```bash
cd engine && go test ./internal/settings/... -race
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/settings/
git commit -m "feat(settings): JSON-backed config store with provider + typewriter default"
```

---

### Task 2: `settings.get` / `settings.set` handlers + provider source wiring (TDD)

Two handlers plus a tiny refactor of `ai.Runner` so it asks a `ProviderSource` interface for the provider on every `Start`, instead of holding a frozen constructor arg. This is the only way to make a provider change take effect on the very next AI run without restart.

**Files:**
- Modify: `engine/internal/ai/runner.go`
- Modify: `engine/internal/ai/runner_test.go`
- Create: `engine/internal/rpc/handlers/settings.go`
- Create: `engine/internal/rpc/handlers/settings_test.go`

- [ ] **Step 1: Failing tests**

`engine/internal/rpc/handlers/settings_test.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

func newSettingsFixture(t *testing.T) *settings.Store {
	t.Helper()
	t.Setenv("LINETTA_HOME", t.TempDir())
	s, err := settings.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestGetSettingsHandler_returnsDefaults(t *testing.T) {
	store := newSettingsFixture(t)
	res, err := GetSettings(store)(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got settings.Config
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Provider != "claude-code-cli" {
		t.Errorf("provider = %q", got.Provider)
	}
	if got.BackupDir == "" {
		t.Error("backup_dir not surfaced")
	}
}

func TestSetSettingsHandler_partial(t *testing.T) {
	store := newSettingsFixture(t)
	res, err := SetSettings(store)(context.Background(), json.RawMessage(`{"provider":"openai-codex"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got settings.Config
	_ = json.Unmarshal(res, &got)
	if got.Provider != "openai-codex" {
		t.Errorf("provider not applied: %+v", got)
	}
}

func TestSetSettingsHandler_invalidProvider(t *testing.T) {
	store := newSettingsFixture(t)
	_, err := SetSettings(store)(context.Background(), json.RawMessage(`{"provider":"nope"}`))
	if err == nil {
		t.Error("expected validation error")
	}
}
```

And to `engine/internal/ai/runner_test.go` add (after existing tests):

```go
type stubProvider struct{ v string }

func (s *stubProvider) Provider() string { return s.v }

func TestRunner_readsProviderOnEachStart(t *testing.T) {
	// Existing runnerFixture exposes a fake client + repo; reuse it.
	f := newRunnerFixture(t)
	src := &stubProvider{v: "claude-code-cli"}
	r := NewRunner(f.notify, f.runs, f.factory, src)

	_, err := r.Start(context.Background(), sampleContext(), func() int64 { return 1 })
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	// Flip the source mid-run; the next Start should observe it.
	src.v = "openai-codex"
	_, err = r.Start(context.Background(), sampleContext(), func() int64 { return 2 })
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if got := f.factory.lastProvider; got != "openai-codex" {
		t.Errorf("factory called with %q on second start, want openai-codex", got)
	}
}
```

Where `f.factory.lastProvider` is recorded by the existing fake factory used in the package's tests; if it does not yet exist, add a `lastProvider string` field to the fixture's factory struct and set it inside the factory func.

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/ai/... ./internal/rpc/handlers/...
```

- [ ] **Step 3: Modify `ai.Runner`**

In `engine/internal/ai/runner.go`:

```go
// ProviderSource yields the current provider id; consulted on every Start so
// settings changes take effect on the next AI call without an engine restart.
type ProviderSource interface {
	Provider() string
}

// Replace the existing `provider string` field on Runner with `src ProviderSource`.
type Runner struct {
	notify  rpc.Notifier
	runs    *store.AIRunsRepo
	factory ClientFactory
	src     ProviderSource
	workDir string

	mu     sync.Mutex
	active map[string]context.CancelFunc
}

// NewRunner now takes a ProviderSource.
func NewRunner(notify rpc.Notifier, runs *store.AIRunsRepo, factory ClientFactory, src ProviderSource) *Runner {
	return &Runner{
		notify:  notify,
		runs:    runs,
		factory: factory,
		src:     src,
		active:  map[string]context.CancelFunc{},
	}
}

// Inside Start, replace every reference to r.provider with `provider := r.src.Provider()`
// and pass `provider` to both r.runs.Insert (Provider: provider) and r.factory(provider, r.workDir).
```

- [ ] **Step 4: Implement handlers**

`engine/internal/rpc/handlers/settings.go`:

```go
package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// GetSettings returns a handler for settings.get.
func GetSettings(store *settings.Store) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		got, err := store.Get(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

// SetSettings returns a handler for settings.set. Accepts a partial patch.
func SetSettings(store *settings.Store) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p settings.Patch
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
		}
		next, err := store.Set(ctx, p)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		return json.Marshal(next)
	}
}
```

- [ ] **Step 5: Run — green**

```bash
cd engine && go test ./internal/ai/... ./internal/rpc/handlers/... ./internal/settings/... -race
```

- [ ] **Step 6: Commit**

```bash
git add engine/internal/ai/ engine/internal/rpc/handlers/settings.go engine/internal/rpc/handlers/settings_test.go
git commit -m "feat(settings): settings.get/set handlers; ai.Runner reads provider per-Start"
```

---

## Phase B: Markdown export backend

### Task 3: Tiptap-JSON → markdown serializer (TDD)

A pure function `DocToMarkdown([]byte) (string, error)`. Mirrors the structure of `docToPlainText` in `engine/internal/ai/context.go` but emits markdown. Mapping:

| Tiptap node                            | Markdown                            |
|----------------------------------------|-------------------------------------|
| `paragraph`                            | text + `\n\n`                       |
| `heading` level=1/2/3                  | `# ` / `## ` / `### ` + text + `\n\n` |
| `blockquote`                           | `> ` prefix on each line + `\n\n`   |
| `hardBreak`                            | `  \n` (two spaces + newline)       |
| `text` with `bold` mark                | `**text**`                          |
| `text` with `italic` mark              | `_text_`                            |
| `text` with both                       | `**_text_**`                        |
| `mention`                              | `@{label}` (plain text)             |
| `horizontalRule`                       | `---\n\n`                           |
| anything else                          | children only (no decoration)       |

Trailing blank-line consolidation: collapse `>=3` newlines to `2`.

**Files:**
- Create: `engine/internal/export/markdown.go`
- Create: `engine/internal/export/markdown_test.go`

- [ ] **Step 1: Failing test**

`engine/internal/export/markdown_test.go`:

```go
package export

import "testing"

func TestDocToMarkdown_paragraphAndBoldItalic(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"paragraph","content":[
	    {"type":"text","text":"안녕 "},
	    {"type":"text","marks":[{"type":"bold"}],"text":"세계"},
	    {"type":"text","text":" "},
	    {"type":"text","marks":[{"type":"italic"}],"text":"여기"}
	  ]}
	]}`
	got, err := DocToMarkdown([]byte(doc))
	if err != nil {
		t.Fatalf("DocToMarkdown: %v", err)
	}
	want := "안녕 **세계** _여기_\n\n"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestDocToMarkdown_headings(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"3장"}]},
	  {"type":"paragraph","content":[{"type":"text","text":"본문"}]}
	]}`
	got, _ := DocToMarkdown([]byte(doc))
	want := "## 3장\n\n본문\n\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDocToMarkdown_blockquoteAndHardBreak(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"blockquote","content":[
	    {"type":"paragraph","content":[
	      {"type":"text","text":"첫 줄"},
	      {"type":"hardBreak"},
	      {"type":"text","text":"둘째 줄"}
	    ]}
	  ]}
	]}`
	got, _ := DocToMarkdown([]byte(doc))
	want := "> 첫 줄  \n> 둘째 줄\n\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDocToMarkdown_mentionRendersAsPlainText(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"paragraph","content":[
	    {"type":"text","text":"오늘은 "},
	    {"type":"mention","attrs":{"id":"e1","label":"해진"}},
	    {"type":"text","text":"이 도시에 도착했다."}
	  ]}
	]}`
	got, _ := DocToMarkdown([]byte(doc))
	want := "오늘은 @해진이 도시에 도착했다.\n\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDocToMarkdown_boldItalicCombined(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"paragraph","content":[
	    {"type":"text","marks":[{"type":"bold"},{"type":"italic"}],"text":"강조"}
	  ]}
	]}`
	got, _ := DocToMarkdown([]byte(doc))
	want := "**_강조_**\n\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDocToMarkdown_emptyDoc(t *testing.T) {
	got, err := DocToMarkdown([]byte(`{"type":"doc","content":[]}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestDocToMarkdown_corruptInput(t *testing.T) {
	_, err := DocToMarkdown([]byte("not-json"))
	if err == nil {
		t.Error("expected parse error")
	}
}
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/export/...
```

- [ ] **Step 3: Implement**

`engine/internal/export/markdown.go`:

```go
// Package export converts Tiptap JSON documents to markdown.
// Mentions are rendered as plain `@label`. The whole-project export
// (project.go) walks the node tree and produces a single document with
// H1/H2/H3 headings derived from depth.
package export

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// DocToMarkdown parses a Tiptap JSON document and returns its markdown rendering.
// Returns an error only when the input is not valid JSON.
func DocToMarkdown(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return "", fmt.Errorf("export: parse doc: %w", err)
	}
	var sb strings.Builder
	walk(node, &sb, false)
	return collapseBlankLines(sb.String()), nil
}

var multiBlankRE = regexp.MustCompile(`\n{3,}`)

func collapseBlankLines(s string) string {
	return multiBlankRE.ReplaceAllString(s, "\n\n")
}

// walk recurses through the node tree. `inBlockquote` is true while inside a
// blockquote so paragraph children prefix `> ` per line.
func walk(v any, sb *strings.Builder, inBlockquote bool) {
	switch t := v.(type) {
	case []any:
		for _, c := range t {
			walk(c, sb, inBlockquote)
		}
	case map[string]any:
		kind, _ := t["type"].(string)
		switch kind {
		case "doc":
			if content, ok := t["content"].([]any); ok {
				walk(content, sb, inBlockquote)
			}
		case "paragraph":
			var inner strings.Builder
			if content, ok := t["content"].([]any); ok {
				walk(content, &inner, false)
			}
			body := inner.String()
			if inBlockquote {
				body = prefixLines(body, "> ")
			}
			sb.WriteString(body)
			sb.WriteString("\n\n")
		case "heading":
			level := 1
			if attrs, ok := t["attrs"].(map[string]any); ok {
				if l, ok := attrs["level"].(float64); ok {
					level = int(l)
				}
			}
			if level < 1 {
				level = 1
			}
			if level > 6 {
				level = 6
			}
			sb.WriteString(strings.Repeat("#", level))
			sb.WriteString(" ")
			if content, ok := t["content"].([]any); ok {
				walk(content, sb, false)
			}
			sb.WriteString("\n\n")
		case "blockquote":
			if content, ok := t["content"].([]any); ok {
				walk(content, sb, true)
			}
		case "horizontalRule":
			sb.WriteString("---\n\n")
		case "hardBreak":
			sb.WriteString("  \n")
		case "mention":
			label := ""
			if attrs, ok := t["attrs"].(map[string]any); ok {
				if s, ok := attrs["label"].(string); ok {
					label = s
				}
			}
			if label != "" {
				sb.WriteString("@")
				sb.WriteString(label)
			}
		case "text":
			text, _ := t["text"].(string)
			bold, italic := false, false
			if marks, ok := t["marks"].([]any); ok {
				for _, m := range marks {
					mm, _ := m.(map[string]any)
					switch mm["type"] {
					case "bold":
						bold = true
					case "italic":
						italic = true
					}
				}
			}
			if italic {
				text = "_" + text + "_"
			}
			if bold {
				text = "**" + text + "**"
			}
			sb.WriteString(text)
		default:
			// Unknown node — recurse children only.
			if content, ok := t["content"].([]any); ok {
				walk(content, sb, inBlockquote)
			}
		}
	}
}

// prefixLines applies `prefix` to every line of `s`. The trailing newline (if any)
// is preserved so the caller can append its own `\n\n` block boundary.
func prefixLines(s, prefix string) string {
	if s == "" {
		return prefix
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4: Run — green**

```bash
cd engine && go test ./internal/export/... -race
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/export/markdown.go engine/internal/export/markdown_test.go
git commit -m "feat(export): Tiptap JSON → markdown serializer with bold/italic/blockquote/mention"
```

---

### Task 4: Project-level export walker (TDD)

`ExportProject(ctx, projectID) (Payload, error)` and `ExportNode(ctx, nodeID) (Payload, error)`. Walks the node tree DFS using the existing `node.Repo.ListByProject`, builds heading levels from depth (depth 1 = `#`, depth 2 = `##`, depth 3+ = `###`). Each leaf appends its `DocToMarkdown` body. After the tree, appends a `## 등장인물` section listing entities (name · kind · role · summary). The project's own title is the doc's `# ` (depth-0).

`Payload` shape:

```go
type Payload struct {
	Markdown          string `json:"markdown"`
	SuggestedFilename string `json:"suggested_filename"`
}
```

Filename for a project: `{slug(title)}.md`. For a node: `{slug(label)}.md` (fall back to `scene.md` when label empty).

**Files:**
- Create: `engine/internal/export/project.go`
- Create: `engine/internal/export/project_test.go`

- [ ] **Step 1: Failing test**

`engine/internal/export/project_test.go`:

```go
package export

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newExportFixture(t *testing.T) (*store.Store, *project.Repo, *node.Repo, *entity.Repo) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, project.NewRepo(s), node.NewRepo(s), entity.NewRepo(s)
}

func TestExportProject_buildsTreeWithHeadingsAndEntitiesAppendix(t *testing.T) {
	_, pr, nr, er := newExportFixture(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, 1, project.NewInput{
		Title: "조용한 도시", Genres: []string{"문학"}, LengthTarget: "novella", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	// p auto-creates 씬 1 leaf at root. We add 1부 → 1장 → 씬 1·씬 2.
	bu1, _ := nr.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1부", "")
	ch1, _ := nr.CreateChild(ctx, bu1.ID, "container", "1장", "")
	scene1, _ := nr.CreateChild(ctx, ch1.ID, "leaf", "씬 1", "해변에서")
	_ = nr.UpdateContent(ctx, scene1.ID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"파도 소리"}]}]}`, 100)
	scene2, _ := nr.CreateChild(ctx, ch1.ID, "leaf", "씬 2", "")
	_ = nr.UpdateContent(ctx, scene2.ID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"두 번째 장면"}]}]}`, 101)

	_, _ = er.Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "해진", Role: "POV", Kind: "character"})

	out, err := ExportProject(ctx, pr, nr, er, p.ID)
	if err != nil {
		t.Fatalf("ExportProject: %v", err)
	}
	if !strings.HasPrefix(out.Markdown, "# 조용한 도시\n\n") {
		t.Errorf("missing project H1 prefix; got prefix %q", out.Markdown[:min(40, len(out.Markdown))])
	}
	if !strings.Contains(out.Markdown, "## 1부") {
		t.Errorf("missing 1부 heading; doc=\n%s", out.Markdown)
	}
	if !strings.Contains(out.Markdown, "### 1장") {
		t.Errorf("missing 1장 heading")
	}
	if !strings.Contains(out.Markdown, "### 씬 1 — 해변에서") {
		t.Errorf("missing scene 1 heading with title")
	}
	if !strings.Contains(out.Markdown, "파도 소리") {
		t.Errorf("missing scene body")
	}
	if !strings.Contains(out.Markdown, "## 등장인물") {
		t.Errorf("missing entities appendix")
	}
	if !strings.Contains(out.Markdown, "해진") {
		t.Errorf("missing entity name in appendix")
	}
	if out.SuggestedFilename != "조용한-도시.md" {
		t.Errorf("filename = %q, want 조용한-도시.md", out.SuggestedFilename)
	}
}

func TestExportNode_returnsLeafBodyOnly(t *testing.T) {
	_, pr, nr, _ := newExportFixture(t)
	ctx := context.Background()
	p, _ := pr.Create(ctx, 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	_ = nr.UpdateContent(ctx, *p.LastOpenedNodeID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"단편"}]}]}`, 100)
	out, err := ExportNode(ctx, nr, *p.LastOpenedNodeID)
	if err != nil {
		t.Fatalf("ExportNode: %v", err)
	}
	if strings.Contains(out.Markdown, "#") {
		t.Errorf("node export should not contain headings; got:\n%s", out.Markdown)
	}
	if !strings.Contains(out.Markdown, "단편") {
		t.Errorf("missing body")
	}
	if out.SuggestedFilename != "씬-1.md" {
		t.Errorf("filename = %q", out.SuggestedFilename)
	}
}

func min(a, b int) int { if a < b { return a }; return b }
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/export/...
```

- [ ] **Step 3: Implement**

`engine/internal/export/project.go`:

```go
package export

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

// Payload is the wire-shape returned by both ExportProject and ExportNode.
type Payload struct {
	Markdown          string `json:"markdown"`
	SuggestedFilename string `json:"suggested_filename"`
}

// ExportProject builds a single markdown document from the project tree plus an
// entities appendix. Heading levels are derived from depth (root containers = ##).
func ExportProject(ctx context.Context, pr *project.Repo, nr *node.Repo, er *entity.Repo, projectID string) (Payload, error) {
	p, err := pr.Get(ctx, projectID)
	if err != nil {
		return Payload{}, err
	}
	flat, err := nr.ListByProject(ctx, projectID)
	if err != nil {
		return Payload{}, err
	}
	children := map[string][]node.Node{}
	for _, n := range flat {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}

	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(p.Title)
	sb.WriteString("\n\n")

	var walk func(parentKey string, depth int) error
	walk = func(parentKey string, depth int) error {
		for _, n := range children[parentKey] {
			level := depth + 2 // depth 0 → ##
			if level > 3 {
				level = 3
			}
			heading := strings.Repeat("#", level) + " " + n.Label
			if n.Title != "" {
				heading += " — " + n.Title
			}
			sb.WriteString(heading)
			sb.WriteString("\n\n")
			if n.Kind == "leaf" && n.ContentDoc != nil {
				body, err := DocToMarkdown([]byte(*n.ContentDoc))
				if err != nil {
					return fmt.Errorf("export node %s: %w", n.ID, err)
				}
				sb.WriteString(body)
			}
			if err := walk(n.ID, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk("", 0); err != nil {
		return Payload{}, err
	}

	// Entities appendix.
	ents, err := er.Search(ctx, projectID, "", 50)
	if err != nil {
		return Payload{}, err
	}
	if len(ents) > 0 {
		sb.WriteString("## 등장인물\n\n")
		for _, e := range ents {
			line := fmt.Sprintf("- **%s** (%s)", e.Name, e.Kind)
			if e.Role != "" {
				line += " · " + e.Role
			}
			if e.Summary != "" {
				line += " — " + e.Summary
			}
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return Payload{
		Markdown:          collapseBlankLines(sb.String()),
		SuggestedFilename: slugify(p.Title) + ".md",
	}, nil
}

// ExportNode renders a single leaf's body (no heading).
func ExportNode(ctx context.Context, nr *node.Repo, nodeID string) (Payload, error) {
	n, err := nr.Get(ctx, nodeID)
	if err != nil {
		return Payload{}, err
	}
	if n.Kind != "leaf" || n.ContentDoc == nil {
		return Payload{}, fmt.Errorf("export: node %s has no content_doc", nodeID)
	}
	md, err := DocToMarkdown([]byte(*n.ContentDoc))
	if err != nil {
		return Payload{}, err
	}
	name := n.Label
	if name == "" {
		name = "scene"
	}
	return Payload{
		Markdown:          md,
		SuggestedFilename: slugify(name) + ".md",
	}, nil
}

// slugify converts a title to a filename-safe slug. Lowercases ASCII letters,
// replaces runs of whitespace/punctuation with `-`. Korean and other letter
// runes are kept verbatim.
func slugify(s string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r > 127: // keep non-ASCII letters (Korean / CJK) verbatim
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "untitled"
	}
	return out
}
```

- [ ] **Step 4: Run — green**

```bash
cd engine && go test ./internal/export/... -race
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/export/project.go engine/internal/export/project_test.go
git commit -m "feat(export): project + node walker with depth-based headings and entities appendix"
```

---

### Task 5: `export.project` / `export.node` handlers (TDD)

**Files:**
- Create: `engine/internal/rpc/handlers/export.go`
- Create: `engine/internal/rpc/handlers/export_test.go`

- [ ] **Step 1: Failing test**

`engine/internal/rpc/handlers/export_test.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
)

func TestExportProjectHandler(t *testing.T) {
	f := newNodeFixture(t)
	er := entity.NewRepo(f.store)
	h := ExportProject(f.proj, f.nodes, er)
	res, err := h(context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var p export.Payload
	if err := json.Unmarshal(res, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.HasPrefix(p.Markdown, "# T\n\n") {
		t.Errorf("missing project H1: %q", p.Markdown)
	}
	if !strings.HasSuffix(p.SuggestedFilename, ".md") {
		t.Errorf("filename = %q", p.SuggestedFilename)
	}
}

func TestExportNodeHandler(t *testing.T) {
	f := newNodeFixture(t)
	// seed content
	_ = f.nodes
	h := ExportNode(f.nodes)
	res, err := h(context.Background(), json.RawMessage(`{"node_id":"`+f.nID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var p export.Payload
	_ = json.Unmarshal(res, &p)
	if strings.Contains(p.Markdown, "#") {
		t.Errorf("node export should not have headings: %q", p.Markdown)
	}
}
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/rpc/handlers/...
```

- [ ] **Step 3: Implement**

`engine/internal/rpc/handlers/export.go`:

```go
package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type exportProjectParams struct {
	ProjectID string `json:"project_id"`
}

// ExportProject returns a handler for export.project.
func ExportProject(pr *project.Repo, nr *node.Repo, er *entity.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p exportProjectParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		out, err := export.ExportProject(ctx, pr, nr, er, p.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(out)
	}
}

type exportNodeParams struct {
	NodeID string `json:"node_id"`
}

// ExportNode returns a handler for export.node.
func ExportNode(nr *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p exportNodeParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		out, err := export.ExportNode(ctx, nr, p.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(out)
	}
}
```

- [ ] **Step 4: Run — green**

```bash
cd engine && go test ./internal/rpc/handlers/... -race
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/rpc/handlers/export.go engine/internal/rpc/handlers/export_test.go
git commit -m "feat(rpc): export.project + export.node handlers"
```

---

## Phase C: Backup daemon

### Task 6: `engine/internal/backup` package (TDD)

`RunDailyIfNeeded(ctx, db, home, now)` returns `(path, didRun, error)`. Skips if today's directory already exists. Otherwise creates `backups/YYYY-MM-DD/` (mode 0700) and runs `VACUUM INTO 'backups/YYYY-MM-DD/library-HHMMSS.db'` against the live DB. After backup, `Prune(home, now)` removes directories older than 14 days. `now` is `time.Time` so tests are deterministic.

Why `VACUUM INTO` and not SQLite `.backup`: the latter requires raw connection access (`sqlite3_backup_init`); `modernc.org/sqlite` exposes `database/sql` only. `VACUUM INTO` is a single SQL statement, produces a clean copy, works on a busy WAL DB.

**Files:**
- Create: `engine/internal/backup/backup.go`
- Create: `engine/internal/backup/backup_test.go`

- [ ] **Step 1: Failing test**

`engine/internal/backup/backup_test.go`:

```go
package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func openSeededStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	home := t.TempDir()
	dbPath := filepath.Join(home, "library.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	if _, err := pr.Create(context.Background(), 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return s, home
}

func TestRunDailyIfNeeded_createsBackupFileFirstTime(t *testing.T) {
	s, home := openSeededStore(t)
	now := time.Date(2026, 5, 26, 9, 15, 30, 0, time.UTC)
	path, did, err := RunDailyIfNeeded(context.Background(), s.DB(), home, now)
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if !did {
		t.Error("did=false on first run")
	}
	if filepath.Base(filepath.Dir(path)) != "2026-05-26" {
		t.Errorf("dir = %s, want 2026-05-26", filepath.Dir(path))
	}
	if filepath.Base(path) != "library-091530.db" {
		t.Errorf("file = %s, want library-091530.db", filepath.Base(path))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("backup is empty")
	}
}

func TestRunDailyIfNeeded_skipsWhenTodayDirExists(t *testing.T) {
	s, home := openSeededStore(t)
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)
	if _, _, err := RunDailyIfNeeded(context.Background(), s.DB(), home, now); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Same day, later time.
	later := time.Date(2026, 5, 26, 18, 0, 0, 0, time.UTC)
	_, did, err := RunDailyIfNeeded(context.Background(), s.DB(), home, later)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if did {
		t.Error("did=true on second same-day run")
	}
}

func TestPrune_removesDirsOlderThan14Days(t *testing.T) {
	_, home := openSeededStore(t)
	root := filepath.Join(home, "backups")
	for _, d := range []string{"2026-05-26", "2026-05-12", "2026-05-11", "not-a-date"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	now := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	if err := Prune(home, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "2026-05-26")); err != nil {
		t.Error("today removed")
	}
	if _, err := os.Stat(filepath.Join(root, "2026-05-12")); err != nil {
		t.Error("14-day window inner edge removed")
	}
	if _, err := os.Stat(filepath.Join(root, "2026-05-11")); !os.IsNotExist(err) {
		t.Error("15-day-old dir not removed")
	}
	if _, err := os.Stat(filepath.Join(root, "not-a-date")); err != nil {
		t.Error("non-date dir incorrectly removed")
	}
}
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/backup/...
```

- [ ] **Step 3: Implement**

`engine/internal/backup/backup.go`:

```go
// Package backup creates SQLite VACUUM INTO snapshots once per day under
// $LINETTA_HOME/backups/YYYY-MM-DD/library-HHMMSS.db and prunes directories
// older than 14 days.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const retentionDays = 14

// RunDailyIfNeeded creates one backup if today's directory does not yet exist.
// Returns the backup file path (or "" if skipped), didRun, and any error.
// The function is safe to call repeatedly on the same day.
func RunDailyIfNeeded(ctx context.Context, db *sql.DB, home string, now time.Time) (string, bool, error) {
	day := now.Format("2006-01-02")
	root := filepath.Join(home, "backups")
	dir := filepath.Join(root, day)

	if _, err := os.Stat(dir); err == nil {
		return "", false, nil // already ran today
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("stat backup dir: %w", err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("mkdir: %w", err)
	}

	filename := fmt.Sprintf("library-%s.db", now.Format("150405"))
	dst := filepath.Join(dir, filename)
	// VACUUM INTO accepts a single-quoted string literal; sanitize is not strictly
	// needed (we control the path) but use Sprintf with %q-equivalent quoting.
	sql := "VACUUM INTO '" + escapeSQLString(dst) + "'"
	if _, err := db.ExecContext(ctx, sql); err != nil {
		return "", false, fmt.Errorf("VACUUM INTO: %w", err)
	}
	return dst, true, nil
}

// Prune deletes backups/YYYY-MM-DD directories whose date is older than 14 days.
// Non-date directory names are left alone.
func Prune(home string, now time.Time) error {
	root := filepath.Join(home, "backups")
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	cutoff := now.AddDate(0, 0, -retentionDays)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		day, err := time.Parse("2006-01-02", e.Name())
		if err != nil {
			continue // skip non-date names
		}
		if day.Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
				return fmt.Errorf("remove %s: %w", e.Name(), err)
			}
		}
	}
	return nil
}

func escapeSQLString(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'')
		}
		out = append(out, s[i])
	}
	return string(out)
}
```

- [ ] **Step 4: Run — green**

```bash
cd engine && go test ./internal/backup/... -race
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/backup/backup.go engine/internal/backup/backup_test.go
git commit -m "feat(backup): daily VACUUM INTO + 14-day retention"
```

---

### Task 7: Backup + retention scheduler (TDD)

`scheduler.go` exposes a `Start(ctx, db, home, runRetention)` that runs the backup + retention thinning **immediately** and then schedules the next call at local midnight. Each subsequent call schedules the day after. The scheduler exits cleanly when `ctx` is cancelled. `runRetention` is a callback so the snapshot retention pass (Task 8) can ride the same timer.

**Files:**
- Create: `engine/internal/backup/scheduler.go`
- Modify: `engine/internal/backup/backup_test.go`

- [ ] **Step 1: Failing test** (append)

```go
func TestStart_runsImmediatelyAndCallsRetention(t *testing.T) {
	s, home := openSeededStore(t)
	calls := make(chan struct{}, 4)
	retentionRan := make(chan struct{}, 4)
	stop := Start(context.Background(), s.DB(), home, func(ctx context.Context) error {
		retentionRan <- struct{}{}
		return nil
	}, func() time.Time { return time.Date(2026, 5, 26, 23, 59, 59, 0, time.Local) },
		func(time.Duration) {}, // sleep stub (no-op for the test)
		func() { calls <- struct{}{} })
	defer stop()

	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("immediate run did not invoke onTick")
	}
	select {
	case <-retentionRan:
	case <-time.After(2 * time.Second):
		t.Fatal("retention callback not invoked")
	}
	// Confirm backup file actually landed.
	root := filepath.Join(home, "backups", "2026-05-26")
	matches, _ := filepath.Glob(filepath.Join(root, "library-*.db"))
	if len(matches) != 1 {
		t.Errorf("expected 1 backup file, got %d", len(matches))
	}
}
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/backup/...
```

- [ ] **Step 3: Implement**

`engine/internal/backup/scheduler.go`:

```go
package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"
)

// RetentionFn is called after each successful backup run. Use it to also thin
// out node_snapshots, etc.
type RetentionFn func(ctx context.Context) error

// Start runs the daily backup loop. It performs one run immediately, then
// arranges the next run for local midnight + 1 minute (to avoid clock drift
// at exactly 00:00:00).
//
// The returned stop func cancels the loop and waits for the current iteration
// to finish.
//
// nowFn / sleepFn / onTick are injection points used by tests; production wires
// them to time.Now, time.Sleep, and a no-op.
func Start(
	ctx context.Context,
	db *sql.DB,
	home string,
	retention RetentionFn,
	nowFn func() time.Time,
	sleepFn func(time.Duration),
	onTick func(),
) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			now := nowFn()
			if _, _, err := RunDailyIfNeeded(ctx, db, home, now); err != nil {
				fmt.Fprintf(os.Stderr, "backup: %v\n", err)
			}
			if err := Prune(home, now); err != nil {
				fmt.Fprintf(os.Stderr, "backup prune: %v\n", err)
			}
			if retention != nil {
				if err := retention(ctx); err != nil {
					fmt.Fprintf(os.Stderr, "snapshot retention: %v\n", err)
				}
			}
			if onTick != nil {
				onTick()
			}
			next := nextLocalMidnight(now)
			wait := next.Sub(now) + time.Minute
			select {
			case <-ctx.Done():
				return
			default:
			}
			sleepFn(wait)
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func nextLocalMidnight(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
}
```

- [ ] **Step 4: Run — green**

```bash
cd engine && go test ./internal/backup/... -race
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/backup/scheduler.go engine/internal/backup/backup_test.go
git commit -m "feat(backup): scheduler runs immediate + nightly with retention callback"
```

---

## Phase D: Snapshot list + restore + retention

### Task 8: Snapshot retention thinning (TDD)

Single SQL pass that runs at boot + midnight. Rules (from spec §7.4):
- `manual` / `ai-replace` — keep forever
- `autosave` < 24h ago — keep all
- `autosave` 24h–30d — keep one per hour (delete duplicates within the same hour bucket)
- `autosave` > 30d — keep one per day (delete duplicates within the same day bucket)

Implementation: delete autosaves where `id NOT IN (kept ids)`. The "kept" set is built per bucket with a CTE selecting the most-recent autosave per `(node_id, bucket)`.

**Files:**
- Create: `engine/internal/snapshot/retention.go`
- Create: `engine/internal/snapshot/retention_test.go`

- [ ] **Step 1: Failing test**

`engine/internal/snapshot/retention_test.go`:

```go
package snapshot

import (
	"context"
	"testing"
	"time"
)

func TestThin_keepsLast24hUntouched(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	// Anchor "now"
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC).UnixMilli()
	for i := 0; i < 10; i++ {
		_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, now-int64(i*5*60*1000)) // every 5 min
	}
	if err := Thin(ctx, r.s.DB(), now); err != nil {
		t.Fatalf("Thin: %v", err)
	}
	got, err := countAutosaves(ctx, r, nodeID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 10 {
		t.Errorf("within-24h autosaves: kept %d, want 10", got)
	}
}

func TestThin_thinsToOnePerHourBetween24hAnd30d(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC).UnixMilli()
	// Two autosaves in the same hour, 25h ago.
	old := now - int64(25*time.Hour/time.Millisecond)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, old)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, old+60*1000) // 1 min later, same hour
	if err := Thin(ctx, r.s.DB(), now); err != nil {
		t.Fatalf("Thin: %v", err)
	}
	got, _ := countAutosaves(ctx, r, nodeID)
	if got != 1 {
		t.Errorf("kept %d in same hour bucket, want 1", got)
	}
}

func TestThin_thinsToOnePerDayBeyond30d(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC).UnixMilli()
	old := now - int64(31*24*time.Hour/time.Millisecond)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, old)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, old+3*60*60*1000) // 3h later, same day
	if err := Thin(ctx, r.s.DB(), now); err != nil {
		t.Fatalf("Thin: %v", err)
	}
	got, _ := countAutosaves(ctx, r, nodeID)
	if got != 1 {
		t.Errorf("kept %d in same day bucket beyond 30d, want 1", got)
	}
}

func TestThin_preservesManualAndAIReplace(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC).UnixMilli()
	old := now - int64(60*24*time.Hour/time.Millisecond)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonManual, old)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAIReplace, old+1000)
	if err := Thin(ctx, r.s.DB(), now); err != nil {
		t.Fatalf("Thin: %v", err)
	}
	var n int
	if err := r.s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM node_snapshots WHERE node_id = ?`, nodeID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("manual+ai-replace kept count = %d, want 2", n)
	}
}

func countAutosaves(ctx context.Context, r *Repo, nodeID string) (int, error) {
	var n int
	err := r.s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM node_snapshots WHERE node_id = ? AND reason = 'autosave'`, nodeID).Scan(&n)
	return n, err
}
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/snapshot/...
```

- [ ] **Step 3: Implement**

`engine/internal/snapshot/retention.go`:

```go
package snapshot

import (
	"context"
	"database/sql"
)

// Thin enforces autosave retention. Manual and ai-replace snapshots are never
// touched. Autosaves:
//   - < 24h ago: keep all
//   - 24h–30d: one per (node_id, hour bucket)
//   - > 30d:   one per (node_id, day bucket)
//
// Implementation strategy: compute "keep" ids in two CTE branches and delete
// every autosave row not in that set.
func Thin(ctx context.Context, db *sql.DB, nowMillis int64) error {
	const dayMs = int64(24 * 60 * 60 * 1000)
	cutoff24h := nowMillis - dayMs
	cutoff30d := nowMillis - 30*dayMs

	// Phase 1: bucket = 24h..30d, hour-grouped, keep most recent per (node, hour).
	if _, err := db.ExecContext(ctx, `
DELETE FROM node_snapshots
 WHERE reason = 'autosave'
   AND created_at <= ?
   AND created_at >  ?
   AND id NOT IN (
     SELECT id FROM (
       SELECT id,
              ROW_NUMBER() OVER (
                PARTITION BY node_id, created_at / (60*60*1000)
                ORDER BY created_at DESC
              ) AS rn
         FROM node_snapshots
        WHERE reason = 'autosave'
          AND created_at <= ?
          AND created_at >  ?
     )
     WHERE rn = 1
   )`, cutoff24h, cutoff30d, cutoff24h, cutoff30d); err != nil {
		return err
	}

	// Phase 2: bucket = > 30d, day-grouped.
	if _, err := db.ExecContext(ctx, `
DELETE FROM node_snapshots
 WHERE reason = 'autosave'
   AND created_at <= ?
   AND id NOT IN (
     SELECT id FROM (
       SELECT id,
              ROW_NUMBER() OVER (
                PARTITION BY node_id, created_at / (24*60*60*1000)
                ORDER BY created_at DESC
              ) AS rn
         FROM node_snapshots
        WHERE reason = 'autosave'
          AND created_at <= ?
     )
     WHERE rn = 1
   )`, cutoff30d, cutoff30d); err != nil {
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run — green**

```bash
cd engine && go test ./internal/snapshot/... -race
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/snapshot/retention.go engine/internal/snapshot/retention_test.go
git commit -m "feat(snapshot): retention thinning (1/hour 24h–30d, 1/day >30d)"
```

---

### Task 9: `snapshots.list_for_node` + `snapshots.restore` (TDD)

Two new RPC methods and one helper on `snapshot.Repo`.

`Repo.ListForNode(ctx, nodeID) ([]Entry, error)` returns entries ordered by `created_at DESC`. Each `Entry`:

```go
type Entry struct {
	ID          string `json:"id"`
	Reason      string `json:"reason"`
	CreatedAt   int64  `json:"created_at"`
	DocPreview  string `json:"doc_preview"`   // first 200 plaintext chars
}
```

`Repo.GetByID(ctx, id) (Snapshot, error)` — used by restore.

The restore RPC takes a `snapshot_id`, looks up the snapshot, snapshots the node's *current* content as `manual` first (so the restore is undoable), then calls `node.Repo.UpdateContent` with the snapshot's doc. Returns the updated `node.Node`.

**Files:**
- Modify: `engine/internal/snapshot/repo.go`
- Modify: `engine/internal/snapshot/repo_test.go`
- Modify: `engine/internal/rpc/handlers/snapshots.go`
- Modify: `engine/internal/rpc/handlers/snapshots_test.go`

- [ ] **Step 1: Failing tests** (append to `repo_test.go`)

```go
func TestListForNode_orderedDesc(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, nodeID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"오래된"}]}]}`, ReasonAutosave, 1000)
	_, _ = r.Create(ctx, nodeID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"중간"}]}]}`, ReasonManual, 2000)
	_, _ = r.Create(ctx, nodeID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"새 거"}]}]}`, ReasonAIReplace, 3000)

	got, err := r.ListForNode(context.Background(), nodeID)
	if err != nil {
		t.Fatalf("ListForNode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Reason != ReasonAIReplace || got[2].Reason != ReasonAutosave {
		t.Errorf("ordering wrong: %+v", got)
	}
	if got[0].DocPreview != "새 거\n" {
		t.Errorf("preview = %q", got[0].DocPreview)
	}
}

func TestGetByID(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	created, _ := r.Create(context.Background(), nodeID, `{"v":1}`, ReasonManual, 1000)
	got, err := r.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("id mismatch: %q vs %q", got.ID, created.ID)
	}
}
```

Append to `snapshots_test.go`:

```go
func TestListForNodeHandler(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	_, _ = f.snaps.Create(ctx, f.nID, `{"v":1}`, snapshot.ReasonManual, 1000)
	_, _ = f.snaps.Create(ctx, f.nID, `{"v":2}`, snapshot.ReasonAutosave, 2000)

	h := ListSnapshotsForNode(f.snaps)
	res, err := h(ctx, json.RawMessage(`{"node_id":"`+f.nID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var entries []snapshot.Entry
	if err := json.Unmarshal(res, &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries", len(entries))
	}
}

func TestRestoreSnapshotHandler_writesCurrentAsManualThenRestores(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	// Seed current content + an older snapshot.
	if err := f.nodes.UpdateContent(ctx, f.nID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"현재"}]}]}`, 5000); err != nil {
		t.Fatalf("seed update: %v", err)
	}
	older, _ := f.snaps.Create(ctx, f.nID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"옛 본문"}]}]}`,
		snapshot.ReasonManual, 1000)

	h := RestoreSnapshot(f.nodes, f.snaps, func() int64 { return 9999 })
	res, err := h(ctx, json.RawMessage(`{"snapshot_id":"`+older.ID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got node.Node
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ContentDoc == nil || !strings.Contains(*got.ContentDoc, "옛 본문") {
		t.Errorf("restore did not apply; doc=%v", got.ContentDoc)
	}
	// New manual snapshot capturing the pre-restore "현재" body must exist.
	entries, err := f.snaps.ListForNode(ctx, f.nID)
	if err != nil {
		t.Fatalf("ListForNode: %v", err)
	}
	var hasPreRestoreManual bool
	for _, e := range entries {
		if e.Reason == snapshot.ReasonManual && e.CreatedAt == 9999 {
			hasPreRestoreManual = true
		}
	}
	if !hasPreRestoreManual {
		t.Errorf("missing pre-restore manual snapshot at 9999; got %+v", entries)
	}
}
```

(Add `"strings"` and `"github.com/devlikebear/linetta/engine/internal/node"` imports to the test file.)

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/snapshot/... ./internal/rpc/handlers/...
```

- [ ] **Step 3: Implement repo additions**

Append to `engine/internal/snapshot/repo.go`:

```go
// Entry is a thin summary of a snapshot for the timeline UI. doc_preview is the
// first 200 plaintext characters (mention atoms rendered as @label, paragraph
// boundaries as \n).
type Entry struct {
	ID         string `json:"id"`
	Reason     string `json:"reason"`
	CreatedAt  int64  `json:"created_at"`
	DocPreview string `json:"doc_preview"`
}

// ListForNode returns every snapshot for the node ordered newest-first.
// doc_preview is computed in Go (cannot do it inline in SQL for Tiptap JSON).
func (r *Repo) ListForNode(ctx context.Context, nodeID string) ([]Entry, error) {
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT id, content_doc, reason, created_at
  FROM node_snapshots
 WHERE node_id = ?
 ORDER BY created_at DESC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var (
			id, doc, reason string
			createdAt       int64
		)
		if err := rows.Scan(&id, &doc, &reason, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, Entry{
			ID:         id,
			Reason:     reason,
			CreatedAt:  createdAt,
			DocPreview: trimRunes(plaintextFromDoc(doc), 200),
		})
	}
	return out, rows.Err()
}

// GetByID returns the full snapshot row.
func (r *Repo) GetByID(ctx context.Context, id string) (Snapshot, error) {
	row := r.s.DB().QueryRowContext(ctx, `
SELECT id, node_id, content_doc, reason, created_at
  FROM node_snapshots
 WHERE id = ?`, id)
	var s Snapshot
	if err := row.Scan(&s.ID, &s.NodeID, &s.ContentDoc, &s.Reason, &s.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{}, err
	}
	return s, nil
}
```

And in the same file add the helpers:

```go
import (
	"encoding/json"
	// ... existing imports
)

// plaintextFromDoc walks the Tiptap doc and concatenates text. Mentions render
// as @label; paragraph/heading/blockquote insert "\n". Same shape as
// ai.docToPlainText but inlined here to avoid an import cycle.
func plaintextFromDoc(raw string) string {
	if raw == "" {
		return ""
	}
	var any interface{}
	if err := json.Unmarshal([]byte(raw), &any); err != nil {
		return ""
	}
	var sb strings.Builder
	walkPlaintext(any, &sb)
	return sb.String()
}

func walkPlaintext(v interface{}, sb *strings.Builder) {
	switch t := v.(type) {
	case map[string]interface{}:
		kind, _ := t["type"].(string)
		if kind == "mention" {
			attrs, _ := t["attrs"].(map[string]interface{})
			if l, _ := attrs["label"].(string); l != "" {
				sb.WriteString("@")
				sb.WriteString(l)
			}
			return
		}
		if kind == "text" {
			if s, ok := t["text"].(string); ok {
				sb.WriteString(s)
			}
			return
		}
		if content, ok := t["content"].([]interface{}); ok {
			for _, c := range content {
				walkPlaintext(c, sb)
			}
		}
		if kind == "paragraph" || kind == "heading" || kind == "blockquote" {
			sb.WriteString("\n")
		}
	case []interface{}:
		for _, c := range t {
			walkPlaintext(c, sb)
		}
	}
}

func trimRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
```

(Add `"strings"` to imports.)

- [ ] **Step 4: Implement handlers**

Append to `engine/internal/rpc/handlers/snapshots.go`:

```go
import (
	// add:
	"github.com/devlikebear/linetta/engine/internal/node"
)

type listSnapshotsParams struct {
	NodeID string `json:"node_id"`
}

// ListSnapshotsForNode returns a handler for snapshots.list_for_node.
func ListSnapshotsForNode(snaps *snapshot.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listSnapshotsParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		entries, err := snaps.ListForNode(ctx, p.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if entries == nil {
			entries = []snapshot.Entry{}
		}
		return json.Marshal(entries)
	}
}

type restoreSnapshotParams struct {
	SnapshotID string `json:"snapshot_id"`
}

// RestoreSnapshot snapshots the node's current body as `manual` (so the restore
// itself is undoable), then writes the snapshot's doc back into the node.
// Returns the updated node.
func RestoreSnapshot(nodes *node.Repo, snaps *snapshot.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p restoreSnapshotParams
		if err := json.Unmarshal(params, &p); err != nil || p.SnapshotID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "snapshot_id required"}
		}
		snap, err := snaps.GetByID(ctx, p.SnapshotID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		current, err := nodes.Get(ctx, snap.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		curDoc := ""
		if current.ContentDoc != nil {
			curDoc = *current.ContentDoc
		}
		if _, err := snaps.Create(ctx, snap.NodeID, curDoc, snapshot.ReasonManual, now()); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if err := nodes.UpdateContent(ctx, snap.NodeID, snap.ContentDoc, now()); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		updated, err := nodes.Get(ctx, snap.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(updated)
	}
}
```

- [ ] **Step 5: Run — green**

```bash
cd engine && go test ./internal/snapshot/... ./internal/rpc/handlers/... -race
```

- [ ] **Step 6: Commit**

```bash
git add engine/internal/snapshot/repo.go engine/internal/snapshot/repo_test.go engine/internal/rpc/handlers/snapshots.go engine/internal/rpc/handlers/snapshots_test.go
git commit -m "feat(snapshot): list_for_node + restore (pre-restore manual snapshot)"
```

---

## Phase E: Tauri plugins + Rust wiring + engine main.go

### Task 10: Add dialog/fs plugins + capability file + wire all new handlers in main.go

**Files:**
- Modify: `apps/desktop/src-tauri/Cargo.toml`
- Modify: `apps/desktop/src-tauri/src/lib.rs`
- Modify: `apps/desktop/src-tauri/tauri.conf.json`
- Create: `apps/desktop/src-tauri/capabilities/default.json`
- Modify: `apps/desktop/package.json`
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1: Add Rust dependencies**

In `apps/desktop/src-tauri/Cargo.toml`, under `[dependencies]`, add:

```toml
tauri-plugin-dialog = "2"
tauri-plugin-fs = "2"
```

- [ ] **Step 2: Register plugins in `lib.rs`**

In `apps/desktop/src-tauri/src/lib.rs`, change the builder chain to:

```rust
tauri::Builder::default()
    .plugin(tauri_plugin_shell::init())
    .plugin(tauri_plugin_dialog::init())
    .plugin(tauri_plugin_fs::init())
    .setup(|app| {
        // ... unchanged
```

- [ ] **Step 3: Create capabilities file**

`apps/desktop/src-tauri/capabilities/default.json`:

```json
{
  "$schema": "../gen/schemas/desktop-schema.json",
  "identifier": "default",
  "description": "Linetta default capability — core + dialog + fs allowlist",
  "windows": ["main"],
  "permissions": [
    "core:default",
    "shell:default",
    "dialog:default",
    "dialog:allow-save",
    "fs:default",
    "fs:allow-write-text-file"
  ]
}
```

- [ ] **Step 4: Reference capability from tauri.conf.json**

In `apps/desktop/src-tauri/tauri.conf.json`, under `app.security`, add:

```json
"security": {
  "csp": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ipc: http://ipc.localhost",
  "capabilities": ["default"]
}
```

- [ ] **Step 5: Add JS plugin packages**

```bash
cd apps/desktop && pnpm add @tauri-apps/plugin-dialog @tauri-apps/plugin-fs
```

(This auto-edits `package.json` and `pnpm-lock.yaml`.)

- [ ] **Step 6: Wire engine main.go**

Replace `engine/cmd/linetta-engine/main.go` to register settings, backup scheduler, retention, and the new handlers. The relevant new section:

```go
import (
	// ... existing
	"github.com/devlikebear/linetta/engine/internal/backup"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/settings"
	// note: ai already imported
)

// ... inside main() after `st, err := store.Open(...)`:

	sst, err := settings.New()
	if err != nil {
		fail("settings: %v", err)
	}

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	snaps := snapshot.NewRepo(st)
	entities := entity.NewRepo(st)
	mentions := mention.NewRepo(st)

	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mentions.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	aiRuns := store.NewAIRunsRepo(st)
	contextBuilder := ai.NewContextBuilder(projects, nodes, mentions)
	runner := ai.NewRunner(s.Notifier(), aiRuns, ai.DefaultClientFactory, sst)

	// Backup + retention scheduler. Runs once at boot, then daily at midnight+1m.
	home, err := paths.Home()
	if err != nil {
		fail("home: %v", err)
	}
	retentionFn := func(ctx context.Context) error {
		return snapshot.Thin(ctx, st.DB(), time.Now().UnixMilli())
	}
	stopBackup := backup.Start(ctx, st.DB(), home, retentionFn,
		time.Now, time.Sleep, nil /* onTick */)
	defer stopBackup()

	clock := func() int64 { return time.Now().UnixMilli() }
	s.Handle("ping", handlers.Ping)
	// ... existing handler registrations unchanged ...
	s.Handle("settings.get", handlers.GetSettings(sst))
	s.Handle("settings.set", handlers.SetSettings(sst))
	s.Handle("snapshots.list_for_node", handlers.ListSnapshotsForNode(snaps))
	s.Handle("snapshots.restore", handlers.RestoreSnapshot(nodes, snaps, clock))
	s.Handle("export.project", handlers.ExportProject(projects, nodes, entities))
	s.Handle("export.node", handlers.ExportNode(nodes))
	_ = export.Payload{} // ensure import retained even if linters complain
```

- [ ] **Step 7: Run all backends**

```bash
cd engine && go test ./... -race
cd ../apps/desktop && pnpm tsc -b
cd src-tauri && cargo check
```

- [ ] **Step 8: Commit**

```bash
git add apps/desktop/src-tauri/Cargo.toml apps/desktop/src-tauri/Cargo.lock apps/desktop/src-tauri/src/lib.rs apps/desktop/src-tauri/tauri.conf.json apps/desktop/src-tauri/capabilities/default.json apps/desktop/package.json apps/desktop/pnpm-lock.yaml engine/cmd/linetta-engine/main.go
git commit -m "feat(shell): register dialog/fs plugins; wire settings/backup/export/restore in engine main"
```

---

## Phase F: Settings page UI

### Task 11: Real Settings page

**Files:**
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/lib/rpc.ts`
- Rewrite: `apps/desktop/src/routes/Settings.tsx`
- Create: `apps/desktop/src/routes/Settings.test.tsx`

- [ ] **Step 1: Extend types + rpc**

In `apps/desktop/src/lib/types.ts`, append:

```ts
export type ProviderID = "claude-code-cli" | "openai-codex";

export interface Settings {
  provider: ProviderID;
  typewriter_default: boolean;
  backup_dir: string;
}

export interface SettingsPatch {
  provider?: ProviderID;
  typewriter_default?: boolean;
}

export interface SnapshotEntry {
  id: string;
  reason: "manual" | "autosave" | "ai-replace";
  created_at: number;
  doc_preview: string;
}

export interface ExportPayload {
  markdown: string;
  suggested_filename: string;
}
```

In `apps/desktop/src/lib/rpc.ts`, append:

```ts
import type {
  // ... existing
  Settings,
  SettingsPatch,
  SnapshotEntry,
  ExportPayload,
} from "./types";

export const settings = {
  get: () => rpcCall<Settings>("settings.get"),
  set: (patch: SettingsPatch) => rpcCall<Settings>("settings.set", patch),
};

// Extend the existing `snapshots` object:
export const snapshots = {
  createManual: (nodeId: string, doc: string) =>
    rpcCall<Snapshot>("snapshots.create_manual", { node_id: nodeId, doc }),
  listForNode: (nodeId: string) =>
    rpcCall<SnapshotEntry[]>("snapshots.list_for_node", { node_id: nodeId }),
  restore: (snapshotId: string) =>
    rpcCall<NodeRow>("snapshots.restore", { snapshot_id: snapshotId }),
};

export const exportApi = {
  project: (projectId: string) =>
    rpcCall<ExportPayload>("export.project", { project_id: projectId }),
  node: (nodeId: string) =>
    rpcCall<ExportPayload>("export.node", { node_id: nodeId }),
};
```

- [ ] **Step 2: Rewrite Settings.tsx**

`apps/desktop/src/routes/Settings.tsx`:

```tsx
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { settings as settingsApi } from "../lib/rpc";
import type { ProviderID, Settings as SettingsRow } from "../lib/types";

export function Settings() {
  const [current, setCurrent] = useState<SettingsRow | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    settingsApi.get()
      .then((s) => { if (!cancelled) setCurrent(s); })
      .catch((e) => { if (!cancelled) setError(String(e)); });
    return () => { cancelled = true; };
  }, []);

  const apply = async (patch: Partial<SettingsRow>) => {
    if (!current) return;
    setSaving(true);
    setError(null);
    try {
      const next = await settingsApi.set(patch);
      setCurrent(next);
      setSavedAt(Date.now());
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <main className="shell">
      <p><Link to="/">← Library</Link></p>
      <h2>설정</h2>
      {error && <p className="error">{error}</p>}
      {!current ? (
        <p className="hint">불러오는 중…</p>
      ) : (
        <div className="settings-form">
          <section className="settings-section">
            <h3>AI 제공자</h3>
            <p className="hint">변경은 다음 AI 호출부터 적용됩니다.</p>
            <label className="radio-row">
              <input
                type="radio"
                name="provider"
                value="claude-code-cli"
                checked={current.provider === "claude-code-cli"}
                onChange={() => apply({ provider: "claude-code-cli" as ProviderID })}
                disabled={saving}
              />
              <span>Claude Code CLI</span>
            </label>
            <label className="radio-row">
              <input
                type="radio"
                name="provider"
                value="openai-codex"
                checked={current.provider === "openai-codex"}
                onChange={() => apply({ provider: "openai-codex" as ProviderID })}
                disabled={saving}
              />
              <span>OpenAI Codex CLI</span>
            </label>
          </section>

          <section className="settings-section">
            <h3>에디터</h3>
            <label className="check-row">
              <input
                type="checkbox"
                checked={current.typewriter_default}
                onChange={(e) => apply({ typewriter_default: e.target.checked })}
                disabled={saving}
              />
              <span>새 씬을 열 때 타이프라이터 스크롤 켜기</span>
            </label>
          </section>

          <section className="settings-section">
            <h3>백업</h3>
            <p className="hint">하루 한 번 자동 백업이 다음 경로에 저장됩니다 (14일 보관).</p>
            <p className="backup-path"><code>{current.backup_dir}</code></p>
          </section>

          <section className="settings-section">
            <h3>엔진 로그</h3>
            <p className="hint">(post-MVP — 다음 단계에서 추가됨)</p>
          </section>

          {savedAt && <p className="settings-saved">저장됨</p>}
        </div>
      )}
    </main>
  );
}
```

Append the styles in `apps/desktop/src/App.css`:

```css
.settings-form { max-width: 560px; margin: 1.5rem 0; }
.settings-section { margin-bottom: 2rem; }
.settings-section h3 { margin: 0 0 0.5rem; font-weight: 500; }
.settings-section .hint { font-size: 0.85rem; color: #999; margin: 0 0 0.5rem; }
.radio-row, .check-row { display: flex; align-items: center; gap: 0.5rem; padding: 0.25rem 0; }
.backup-path { font-size: 0.85rem; word-break: break-all; }
.settings-saved { font-size: 0.85rem; color: #5d8; }
```

- [ ] **Step 3: Smoke test**

`apps/desktop/src/routes/Settings.test.tsx` (vitest is not yet set up — if `pnpm test` fails because no vitest config exists, treat this test as optional documentation and skip Step 3; Step 4 build still must pass):

```tsx
// Skip if vitest is unavailable in the repo; this is a smoke render only.
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, it, expect, vi } from "vitest";
import { Settings } from "./Settings";

vi.mock("../lib/rpc", () => ({
  settings: {
    get: vi.fn().mockResolvedValue({
      provider: "claude-code-cli",
      typewriter_default: false,
      backup_dir: "/tmp/linetta/backups",
    }),
    set: vi.fn(),
  },
}));

describe("Settings page", () => {
  it("renders provider radios + backup path", async () => {
    render(<MemoryRouter><Settings /></MemoryRouter>);
    expect(await screen.findByText(/Claude Code CLI/)).toBeInTheDocument();
    expect(screen.getByText(/OpenAI Codex CLI/)).toBeInTheDocument();
    expect(screen.getByText(/\/tmp\/linetta\/backups/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 4: Build**

```bash
cd apps/desktop && pnpm tsc -b && pnpm build
```

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts apps/desktop/src/routes/Settings.tsx apps/desktop/src/routes/Settings.test.tsx apps/desktop/src/App.css
git commit -m "feat(settings): real Settings page with provider/typewriter/backup-path"
```

---

## Phase G: Cmd+K extensions + VersionSheet + Export integration

### Task 12: VersionSheet component + export save helper

The `VersionSheet` is a right slide-in like `EntitySheet`. It shows a timeline of `SnapshotEntry` rows (manual + ai-replace on top, autosaves below grouped by date), each rendered as a button. Selecting an entry shows a plaintext preview pane and a "이 버전으로 복원" button.

The export save helper opens the OS save dialog (via `@tauri-apps/plugin-dialog`), then writes the markdown via `@tauri-apps/plugin-fs`.

**Files:**
- Create: `apps/desktop/src/lib/exportSave.ts`
- Create: `apps/desktop/src/components/VersionSheet.tsx`
- Create: `apps/desktop/src/components/VersionSheet.css`

- [ ] **Step 1: Export save helper**

`apps/desktop/src/lib/exportSave.ts`:

```ts
import { save } from "@tauri-apps/plugin-dialog";
import { writeTextFile } from "@tauri-apps/plugin-fs";
import type { ExportPayload } from "./types";

/** Open the OS save dialog seeded with the suggested filename, then write the
 *  markdown to disk. Returns the chosen path, or null if the user cancelled. */
export async function saveExportedMarkdown(payload: ExportPayload): Promise<string | null> {
  const path = await save({
    defaultPath: payload.suggested_filename,
    filters: [{ name: "Markdown", extensions: ["md"] }],
  });
  if (!path) return null;
  await writeTextFile(path, payload.markdown);
  return path;
}
```

- [ ] **Step 2: VersionSheet component**

`apps/desktop/src/components/VersionSheet.tsx`:

```tsx
import { useEffect, useState } from "react";
import { snapshots } from "../lib/rpc";
import type { NodeRow, SnapshotEntry } from "../lib/types";
import "./VersionSheet.css";

interface Props {
  nodeId: string | null;
  onClose: () => void;
  onRestored: (node: NodeRow) => void;
}

const REASON_LABEL: Record<SnapshotEntry["reason"], string> = {
  manual: "수동 저장",
  "ai-replace": "AI 교체 전",
  autosave: "자동 저장",
};

function formatTime(ts: number): string {
  const d = new Date(ts);
  const yyyy = d.getFullYear();
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  const hh = String(d.getHours()).padStart(2, "0");
  const mi = String(d.getMinutes()).padStart(2, "0");
  return `${yyyy}-${mm}-${dd} ${hh}:${mi}`;
}

export function VersionSheet({ nodeId, onClose, onRestored }: Props) {
  const [entries, setEntries] = useState<SnapshotEntry[] | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [restoring, setRestoring] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!nodeId) return;
    setEntries(null);
    setError(null);
    snapshots.listForNode(nodeId)
      .then((list) => {
        setEntries(list);
        setSelectedId(list[0]?.id ?? null);
      })
      .catch((e) => setError(String(e)));
  }, [nodeId]);

  if (!nodeId) return null;
  const selected = entries?.find((e) => e.id === selectedId) ?? null;

  const onRestore = async () => {
    if (!selected) return;
    setRestoring(true);
    setError(null);
    try {
      const node = await snapshots.restore(selected.id);
      onRestored(node);
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setRestoring(false);
    }
  };

  // Group: "주요" (manual + ai-replace) on top, then autosaves grouped by YYYY-MM-DD.
  const major: SnapshotEntry[] = [];
  const auto: SnapshotEntry[] = [];
  (entries ?? []).forEach((e) => {
    if (e.reason === "autosave") auto.push(e);
    else major.push(e);
  });
  const autoByDay: { day: string; rows: SnapshotEntry[] }[] = [];
  for (const e of auto) {
    const day = new Date(e.created_at).toISOString().slice(0, 10);
    const last = autoByDay[autoByDay.length - 1];
    if (last && last.day === day) last.rows.push(e);
    else autoByDay.push({ day, rows: [e] });
  }

  return (
    <aside className="version-sheet" onMouseDown={(e) => e.stopPropagation()}>
      <header className="version-head">
        <span>이전 버전</span>
        <button type="button" className="version-close" onClick={onClose} aria-label="닫기">×</button>
      </header>
      {error && <p className="version-error">{error}</p>}
      {!entries && !error && <p className="version-loading">불러오는 중…</p>}
      {entries && entries.length === 0 && <p className="version-empty">아직 저장된 버전이 없습니다.</p>}
      {entries && entries.length > 0 && (
        <div className="version-body">
          <div className="version-timeline">
            {major.length > 0 && (
              <div className="version-group">
                <p className="version-group-head">주요 저장</p>
                {major.map((e) => (
                  <button
                    key={e.id}
                    type="button"
                    className={`version-row${e.id === selectedId ? " sel" : ""}`}
                    onClick={() => setSelectedId(e.id)}
                  >
                    <span className="version-reason">{REASON_LABEL[e.reason]}</span>
                    <span className="version-time">{formatTime(e.created_at)}</span>
                  </button>
                ))}
              </div>
            )}
            {autoByDay.map((g) => (
              <div className="version-group" key={g.day}>
                <p className="version-group-head">자동 저장 · {g.day}</p>
                {g.rows.map((e) => (
                  <button
                    key={e.id}
                    type="button"
                    className={`version-row${e.id === selectedId ? " sel" : ""}`}
                    onClick={() => setSelectedId(e.id)}
                  >
                    <span className="version-time">{formatTime(e.created_at)}</span>
                  </button>
                ))}
              </div>
            ))}
          </div>
          <div className="version-preview">
            <h5>미리보기</h5>
            <pre>{selected?.doc_preview || "(빈 본문)"}</pre>
            <div className="version-actions">
              <button type="button" onClick={onClose} disabled={restoring}>취소</button>
              <button
                type="button"
                className="primary"
                onClick={onRestore}
                disabled={restoring || !selected}
              >
                {restoring ? "복원 중…" : "이 버전으로 복원"}
              </button>
            </div>
          </div>
        </div>
      )}
    </aside>
  );
}
```

`apps/desktop/src/components/VersionSheet.css`:

```css
.version-sheet {
  position: fixed; top: 0; right: 0; bottom: 0;
  width: 360px; max-width: 90vw;
  background: #111; color: #eee;
  border-left: 1px solid #2a2a2a;
  box-shadow: -4px 0 24px rgba(0,0,0,0.4);
  z-index: 80;
  display: flex; flex-direction: column;
  font-size: 14px;
}
.version-head { display: flex; align-items: center; justify-content: space-between; padding: 0.75rem 1rem; border-bottom: 1px solid #2a2a2a; font-weight: 500; }
.version-close { background: none; border: none; color: #ccc; font-size: 1.4rem; cursor: pointer; }
.version-body { display: flex; flex: 1; min-height: 0; }
.version-timeline { width: 180px; padding: 0.5rem; overflow-y: auto; border-right: 1px solid #2a2a2a; }
.version-group-head { font-size: 0.75rem; color: #888; margin: 0.6rem 0 0.25rem; text-transform: uppercase; letter-spacing: 0.05em; }
.version-row { display: flex; flex-direction: column; align-items: flex-start; width: 100%; padding: 0.4rem 0.5rem; background: none; border: none; color: inherit; text-align: left; cursor: pointer; border-radius: 4px; }
.version-row:hover { background: #1a1a1a; }
.version-row.sel { background: #233; }
.version-reason { font-size: 0.85rem; color: #cdd; }
.version-time { font-size: 0.78rem; color: #888; }
.version-preview { flex: 1; padding: 0.75rem 1rem; display: flex; flex-direction: column; min-width: 0; }
.version-preview h5 { margin: 0 0 0.5rem; font-weight: 500; color: #aaa; font-size: 0.85rem; }
.version-preview pre { flex: 1; white-space: pre-wrap; font-family: serif; font-size: 0.95rem; color: #ddd; background: #0c0c0c; padding: 0.6rem; border-radius: 6px; overflow-y: auto; }
.version-actions { display: flex; justify-content: flex-end; gap: 0.5rem; margin-top: 0.75rem; }
.version-actions .primary { background: #345; color: #fff; border: none; border-radius: 4px; padding: 0.4rem 0.9rem; cursor: pointer; }
.version-actions button:not(.primary) { background: none; border: 1px solid #444; color: #ccc; border-radius: 4px; padding: 0.4rem 0.9rem; cursor: pointer; }
.version-empty, .version-loading, .version-error { padding: 1rem; color: #999; font-size: 0.9rem; }
.version-error { color: #f88; }
```

- [ ] **Step 3: Build**

```bash
cd apps/desktop && pnpm tsc -b
```

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src/lib/exportSave.ts apps/desktop/src/components/VersionSheet.tsx apps/desktop/src/components/VersionSheet.css
git commit -m "feat(workspace): VersionSheet + saveExportedMarkdown helper"
```

---

### Task 13: Wire Cmd+K extensions in Workspace.tsx (export, version, ZEN entry)

Add four new palette entries and a `versionSheetNodeId` state. Also fold typewriter default from settings into the initial state.

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx`

- [ ] **Step 1: Wire state + commands**

At the top of `Workspace.tsx`, add an import:

```ts
import { settings as settingsApi, exportApi } from "../lib/rpc";
import { saveExportedMarkdown } from "../lib/exportSave";
import { VersionSheet } from "../components/VersionSheet";
```

Add a new state hook and initial-load effect after the existing `useState` block:

```ts
const [versionSheetNodeId, setVersionSheetNodeId] = useState<string | null>(null);
const [zenOpen, setZenOpen] = useState(false);

// Apply typewriter default from settings exactly once on mount.
useEffect(() => {
  let cancelled = false;
  settingsApi.get()
    .then((s) => { if (!cancelled) setTypewriter(s.typewriter_default); })
    .catch(() => { /* benign */ });
  return () => { cancelled = true; };
}, []);
```

In the `commands` `useMemo`, append (before `return cmds;`):

```ts
cmds.push({
  id: "version-restore",
  section: "프로젝트",
  label: "이 씬의 이전 버전",
  hint: "복원",
  run: () => setVersionSheetNodeId(load.node.id),
});
cmds.push({
  id: "export-project",
  section: "내보내기",
  label: "프로젝트 (.md)",
  run: async () => {
    try {
      const payload = await exportApi.project(load.project.id);
      const path = await saveExportedMarkdown(payload);
      if (path) showToast("내보내기 완료");
    } catch (e) {
      showToast("내보내기 실패: " + String(e));
    }
  },
});
cmds.push({
  id: "export-node",
  section: "내보내기",
  label: "이 씬 (.md)",
  run: async () => {
    try {
      const payload = await exportApi.node(load.node.id);
      const path = await saveExportedMarkdown(payload);
      if (path) showToast("내보내기 완료");
    } catch (e) {
      showToast("내보내기 실패: " + String(e));
    }
  },
});
cmds.push({
  id: "go-settings",
  section: "프로젝트",
  label: "설정 열기",
  run: () => navigate("/settings"),
});
```

Inside the JSX (near the existing `<EntitySheet ...>` block), add:

```tsx
{versionSheetNodeId && (
  <VersionSheet
    nodeId={versionSheetNodeId}
    onClose={() => {
      setVersionSheetNodeId(null);
      focusEditor();
    }}
    onRestored={(updatedNode) => {
      const docStr = updatedNode.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`;
      setLoad((prev) => prev ? { ...prev, node: updatedNode, initialDoc: JSON.parse(docStr) } : prev);
      setCharCount(updatedNode.word_count);
      showToast("이전 버전으로 복원되었습니다");
    }}
  />
)}
```

- [ ] **Step 2: Build**

```bash
cd apps/desktop && pnpm tsc -b
```

- [ ] **Step 3: Commit**

```bash
git add apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(workspace): Cmd+K — version restore + export + settings; typewriter default from settings"
```

---

## Phase H: ZEN mode

### Task 14: ZenMode component + Workspace integration

ZEN reuses the existing Tiptap editor instance: the Workspace owns the editor; ZEN renders an overlay above everything that styles the editor differently and exposes the cursor-preserving exit. The simplest reliable approach: when ZEN opens, mount the existing TiptapEditor inside the overlay (by toggling a CSS class on a wrapping div, rendered conditionally). To avoid losing focus during the re-parent, we re-mount the Workspace's `<TiptapEditor>` only when ZEN actually toggles.

Concretely: We render two outer layouts conditionally — Edit/AI layout, or ZEN overlay — passing the same `key={load.node.id}` and `initialDoc`. **Exact cursor preservation is required (spec §4.5):** before swapping layouts, capture the current ProseMirror selection `{ from, to }` from the editor; after the new editor mounts, call a new `setSelection({from, to})` method on the `TiptapHandle` ref. This works in both directions (Edit→ZEN and ZEN→Edit) because the underlying `content_doc` is the same — the swap only changes the wrapping layout, and the doc-relative offsets remain valid. If the captured range is out of bounds (e.g., doc shrunk because of an AI replace), fall back to end-of-doc.

`TiptapEditor` must therefore expose two new imperative methods on its handle:

- `getSelection(): { from: number; to: number } | null` — returns the current ProseMirror selection or null if no editor view exists.
- `setSelection(sel: { from: number; to: number }): void` — clamps `from`/`to` to doc size and dispatches a `setSelection` transaction; calls `view.focus()` afterwards.

The `Tiptap.tsx` component already uses `useImperativeHandle`; extend it. Update `apps/desktop/src/components/editor/Tiptap.tsx` and its types accordingly **as a sub-step of Task 14 before the ZenMode wiring** — see Step 0 below.

**Files:**
- Create: `apps/desktop/src/components/ZenMode.tsx`
- Create: `apps/desktop/src/components/ZenMode.css`
- Modify: `apps/desktop/src/routes/Workspace.tsx`

- [ ] **Step 0: Extend `TiptapEditor` handle with selection get/set**

In `apps/desktop/src/components/editor/Tiptap.tsx`, locate the existing `useImperativeHandle` block (it should already expose `focus` and possibly `getDoc`). Add two methods:

```ts
useImperativeHandle(ref, () => ({
  focus: () => editor?.view.focus(),
  getDoc: () => editor?.getJSON() ?? {},
  getSelection: () => {
    if (!editor) return null;
    const { from, to } = editor.state.selection;
    return { from, to };
  },
  setSelection: (sel: { from: number; to: number }) => {
    if (!editor) return;
    const size = editor.state.doc.content.size;
    const from = Math.min(Math.max(0, sel.from), size);
    const to = Math.min(Math.max(from, sel.to), size);
    editor.commands.setTextSelection({ from, to });
    editor.view.focus();
  },
}), [editor]);
```

Update the exported `TiptapHandle` interface:

```ts
export interface TiptapHandle {
  focus: () => void;
  getDoc: () => object;
  getSelection: () => { from: number; to: number } | null;
  setSelection: (sel: { from: number; to: number }) => void;
}
```

Run `pnpm tsc -b` to confirm no type errors. Commit:

```bash
git add apps/desktop/src/components/editor/Tiptap.tsx
git commit -m "feat(editor): expose getSelection/setSelection on TiptapHandle"
```

- [ ] **Step 1: Implement ZenMode**

`apps/desktop/src/components/ZenMode.tsx`:

```tsx
import { useEffect, useRef, useState } from "react";
import "./ZenMode.css";
import { TiptapEditor, type TiptapHandle } from "./editor/Tiptap";

interface Props {
  initialDoc: object;
  charCount: number;
  sceneLabel: string;
  /** Target word count for the progress bar; 0 disables the bar. */
  target?: number;
  onChange: (doc: object) => void;
  onCharCount: (n: number) => void;
  onManualSave: (doc: object) => void;
  onExit: () => void;
}

export function ZenMode({
  initialDoc, charCount, sceneLabel, target = 0,
  onChange, onCharCount, onManualSave, onExit,
}: Props) {
  const [showBar, setShowBar] = useState(false);
  const editorRef = useRef<TiptapHandle>(null);
  const hideTimer = useRef<number | null>(null);

  // ESC and Cmd+Period exit.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const isMac = navigator.platform.toLowerCase().includes("mac");
      const mod = isMac ? e.metaKey : e.ctrlKey;
      if (e.key === "Escape") {
        e.preventDefault();
        onExit();
      } else if (mod && e.key === ".") {
        e.preventDefault();
        onExit();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onExit]);

  // Focus immediately on enter.
  useEffect(() => {
    window.setTimeout(() => editorRef.current?.focus(), 0);
  }, []);

  // Hover top 8px → flash progress bar for 2s.
  const onPointerMove = (e: React.PointerEvent) => {
    if (e.clientY > 8) return;
    setShowBar(true);
    if (hideTimer.current != null) window.clearTimeout(hideTimer.current);
    hideTimer.current = window.setTimeout(() => setShowBar(false), 2000);
  };

  const progressPercent = target > 0 ? Math.min(100, Math.round((charCount / target) * 100)) : 0;

  return (
    <div className="zen-root" onPointerMove={onPointerMove}>
      {showBar && target > 0 && (
        <div className="zen-progress">
          <div className="zen-progress-fill" style={{ width: `${progressPercent}%` }} />
        </div>
      )}
      <div className="zen-canvas">
        <TiptapEditor
          ref={editorRef}
          initialDoc={initialDoc}
          onChange={onChange}
          onCharCount={onCharCount}
          onManualSave={onManualSave}
        />
      </div>
      <div className="zen-meta">
        ESC로 나가기 · {charCount}자 · 씬 {sceneLabel}
      </div>
    </div>
  );
}
```

`apps/desktop/src/components/ZenMode.css`:

```css
.zen-root {
  position: fixed; inset: 0;
  background: #000;
  color: #f4f1ea;
  z-index: 100;
  display: flex; flex-direction: column;
  font-family: "New York", "Times New Roman", serif;
}
.zen-progress {
  position: absolute; top: 0; left: 50%; transform: translateX(-50%);
  width: 60%; height: 2px; background: rgba(255,255,255,0.08);
  transition: opacity 0.4s ease;
}
.zen-progress-fill { height: 100%; background: rgba(255,255,255,0.45); transition: width 0.4s ease; }
.zen-canvas { flex: 1; display: flex; justify-content: center; padding-top: 8vh; overflow-y: auto; }
.zen-canvas .tiptap-wrap { background: transparent; max-width: 65ch; width: 100%; }
.zen-canvas .ProseMirror { color: #f4f1ea; background: transparent; font-size: 1.15rem; line-height: 1.85; outline: none; }
.zen-canvas .ProseMirror::selection { background: rgba(255,255,255,0.18); }
.zen-meta {
  position: fixed; bottom: 1.5rem; left: 50%; transform: translateX(-50%);
  font-size: 0.8rem; color: rgba(244,241,234,0.4); letter-spacing: 0.04em;
}
```

- [ ] **Step 2: Wire ZEN entry in Workspace.tsx with selection preservation**

Add state for ZEN + a ref to the saved selection. The Workspace already holds an `editorRef` for the Edit-mode TiptapEditor; we reuse it:

```ts
const [zenOpen, setZenOpen] = useState(false);
const savedSelectionRef = useRef<{ from: number; to: number } | null>(null);

function enterZen() {
  savedSelectionRef.current = editorRef.current?.getSelection() ?? null;
  setZenOpen(true);
}

function exitZen() {
  // capture ZEN's selection too, so cursor lands where the writer left it
  // (could be mid-doc if they navigated inside ZEN).
  savedSelectionRef.current = zenEditorRef.current?.getSelection() ?? savedSelectionRef.current;
  setZenOpen(false);
}
```

Replace the existing `<span className="ws-zen">ZEN</span>` with a real button:

```tsx
<button
  type="button"
  className="mode-toggle ws-zen-btn"
  onClick={enterZen}
>
  ZEN
</button>
```

After ZEN exits, the Edit-mode `<TiptapEditor>` remounts. Restore the saved selection in an effect keyed on `zenOpen`:

```ts
useEffect(() => {
  if (!zenOpen && savedSelectionRef.current) {
    // wait one frame for the Edit-mode editor to mount
    const id = window.requestAnimationFrame(() => {
      const sel = savedSelectionRef.current;
      if (sel) editorRef.current?.setSelection(sel);
    });
    return () => window.cancelAnimationFrame(id);
  }
  return undefined;
}, [zenOpen]);
```

Likewise, when ZEN opens, restore the saved selection into the ZEN editor (so the cursor stays put across the swap):

In `ZenMode.tsx`, replace the existing focus-on-mount effect with one that accepts an `initialSelection` prop and applies it:

```tsx
useEffect(() => {
  window.requestAnimationFrame(() => {
    if (initialSelection) {
      editorRef.current?.setSelection(initialSelection);
    } else {
      editorRef.current?.focus();
    }
  });
}, []);
```

Add `initialSelection?: { from: number; to: number } | null;` to ZenMode's `Props`.

Pass it from Workspace:

```tsx
{zenOpen && (
  <ZenMode
    initialDoc={load.initialDoc}
    initialSelection={savedSelectionRef.current}
    charCount={charCount}
    sceneLabel={load.node.label}
    onChange={(doc) => { debouncedSave(doc); }}
    onCharCount={setCharCount}
    onManualSave={handleManualSave}
    onExit={exitZen}
  />
)}
```

Add a second ref `zenEditorRef` that you forward to the ZenMode (expose a `selectionRef` prop, or use `useImperativeHandle` on ZenMode — simplest: have ZenMode accept an `onMountEditor?: (handle: TiptapHandle) => void` callback and the Workspace stores it):

```tsx
const zenEditorRef = useRef<TiptapHandle | null>(null);

// In ZenMode JSX:
<TiptapEditor
  ref={(h) => {
    editorRef.current = h;            // inside ZenMode's own ref
    onMountEditor?.(h);               // forward to parent
  }}
  ...
/>
```

Update ZenMode props: `onMountEditor?: (handle: TiptapHandle | null) => void`.

Add import at the top:

```ts
import { ZenMode } from "../components/ZenMode";
```

Add a Cmd+K entry to enter ZEN (right after the export commands in Task 13):

```ts
cmds.push({
  id: "enter-zen",
  section: "보기",
  label: "ZEN 모드 열기",
  hint: "ESC로 종료",
  run: () => setZenOpen(true),
});
```

Append minor CSS to `App.css`:

```css
.ws-zen-btn { font-weight: 500; }
```

- [ ] **Step 3: Build**

```bash
cd apps/desktop && pnpm tsc -b && pnpm build
```

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src/components/ZenMode.tsx apps/desktop/src/components/ZenMode.css apps/desktop/src/routes/Workspace.tsx apps/desktop/src/App.css
git commit -m "feat(workspace): ZEN mode — pure-black serif overlay with ESC/Cmd+. exit"
```

---

## Phase I: E2E smoke + milestone tag

### Task 15: Full MVP end-to-end walk-through + tag

This is the closing smoke. We exercise every MVP DoD item in sequence using one fresh `$LINETTA_HOME`.

- [ ] **Step 1: Pre-warm**

```bash
./scripts/build-engine.sh
(cd apps/desktop/src-tauri && cargo build) >/dev/null 2>&1 || true
```

- [ ] **Step 2: Run dev**

```bash
rm -rf /tmp/linetta-mvp
LINETTA_HOME=/tmp/linetta-mvp ./scripts/dev.sh
```

- [ ] **Step 3: Full MVP walk-through (touches every DoD item 1–13)**

1. **Library + new-project modal (#2):** New-project modal auto-opens on empty library. Create `MVP 검증` with genre `문학` and length `short`. Workspace opens on the seeded `씬 1` (DoD #1, #2).
2. **Edit mode + typewriter + autosave + Cmd+S (#3):** Type ~3 paragraphs of prose. Confirm the right context panel shows `씬 상태 / 인물·장소`. Toggle typewriter via the panel; current line locks to center. Press `Cmd+S`; toast `스냅샷 저장됨` appears.
3. **@mention + Entity Sheet (#4, #5):** Type `@해진` → picker opens → "새 인물로 추가" → EntitySheet slides in. Fill `역할: POV`, `요약: 사진작가`, add attribute `나이: 32`. Save. Double-click the rendered `@해진` to re-open the sheet. Close.
4. **AI mode (#6):** Toggle `AI`. Click `재작성` chip → PROMPT fills. Click `생성`. Streaming should appear within ~2s. When done, click `커서에 삽입`. Returns to Edit; a new paragraph appears.
5. **Cmd+K (#8) + Outline (#9):** Open `Cmd+K`. Use "여기 옆에 새 장" to add `2장`, which seeds `씬 1` inside it. Hover the left edge → Outline slides in. Click `씬 2` (or the new node) to jump.
6. **ZEN (#7):** Click the top-right `ZEN` button. Screen turns black, serif, no chrome. Type a line. Hover the top edge — progress bar flashes briefly. Press `ESC` to exit; the line you typed remains in Edit mode.
7. **Version restore (#10):** Type a different line ("This will be undone."). Wait ~2s for autosave to settle. Open `Cmd+K → "이 씬의 이전 버전"`. VersionSheet appears with timeline. Select the entry from before step 6 (preview text matches). Click `이 버전으로 복원`. Toast `이전 버전으로 복원되었습니다` appears; the "This will be undone." line is gone. Verify the pre-restore content is itself now in the timeline as a manual snapshot (re-open the sheet).
8. **Export project (#11):** `Cmd+K → "내보내기 → 프로젝트 (.md)"`. Save dialog opens with `mvp-검증.md`. Save to `/tmp/linetta-mvp-export/`. Confirm:
   ```bash
   head -30 /tmp/linetta-mvp-export/mvp-검증.md
   ```
   File starts with `# MVP 검증`, contains `## 등장인물`, contains `해진`.
9. **Export node (#11):** `Cmd+K → "내보내기 → 이 씬 (.md)"`. Save. Confirm file has no `#` headings.
10. **Settings (#12):** `Cmd+K → "설정 열기"`. Page shows provider radios (Claude Code CLI selected), typewriter checkbox (checked from step 2), backup_dir = `/tmp/linetta-mvp/backups`. Toggle typewriter off. Return to workspace via `← Library` → reopen project. Typewriter should be off.
11. **Backup (#13):**
    ```bash
    ls /tmp/linetta-mvp/backups/
    ```
    There should be one `YYYY-MM-DD` dir containing one `library-HHMMSS.db` file.
12. **Provider change effect:** Go to Settings, switch provider to `OpenAI Codex CLI`. Return to Workspace, switch to AI mode, click `생성`. Check the latest `ai_runs` row:
    ```bash
    sqlite3 /tmp/linetta-mvp/library.db "SELECT provider FROM ai_runs ORDER BY started_at DESC LIMIT 1"
    ```
    Should report `openai-codex` (regardless of whether the call succeeded). Switch back to Claude Code CLI.

If any step fails, halt and report which step + the observed symptom + the relevant log lines (engine stderr is forwarded to the dev terminal).

- [ ] **Step 4: Final test suites**

```bash
cd engine && go test ./... -race
cd ../apps/desktop && pnpm tsc -b && pnpm build
cd src-tauri && cargo check
```

All three must be green.

- [ ] **Step 5: Tag**

```bash
git tag plan-5-ai-mode-done   # backfilled: the AI-mode smoke was rolled into Task 15
git tag plan-6-mvp-completion-done
git tag linetta-mvp
```

---

## Definition of Done

- `cd engine && go test ./... -race` green.
- `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- `cd apps/desktop/src-tauri && cargo check` green.
- Manual walk-through (Task 15) succeeds end-to-end with real `claude` CLI on PATH.
- Tags `plan-5-ai-mode-done`, `plan-6-mvp-completion-done`, and `linetta-mvp` all exist.
- DoD coverage:
  - #7 ZEN mode — Task 14
  - #10 Version restore — Tasks 8, 9, 12, 13
  - #11 Export markdown — Tasks 3, 4, 5, 12, 13
  - #12 Settings — Tasks 1, 2, 11
  - #13 Auto backup — Tasks 6, 7, 10

This is the final MVP plan; the next milestone (post-MVP P1) opens Thread + Beat surfaces — out of scope.
