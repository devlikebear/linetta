# Plan 1 — Schema, Projects, Library

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist Linetta's full schema in SQLite under the OS app-data dir, expose Project CRUD over JSONRPC, and surface a Library screen in React where the user can create a project, see recent cards, navigate to a Workspace placeholder, and browse an archive page.

**Architecture:** A new `engine/internal/store` opens a single SQLite database (WAL, foreign keys on) at `$LINETTA_HOME` (default macOS `~/Library/Application Support/com.devlikebear.linetta`). An embedded migration system applies the full Plan 1 schema (`0001_init.sql`) — every table from the design doc, not just the ones with UI today. A `engine/internal/project` package wraps Project domain logic and SQLite I/O. RPC handlers (`projects.create`, `projects.list`, `projects.get`, `projects.archive`) hang off the existing `rpc.Server`. On the React side, `react-router-dom` adds three routes: `/` (Library), `/workspace/:projectId` (placeholder), `/library/all` (archive).

**Tech Stack additions:**
- `modernc.org/sqlite` v1.50.x — pure-Go SQLite driver
- `github.com/google/uuid` — Project/Node/Entity ID generation
- `react-router-dom` v6.x — Library / Workspace / Archive routing
- A small in-house migration runner using `embed.FS`

**Spec reference:** `docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md` — §3 (Data Model), §4.1–§4.2 (Library and New Project), §6 (Library Behavior), §11.1 items 1–2.

---

## Pre-flight

- [ ] **Step P1: Plan 0 is done**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git describe --tags --exact-match plan-0-scaffold-done >/dev/null && echo ok
./scripts/dev.sh &
# Wait a few seconds, click Ping engine, see "engine says: pong", Ctrl-C.
```
If Plan 0 is not green, stop and fix it first.

- [ ] **Step P2: Confirm no uncommitted work**

`git status --short` — must be empty.

- [ ] **Step P3: Confirm tools**

```bash
go version          # 1.25+ ok
node --version      # 18+
pnpm --version      # 8+
cargo --version
```

---

## File Structure (created or modified by this plan)

```
linetta/
├── engine/
│   ├── go.mod                           (modified — adds modernc.org/sqlite, google/uuid)
│   ├── internal/
│   │   ├── paths/
│   │   │   ├── paths.go                 (new)
│   │   │   └── paths_test.go            (new)
│   │   ├── store/
│   │   │   ├── store.go                 (new)
│   │   │   ├── store_test.go            (new)
│   │   │   ├── migrations.go            (new)
│   │   │   ├── migrations_test.go       (new)
│   │   │   ├── embed.go                 (new — go:embed)
│   │   │   └── migrations/
│   │   │       └── 0001_init.sql        (new)
│   │   ├── project/
│   │   │   ├── project.go               (new — domain types)
│   │   │   ├── repo.go                  (new — SQLite repo)
│   │   │   └── repo_test.go             (new)
│   │   └── rpc/
│   │       └── handlers/
│   │           ├── projects.go          (new)
│   │           └── projects_test.go     (new)
│   └── cmd/linetta-engine/
│       └── main.go                      (modified — opens store, registers project handlers)
└── apps/desktop/
    ├── package.json                     (modified — adds react-router-dom)
    ├── src/
    │   ├── App.tsx                      (replaced — now a router shell)
    │   ├── App.css                      (extended for Library + Modal styles)
    │   ├── lib/
    │   │   ├── rpc.ts                   (extended — projects.*)
    │   │   └── types.ts                 (new — Project, NewProjectInput, etc.)
    │   ├── routes/
    │   │   ├── Library.tsx              (new)
    │   │   ├── LibraryAll.tsx           (new — archive view)
    │   │   ├── Workspace.tsx            (new — placeholder)
    │   │   └── Settings.tsx             (new — placeholder)
    │   └── components/
    │       ├── NewProjectModal.tsx      (new)
    │       └── ProjectCard.tsx          (new)
```

Engine schema lives entirely in `0001_init.sql`. All 11 tables (projects, nodes, entities, mentions, relationships, threads, beats, notes, node_snapshots, ai_runs, schema_migrations) are created in this one migration even though only `projects` is exercised by Plan 1 RPC. Future plans extend handlers, not the schema.

---

## Task 1: `paths` package — resolve DB location (TDD)

**Files:**
- Create: `engine/internal/paths/paths.go`
- Create: `engine/internal/paths/paths_test.go`

- [ ] **Step 1: Write the failing test**

Write `engine/internal/paths/paths_test.go`:
```go
package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHome_envOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LINETTA_HOME", tmp)

	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if got != tmp {
		t.Errorf("Home = %q, want %q", got, tmp)
	}
}

func TestHome_default_macos(t *testing.T) {
	t.Setenv("LINETTA_HOME", "")
	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	// We don't assert the exact value (it's user-dependent) — only that it
	// ends with the identifier and is under the user's home.
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(got, home) {
		t.Errorf("Home %q should be under user home %q", got, home)
	}
	if filepath.Base(got) != "com.devlikebear.linetta" {
		t.Errorf("Home base = %q, want com.devlikebear.linetta", filepath.Base(got))
	}
}

func TestDBPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LINETTA_HOME", tmp)
	got, err := DBPath()
	if err != nil {
		t.Fatalf("DBPath: %v", err)
	}
	want := filepath.Join(tmp, "library.db")
	if got != want {
		t.Errorf("DBPath = %q, want %q", got, want)
	}
}

func TestEnsureHome_createsDir(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "nested", "linetta")
	t.Setenv("LINETTA_HOME", tmp)

	if err := EnsureHome(); err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
	info, err := os.Stat(tmp)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", tmp)
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/paths/...
```
Expected: undefined Home, DBPath, EnsureHome.

- [ ] **Step 3: Implement**

Write `engine/internal/paths/paths.go`:
```go
// Package paths resolves where Linetta keeps its data on disk.
// All callers should go through this package — never hard-code paths.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// AppIdentifier is used as the directory name under the OS app-data location.
const AppIdentifier = "com.devlikebear.linetta"

// Home returns the directory under which Linetta stores its database, settings,
// and backups. Honors LINETTA_HOME if non-empty; otherwise uses the OS default.
func Home() (string, error) {
	if v := os.Getenv("LINETTA_HOME"); v != "" {
		return v, nil
	}
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", AppIdentifier), nil
	case "linux":
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, AppIdentifier), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", AppIdentifier), nil
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, AppIdentifier), nil
		}
		return "", fmt.Errorf("APPDATA unset on Windows")
	default:
		return "", fmt.Errorf("unsupported os: %s", runtime.GOOS)
	}
}

// DBPath returns the absolute path to library.db.
func DBPath() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "library.db"), nil
}

