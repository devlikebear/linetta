# Plan 0 — Greenfield reset + Tauri/Go/React scaffold

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wipe the existing Linetta codebase and stand up a clean Tauri 2 (Rust) + React/Vite + Go engine architecture with a stdio JSONRPC bridge proven by a `ping → pong` round-trip from the React UI all the way to the Go engine.

**Architecture:** Tauri shell spawns the Go engine binary as a stdio sidecar. The shell exposes Tauri commands to the React frontend; each command translates to a JSONRPC call written to the engine's stdin and reads the response from its stdout. The engine ships as a separate binary built per-target-triple and bundled into the `.app` by Tauri.

**Tech Stack:**
- Go 1.25 (engine)
- Tauri 2.x (Rust shell)
- React 18 + Vite 5 + TypeScript (frontend)
- `github.com/devlikebear/tars` v0.32.72 (only the `pkg/llm` subtree is used; imported in this plan to validate the module path even though no LLM call is made yet)
- pnpm for the JS workspace

**Spec reference:** `docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md` — §2 (Architecture), §12 (Repo Structure), §13 (Build and Dev Workflow).

---

## Pre-flight

This plan deletes a working application. Verify before starting:

- [ ] **Step P1: Confirm no uncommitted work**

Run: `git status --short`
Expected: empty output. If not, commit or stash first.

- [ ] **Step P2: Confirm the design doc and Plan 0 are tracked**

Run: `git log --oneline -- docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md`
Expected: at least one commit.

- [ ] **Step P3: Confirm developer toolchain present**

Run each and verify version:
```bash
go version              # expected: go1.25.x
node --version          # expected: v20+ (Vite 5 requires Node 18+)
pnpm --version          # expected: 8+ — install via `npm i -g pnpm` if missing
cargo --version         # expected: 1.78+ — Tauri 2 needs recent Rust
rustup target list --installed   # macOS: aarch64-apple-darwin and/or x86_64-apple-darwin
```

If `cargo-tauri` is missing, install: `cargo install tauri-cli --version '^2.0.0' --locked`.

- [ ] **Step P4: Confirm tars module is accessible**

Run: `go env GOPROXY` then `GOFLAGS=-mod=mod go list -m github.com/devlikebear/tars@v0.32.72`
Expected: prints `github.com/devlikebear/tars v0.32.72` with no error. If your environment uses a local replace directive elsewhere, you can use that instead — note the choice for Task 3.

---

## File Structure (created by this plan)

```
linetta/
├── apps/
│   └── desktop/
│       ├── package.json
│       ├── pnpm-lock.yaml          (generated)
│       ├── index.html
│       ├── vite.config.ts
│       ├── tsconfig.json
│       ├── tsconfig.node.json
│       ├── src/
│       │   ├── main.tsx
│       │   ├── App.tsx
│       │   ├── App.css
│       │   └── vite-env.d.ts
│       └── src-tauri/
│           ├── Cargo.toml
│           ├── tauri.conf.json
│           ├── build.rs
│           ├── icons/              (placeholder PNGs from `cargo tauri init`)
│           ├── binaries/           (engine binary lands here pre-build; gitignored)
│           └── src/
│               ├── main.rs
│               ├── engine.rs
│               └── jsonrpc.rs
├── engine/
│   ├── go.mod
│   ├── go.sum                      (generated)
│   ├── cmd/
│   │   └── linetta-engine/
│   │       └── main.go
│   └── internal/
│       └── rpc/
│           ├── codec.go
│           ├── codec_test.go
│           ├── server.go
│           ├── server_test.go
│           └── handlers/
│               ├── ping.go
│               └── ping_test.go
├── scripts/
│   ├── build-engine.sh
│   └── dev.sh
├── .gitignore                      (rewritten)
└── README.md                       (rewritten)
```

Deleted by this plan: `bin/`, `cmd/`, `examples/`, `internal/`, `macos/`, `docs/plan/`, `Makefile`, top-level `go.mod`, top-level `go.sum`, `.linetta/`, `.tessera/`, top-level `README.md`.

Preserved: `docs/superpowers/`, `LICENSE` (if present), `.git/`.

---

## Task 1: Wipe the previous codebase

**Files:**
- Delete: `bin/`, `cmd/`, `examples/`, `internal/`, `macos/`, `docs/plan/`, `Makefile`, `go.mod`, `go.sum`, `.linetta/`, `.tessera/`, `README.md`
- Keep untouched: `docs/superpowers/`, `LICENSE` (if exists), `.git/`, `.gitignore` (rewritten in Task 2)

- [ ] **Step 1: Take a safety branch tag in case of regret**

```bash
git tag pre-greenfield-$(date +%Y%m%d)
```
Expected: silent success.

- [ ] **Step 2: Remove the previous code**

```bash
git rm -rf bin cmd examples internal macos docs/plan Makefile go.mod go.sum README.md
rm -rf .linetta .tessera
```
Expected: long list of `rm` lines. Untracked `.linetta`/`.tessera` directories disappear (they were gitignored).

- [ ] **Step 3: Verify only expected top-level items remain**

Run: `ls -1A`
Expected output set (no extras, no unexpected files):
```
.git
.gitignore
.superpowers
LICENSE
docs
```
If `LICENSE` is missing, that is acceptable — note it for later but do not block.

- [ ] **Step 4: Commit the reset**

```bash
git add -A
git commit -m "chore(reset): wipe pre-rebuild codebase (Plan 0)"
```
Expected: a commit with deletions only.

---

## Task 2: Rewrite `.gitignore` and add stub `README.md`

**Files:**
- Modify: `.gitignore`
- Create: `README.md`

- [ ] **Step 1: Overwrite `.gitignore`**

Write the file `/Users/changheonshin/workspace/myworks/linetta/.gitignore`:
```gitignore
# OS
.DS_Store

# Brainstorm scratch
.superpowers/

# Go
*.test
*.out
/engine/bin/

# Node / pnpm / Vite
node_modules/
apps/desktop/dist/
apps/desktop/.vite/

# Rust / Tauri
apps/desktop/src-tauri/target/
apps/desktop/src-tauri/binaries/

# Local app data (engine writes here in dev when LINETTA_HOME=./.linetta)
/.linetta/

# IDE
.idea/
.vscode/
```

- [ ] **Step 2: Write a stub `README.md`**

Write `/Users/changheonshin/workspace/myworks/linetta/README.md`:
```markdown
# Linetta

Immersive desktop writing app. Greenfield rebuild in progress.

See `docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md`
for the design and `docs/superpowers/plans/` for the implementation plans.

## Stack

- Tauri 2 (Rust shell) + React 18 + Vite + TypeScript
- Go 1.25 engine with `github.com/devlikebear/tars` as the LLM library

## Dev (after Plan 0)

```sh
./scripts/dev.sh
```
```

- [ ] **Step 3: Commit**

```bash
git add .gitignore README.md
git commit -m "chore(scaffold): gitignore + stub README"
```

---

## Task 3: Initialize the Go engine module

**Files:**
- Create: `engine/go.mod`
- Create: `engine/cmd/linetta-engine/main.go` (stub)

- [ ] **Step 1: Create the module**

```bash
mkdir -p engine/cmd/linetta-engine
cd engine
go mod init github.com/devlikebear/linetta/engine
cd ..
```
Expected: `engine/go.mod` created with one line: `module github.com/devlikebear/linetta/engine` and a `go 1.25...` directive.

- [ ] **Step 2: Add `tars` dependency**

```bash
cd engine
go get github.com/devlikebear/tars@v0.32.72
cd ..
```
Expected: `engine/go.mod` now has `require github.com/devlikebear/tars v0.32.72` (plus indirect deps). `engine/go.sum` created.

- [ ] **Step 3: Write a placeholder `main.go` that compiles**

Write `/Users/changheonshin/workspace/myworks/linetta/engine/cmd/linetta-engine/main.go`:
```go
package main

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/devlikebear/tars/pkg/llm" // pin import to validate module path
)

func main() {
	stdio := flag.Bool("stdio", false, "serve JSONRPC over stdin/stdout")
	flag.Parse()

	if !*stdio {
		fmt.Fprintln(os.Stderr, "linetta-engine: --stdio required (other modes land in later plans)")
		os.Exit(2)
	}

	// Wired up in Task 6.
	fmt.Fprintln(os.Stderr, "linetta-engine: stdio mode placeholder")
}
```

- [ ] **Step 4: Verify it builds**

```bash
cd engine
go build ./...
cd ..
```
Expected: silent success. `engine/cmd/linetta-engine/linetta-engine` may or may not appear depending on build mode — clean it: `rm -f engine/cmd/linetta-engine/linetta-engine`.

- [ ] **Step 5: Commit**

```bash
git add engine/go.mod engine/go.sum engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): init Go module with tars/pkg/llm import"
```

---

## Task 4: JSONRPC line-delimited codec (TDD)

We use newline-delimited JSON (NDJSON) — one JSONRPC message per line — for stdio framing. JSON-encoded Tiptap docs contain no raw newlines (JSON escapes them), so this is safe and trivial to parse.

**Files:**
- Create: `engine/internal/rpc/codec.go`
- Create: `engine/internal/rpc/codec_test.go`

- [ ] **Step 1: Write the failing test**

Write `/Users/changheonshin/workspace/myworks/linetta/engine/internal/rpc/codec_test.go`:
```go
package rpc

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadMessage_singleLine(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	c := NewCodec(in, &bytes.Buffer{})

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if msg.Method != "ping" {
		t.Errorf("method = %q, want %q", msg.Method, "ping")
	}
	if msg.ID == nil || string(msg.ID) != "1" {
		t.Errorf("id raw = %q, want %q", string(msg.ID), "1")
	}
}

func TestReadMessage_eof(t *testing.T) {
	in := strings.NewReader("")
	c := NewCodec(in, &bytes.Buffer{})

	_, err := c.Read()
	if err == nil {
		t.Fatal("Read on empty stream should return io.EOF, got nil")
	}
}

func TestWriteMessage_appendsNewline(t *testing.T) {
	out := &bytes.Buffer{}
	c := NewCodec(strings.NewReader(""), out)

	if err := c.Write(Message{
		JSONRPC: "2.0",
		ID:      RawID(`7`),
		Result:  RawJSON(`"pong"`),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := out.String()
	want := `{"jsonrpc":"2.0","id":7,"result":"pong"}` + "\n"
	if got != want {
		t.Errorf("write: got %q, want %q", got, want)
	}
}

func TestWriteMessage_notification(t *testing.T) {
	out := &bytes.Buffer{}
	c := NewCodec(strings.NewReader(""), out)

	if err := c.Write(Message{
		JSONRPC: "2.0",
		Method:  "ai.delta",
		Params:  RawJSON(`{"text":"hi"}`),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := out.String()
	want := `{"jsonrpc":"2.0","method":"ai.delta","params":{"text":"hi"}}` + "\n"
	if got != want {
		t.Errorf("write: got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the test — expect compile failure**

```bash
cd engine
go test ./internal/rpc/...
cd ..
```
Expected: build error mentioning `NewCodec`, `Message`, `RawID`, `RawJSON` undefined.

- [ ] **Step 3: Implement the codec**

Write `/Users/changheonshin/workspace/myworks/linetta/engine/internal/rpc/codec.go`:
```go
// Package rpc implements a minimal newline-delimited JSONRPC 2.0 codec
// over arbitrary io.Reader/Writer pairs (typically os.Stdin/os.Stdout).
package rpc

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
)