// EnsureHome creates the Home directory if it does not exist (mode 0700).
func EnsureHome() error {
	home, err := Home()
	if err != nil {
		return err
	}
	return os.MkdirAll(home, 0o700)
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/paths/...
```

- [ ] **Step 5: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/paths
git commit -m "feat(paths): resolve LINETTA_HOME / DB path / mkdir helper"
```

---

## Task 2: Embed migrations + apply (TDD)

**Files:**
- Create: `engine/internal/store/embed.go`
- Create: `engine/internal/store/migrations/0001_init.sql` (full schema — see Task 3)
- Create: `engine/internal/store/migrations.go`
- Create: `engine/internal/store/migrations_test.go`

Note: this task writes the migration framework AND `0001_init.sql` content in two steps. The SQL file is large so it gets its own task block below (Task 3). Task 2 produces the runner and a minimal "smoke" SQL file; Task 3 replaces that smoke SQL with the real schema.

- [ ] **Step 1: Add sqlite + uuid dependencies**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go get modernc.org/sqlite@v1.50.1
go get github.com/google/uuid@v1.6.0
```

- [ ] **Step 2: Write the failing migration test**

Create `engine/internal/store/migrations/0001_init.sql` for now with the smallest possible content (replaced in Task 3):
```sql
-- placeholder; replaced in Task 3
CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at INTEGER NOT NULL
);
```

Write `engine/internal/store/migrations_test.go`:
```go
package store

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestApplyMigrations_appliesOnce(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("second apply (idempotent): %v", err)
	}

	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n < 1 {
		t.Errorf("expected at least one migration recorded, got %d", n)
	}
}
```

- [ ] **Step 3: Run — expect failure (ApplyMigrations undefined)**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/store/...
```

- [ ] **Step 4: Implement embed + runner**

Write `engine/internal/store/embed.go`:
```go
package store

import "embed"

// MigrationsFS holds the SQL migration files. They are embedded at build time
// so the binary is self-contained.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
```

Write `engine/internal/store/migrations.go`:
```go
package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ApplyMigrations runs every embedded migration whose version is greater than
// the highest version already recorded in schema_migrations. Idempotent: a
// second call is a no-op.
func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	if err := ensureMigrationTable(ctx, db); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	entries, err := fs.ReadDir(MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version, err := parseVersion(e.Name())
		if err != nil {
			return err
		}
		if _, done := applied[version]; done {
			continue
		}
		body, err := fs.ReadFile(MigrationsFS, path.Join("migrations", e.Name()))
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		if err := applyOne(ctx, db, version, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", e.Name(), err)
		}
	}
	return nil
}

func ensureMigrationTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at INTEGER NOT NULL
)`)
	return err
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[int]struct{}, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]struct{}{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = struct{}{}
	}
	return out, rows.Err()
}