// RawJSON is a json.RawMessage alias for clarity in this package.
type RawJSON = json.RawMessage

// RawID is the raw bytes of a JSONRPC id, which may be a number or a string.
// Using json.RawMessage avoids forcing a type choice at the codec layer.
type RawID = json.RawMessage

// Message is a JSONRPC 2.0 frame — request, response, or notification.
// Only fields with non-zero values are emitted on the wire.
type Message struct {
	JSONRPC string  `json:"jsonrpc"`
	ID      RawID   `json:"id,omitempty"`
	Method  string  `json:"method,omitempty"`
	Params  RawJSON `json:"params,omitempty"`
	Result  RawJSON `json:"result,omitempty"`
	Error   *Error  `json:"error,omitempty"`
}

// Error is the JSONRPC error object.
type Error struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Data    RawJSON `json:"data,omitempty"`
}

// Codec reads and writes newline-delimited JSONRPC messages.
type Codec struct {
	r *bufio.Reader
	w io.Writer
}

// NewCodec returns a Codec backed by the given streams.
func NewCodec(r io.Reader, w io.Writer) *Codec {
	return &Codec{
		r: bufio.NewReaderSize(r, 1<<20), // 1 MiB; Tiptap docs fit comfortably
		w: w,
	}
}

// Read reads one message. Returns io.EOF when the stream is closed.
func (c *Codec) Read() (Message, error) {
	line, err := c.r.ReadBytes('\n')
	if len(line) == 0 && err != nil {
		return Message{}, err
	}
	// Strip the trailing newline (and possibly a stray CR on Windows pipes).
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	if len(line) == 0 {
		return Message{}, errors.New("rpc: empty message line")
	}
	var msg Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

// Write writes one message followed by a newline.
func (c *Codec) Write(m Message) error {
	if m.JSONRPC == "" {
		m.JSONRPC = "2.0"
	}
	buf, err := json.Marshal(m)
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	_, err = c.w.Write(buf)
	return err
}
```

- [ ] **Step 4: Run the tests — expect pass**

```bash
cd engine
go test ./internal/rpc/...
cd ..
```
Expected: `ok   github.com/devlikebear/linetta/engine/internal/rpc`.

- [ ] **Step 5: Commit**

```bash
git add engine/internal/rpc/codec.go engine/internal/rpc/codec_test.go
git commit -m "feat(rpc): NDJSON JSONRPC codec"
```

---

## Task 5: RPC handler registry + `ping` handler (TDD)

**Files:**
- Create: `engine/internal/rpc/server.go`
- Create: `engine/internal/rpc/server_test.go`
- Create: `engine/internal/rpc/handlers/ping.go`
- Create: `engine/internal/rpc/handlers/ping_test.go`

- [ ] **Step 1: Write the failing handler test**

Write `/Users/changheonshin/workspace/myworks/linetta/engine/internal/rpc/handlers/ping_test.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPing(t *testing.T) {
	got, err := Ping(context.Background(), nil)
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	var s string
	if err := json.Unmarshal(got, &s); err != nil {
		t.Fatalf("Ping result not JSON string: %v (raw=%s)", err, string(got))
	}
	if s != "pong" {
		t.Errorf("ping = %q, want %q", s, "pong")
	}
}
```

- [ ] **Step 2: Run — expect build failure (Ping undefined)**

```bash
cd engine
go test ./internal/rpc/handlers/...
cd ..
```
Expected: `undefined: Ping`.

- [ ] **Step 3: Implement `Ping`**

Write `/Users/changheonshin/workspace/myworks/linetta/engine/internal/rpc/handlers/ping.go`:
```go
// Package handlers contains the RPC method implementations exposed by the
// Linetta engine.
package handlers

import (
	"context"
	"encoding/json"
)

// Ping is the proof-of-life handler. It ignores its params and returns "pong".
func Ping(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`"pong"`), nil
}
```

- [ ] **Step 4: Run — expect pass**

```bash
cd engine
go test ./internal/rpc/handlers/...
cd ..
```
Expected: PASS.

- [ ] **Step 5: Write the failing server test**

Write `/Users/changheonshin/workspace/myworks/linetta/engine/internal/rpc/server_test.go`:
```go
package rpc

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestServer_ping_roundtrip(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	out := &lineCapture{}

	s := NewServer()
	s.Handle("ping", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"pong"`), nil
	})

	if err := s.Serve(context.Background(), in, out); err != nil && err != io.EOF {
		t.Fatalf("Serve: %v", err)
	}

	want := `{"jsonrpc":"2.0","id":1,"result":"pong"}` + "\n"
	if got := out.String(); got != want {
		t.Errorf("response = %q, want %q", got, want)
	}
}

func TestServer_method_not_found(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"nope"}` + "\n")
	out := &lineCapture{}

	s := NewServer()
	if err := s.Serve(context.Background(), in, out); err != nil && err != io.EOF {
		t.Fatalf("Serve: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `"code":-32601`) {
		t.Errorf("expected method-not-found code in %q", got)
	}
	if !strings.Contains(got, `"id":2`) {
		t.Errorf("expected id=2 in %q", got)
	}
}

func TestServer_notification_is_silent(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"ping"}` + "\n")
	out := &lineCapture{}

	s := NewServer()
	s.Handle("ping", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"pong"`), nil
	})

	if err := s.Serve(context.Background(), in, out); err != nil && err != io.EOF {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("notification produced output: %q", out.String())
	}
}

// lineCapture is a tiny io.Writer that also exposes the buffer string.
type lineCapture struct{ b strings.Builder }