func applyOne(ctx context.Context, db *sql.DB, version int, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`,
		version, time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

// parseVersion accepts filenames like "0001_init.sql" and returns 1.
func parseVersion(name string) (int, error) {
	base := strings.TrimSuffix(name, ".sql")
	prefix := base
	if i := strings.Index(base, "_"); i >= 0 {
		prefix = base[:i]
	}
	v, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("bad migration filename %q: %w", name, err)
	}
	return v, nil
}
```

- [ ] **Step 5: Run — expect PASS**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/store/...
```

- [ ] **Step 6: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/go.mod engine/go.sum engine/internal/store
git commit -m "feat(store): embed migration runner + placeholder 0001"
```

---

## Task 3: Initial schema migration `0001_init.sql`

**Files:**
- Modify: `engine/internal/store/migrations/0001_init.sql` (full schema)

- [ ] **Step 1: Replace `0001_init.sql` with the full schema**

Use Write to fully replace `engine/internal/store/migrations/0001_init.sql` with the following content (copied verbatim from spec §3, minus the `schema_migrations` table since the runner creates it itself):

```sql
-- 작품
CREATE TABLE projects (
  id            TEXT PRIMARY KEY,
  title         TEXT NOT NULL,
  genres        TEXT NOT NULL,
  length_target TEXT NOT NULL,
  default_pov   TEXT NOT NULL,
  style_notes   TEXT NOT NULL DEFAULT '',
  word_count    INTEGER NOT NULL DEFAULT 0,
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
  ordinal     INTEGER NOT NULL,
  kind        TEXT NOT NULL,
  label       TEXT NOT NULL,
  title       TEXT NOT NULL DEFAULT '',
  content_doc TEXT,
  status      TEXT NOT NULL DEFAULT 'draft',
  word_count  INTEGER NOT NULL DEFAULT 0,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);
CREATE INDEX idx_nodes_project ON nodes(project_id, parent_id, ordinal);

-- Entity (인물·장소·물건·개념)
CREATE TABLE entities (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL,
  name        TEXT NOT NULL,
  aliases     TEXT NOT NULL DEFAULT '[]',
  role        TEXT NOT NULL DEFAULT '',
  summary     TEXT NOT NULL DEFAULT '',
  attributes  TEXT NOT NULL DEFAULT '{}',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL,
  UNIQUE(project_id, name)
);

-- Mention: Node ↔ Entity
CREATE TABLE mentions (
  id         TEXT PRIMARY KEY,
  node_id    TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  entity_id  TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  position   INTEGER NOT NULL,
  surface    TEXT NOT NULL
);
CREATE INDEX idx_mentions_node ON mentions(node_id);
CREATE INDEX idx_mentions_entity ON mentions(entity_id);

-- Relationship: Entity ↔ Entity
CREATE TABLE relationships (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  from_id    TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  to_id      TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  label      TEXT NOT NULL,
  notes      TEXT NOT NULL DEFAULT ''
);

-- Thread (스토리라인)
CREATE TABLE threads (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  color      TEXT NOT NULL DEFAULT '#666',
  summary    TEXT NOT NULL DEFAULT '',
  closed_at  INTEGER
);

-- Beat: Thread의 마디
CREATE TABLE beats (
  id         TEXT PRIMARY KEY,
  thread_id  TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  node_id    TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  ordinal    INTEGER NOT NULL,
  label      TEXT NOT NULL DEFAULT '',
  intensity  INTEGER NOT NULL DEFAULT 1
);

-- Note: 마진 노트
CREATE TABLE notes (
  id         TEXT PRIMARY KEY,
  node_id    TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  anchor     INTEGER NOT NULL,
  body       TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

-- Version snapshot
CREATE TABLE node_snapshots (
  id          TEXT PRIMARY KEY,
  node_id     TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  content_doc TEXT NOT NULL,
  reason      TEXT NOT NULL,
  created_at  INTEGER NOT NULL
);
CREATE INDEX idx_snapshots_node ON node_snapshots(node_id, created_at);

-- AI 호출 이력
CREATE TABLE ai_runs (
  id           TEXT PRIMARY KEY,
  project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id      TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  provider     TEXT NOT NULL,
  prompt       TEXT NOT NULL,
  context_json TEXT NOT NULL,
  output       TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL,
  error        TEXT,
  started_at   INTEGER NOT NULL,
  ended_at     INTEGER
);
```

- [ ] **Step 2: Run migrations test — still PASS**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/store/...
```

- [ ] **Step 3: Add a stronger test that asserts the projects table exists**

Append to `engine/internal/store/migrations_test.go`:
```go
func TestApplyMigrations_createsProjectsTable(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Insert a sentinel row matching the schema.
	_, err = db.ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
VALUES ('p1', 'Test', '["SF"]', 'novel', 'first', 0, 0)`)
	if err != nil {
		t.Fatalf("insert into projects: %v", err)
	}
}
```

- [ ] **Step 4: Run — PASS**

```bash
go test ./internal/store/...
```

- [ ] **Step 5: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/store/migrations/0001_init.sql engine/internal/store/migrations_test.go
git commit -m "feat(store): initial schema migration (full)"
```

---

## Task 4: `store.Open` — open SQLite with pragmas + run migrations (TDD)

**Files:**
- Create: `engine/internal/store/store.go`
- Create: `engine/internal/store/store_test.go`

- [ ] **Step 1: Write the failing test**

Write `engine/internal/store/store_test.go`:
```go
package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpen_appliesMigrations_andEnablesFKs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()

	s, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// projects table exists?
	if _, err := s.DB().ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
VALUES ('p1', 'Hello', '["SF"]', 'novel', 'first', 0, 0)`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// foreign_keys ON?
	var fk int
	if err := s.DB().QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	// journal_mode WAL?
	var jm string
	if err := s.DB().QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&jm); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if jm != "wal" {
		t.Errorf("journal_mode = %q, want wal", jm)
	}
}

func TestOpen_idempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()

	s1, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open #1: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open #2: %v", err)
	}
	defer s2.Close()
}
```

- [ ] **Step 2: Run — expect failure (Open, Store undefined)**

```bash
go test ./internal/store/...
```

- [ ] **Step 3: Implement**

Write `engine/internal/store/store.go`:
```go
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite handle with helpers and a guaranteed migration pass.
type Store struct {
	db *sql.DB
}

// Open opens or creates the database at path, applies pragmas, runs migrations,
// and returns the Store. Closes any partial state on error.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA foreign_keys=ON`,
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pragma %q: %w", pragma, err)
		}
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// DB exposes the raw *sql.DB for repos that need it.
func (s *Store) DB() *sql.DB { return s.db }

// Close shuts down the underlying connection pool.
func (s *Store) Close() error { return s.db.Close() }
```

- [ ] **Step 4: Run — PASS**

```bash
go test ./internal/store/...
```

- [ ] **Step 5: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/store/store.go engine/internal/store/store_test.go
git commit -m "feat(store): Open() with pragmas + migrations"
```

---

## Task 5: `project` package — domain types + repo (TDD)

**Files:**
- Create: `engine/internal/project/project.go`
- Create: `engine/internal/project/repo.go`
- Create: `engine/internal/project/repo_test.go`

The MVP needs: create (with auto-first-leaf), list (sorted by last opened, optional includeArchived), get by id, archive, set last_opened_node_id. Nodes for the first leaf are inserted by the project create call — we don't need a separate node repo yet.

- [ ] **Step 1: Define domain types**

Write `engine/internal/project/project.go`:
```go
// Package project owns Project domain types and the SQLite-backed Repo.
package project

// Project is the on-wire and on-disk representation of a single work.
// Genres is a JSON-array stored as TEXT in SQLite; the repo handles conversion.
type Project struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Genres           []string `json:"genres"`
	LengthTarget     string   `json:"length_target"` // flash|short|novella|novel|series
	DefaultPOV       string   `json:"default_pov"`   // first|third_limited|omniscient
	StyleNotes       string   `json:"style_notes"`
	WordCount        int      `json:"word_count"`
	LastOpenedNodeID *string  `json:"last_opened_node_id,omitempty"`
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
	ArchivedAt       *int64   `json:"archived_at,omitempty"`
}

// NewInput is what the UI submits from the New Project modal.
type NewInput struct {
	Title        string   `json:"title"`
	Genres       []string `json:"genres"`
	LengthTarget string   `json:"length_target"`
	DefaultPOV   string   `json:"default_pov"`
}

// ListFilter selects which projects to return.
type ListFilter struct {
	IncludeArchived bool `json:"include_archived"`
	Limit           int  `json:"limit"`
}
```

- [ ] **Step 2: Write the failing repo test**

Write `engine/internal/project/repo_test.go`:
```go
package project

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestRepo_Create_returnsProjectWithGeneratedID_andFirstLeafNode(t *testing.T) {
	s := openStore(t)
	r := NewRepo(s)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	p, err := r.Create(ctx, now, NewInput{
		Title:        "은하의 노래",
		Genres:       []string{"SF", "문학"},
		LengthTarget: "novel",
		DefaultPOV:   "first",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == "" {
		t.Error("Create: missing ID")
	}
	if p.Title != "은하의 노래" {
		t.Errorf("title = %q", p.Title)
	}
	if got, want := len(p.Genres), 2; got != want {
		t.Errorf("genres len = %d, want %d", got, want)
	}
	if p.LastOpenedNodeID == nil || *p.LastOpenedNodeID == "" {
		t.Error("Create: last_opened_node_id should point to the auto-created first leaf")
	}

	// First leaf node exists?
	var (
		nodeID, label, kind string
		nodeProjectID       string
	)
	err = s.DB().QueryRowContext(ctx, `
SELECT id, project_id, kind, label FROM nodes WHERE id = ?`, *p.LastOpenedNodeID).
		Scan(&nodeID, &nodeProjectID, &kind, &label)
	if err != nil {
		t.Fatalf("first leaf row: %v", err)
	}
	if nodeProjectID != p.ID {
		t.Errorf("node.project_id = %q, want %q", nodeProjectID, p.ID)
	}
	if kind != "leaf" {
		t.Errorf("node.kind = %q, want leaf", kind)
	}
	if label != "씬 1" {
		t.Errorf("node.label = %q, want %q", label, "씬 1")
	}
}

func TestRepo_List_recentFirst(t *testing.T) {
	s := openStore(t)
	r := NewRepo(s)
	ctx := context.Background()

	_, _ = r.Create(ctx, 1000, NewInput{Title: "A", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	_, _ = r.Create(ctx, 2000, NewInput{Title: "B", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	_, _ = r.Create(ctx, 3000, NewInput{Title: "C", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})

	got, err := r.List(ctx, ListFilter{Limit: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Title != "C" || got[1].Title != "B" || got[2].Title != "A" {
		t.Errorf("order = %q,%q,%q; want C,B,A", got[0].Title, got[1].Title, got[2].Title)
	}
}

func TestRepo_List_excludesArchivedByDefault(t *testing.T) {
	s := openStore(t)
	r := NewRepo(s)
	ctx := context.Background()

	a, _ := r.Create(ctx, 1000, NewInput{Title: "kept", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	b, _ := r.Create(ctx, 2000, NewInput{Title: "gone", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	if err := r.Archive(ctx, b.ID, 9999); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	defaultList, err := r.List(ctx, ListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultList) != 1 || defaultList[0].ID != a.ID {
		t.Errorf("default list should be just a, got %d entries", len(defaultList))
	}

	allList, err := r.List(ctx, ListFilter{IncludeArchived: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(allList) != 2 {
		t.Errorf("with archived: got %d, want 2", len(allList))
	}
}

func TestRepo_Get(t *testing.T) {
	s := openStore(t)
	r := NewRepo(s)
	ctx := context.Background()

	p, _ := r.Create(ctx, 1000, NewInput{Title: "x", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	got, err := r.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "x" {
		t.Errorf("Get returned wrong project: %+v", got)
	}

	_, err = r.Get(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("Get of missing: err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 3: Run — failure**

```bash
go test ./internal/project/...
```

- [ ] **Step 4: Implement the repo**

Write `engine/internal/project/repo.go`:
```go
package project

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned by Get when the project id doesn't exist.
var ErrNotFound = errors.New("project not found")

// Repo persists Projects (and the auto-created first leaf node) in SQLite.
type Repo struct {
	s *store.Store
}

// NewRepo returns a Repo backed by the given Store.
func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Create inserts a new project at the given timestamp, plus its first leaf
// node ("씬 1"), and returns the resulting Project (with last_opened_node_id set).
func (r *Repo) Create(ctx context.Context, now int64, in NewInput) (Project, error) {
	if in.Title == "" {
		return Project{}, fmt.Errorf("create project: title required")
	}
	if in.LengthTarget == "" || in.DefaultPOV == "" {
		return Project{}, fmt.Errorf("create project: length_target and default_pov required")
	}

	projectID := uuid.NewString()
	nodeID := uuid.NewString()
	genresJSON, err := json.Marshal(in.Genres)
	if err != nil {
		return Project{}, err
	}

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, style_notes,
                      word_count, last_opened_node_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, '', 0, ?, ?, ?)`,
		projectID, in.Title, string(genresJSON), in.LengthTarget, in.DefaultPOV,
		nodeID, now, now); err != nil {
		return Project{}, err
	}
	// Empty Tiptap doc: single empty paragraph.
	const emptyDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`
	if _, err := tx.ExecContext(ctx, `
INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title,
                   content_doc, status, word_count, created_at, updated_at)
VALUES (?, ?, NULL, 0, 'leaf', '씬 1', '', ?, 'draft', 0, ?, ?)`,
		nodeID, projectID, emptyDoc, now, now); err != nil {
		return Project{}, err
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}

	return r.Get(ctx, projectID)
}