func (l *lineCapture) Write(p []byte) (int, error) { return l.b.Write(p) }
func (l *lineCapture) String() string              { return l.b.String() }
func (l *lineCapture) Len() int                    { return l.b.Len() }
```

- [ ] **Step 6: Run — expect build failure (`NewServer`, `Serve`, `Handle` undefined)**

```bash
cd engine
go test ./internal/rpc/...
cd ..
```

- [ ] **Step 7: Implement the server**

Write `/Users/changheonshin/workspace/myworks/linetta/engine/internal/rpc/server.go`:
```go
package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// Handler is the signature for a JSONRPC method implementation. It receives a
// context (cancelled when Serve returns) and the raw params; it must return
// either a JSON-encoded result or an error.
type Handler func(ctx context.Context, params json.RawMessage) (json.RawMessage, error)

// Standard JSONRPC error codes (subset).
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// MethodError lets a handler return a typed JSONRPC error.
type MethodError struct {
	Code    int
	Message string
	Data    json.RawMessage
}

func (e *MethodError) Error() string { return e.Message }

// Server is a tiny JSONRPC 2.0 server dispatching to in-process handlers.
// One Server instance is bound to one stdio pair via Serve.
type Server struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewServer returns a Server with no handlers registered.
func NewServer() *Server { return &Server{handlers: map[string]Handler{}} }

// Handle registers a handler for a method. Overwrites previous registrations
// silently — registration happens once at boot.
func (s *Server) Handle(method string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = h
}

// Serve reads one message at a time from r and writes responses to w. It
// returns io.EOF when r is exhausted. Notifications (id == nil) never get a
// response. Each request is dispatched on the calling goroutine — concurrency
// is the writer's responsibility to add later if needed.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	codec := NewCodec(r, w)
	for {
		msg, err := codec.Read()
		if errors.Is(err, io.EOF) {
			return io.EOF
		}
		if err != nil {
			// Best-effort parse-error response with null id.
			_ = codec.Write(Message{
				ID:    json.RawMessage(`null`),
				Error: &Error{Code: CodeParseError, Message: err.Error()},
			})
			continue
		}

		isNotification := len(msg.ID) == 0
		s.mu.RLock()
		h, ok := s.handlers[msg.Method]
		s.mu.RUnlock()

		if !ok {
			if !isNotification {
				_ = codec.Write(Message{
					ID:    msg.ID,
					Error: &Error{Code: CodeMethodNotFound, Message: "method not found: " + msg.Method},
				})
			}
			continue
		}

		result, herr := h(ctx, msg.Params)
		if isNotification {
			continue
		}
		if herr != nil {
			var me *MethodError
			if errors.As(herr, &me) {
				_ = codec.Write(Message{ID: msg.ID, Error: &Error{Code: me.Code, Message: me.Message, Data: me.Data}})
			} else {
				_ = codec.Write(Message{ID: msg.ID, Error: &Error{Code: CodeInternalError, Message: herr.Error()}})
			}
			continue
		}
		_ = codec.Write(Message{ID: msg.ID, Result: result})
	}
}
```

- [ ] **Step 8: Run — expect pass**

```bash
cd engine
go test ./...
cd ..
```
Expected: all packages PASS.

- [ ] **Step 9: Commit**

```bash
git add engine/internal/rpc/server.go engine/internal/rpc/server_test.go engine/internal/rpc/handlers/ping.go engine/internal/rpc/handlers/ping_test.go
git commit -m "feat(rpc): server with handler registry and ping handler"
```

---

## Task 6: Wire `main.go` to the RPC server + manual stdio smoke test

**Files:**
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1: Replace the placeholder `main.go`**

Write `/Users/changheonshin/workspace/myworks/linetta/engine/cmd/linetta-engine/main.go`:
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

	_ "github.com/devlikebear/tars/pkg/llm" // validate module path; LLM wiring lands in Plan 5

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
)

func main() {
	stdio := flag.Bool("stdio", false, "serve JSONRPC over stdin/stdout")
	flag.Parse()

	if !*stdio {
		fmt.Fprintln(os.Stderr, "linetta-engine: --stdio required (other modes land in later plans)")
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	s := rpc.NewServer()
	s.Handle("ping", handlers.Ping)

	if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(os.Stderr, "linetta-engine: serve: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build**

```bash
cd engine
go build ./...
cd ..
```
Expected: silent success.

- [ ] **Step 3: Manual smoke test (one-shot ping)**

```bash
cd engine
echo '{"jsonrpc":"2.0","id":1,"method":"ping"}' | go run ./cmd/linetta-engine --stdio
cd ..
```
Expected stdout:
```
{"jsonrpc":"2.0","id":1,"result":"pong"}
```

- [ ] **Step 4: Commit**

```bash
git add engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): wire main to RPC server (ping ready)"
```

---

## Task 7: Initialize the Tauri 2 app + React/Vite skeleton

**Files:**
- Create: `apps/desktop/package.json`
- Create: `apps/desktop/index.html`
- Create: `apps/desktop/vite.config.ts`
- Create: `apps/desktop/tsconfig.json`
- Create: `apps/desktop/tsconfig.node.json`
- Create: `apps/desktop/src/main.tsx`
- Create: `apps/desktop/src/App.tsx`
- Create: `apps/desktop/src/App.css`
- Create: `apps/desktop/src/vite-env.d.ts`
- Create: `apps/desktop/src-tauri/Cargo.toml`
- Create: `apps/desktop/src-tauri/tauri.conf.json`
- Create: `apps/desktop/src-tauri/build.rs`
- Create: `apps/desktop/src-tauri/src/main.rs` (placeholder; wired in Task 8/9)
- Create: `apps/desktop/src-tauri/icons/` (use `cargo tauri icon` later; for Plan 0 ship a 32x32 PNG placeholder)

We avoid `cargo tauri init` because it is interactive and creates a lot of files we want to control. We write the files directly.

- [ ] **Step 1: Make directories**

```bash
mkdir -p apps/desktop/src apps/desktop/src-tauri/src apps/desktop/src-tauri/icons apps/desktop/src-tauri/binaries
```

- [ ] **Step 2: Write `apps/desktop/package.json`**

```json
{
  "name": "linetta-desktop",
  "version": "0.0.1",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview",
    "tauri": "tauri"
  },
  "dependencies": {
    "@tauri-apps/api": "^2.0.0",
    "react": "^18.3.1",
    "react-dom": "^18.3.1"
  },
  "devDependencies": {
    "@tauri-apps/cli": "^2.0.0",
    "@types/react": "^18.3.3",
    "@types/react-dom": "^18.3.0",
    "@vitejs/plugin-react": "^4.3.1",
    "typescript": "^5.5.4",
    "vite": "^5.4.0"
  }
}
```

- [ ] **Step 3: Write `apps/desktop/vite.config.ts`**

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Tauri dev server config: fixed port, no minification of dev assets.
// https://tauri.app/v2/guides/getting-started/setup/vite/
export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
  },
  envPrefix: ["VITE_", "TAURI_"],
  build: {
    target: "es2021",
    minify: !process.env.TAURI_DEBUG ? "esbuild" : false,
    sourcemap: !!process.env.TAURI_DEBUG,
  },
});
```