// Get returns the project by id, or ErrNotFound.
func (r *Repo) Get(ctx context.Context, id string) (Project, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	p, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

// List returns recent projects sorted by updated_at DESC.
func (r *Repo) List(ctx context.Context, f ListFilter) ([]Project, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := baseSelect
	if !f.IncludeArchived {
		q += ` WHERE archived_at IS NULL`
	}
	q += ` ORDER BY updated_at DESC LIMIT ?`

	rows, err := r.s.DB().QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Archive soft-deletes by setting archived_at; idempotent on already-archived.
func (r *Repo) Archive(ctx context.Context, id string, now int64) error {
	res, err := r.s.DB().ExecContext(ctx, `
UPDATE projects SET archived_at = COALESCE(archived_at, ?), updated_at = ?
WHERE id = ?`, now, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

const baseSelect = `
SELECT id, title, genres, length_target, default_pov, style_notes,
       word_count, last_opened_node_id, created_at, updated_at, archived_at
FROM projects`

// scanner is the small subset of *sql.Row / *sql.Rows we need.
type scanner interface {
	Scan(...any) error
}

func scan(row scanner) (Project, error) {
	var (
		p          Project
		genresJSON string
		lastNode   sql.NullString
		archivedAt sql.NullInt64
	)
	if err := row.Scan(&p.ID, &p.Title, &genresJSON, &p.LengthTarget, &p.DefaultPOV,
		&p.StyleNotes, &p.WordCount, &lastNode, &p.CreatedAt, &p.UpdatedAt, &archivedAt); err != nil {
		return Project{}, err
	}
	if err := json.Unmarshal([]byte(genresJSON), &p.Genres); err != nil {
		return Project{}, fmt.Errorf("decode genres: %w", err)
	}
	if lastNode.Valid {
		v := lastNode.String
		p.LastOpenedNodeID = &v
	}
	if archivedAt.Valid {
		v := archivedAt.Int64
		p.ArchivedAt = &v
	}
	return p, nil
}
```

- [ ] **Step 5: Run — PASS**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/project/...
```

- [ ] **Step 6: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/project
git commit -m "feat(project): repo with Create (+first leaf), Get, List, Archive"
```

---

## Task 6: Project RPC handlers (TDD)

**Files:**
- Create: `engine/internal/rpc/handlers/projects.go`
- Create: `engine/internal/rpc/handlers/projects_test.go`

We add 4 handlers: `projects.create`, `projects.list`, `projects.get`, `projects.archive`. They take a `*project.Repo` via closure construction.

- [ ] **Step 1: Write the failing test**

Write `engine/internal/rpc/handlers/projects_test.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newRepo(t *testing.T) *project.Repo {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return project.NewRepo(s)
}

func TestCreateProjectHandler(t *testing.T) {
	repo := newRepo(t)
	h := CreateProject(repo, func() int64 { return 12345 })

	params := json.RawMessage(`{
		"title": "Test",
		"genres": ["SF"],
		"length_target": "short",
		"default_pov": "first"
	}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var p project.Project
	if err := json.Unmarshal(res, &p); err != nil {
		t.Fatalf("unmarshal: %v (raw=%s)", err, string(res))
	}
	if p.Title != "Test" {
		t.Errorf("title = %q", p.Title)
	}
	if p.CreatedAt != 12345 {
		t.Errorf("created_at = %d, want 12345 (clock injected)", p.CreatedAt)
	}
}

func TestListProjectsHandler(t *testing.T) {
	repo := newRepo(t)
	clock := int64(1000)
	create := CreateProject(repo, func() int64 {
		clock += 100
		return clock
	})
	for _, name := range []string{"A", "B", "C"} {
		params, _ := json.Marshal(project.NewInput{Title: name, Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
		if _, err := create(context.Background(), params); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	list := ListProjects(repo)
	res, err := list(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var out []project.Project
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("got %d projects, want 3", len(out))
	}
	if out[0].Title != "C" {
		t.Errorf("first = %q, want C (most recent)", out[0].Title)
	}
}

func TestArchiveAndGetProject(t *testing.T) {
	repo := newRepo(t)
	create := CreateProject(repo, func() int64 { return 1 })
	res, _ := create(context.Background(), json.RawMessage(`{"title":"x","genres":["SF"],"length_target":"short","default_pov":"first"}`))
	var created project.Project
	_ = json.Unmarshal(res, &created)

	arch := ArchiveProject(repo, func() int64 { return 99 })
	if _, err := arch(context.Background(), json.RawMessage(`{"id":"`+created.ID+`"}`)); err != nil {
		t.Fatalf("archive: %v", err)
	}

	get := GetProject(repo)
	gotRes, err := get(context.Background(), json.RawMessage(`{"id":"`+created.ID+`"}`))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	var fetched project.Project
	_ = json.Unmarshal(gotRes, &fetched)
	if fetched.ArchivedAt == nil || *fetched.ArchivedAt != 99 {
		t.Errorf("archived_at = %v, want 99", fetched.ArchivedAt)
	}
}
```

- [ ] **Step 2: Run — failure (undefined CreateProject, ListProjects, ArchiveProject, GetProject)**

```bash
go test ./internal/rpc/handlers/...
```

- [ ] **Step 3: Implement**

Write `engine/internal/rpc/handlers/projects.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// Clock is an injected nanosecond-precision-ish source. Tests pass deterministic
// values; production passes time.Now().UnixMilli wrappers.
type Clock func() int64

// CreateProject returns a handler for projects.create.
func CreateProject(repo *project.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in project.NewInput
		if len(params) > 0 {
			if err := json.Unmarshal(params, &in); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
		}
		p, err := repo.Create(ctx, now(), in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(p)
	}
}

// listParams mirrors project.ListFilter on the wire.
type listParams struct {
	IncludeArchived bool `json:"include_archived"`
	Limit           int  `json:"limit"`
}

// ListProjects returns a handler for projects.list.
func ListProjects(repo *project.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p) // tolerate empty / partial
		}
		list, err := repo.List(ctx, project.ListFilter{IncludeArchived: p.IncludeArchived, Limit: p.Limit})
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		// Always emit an array, never null.
		if list == nil {
			list = []project.Project{}
		}
		return json.Marshal(list)
	}
}

// idParam is the shared shape for handlers that take a single project id.
type idParam struct {
	ID string `json:"id"`
}

// GetProject returns a handler for projects.get.
func GetProject(repo *project.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		got, err := repo.Get(ctx, p.ID)
		if errors.Is(err, project.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

// ArchiveProject returns a handler for projects.archive.
func ArchiveProject(repo *project.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Archive(ctx, p.ID, now()); err != nil {
			if errors.Is(err, project.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
```

- [ ] **Step 4: Run — PASS**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./...
```
Expected: all packages green.

- [ ] **Step 5: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/rpc/handlers/projects.go engine/internal/rpc/handlers/projects_test.go
git commit -m "feat(rpc): projects.create/list/get/archive handlers"
```

---

## Task 7: Wire store + project handlers into `main.go`

**Files:**
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1: Replace `main.go`**

Use Write to fully replace `engine/cmd/linetta-engine/main.go` with:
```go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/devlikebear/tars/pkg/llm" // pin

	"github.com/devlikebear/linetta/engine/internal/paths"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func main() {
	stdio := flag.Bool("stdio", false, "serve JSONRPC over stdin/stdout")
	flag.Parse()

	if !*stdio {
		fmt.Fprintln(os.Stderr, "linetta-engine: --stdio required (other modes land in later plans)")
		os.Exit(2)
	}

	if err := paths.EnsureHome(); err != nil {
		fail("ensure home: %v", err)
	}
	dbPath, err := paths.DBPath()
	if err != nil {
		fail("db path: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	st, err := store.Open(ctx, dbPath)
	if err != nil {
		fail("open store: %v", err)
	}
	defer st.Close()

	repo := project.NewRepo(st)
	clock := func() int64 { return time.Now().UnixMilli() }

	s := rpc.NewServer()
	s.Handle("ping", handlers.Ping)
	s.Handle("projects.create", handlers.CreateProject(repo, clock))
	s.Handle("projects.list", handlers.ListProjects(repo))
	s.Handle("projects.get", handlers.GetProject(repo))
	s.Handle("projects.archive", handlers.ArchiveProject(repo, clock))

	if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, io.EOF) {
		fail("serve: %v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "linetta-engine: "+format+"\n", args...)
	os.Exit(1)
}
```

- [ ] **Step 2: Build + smoke**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go build -o /tmp/linetta-engine-build ./cmd/linetta-engine

# Use a temp LINETTA_HOME so we don't touch the user's data
LINETTA_HOME=/tmp/linetta-plan1-smoke /tmp/linetta-engine-build --stdio <<EOF
{"jsonrpc":"2.0","id":1,"method":"projects.list"}
{"jsonrpc":"2.0","id":2,"method":"projects.create","params":{"title":"Smoke","genres":["SF"],"length_target":"short","default_pov":"first"}}
{"jsonrpc":"2.0","id":3,"method":"projects.list"}
EOF
rm -f /tmp/linetta-engine-build
rm -rf /tmp/linetta-plan1-smoke
```

Expected stdout: three lines.
- Line 1: `{"jsonrpc":"2.0","id":1,"result":[]}`
- Line 2: a JSON project record with the inserted fields
- Line 3: an array with the one project

- [ ] **Step 3: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): wire store + project handlers"
```

- [ ] **Step 4: Rebuild the dev engine binary**

```bash
./scripts/build-engine.sh
```

This refreshes the binary the Tauri sidecar will spawn.

---

## Task 8: React Router + frontend dependencies

**Files:**
- Modify: `apps/desktop/package.json` (adds `react-router-dom`)
- Modify: `apps/desktop/src/main.tsx` (wraps in `BrowserRouter`)
- Modify: `apps/desktop/src/App.tsx` (becomes a `<Routes>` shell)
- Create: `apps/desktop/src/routes/Library.tsx` (stub for this task)
- Create: `apps/desktop/src/routes/Workspace.tsx` (stub)
- Create: `apps/desktop/src/routes/Settings.tsx` (stub)
- Create: `apps/desktop/src/routes/LibraryAll.tsx` (stub)

- [ ] **Step 1: Add dependency**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop
pnpm add react-router-dom@^6
```

- [ ] **Step 2: Write `apps/desktop/src/main.tsx`**

```tsx
import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { App } from "./App";
import "./App.css";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");

ReactDOM.createRoot(root).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>,
);
```

- [ ] **Step 3: Write `apps/desktop/src/App.tsx`**

```tsx
import { Routes, Route, Navigate } from "react-router-dom";
import { Library } from "./routes/Library";
import { LibraryAll } from "./routes/LibraryAll";
import { Workspace } from "./routes/Workspace";
import { Settings } from "./routes/Settings";

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Library />} />
      <Route path="/library/all" element={<LibraryAll />} />
      <Route path="/workspace/:projectId" element={<Workspace />} />
      <Route path="/settings" element={<Settings />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
```

- [ ] **Step 4: Write stub route components**

`apps/desktop/src/routes/Library.tsx`:
```tsx
export function Library() {
  return <p>Library — implemented in Task 10</p>;
}
```

`apps/desktop/src/routes/LibraryAll.tsx`:
```tsx
export function LibraryAll() {
  return <p>Full library — implemented in Task 12</p>;
}
```

`apps/desktop/src/routes/Workspace.tsx`:
```tsx
import { useParams, Link } from "react-router-dom";

export function Workspace() {
  const { projectId } = useParams();
  return (
    <main className="shell">
      <p>
        <Link to="/">← Library</Link>
      </p>
      <h2>Workspace placeholder</h2>
      <p className="hint">
        Project <code>{projectId}</code>. The editor lands in Plan 2.
      </p>
    </main>
  );
}
```

`apps/desktop/src/routes/Settings.tsx`:
```tsx
import { Link } from "react-router-dom";

export function Settings() {
  return (
    <main className="shell">
      <p>
        <Link to="/">← Library</Link>
      </p>
      <h2>Settings</h2>
      <p className="hint">Provider selection lands in Plan 6.</p>
    </main>
  );
}
```

- [ ] **Step 5: Type-check + smoke build**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop
pnpm tsc -b
pnpm build
```
Expected: silent success, `dist/` regenerated.

- [ ] **Step 6: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/package.json apps/desktop/pnpm-lock.yaml apps/desktop/src
git commit -m "feat(desktop): add react-router and route stubs"
```

---

## Task 9: RPC client types + project APIs

**Files:**
- Create: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/lib/rpc.ts`

- [ ] **Step 1: Write `types.ts`**

```ts
// Mirrors engine/internal/project Project struct (JSON tag names).
export type LengthTarget = "flash" | "short" | "novella" | "novel" | "series";
export type DefaultPOV = "first" | "third_limited" | "omniscient";

export interface Project {
  id: string;
  title: string;
  genres: string[];
  length_target: LengthTarget;
  default_pov: DefaultPOV;
  style_notes: string;
  word_count: number;
  last_opened_node_id?: string;
  created_at: number;
  updated_at: number;
  archived_at?: number;
}

export interface NewProjectInput {
  title: string;
  genres: string[];
  length_target: LengthTarget;
  default_pov: DefaultPOV;
}

export interface ListProjectsParams {
  include_archived?: boolean;
  limit?: number;
}
```

- [ ] **Step 2: Replace `apps/desktop/src/lib/rpc.ts`**

```ts
import { invoke } from "@tauri-apps/api/core";
import type {
  ListProjectsParams,
  NewProjectInput,
  Project,
} from "./types";

// Tauri commands defined in src-tauri.

export async function enginePing(): Promise<string> {
  return invoke<string>("engine_ping");
}

export async function rpcCall<T>(method: string, params?: unknown): Promise<T> {
  return invoke<T>("engine_call", { method, params: params ?? null });
}

export const projects = {
  create: (input: NewProjectInput) => rpcCall<Project>("projects.create", input),
  list: (params: ListProjectsParams = {}) => rpcCall<Project[]>("projects.list", params),
  get: (id: string) => rpcCall<Project>("projects.get", { id }),
  archive: (id: string) => rpcCall<{ ok: true }>("projects.archive", { id }),
};
```

This introduces a generic `engine_call` Tauri command so we don't need to add one per JSONRPC method. The Rust side gets that wired in **Step 3** below.

- [ ] **Step 3: Add `engine_call` to the Rust shell**

Modify `apps/desktop/src-tauri/src/lib.rs` — add the new command and register it. Replace the file with exactly:

```rust
mod engine;
mod jsonrpc;

use std::sync::Arc;
use serde_json::Value;
use tauri::Manager;

pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .setup(|app| {
            let handle = app.handle().clone();
            tauri::async_runtime::block_on(async move {
                match engine::spawn(&handle).await {
                    Ok(engine_handle) => {
                        handle.manage(EngineState {
                            client: engine_handle.client.clone(),
                            _engine: Arc::new(engine_handle),
                        });
                    }
                    Err(e) => {
                        eprintln!("[linetta] failed to spawn engine: {e:#}");
                    }
                }
            });
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![engine_ping, engine_call])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

struct EngineState {
    client: Arc<jsonrpc::Client>,
    _engine: Arc<engine::EngineHandle>,
}

#[tauri::command]
async fn engine_ping(state: tauri::State<'_, EngineState>) -> Result<String, String> {
    let result = state
        .client
        .call("ping", None)
        .await
        .map_err(|e| e.to_string())?;
    result
        .as_str()
        .map(|s| s.to_string())
        .ok_or_else(|| format!("ping result is not a string: {result}"))
}

#[tauri::command]
async fn engine_call(
    state: tauri::State<'_, EngineState>,
    method: String,
    params: Option<Value>,
) -> Result<Value, String> {
    state
        .client
        .call(&method, params)
        .await
        .map_err(|e| e.to_string())
}
```

- [ ] **Step 4: Compile check**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop/src-tauri
cargo check
cd ../..
pnpm --filter . tsc -b 2>&1 || (cd apps/desktop && pnpm tsc -b)
```
Expected: both silent.

- [ ] **Step 5: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/lib apps/desktop/src-tauri/src/lib.rs
git commit -m "feat(desktop): generic engine_call command + projects API client"
```

---

## Task 10: Library screen — cards + new-work flow

**Files:**
- Create: `apps/desktop/src/components/ProjectCard.tsx`
- Create: `apps/desktop/src/components/NewProjectModal.tsx`
- Modify: `apps/desktop/src/routes/Library.tsx` (replace stub)
- Modify: `apps/desktop/src/App.css` (Library + modal styles)

- [ ] **Step 1: Write `ProjectCard.tsx`**

```tsx
import type { Project, LengthTarget } from "../lib/types";
import { Link } from "react-router-dom";

const LENGTH_LABEL: Record<LengthTarget, string> = {
  flash: "플래시",
  short: "단편",
  novella: "중편",
  novel: "장편",
  series: "시리즈",
};

function humanCount(words: number): string {
  if (words === 0) return "초안 시작 전";
  if (words < 1000) return `${words}자`;
  if (words < 10_000) return `${words.toLocaleString("ko-KR")}자`;
  return `${(words / 1000).toFixed(0)}k`;
}

export function ProjectCard({ project }: { project: Project }) {
  const meta = `${LENGTH_LABEL[project.length_target]} · ${humanCount(project.word_count)}`;
  return (
    <Link to={`/workspace/${project.id}`} className="card">
      <p className="card-title">{project.title}</p>
      <p className="card-meta">{meta}</p>
    </Link>
  );
}
```

- [ ] **Step 2: Write `NewProjectModal.tsx`**

```tsx
import { useState, useEffect, type FormEvent } from "react";
import type { NewProjectInput, LengthTarget, DefaultPOV } from "../lib/types";

const DEFAULT_GENRES = ["SF", "판타지", "추리", "문학"];
const LENGTHS: { value: LengthTarget; label: string }[] = [
  { value: "flash", label: "플래시" },
  { value: "short", label: "단편" },
  { value: "novella", label: "중편" },
  { value: "novel", label: "장편" },
  { value: "series", label: "시리즈" },
];
const POVS: { value: DefaultPOV; label: string }[] = [
  { value: "first", label: "1인칭" },
  { value: "third_limited", label: "3인칭 제한" },
  { value: "omniscient", label: "전지적" },
];

interface Props {
  open: boolean;
  onClose: () => void;
  onSubmit: (input: NewProjectInput) => Promise<void>;
}

export function NewProjectModal({ open, onClose, onSubmit }: Props) {
  const [title, setTitle] = useState("");
  const [genres, setGenres] = useState<string[]>([]);
  const [customGenre, setCustomGenre] = useState("");
  const [length, setLength] = useState<LengthTarget>("short");
  const [pov, setPov] = useState<DefaultPOV>("first");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      setTitle("");
      setGenres([]);
      setCustomGenre("");
      setLength("short");
      setPov("first");
      setError(null);
    }
  }, [open]);

  if (!open) return null;

  const toggleGenre = (g: string) => {
    setGenres((prev) => (prev.includes(g) ? prev.filter((x) => x !== g) : [...prev, g]));
  };

  const addCustomGenre = () => {
    const g = customGenre.trim();
    if (!g || genres.includes(g)) {
      setCustomGenre("");
      return;
    }
    setGenres((prev) => [...prev, g]);
    setCustomGenre("");
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!title.trim()) {
      setError("제목을 입력하세요");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await onSubmit({
        title: title.trim(),
        genres,
        length_target: length,
        default_pov: pov,
      });
    } catch (err) {
      setError(String(err));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form className="modal" onClick={(e) => e.stopPropagation()} onSubmit={handleSubmit}>
        <h2>새 작품</h2>

        <label className="field">
          <span>제목</span>
          <input value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
        </label>

        <div className="field">
          <span>장르 (다중 선택)</span>
          <div className="chips">
            {[...DEFAULT_GENRES, ...genres.filter((g) => !DEFAULT_GENRES.includes(g))].map((g) => (
              <button
                type="button"
                key={g}
                className={`chip${genres.includes(g) ? " on" : ""}`}
                onClick={() => toggleGenre(g)}
              >
                {g}
              </button>
            ))}
            <input
              className="chip-input"
              placeholder="+"
              value={customGenre}
              onChange={(e) => setCustomGenre(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  addCustomGenre();
                }
              }}
            />
          </div>
        </div>

        <div className="field">
          <span>예상 분량</span>
          <div className="chips">
            {LENGTHS.map((l) => (
              <button
                type="button"
                key={l.value}
                className={`chip${length === l.value ? " on" : ""}`}
                onClick={() => setLength(l.value)}
              >
                {l.label}
              </button>
            ))}
          </div>
        </div>

        <div className="field">
          <span>기본 시점</span>
          <div className="chips">
            {POVS.map((p) => (
              <button
                type="button"
                key={p.value}
                className={`chip${pov === p.value ? " on" : ""}`}
                onClick={() => setPov(p.value)}
              >
                {p.label}
              </button>
            ))}
          </div>
        </div>

        {error && <p className="error">{error}</p>}

        <div className="modal-actions">
          <button type="button" onClick={onClose} disabled={submitting}>취소</button>
          <button type="submit" disabled={submitting}>{submitting ? "생성 중…" : "시작"}</button>
        </div>
      </form>
    </div>
  );
}
```

- [ ] **Step 3: Replace `Library.tsx`**

```tsx
import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { projects as projectsApi } from "../lib/rpc";
import type { Project, NewProjectInput } from "../lib/types";
import { ProjectCard } from "../components/ProjectCard";
import { NewProjectModal } from "../components/NewProjectModal";

const RECENT_LIMIT = 5;

export function Library() {
  const [recent, setRecent] = useState<Project[]>([]);
  const [totalRecent, setTotalRecent] = useState<number>(0);
  const [modalOpen, setModalOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();

  const refresh = async () => {
    setLoading(true);
    setError(null);
    try {
      const all = await projectsApi.list({ limit: RECENT_LIMIT + 1 });
      setRecent(all.slice(0, RECENT_LIMIT));
      setTotalRecent(all.length);
      if (all.length === 0) setModalOpen(true);
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  const handleCreate = async (input: NewProjectInput) => {
    const created = await projectsApi.create(input);
    setModalOpen(false);
    navigate(`/workspace/${created.id}`);
  };

  return (
    <main className="library">
      <header className="library-top">
        <button className="icon-btn" aria-label="라이브러리 옵션" disabled>···</button>
        <Link to="/settings" className="icon-btn">설정</Link>
      </header>

      <section className="library-center">
        <h1 className="library-heading">Linetta</h1>

        <button className="new-button" onClick={() => setModalOpen(true)}>
          + 새 작품
        </button>

        {loading ? (
          <p className="hint">불러오는 중…</p>
        ) : error ? (
          <p className="error">{error}</p>
        ) : recent.length === 0 ? null : (
          <>
            <p className="library-label">최근 작품 · {recent.length}개</p>
            <div className="card-grid">
              {recent.map((p) => (
                <ProjectCard key={p.id} project={p} />
              ))}
            </div>
            {totalRecent > RECENT_LIMIT && (
              <Link to="/library/all" className="library-all-link">
                전체 라이브러리 →
              </Link>
            )}
          </>
        )}
      </section>

      <NewProjectModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        onSubmit={handleCreate}
      />
    </main>
  );
}
```

- [ ] **Step 4: Extend `App.css`**

Append (do not replace — preserve existing rules) the following to `apps/desktop/src/App.css`:

```css
/* Library */
.library {
  min-height: 100vh;
  display: grid;
  grid-template-rows: auto 1fr;
}

.library-top {
  display: flex;
  justify-content: space-between;
  padding: 0.75rem 1rem;
}

.icon-btn {
  background: none;
  border: none;
  font-family: inherit;
  font-size: 0.875rem;
  color: #6b6b6b;
  cursor: pointer;
  text-decoration: none;
}

.library-center {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1.5rem;
  padding: 6vh 2rem 4vh;
}

.library-heading {
  font-size: 2.5rem;
  margin: 0;
  letter-spacing: 0.02em;
}

.new-button {
  padding: 0.65rem 1.5rem;
  font: inherit;
  background: transparent;
  border: 1px solid #1a1a1a;
  border-radius: 4px;
  cursor: pointer;
}

.library-label {
  margin: 1.5rem 0 0;
  color: #6b6b6b;
  font-size: 0.875rem;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 200px));
  gap: 1rem;
  width: min(900px, 90%);
}

.card {
  display: block;
  padding: 1rem;
  border: 1px solid #d8d6cf;
  border-radius: 6px;
  background: #fffefb;
  text-decoration: none;
  color: inherit;
}

.card:hover {
  background: #f6f4ee;
}

.card-title {
  margin: 0 0 0.5rem;
  font-weight: 500;
  min-height: 2.5em;
}

.card-meta {
  margin: 0;
  color: #6b6b6b;
  font-size: 0.875rem;
}

.library-all-link {
  margin-top: 1rem;
  color: #6b6b6b;
  text-decoration: none;
}

.library-all-link:hover {
  color: #1a1a1a;
}

.error {
  color: #a8312f;
}

/* Modal */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(20, 20, 20, 0.35);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem;
  z-index: 10;
}

.modal {
  background: #faf9f6;
  border-radius: 6px;
  padding: 2rem;
  width: min(420px, 100%);
  display: flex;
  flex-direction: column;
  gap: 1rem;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.18);
}

.modal h2 {
  margin: 0 0 0.5rem;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.field > span {
  color: #6b6b6b;
  font-size: 0.875rem;
}

.field input {
  font: inherit;
  padding: 0.5rem 0.75rem;
  border: 1px solid #d8d6cf;
  border-radius: 4px;
  background: #fffefb;
}

.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
  align-items: center;
}

.chip {
  font: inherit;
  font-size: 0.875rem;
  padding: 0.25rem 0.7rem;
  border: 1px solid #c8c5bd;
  background: transparent;
  border-radius: 999px;
  cursor: pointer;
}

.chip.on {
  background: #1a1a1a;
  color: #faf9f6;
  border-color: #1a1a1a;
}

.chip-input {
  width: 4ch;
  padding: 0.25rem 0.5rem;
  border: 1px dashed #c8c5bd;
  border-radius: 999px;
  background: transparent;
  font: inherit;
  font-size: 0.875rem;
  text-align: center;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.modal-actions button {
  padding: 0.5rem 1.25rem;
  font: inherit;
  background: white;
  border: 1px solid #1a1a1a;
  border-radius: 4px;
  cursor: pointer;
}

.modal-actions button[type="submit"] {
  background: #1a1a1a;
  color: #faf9f6;
}
```

- [ ] **Step 5: Smoke-build**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop
pnpm tsc -b
pnpm build
```

- [ ] **Step 6: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src
git commit -m "feat(library): card grid + new project modal"
```

---

## Task 11: Workspace placeholder route (real)

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx` (replace stub with a project-loading view)

- [ ] **Step 1: Replace `Workspace.tsx`**

```tsx
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { projects as projectsApi } from "../lib/rpc";
import type { Project } from "../lib/types";

export function Workspace() {
  const { projectId } = useParams();
  const [project, setProject] = useState<Project | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!projectId) return;
    projectsApi.get(projectId).then(setProject).catch((e) => setError(String(e)));
  }, [projectId]);

  return (
    <main className="shell">
      <p>
        <Link to="/">← Library</Link>
      </p>
      {error && <p className="error">{error}</p>}
      {!error && !project && <p className="hint">불러오는 중…</p>}
      {project && (
        <>
          <h2>{project.title}</h2>
          <p className="hint">
            장르: {project.genres.join(", ") || "—"} · {project.length_target} · {project.default_pov}
          </p>
          <p className="hint">
            <code>씬 1</code>의 본문 편집기는 Plan 2에서 추가됩니다.
          </p>
        </>
      )}
    </main>
  );
}
```

- [ ] **Step 2: Smoke**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop
pnpm tsc -b
```

- [ ] **Step 3: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(workspace): fetch and display project metadata (placeholder)"
```

---

## Task 12: Full Library page (archive view)

**Files:**
- Modify: `apps/desktop/src/routes/LibraryAll.tsx`

- [ ] **Step 1: Replace `LibraryAll.tsx`**

```tsx
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { projects as projectsApi } from "../lib/rpc";
import type { Project } from "../lib/types";

type Tab = "active" | "archived";

export function LibraryAll() {
  const [tab, setTab] = useState<Tab>("active");
  const [items, setItems] = useState<Project[]>([]);
  const [error, setError] = useState<string | null>(null);

  const load = async (which: Tab) => {
    setError(null);
    try {
      const all = await projectsApi.list({ include_archived: true, limit: 200 });
      const filtered = which === "active"
        ? all.filter((p) => !p.archived_at)
        : all.filter((p) => !!p.archived_at);
      setItems(filtered);
    } catch (e) {
      setError(String(e));
    }
  };

  useEffect(() => { load(tab); }, [tab]);

  const archive = async (id: string) => {
    await projectsApi.archive(id);
    await load(tab);
  };

  return (
    <main className="shell library-all">
      <p>
        <Link to="/">← Library</Link>
      </p>
      <h2>전체 라이브러리</h2>

      <div className="tabs">
        <button className={`chip${tab === "active" ? " on" : ""}`} onClick={() => setTab("active")}>
          진행 중
        </button>
        <button className={`chip${tab === "archived" ? " on" : ""}`} onClick={() => setTab("archived")}>
          보관됨
        </button>
      </div>

      {error && <p className="error">{error}</p>}

      <ul className="all-list">
        {items.map((p) => (
          <li key={p.id} className="all-row">
            <Link to={`/workspace/${p.id}`} className="all-row-link">
              <span className="all-row-title">{p.title}</span>
              <span className="all-row-meta">{p.length_target} · {p.word_count}자</span>
            </Link>
            {tab === "active" && (
              <button className="chip" onClick={() => archive(p.id)}>아카이브</button>
            )}
          </li>
        ))}
        {items.length === 0 && <li className="hint">없음</li>}
      </ul>
    </main>
  );
}
```

- [ ] **Step 2: Add styles**

Append to `apps/desktop/src/App.css`:

```css
.library-all {
  max-width: 720px;
  margin: 0 auto;
  padding: 2rem;
}

.tabs {
  display: flex;
  gap: 0.5rem;
  margin: 1rem 0;
}

.all-list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.all-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 0;
  border-bottom: 1px solid #e3e1d9;
}

.all-row-link {
  flex: 1;
  display: flex;
  justify-content: space-between;
  text-decoration: none;
  color: inherit;
  gap: 1rem;
}

.all-row-meta {
  color: #6b6b6b;
  font-size: 0.875rem;
}
```

- [ ] **Step 3: Smoke**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop
pnpm tsc -b
pnpm build
```

- [ ] **Step 4: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/routes/LibraryAll.tsx apps/desktop/src/App.css
git commit -m "feat(library): full library page with archive tab"
```

---

## Task 13: End-to-end smoke + milestone tag

This task is interactive. The controller runs `./scripts/dev.sh` and walks through the flow with the user. No code changes.

- [ ] **Step 1: Use a clean LINETTA_HOME for the demo**

```bash
rm -rf /tmp/linetta-plan1
LINETTA_HOME=/tmp/linetta-plan1 ./scripts/dev.sh
```

The Tauri window opens. Because there are 0 projects, the modal opens automatically.

- [ ] **Step 2: Create a project**

In the modal:
- 제목: "은하의 노래"
- 장르: SF, 문학
- 예상 분량: 단편
- 기본 시점: 1인칭

Click `시작`. The window should navigate to `/workspace/<uuid>` and show project metadata, with the "Plan 2에서 추가" note.

- [ ] **Step 3: Return to Library, confirm card**

Click `← Library`. The card grid shows one card titled "은하의 노래" with meta "단편 · 초안 시작 전".

- [ ] **Step 4: Verify archive flow**

Click `+ 새 작품` and create another project ("두 번째"). Click `← Library` — there should now be 2 cards. Click `전체 라이브러리 →` (only visible if 5+ items — for this smoke, you may skip this step and instead navigate to `/library/all` via address bar / by adding 4 more projects).

In `/library/all`, click "아카이브" next to one project. It moves to the "보관됨" tab.

- [ ] **Step 5: Confirm engine logs are clean**

In the terminal where `./scripts/dev.sh` ran, scroll back — there should be no panic output from either Rust or Go.

- [ ] **Step 6: Stop dev and tag**

```bash
# Ctrl-C the dev process.
cd /Users/changheonshin/workspace/myworks/linetta
git tag plan-1-library-done
```

---

## Self-review checklist (run after writing the plan, not at execution time)

1. **Spec coverage**
   - §3 (Data Model): full schema applied in Task 3. ✓
   - §4.1 (Library): card grid + 전체 라이브러리 link in Tasks 10 + 12. ✓
   - §4.2 (New Project): modal with title/genres/length/POV + auto first leaf in Tasks 5 + 10. ✓
   - §6 (Library behavior): empty → modal autoplay, recent-first sort, archive flow, last_opened_node_id. ✓ (last_opened_node_id is set on create — Plan 2 will throttled-update it as the user activates leaves.)
   - §11.1 items 1–2: bootable .app (already in Plan 0), Library + new-project modal + archive page. ✓

2. **Placeholder scan**
   - "Plan 2에서 편집 가능" appears in Workspace.tsx — intentional placeholder, called out as such.
   - Settings.tsx says "Plan 6에서 추가됨" — intentional placeholder.
   - All code blocks contain complete content; no TBD/TODO.

3. **Type consistency**
   - Go `project.Project` JSON tags ↔ TS `Project` interface — `length_target`, `default_pov`, `last_opened_node_id`, `archived_at`. ✓
   - `projects.create` / `projects.list` / `projects.get` / `projects.archive` JSONRPC methods match handler registrations in Task 7's `main.go`. ✓
   - Tauri command `engine_call(method, params)` exists in `lib.rs` (Task 9) and is invoked by `rpcCall()` in `rpc.ts` (Task 9). ✓

4. **Cross-task dependencies**
   - Task 4 → 5 (Store before Repo)
   - Task 5 → 6 (Repo before handlers)
   - Task 6 → 7 (handlers before main wiring)
   - Task 7 → 13 (engine wired before smoke)
   - Task 8 → 9 → 10 (router → API client → Library UI)
   - Task 10 + 11 + 12 are independent UI screens, all needed for Task 13.

---

## Definition of Done

Plan 1 is done when ALL of the following are true:

- `cd engine && go test ./...` is green.
- `cd apps/desktop && pnpm tsc -b && pnpm build` is green.
- `cd apps/desktop/src-tauri && cargo check` is green.
- Running `LINETTA_HOME=/tmp/linetta-plan1 ./scripts/dev.sh` opens a Tauri window.
- Empty library shows the new-project modal automatically. Submitting it creates a project and routes to `/workspace/<id>`.
- Going back to `/` shows the new project as a card.
- `/library/all` lets you archive a project; archived items appear under the "보관됨" tab.
- Tag `plan-1-library-done` exists.

When done, the next plan is **Plan 2 — Nodes + Workspace Edit mode (no @mention)**.