- [ ] **Step 4: Write `apps/desktop/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2021",
    "useDefineForClassFields": true,
    "lib": ["ES2021", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

- [ ] **Step 5: Write `apps/desktop/tsconfig.node.json`**

```json
{
  "compilerOptions": {
    "composite": true,
    "skipLibCheck": true,
    "module": "ESNext",
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true,
    "strict": true
  },
  "include": ["vite.config.ts"]
}
```

- [ ] **Step 6: Write `apps/desktop/index.html`**

```html
<!doctype html>
<html lang="ko">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Linetta</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 7: Write `apps/desktop/src/main.tsx`**

```tsx
import React from "react";
import ReactDOM from "react-dom/client";
import { App } from "./App";
import "./App.css";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");

ReactDOM.createRoot(root).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
```

- [ ] **Step 8: Write `apps/desktop/src/App.tsx` (placeholder; wired to Tauri in Task 10)**

```tsx
import { useState } from "react";

export function App() {
  const [status, setStatus] = useState<string>("idle");
  return (
    <main className="shell">
      <h1>Linetta</h1>
      <p className="hint">Engine bridge smoke test</p>
      <button
        type="button"
        onClick={() => setStatus("not wired yet — see Task 10")}
      >
        Ping engine
      </button>
      <p className="status">{status}</p>
    </main>
  );
}
```

- [ ] **Step 9: Write `apps/desktop/src/App.css`**

```css
:root {
  color-scheme: light dark;
  font-family: ui-serif, Georgia, "Apple SD Gothic Neo", serif;
  font-size: 16px;
}

body {
  margin: 0;
  background: #faf9f6;
  color: #1a1a1a;
}

.shell {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
  padding: 4rem 2rem;
}

.shell h1 {
  font-size: 2rem;
  margin: 0;
}

.shell .hint {
  color: #6b6b6b;
  margin: 0;
}

.shell button {
  padding: 0.5rem 1.25rem;
  font-family: inherit;
  font-size: 1rem;
  background: white;
  border: 1px solid #1a1a1a;
  border-radius: 4px;
  cursor: pointer;
}

.shell .status {
  font-family: ui-monospace, SFMono-Regular, monospace;
  color: #1a1a1a;
}
```

- [ ] **Step 10: Write `apps/desktop/src/vite-env.d.ts`**

```ts
/// <reference types="vite/client" />
```

- [ ] **Step 11: Write `apps/desktop/src-tauri/Cargo.toml`**

```toml
[package]
name = "linetta-desktop"
version = "0.0.1"
edition = "2021"
rust-version = "1.78"

[lib]
name = "linetta_desktop_lib"
crate-type = ["staticlib", "cdylib", "rlib"]

[build-dependencies]
tauri-build = { version = "2", features = [] }

[dependencies]
tauri = { version = "2", features = [] }
tauri-plugin-shell = "2"
serde = { version = "1", features = ["derive"] }
serde_json = "1"
tokio = { version = "1", features = ["rt-multi-thread", "io-util", "process", "sync", "macros", "time"] }
anyhow = "1"
once_cell = "1"

[features]
default = ["custom-protocol"]
custom-protocol = ["tauri/custom-protocol"]
```

- [ ] **Step 12: Write `apps/desktop/src-tauri/tauri.conf.json`**

`externalBin` references the path stem; Tauri appends `-{target-triple}` and the OS executable suffix. `build-engine.sh` (Task 11) produces those.

```json
{
  "$schema": "https://schema.tauri.app/config/2",
  "productName": "Linetta",
  "version": "0.0.1",
  "identifier": "com.devlikebear.linetta",
  "build": {
    "frontendDist": "../dist",
    "devUrl": "http://localhost:1420",
    "beforeDevCommand": "pnpm dev",
    "beforeBuildCommand": "pnpm build"
  },
  "app": {
    "windows": [
      {
        "title": "Linetta",
        "width": 1100,
        "height": 720,
        "minWidth": 720,
        "minHeight": 480,
        "resizable": true,
        "fullscreen": false
      }
    ],
    "security": {
      "csp": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ipc: http://ipc.localhost"
    }
  },
  "bundle": {
    "active": true,
    "targets": ["app", "dmg"],
    "icon": ["icons/icon.png"],
    "externalBin": ["binaries/linetta-engine"]
  }
}
```

- [ ] **Step 13: Write `apps/desktop/src-tauri/build.rs`**

```rust
fn main() {
    tauri_build::build();
}
```

- [ ] **Step 14: Write a placeholder `apps/desktop/src-tauri/src/main.rs` (will be replaced in Task 8)**

```rust
// Wired in Task 8.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    linetta_desktop_lib::run();
}
```

Also create `apps/desktop/src-tauri/src/lib.rs`:
```rust
// Wired in Task 8.
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
```

- [ ] **Step 15: Add a placeholder icon**

```bash
# Tauri 2 requires at least one icon. Use ImageMagick if present; otherwise download a 1x1 PNG.
if command -v magick >/dev/null; then
  magick -size 512x512 canvas:'#1a1a1a' apps/desktop/src-tauri/icons/icon.png
else
  printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x03\x00\x05\xfe\x02\xfe\xa78\xfd\x96\x00\x00\x00\x00IEND\xaeB`\x82' > apps/desktop/src-tauri/icons/icon.png
fi
```
Expected: a `icon.png` exists. Real icons are a polish task; this unblocks `tauri build`.

- [ ] **Step 16: Install JS deps**

```bash
cd apps/desktop
pnpm install
cd ../..
```
Expected: `pnpm-lock.yaml` created, `node_modules/` populated.

- [ ] **Step 17: Smoke-build the frontend alone**

```bash
cd apps/desktop
pnpm build
cd ../..
```
Expected: `apps/desktop/dist/` produced, no TS errors.

- [ ] **Step 18: Commit the scaffold**

```bash
git add apps/desktop/package.json apps/desktop/pnpm-lock.yaml apps/desktop/index.html apps/desktop/vite.config.ts apps/desktop/tsconfig.json apps/desktop/tsconfig.node.json apps/desktop/src apps/desktop/src-tauri/Cargo.toml apps/desktop/src-tauri/tauri.conf.json apps/desktop/src-tauri/build.rs apps/desktop/src-tauri/src apps/desktop/src-tauri/icons
git commit -m "feat(desktop): Tauri 2 + React/Vite scaffold"
```

---

## Task 8: Implement Rust JSONRPC stdio client + engine sidecar

**Files:**
- Create: `apps/desktop/src-tauri/src/jsonrpc.rs`
- Create: `apps/desktop/src-tauri/src/engine.rs`
- Modify: `apps/desktop/src-tauri/src/lib.rs`

This Rust code spawns the Go engine as a child process, owns its stdin/stdout, and provides an async `call(method, params) -> Result<Value>` API.

- [ ] **Step 1: Write `apps/desktop/src-tauri/src/jsonrpc.rs`**

```rust
//! NDJSON JSONRPC 2.0 client. One `Client` owns a child process's stdin/stdout
//! and serializes calls through a request/response map keyed by id.

use anyhow::{anyhow, Result};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::Arc;
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::process::{ChildStdin, ChildStdout};
use tokio::sync::{oneshot, Mutex};

#[derive(Serialize)]
struct Request<'a> {
    jsonrpc: &'a str,
    id: i64,
    method: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    params: Option<&'a Value>,
}

#[derive(Deserialize)]
struct Response {
    #[allow(dead_code)]
    jsonrpc: Option<String>,
    id: Option<Value>,
    #[serde(default)]
    result: Option<Value>,
    #[serde(default)]
    error: Option<RpcError>,
    #[serde(default)]
    method: Option<String>,
    #[serde(default)]
    params: Option<Value>,
}

#[derive(Deserialize, Debug)]
pub struct RpcError {
    pub code: i64,
    pub message: String,
}

type Pending = Arc<Mutex<std::collections::HashMap<i64, oneshot::Sender<Result<Value, RpcError>>>>>;

pub struct Client {
    next_id: AtomicI64,
    stdin: Mutex<ChildStdin>,
    pending: Pending,
}

impl Client {
    /// Spawn the reader task and return a Client. Notifications (no id) are
    /// dropped in Plan 0; the AI streaming work in Plan 5 will replace this.
    pub fn new(stdin: ChildStdin, stdout: ChildStdout) -> Arc<Self> {
        let pending: Pending = Arc::new(Mutex::new(Default::default()));
        let client = Arc::new(Client {
            next_id: AtomicI64::new(1),
            stdin: Mutex::new(stdin),
            pending: pending.clone(),
        });
        let pending_for_reader = pending.clone();
        tokio::spawn(async move {
            let mut reader = BufReader::new(stdout).lines();
            while let Ok(Some(line)) = reader.next_line().await {
                let resp: Response = match serde_json::from_str(&line) {
                    Ok(r) => r,
                    Err(_) => continue, // drop malformed lines
                };
                // Notifications (no id, has method) are ignored in Plan 0.
                if resp.method.is_some() && resp.id.is_none() {
                    let _ = resp.params; // touch field
                    continue;
                }
                let id = match resp.id.as_ref().and_then(|v| v.as_i64()) {
                    Some(n) => n,
                    None => continue,
                };
                let tx = pending_for_reader.lock().await.remove(&id);
                if let Some(tx) = tx {
                    let payload: Result<Value, RpcError> = if let Some(err) = resp.error {
                        Err(err)
                    } else {
                        Ok(resp.result.unwrap_or(Value::Null))
                    };
                    let _ = tx.send(payload);
                }
            }
        });
        client
    }

    pub async fn call(&self, method: &str, params: Option<Value>) -> Result<Value> {
        let id = self.next_id.fetch_add(1, Ordering::Relaxed);
        let (tx, rx) = oneshot::channel();
        self.pending.lock().await.insert(id, tx);

        let payload = serde_json::to_string(&Request {
            jsonrpc: "2.0",
            id,
            method,
            params: params.as_ref(),
        })?;
        {
            let mut stdin = self.stdin.lock().await;
            stdin.write_all(payload.as_bytes()).await?;
            stdin.write_all(b"\n").await?;
            stdin.flush().await?;
        }

        match rx.await {
            Ok(Ok(v)) => Ok(v),
            Ok(Err(e)) => Err(anyhow!("rpc error {}: {}", e.code, e.message)),
            Err(_) => Err(anyhow!("rpc channel closed before reply")),
        }
    }
}
```

- [ ] **Step 2: Write `apps/desktop/src-tauri/src/engine.rs`**

```rust
//! Engine lifecycle: locate the bundled `linetta-engine` binary, spawn it with
//! `--stdio`, and surface a `Client` for the rest of the app to use.

use crate::jsonrpc::Client;
use anyhow::{anyhow, Result};
use std::process::Stdio;
use std::sync::Arc;
use tauri::Manager;
use tokio::process::Command;

pub struct EngineHandle {
    pub client: Arc<Client>,
    // Keep the child alive for the duration of the app; dropping it kills the
    // process (Tokio's Drop terminates child if not awaited).
    pub _child: tokio::process::Child,
}

pub async fn spawn(app: &tauri::AppHandle) -> Result<EngineHandle> {
    let binary = resolve_binary(app)?;
    let mut child = Command::new(&binary)
        .arg("--stdio")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::inherit())
        .spawn()
        .map_err(|e| anyhow!("spawn {}: {}", binary.display(), e))?;

    let stdin = child.stdin.take().ok_or_else(|| anyhow!("child has no stdin"))?;
    let stdout = child.stdout.take().ok_or_else(|| anyhow!("child has no stdout"))?;

    let client = Client::new(stdin, stdout);
    Ok(EngineHandle { client, _child: child })
}

fn resolve_binary(app: &tauri::AppHandle) -> Result<std::path::PathBuf> {
    // In production: Tauri places externalBin entries in the resource dir,
    // postfixed with the target triple. In dev: scripts/dev.sh symlinks the
    // dev-built engine to apps/desktop/src-tauri/binaries/linetta-engine-{triple}.
    let triple = std::env::var("LINETTA_TARGET_TRIPLE")
        .or_else(|_| current_target_triple())
        .map_err(|e| anyhow!("resolve target triple: {}", e))?;
    let resource_name = format!("linetta-engine-{}{}", triple, std::env::consts::EXE_SUFFIX);

    if let Ok(path) = app.path().resolve(&resource_name, tauri::path::BaseDirectory::Resource) {
        if path.exists() {
            return Ok(path);
        }
    }
    // Dev fallback: alongside the running binary or in src-tauri/binaries/.
    let exe_dir = std::env::current_exe()
        .ok()
        .and_then(|p| p.parent().map(|p| p.to_path_buf()));
    if let Some(dir) = exe_dir {
        let dev_path = dir.join(&resource_name);
        if dev_path.exists() {
            return Ok(dev_path);
        }
    }
    let cwd_path = std::env::current_dir()?
        .join("src-tauri")
        .join("binaries")
        .join(&resource_name);
    if cwd_path.exists() {
        return Ok(cwd_path);
    }
    Err(anyhow!("engine binary not found: {}", resource_name))
}

fn current_target_triple() -> std::result::Result<String, std::env::VarError> {
    // Best-effort detection without pulling in another crate.
    let arch = if cfg!(target_arch = "aarch64") { "aarch64" } else { "x86_64" };
    let os = if cfg!(target_os = "macos") {
        "apple-darwin"
    } else if cfg!(target_os = "linux") {
        "unknown-linux-gnu"
    } else if cfg!(target_os = "windows") {
        "pc-windows-msvc"
    } else {
        return Err(std::env::VarError::NotPresent);
    };
    Ok(format!("{}-{}", arch, os))
}
```

- [ ] **Step 3: Replace `apps/desktop/src-tauri/src/lib.rs`**

```rust
mod engine;
mod jsonrpc;

use std::sync::Arc;
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
        .invoke_handler(tauri::generate_handler![engine_ping])
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
```

- [ ] **Step 4: Cargo check (catch syntax / type errors before the full build)**

```bash
cd apps/desktop/src-tauri
cargo check
cd ../../..
```
Expected: warnings allowed, errors none.

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src-tauri/src/jsonrpc.rs apps/desktop/src-tauri/src/engine.rs apps/desktop/src-tauri/src/lib.rs
git commit -m "feat(desktop): JSONRPC stdio client + engine sidecar lifecycle"
```

---

## Task 9: Wire React to the `engine_ping` Tauri command

**Files:**
- Modify: `apps/desktop/src/App.tsx`
- Create: `apps/desktop/src/lib/rpc.ts`

- [ ] **Step 1: Write the RPC helper**

Write `/Users/changheonshin/workspace/myworks/linetta/apps/desktop/src/lib/rpc.ts`:
```ts
import { invoke } from "@tauri-apps/api/core";

export async function enginePing(): Promise<string> {
  return invoke<string>("engine_ping");
}
```

- [ ] **Step 2: Update `App.tsx` to call `enginePing`**

Write `/Users/changheonshin/workspace/myworks/linetta/apps/desktop/src/App.tsx`:
```tsx
import { useState } from "react";
import { enginePing } from "./lib/rpc";

export function App() {
  const [status, setStatus] = useState<string>("idle");
  const [pending, setPending] = useState<boolean>(false);

  const onPing = async () => {
    setPending(true);
    try {
      const reply = await enginePing();
      setStatus(`engine says: ${reply}`);
    } catch (err) {
      setStatus(`error: ${String(err)}`);
    } finally {
      setPending(false);
    }
  };

  return (
    <main className="shell">
      <h1>Linetta</h1>
      <p className="hint">Engine bridge smoke test</p>
      <button type="button" onClick={onPing} disabled={pending}>
        {pending ? "pinging…" : "Ping engine"}
      </button>
      <p className="status">{status}</p>
    </main>
  );
}
```

- [ ] **Step 3: Type-check the frontend**

```bash
cd apps/desktop
pnpm tsc -b
cd ../..
```
Expected: silent success.

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src/App.tsx apps/desktop/src/lib/rpc.ts
git commit -m "feat(desktop): UI calls engine_ping Tauri command"
```

---

## Task 10: Build and dev scripts

**Files:**
- Create: `scripts/build-engine.sh`
- Create: `scripts/dev.sh`

- [ ] **Step 1: Write `scripts/build-engine.sh`**

```bash
mkdir -p scripts
```

Write `/Users/changheonshin/workspace/myworks/linetta/scripts/build-engine.sh`:
```bash
#!/usr/bin/env bash
# Build the Go engine into apps/desktop/src-tauri/binaries/ with the target-triple
# suffix Tauri's externalBin expects.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT}/apps/desktop/src-tauri/binaries"
mkdir -p "${OUT_DIR}"

# Determine the host target triple Tauri uses.
if [[ "$(uname -s)" == "Darwin" ]]; then
  if [[ "$(uname -m)" == "arm64" ]]; then
    TRIPLE="aarch64-apple-darwin"
  else
    TRIPLE="x86_64-apple-darwin"
  fi
elif [[ "$(uname -s)" == "Linux" ]]; then
  TRIPLE="x86_64-unknown-linux-gnu"
else
  echo "Unsupported host: $(uname -s)" >&2
  exit 1
fi

OUT="${OUT_DIR}/linetta-engine-${TRIPLE}"

echo "Building engine -> ${OUT}"
(
  cd "${ROOT}/engine"
  go build -o "${OUT}" ./cmd/linetta-engine
)
echo "ok"
```

- [ ] **Step 2: Write `scripts/dev.sh`**

Write `/Users/changheonshin/workspace/myworks/linetta/scripts/dev.sh`:
```bash
#!/usr/bin/env bash
# Build the engine once, then start `tauri dev` (which runs `pnpm dev` for Vite).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"${ROOT}/scripts/build-engine.sh"

cd "${ROOT}/apps/desktop"
pnpm tauri dev "$@"
```

- [ ] **Step 3: Make them executable**

```bash
chmod +x scripts/build-engine.sh scripts/dev.sh
```

- [ ] **Step 4: Smoke-build the engine via the script**

```bash
./scripts/build-engine.sh
ls -la apps/desktop/src-tauri/binaries/
```
Expected: a `linetta-engine-{triple}` binary is present.

- [ ] **Step 5: Commit**

```bash
git add scripts/build-engine.sh scripts/dev.sh
git commit -m "chore(scripts): build-engine.sh + dev.sh"
```

---

## Task 11: End-to-end smoke test (`scripts/dev.sh`)

This is a manual verification step — no code changes — and it is the acceptance gate for Plan 0.

- [ ] **Step 1: Run dev**

```bash
./scripts/dev.sh
```
Expected behavior:
1. Engine binary is rebuilt (script prints `ok`).
2. Vite dev server starts on `http://localhost:1420`.
3. Rust builds (first time will take a couple of minutes).
4. A Tauri window opens titled "Linetta" with the heading, hint, and `Ping engine` button.

- [ ] **Step 2: Click `Ping engine` in the running window**

Expected on screen: `engine says: pong`.

- [ ] **Step 3: Watch the terminal**

You should see no panics on either the Rust or Go side. If the binary path resolution failed, you would see `[linetta] failed to spawn engine:` on stderr — re-run `./scripts/build-engine.sh` and try again.

- [ ] **Step 4: Stop dev (Ctrl-C in the terminal where `scripts/dev.sh` is running)**

Expected: clean shutdown; the engine child should be reaped (no orphan `linetta-engine` in `pgrep linetta-engine` afterwards). If an orphan is found, log it as an issue to address in Plan 5 (engine lifecycle hardens then) and `pkill -f linetta-engine` to clean up.

- [ ] **Step 5: Tag the milestone**

```bash
git tag plan-0-scaffold-done
```

---

## Self-review checklist (run after writing the plan, not at execution time)

1. **Spec coverage** — Plan 0 covers spec §2 (architecture), §12 (repo structure), §13 (build flow). It does **not** cover any feature work; that lands in Plans 1–6.
2. **Placeholder scan** — no "TBD", "TODO", or `_placeholder` references remain; the only placeholders intentionally noted are: the Tauri icon (replaced later) and the dev-only hint line called out in the design doc §4.3.
3. **Type consistency** — `engine_ping` (Tauri command, snake_case as Tauri requires) ↔ `enginePing` (TS wrapper) ↔ JSONRPC method `ping`. Consistent throughout.
4. **Cross-task dependencies** — Task 8 depends on Task 6's `--stdio` behavior; Task 9 depends on Task 8's `engine_ping` Tauri command; Task 11 depends on Task 10's scripts.

---

## Definition of Done

Plan 0 is done when **all** of the following are true:

- `git log --oneline` shows the commits from Tasks 1–10, in order, on `main`.
- `./scripts/build-engine.sh` produces `apps/desktop/src-tauri/binaries/linetta-engine-{triple}`.
- `cd engine && go test ./...` is green.
- `cd apps/desktop && pnpm tsc -b` is green.
- `cd apps/desktop/src-tauri && cargo check` is green.
- Running `./scripts/dev.sh` opens a Tauri window; clicking `Ping engine` shows `engine says: pong`.
- Tag `plan-0-scaffold-done` exists.

When done, return to brainstorming (or directly to writing-plans) to author Plan 1 (Schema + Projects + Library).
