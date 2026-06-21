# Mobile Engine Embed (Go c-archive/c-shared FFI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Linetta Go engine embeddable in-process via a C ABI so it can ship inside the Tauri app on desktop AND on mobile (iOS, Android), replacing the desktop-only sidecar process — which is impossible on iOS (the OS forbids spawning bundled subprocesses).

**Architecture:** Extract the engine bootstrap into a reusable `engineapp` package shared by (a) the existing `linetta-engine --stdio` sidecar and (b) a new `cmd/linetta-ffi` C ABI built with `buildmode=c-archive` (Apple) / `buildmode=c-shared` (Android). A new `mobile` build-tag profile gates desktop-only features that cannot run on mobile (git-sync and CLI-detect both shell out via `os/exec`, which is forbidden on iOS). Tauri's `build.rs` compiles and links the archive on desktop first (where behavior is fully verifiable), then the same archive is packaged into iOS (xcframework) and Android (jniLibs `.so`) targets. The Rust `ffi` module replaces the child-process client behind the **same** `engine_call`/`engine_ping`/`engine_status` Tauri commands, so the frontend is untouched.

**Tech Stack:** Go 1.26.2 (`buildmode=c-archive`/`c-shared`, cgo), Rust/Tauri 2 (incl. Tauri mobile), `cc` crate / `cargo:rustc-link-*` for linking, Xcode iOS toolchain (device + simulator slices → `xcframework`), Android NDK (per-ABI `.so`), macOS/iOS Security framework (existing cgo in `secrets_darwin.go`).

The detailed task checklist below is retained as the original implementation script. The authoritative current status is this section plus the verification evidence in `README.md`, `packaging/README.md`, Makefile targets, workflows, scripts, and tests.

## Implementation Status (2026-06-21)

Completed in the current implementation branch:

- Phases 1-3: extracted `engine/internal/engineapp`, added `cmd/linetta-ffi`, linked the desktop Tauri shell to the embedded Go engine through Rust FFI, and removed desktop sidecar packaging.
- Phase 4: added the `mobile` build tag profile for exec-based desktop features (`gitsync`, `clidetect`) and made the Android unsupported secret-store behavior explicit.
- Phase 5 foundation: added iOS/Android mobile engine build scripts, `make test-mobile-engine`, `make build-mobile-engine-ios`, `make build-mobile-engine-android`, and `.github/workflows/mobile-engine.yml`.
- Phase 6 wiring: initialized the ignored Tauri iOS/Android native projects, made `build.rs` target-aware for desktop/iOS/Android, added the Tauri mobile entry point, routes mobile engine data through the app data directory via `LINETTA_HOME`, and verified Rust links against the embedded engine for iOS simulator/device and Android arm64/x86_64/armv7.
- iOS app smoke: added `make smoke-mobile-ios-sim` / `scripts/smoke-ios-simulator.sh`, which generates a no-sign arm64 simulator app bundle, installs/launches it on an available iPhone simulator, confirms the embedded FFI engine symbols are linked into the app executable, and confirms `library.db` is created under the simulator app container.
- Android app smoke: generated a debug arm64 APK with `pnpm tauri android build --debug --apk --target aarch64 --ci`; added CI coverage for that packaging path and `scripts/build-android-release-smoke.sh` to verify release APK/AAB signing with a temporary local keystore.
- Mobile UI pass: adapted the workspace for phone-sized viewports with an outline drawer, horizontally scrollable command bar, editor width constraints, and companion/fact/context panels as bottom sheets; scoped those bottom-sheet rules to the workspace so ThreadView panels stay in document flow; added responsive guardrails for Library, Settings, and ThreadView. Playwright/Chrome metrics verify no horizontal overflow at 390px mobile and 1280px desktop widths across Library, Settings, ThreadView, and Workspace.
- Release/documentation cleanup: updated README, packaging docs, Flathub packaging, Makefile, distribution validation, moved the standalone debug engine output to `engine/bin`, and added `.github/workflows/mobile-release.yml` for manual signed mobile app artifacts.

Current blockers / follow-up before calling iOS/Android apps fully shipped:

- Full iOS sign/export still needs Apple team/signing/provisioning secrets; no-sign simulator build and launch are verified locally, but a signed `.ipa` export and real device/TestFlight smoke remain release tasks.
- Android engine, debug APK, and temporary-keystore release APK/AAB builds are verified locally with NDK `27.2.12479018`; production release still requires real upload keystore secrets and CI/device smoke.
- Mobile product UI has responsive coverage for the main shell screens in this branch. Real device QA is still needed for keyboard/safe-area behavior, onboarding spotlight placement, command palette ergonomics, and store-build runtime behavior.

## Verified Foundation (spike results, 2026-06-21)

These are PROVEN, not assumptions — do not re-litigate them, but DO re-run the listed verification commands as regression gates where a task references them:

- **iOS device c-archive**: `CGO_ENABLED=1 GOOS=ios GOARCH=arm64 CC=<iphoneos-clangwrap> go build -buildmode=c-archive ./cmd/linetta-engine` links a 33MB `.a`; `vtool -show` on an extracted object reports `platform IOS`, minos 13.0.
- **iOS simulator c-archive**: same with the `iphonesimulator` SDK wrapper reports `platform IOSSIMULATOR`.
- **Android c-shared compatibility**: NDK `27.2.12479018` builds `liblinetta.so` for `arm64-v8a`, `x86_64`, and `armeabi-v7a`; Tauri Android debug build packages the arm64 engine library into a working APK artifact.
- **iOS simulator app compatibility**: Xcode's matching iOS 26.5 simulator runtime builds and launches a no-sign Tauri simulator `.app` via `make smoke-mobile-ios-sim`; the app executable exports the five `LinettaEngine*` C symbols and creates `library.db` in the simulator app container.
- **DB**: `modernc.org/sqlite` is pure-Go — no cgo, cross-compiles to both mobile OSes.

## Original runtime gaps resolved in this branch

- `internal/gitsync/gitsync.go` is gated behind `!mobile`, and mobile builds use a not-supported stub instead of compiling `os/exec` calls into iOS/Android.
- `internal/clidetect/clidetect.go` is gated behind `!mas && !mobile`, and the mobile/MAS profile reports unavailable CLI providers without shelling out.
- Android and other non-Apple targets intentionally use `unsupportedSecretStore`, covered by tests so missing secure persistence is explicit rather than accidental. A native Android Keystore backend remains a product follow-up outside this embed plan.
- iOS simulator startup now routes `LINETTA_HOME` through the app data directory and creates `library.db` in the sandbox. Full iOS Keychain runtime behavior under signed entitlements remains part of signed-device release QA.

## Global Constraints

- Go toolchain `go1.26.2`. Module path verbatim: `github.com/devlikebear/linetta/engine`.
- The C ABI is the ONLY new public surface from Go. Exported cgo functions: `LinettaEngineStart`, `LinettaEngineCall`, `LinettaEngineStop`, `LinettaEngineFreeCString`, `LinettaEngineSetNotifyCallback`. Identical names across Go (Phases 2–4) and Rust `extern "C"` (Phase 3).
- The FFI request/response contract is the SAME JSON-RPC 2.0 envelope already used over stdio (`engine/internal/rpc/codec.go`). One JSON-RPC request string in, one response string out. Do NOT invent a new wire format.
- Build tags compose, do not replace: existing `mas` vs `!mas` selects folder-sync (`foldersync_staged.go`/`foldersync_direct.go`) and git-sync (`gitsync_enabled.go`/`gitsync_disabled.go`). The NEW `mobile` tag is orthogonal and gates `os/exec`-based features. A build may be `mobile` without `mas` and vice versa. Never break the existing `mas` build.
- cgo is REQUIRED (`CGO_ENABLED=1`) for `c-archive`/`c-shared`. On Apple it is already required by `secrets_darwin.go`. On Android the engine path is cgo-free but the buildmode still needs the NDK C toolchain for the runtime/link.
- Do NOT remove the sidecar until the desktop FFI path is validated end-to-end (Phase 3). `cmd/linetta-engine` (stdio) is RETAINED as a CLI/debug tool even after un-bundling.
- Preserve Tauri command names/signatures: `engine_ping() -> String`, `engine_call(method, params) -> Value`, `engine_status() -> EngineStatus`. Frontend untouched.
- Streaming notifications (`ai.delta`, `companion.delta`, etc.) flow over a registered C callback carrying the same JSON notification string; Rust re-emits them as Tauri events exactly as the stdout reader does today.

## Phase Map (execution order + dependencies)

1. **Phase 1 — `engineapp` extraction** (engine): shared bootstrap. No platform risk.
2. **Phase 2 — FFI C ABI** (engine): `cmd/linetta-ffi`, depends on Phase 1.
3. **Phase 3 — Desktop integration + verify** (Tauri macOS): build.rs + ffi.rs + lib.rs, remove sidecar. Depends on Phase 2. **This is where streaming/notifications are actually run and verified** — the cheapest place to prove the mechanism.
4. **Phase 4 — `mobile` build-tag profile** (engine): gate `os/exec` features so the FFI cmd compiles+runs safely on mobile. Depends on Phase 2 (gates the same code the FFI cmd pulls in).
5. **Phase 5 — Mobile build verification** (engine + packaging): produce iOS `xcframework` (device+sim) and Android per-ABI `.so` from `cmd/linetta-ffi` under the `mobile` tag. Depends on Phases 2+4. Re-uses the verified spike commands.
6. **Phase 6 — Tauri mobile wiring** (Tauri iOS/Android): integrate the archives into Tauri mobile targets. Depends on Phase 5. **REQUIRES ITS OWN SPIKE FIRST** — see Phase 6 preamble; do not execute its tasks until the spike resolves the flagged Tauri-mobile-integration unknowns.

---

## Phase 1 — `engineapp` extraction

### Task 1: Extract engine bootstrap into `engineapp` package

**Files:**
- Create: `engine/internal/engineapp/engineapp.go`
- Create: `engine/internal/engineapp/engineapp_test.go`
- Read for reference: `engine/cmd/linetta-engine/main.go` (current bootstrap: store open, handler registration, `rpc.Server` construction, stdio serve at ~line 301)

**Interfaces:**
- Consumes: existing `internal/store`, `internal/rpc`, `internal/rpc/handlers`, `internal/paths` (unchanged).
- Produces:
  - `type Options struct { Home string }` (empty `Home` → `paths.Home()`).
  - `func Open(ctx context.Context, opts Options) (*App, error)`
  - `func (a *App) Handle(ctx context.Context, request []byte) ([]byte, error)` — one JSON-RPC request → marshalled JSON-RPC response (nil for no-reply notifications).
  - `func (a *App) SetNotifier(fn func(method string, params json.RawMessage))`
  - `func (a *App) Close() error`

- [ ] **Step 1: Write the failing test**

```go
// engine/internal/engineapp/engineapp_test.go
package engineapp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAppHandlePing(t *testing.T) {
	ctx := context.Background()
	app, err := Open(ctx, Options{Home: t.TempDir()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer app.Close()

	resp, err := app.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct{ Message string `json:"message"` } `json:"error"`
	}
	if err := json.Unmarshal(resp, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error != nil {
		t.Fatalf("ping error: %s", env.Error.Message)
	}
	if string(env.Result) != `"pong"` {
		t.Fatalf("ping result = %s, want \"pong\"", env.Result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd engine && go test ./internal/engineapp/ -run TestAppHandlePing -v`
Expected: FAIL — `engineapp` / `Open` undefined.

- [ ] **Step 3: Write minimal implementation**

Move the store-open + handler-registration + server-construction from `cmd/linetta-engine/main.go` into `engineapp.go`, preserving the exact registration sequence so no handler is dropped.

```go
// engine/internal/engineapp/engineapp.go
package engineapp

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/paths"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type Options struct{ Home string }

type App struct {
	store  *store.Store
	server *rpc.Server
	closers []func() error
}

func Open(ctx context.Context, opts Options) (*App, error) {
	home := opts.Home
	if home == "" {
		h, err := paths.Home()
		if err != nil {
			return nil, err
		}
		home = h
	}
	st, err := store.Open(ctx, home) // MATCH current main.go store-open call
	if err != nil {
		return nil, err
	}
	srv := rpc.NewServer()       // MATCH current constructor name
	handlers.Register(srv, st)   // MATCH current registration call(s)
	return &App{store: st, server: srv, closers: []func() error{st.Close}}, nil
}

func (a *App) Handle(ctx context.Context, request []byte) ([]byte, error) {
	return a.server.HandleMessage(ctx, request) // MATCH the dispatch the stdio loop calls per-line
}

func (a *App) SetNotifier(fn func(method string, params json.RawMessage)) {
	a.server.SetNotifier(fn) // MATCH/ADD the notification sink (see NOTE)
}

func (a *App) Close() error {
	var first error
	for i := len(a.closers) - 1; i >= 0; i-- {
		if err := a.closers[i](); err != nil && first == nil {
			first = err
		}
	}
	return first
}
```

NOTE: `rpc.NewServer`, `handlers.Register`, `server.HandleMessage`, `server.SetNotifier` are placeholders — replace with the ACTUAL API from `engine/internal/rpc/server.go` and current `main.go`. If the server has no single message-dispatch entrypoint, extract one (the stdio loop already calls into it per line — expose that). If notifications currently write directly to stdout instead of through a settable sink, ADD a `SetNotifier`/notifier field to the server in this task and route the stdout writer through it — Phase 2 and Phase 3 depend on this seam existing.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd engine && go test ./internal/engineapp/ -run TestAppHandlePing -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add engine/internal/engineapp/
git commit -m "feat(engine): extract bootstrap into engineapp package"
```

---

### Task 2: Route the sidecar through `engineapp` (stdio behavior identical)

**Files:**
- Modify: `engine/cmd/linetta-engine/main.go`
- Create: `engine/cmd/linetta-engine/stdio_smoke_test.go`
- Reference (do not change their tags): `foldersync_staged.go`, `foldersync_direct.go`, `gitsync_enabled.go`, `gitsync_disabled.go`

**Interfaces:**
- Consumes: `engineapp.Open/Handle/SetNotifier/Close` (Task 1).
- Produces: `func serveStdio(ctx context.Context, in io.Reader, out io.Writer, home string) error` (testable extraction of the read loop). No change to the `--stdio` flag gate or the platform setup hooks.

- [ ] **Step 1: Write the failing test**

```go
// engine/cmd/linetta-engine/stdio_smoke_test.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStdioPing(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := serveStdio(ctx, in, &out, t.TempDir()); err != nil {
		t.Fatalf("serveStdio: %v", err)
	}
	var env struct{ Result json.RawMessage `json:"result"` }
	line := strings.TrimSpace(out.String())
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		t.Fatalf("unmarshal %q: %v", line, err)
	}
	if string(env.Result) != `"pong"` {
		t.Fatalf("result = %s, want \"pong\"", env.Result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd engine && go test ./cmd/linetta-engine/ -run TestStdioPing -v`
Expected: FAIL — `serveStdio` undefined.

- [ ] **Step 3: Write minimal implementation**

Refactor `main.go` to bootstrap via `engineapp.Open` and extract the read loop into `serveStdio`. `main` wires `os.Stdin`/`os.Stdout`/`home=""`. Keep the `--stdio` gate and the platform setup hooks exactly where they run today.

```go
func serveStdio(ctx context.Context, in io.Reader, out io.Writer, home string) error {
	app, err := engineapp.Open(ctx, engineapp.Options{Home: home})
	if err != nil {
		return err
	}
	defer app.Close()

	var mu sync.Mutex // guard the shared writer: responses + async notifications
	write := func(b []byte) error {
		mu.Lock()
		defer mu.Unlock()
		_, err := out.Write(append(b, '\n'))
		return err
	}
	app.SetNotifier(func(method string, params json.RawMessage) {
		n, _ := json.Marshal(rpc.Notification{JSONRPC: "2.0", Method: method, Params: params})
		_ = write(n)
	})

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		resp, err := app.Handle(ctx, scanner.Bytes())
		if err != nil {
			// preserve current main.go error-to-stdout behavior
		}
		if resp != nil {
			if err := write(resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
```

NOTE: match the ACTUAL notification struct / encoder in current `main.go` (~line 301). The mutex is required so the async notifier and the response writer don't interleave partial lines on the shared stream.

- [ ] **Step 4: Run test + sidecar build**

Run: `cd engine && go test ./cmd/linetta-engine/ -run TestStdioPing -v && go build ./cmd/linetta-engine`
Expected: PASS and builds.

- [ ] **Step 5: Full engine suite (no regressions)**

Run: `cd engine && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add engine/cmd/linetta-engine/
git commit -m "refactor(engine): route sidecar stdio loop through engineapp"
```

---

## Phase 2 — FFI C ABI

### Task 3: Add `cmd/linetta-ffi` C ABI (calls, no notifications yet)

**Files:**
- Create: `engine/cmd/linetta-ffi/ffi.go`
- Create: `engine/cmd/linetta-ffi/ffi_test.go`

**Interfaces:**
- Consumes: `engineapp.Open/Handle/Close` (Task 1).
- Produces (cgo `//export`): `LinettaEngineStart(home *C.char) C.int`, `LinettaEngineCall(request *C.char) *C.char`, `LinettaEngineFreeCString(s *C.char)`, `LinettaEngineStop() C.int`. Plus Go-typed inner helpers `startEngine(home string) error`, `handleRequest(request []byte) []byte`, `stopEngine() error` (the exports wrap these; tests exercise the helpers since cgo exports aren't directly callable from Go tests).

- [ ] **Step 1: Write the failing test**

```go
// engine/cmd/linetta-ffi/ffi_test.go
package main

import (
	"encoding/json"
	"testing"
)

func TestHandleRequestPing(t *testing.T) {
	if err := startEngine(t.TempDir()); err != nil {
		t.Fatalf("startEngine: %v", err)
	}
	defer stopEngine()
	resp := handleRequest([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	var env struct{ Result json.RawMessage `json:"result"` }
	if err := json.Unmarshal(resp, &env); err != nil {
		t.Fatalf("unmarshal %q: %v", resp, err)
	}
	if string(env.Result) != `"pong"` {
		t.Fatalf("result = %s, want \"pong\"", env.Result)
	}
}

func TestHandleRequestBeforeStart(t *testing.T) {
	_ = stopEngine()
	resp := handleRequest([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	var env struct{ Error *struct{ Message string `json:"message"` } `json:"error"` }
	if err := json.Unmarshal(resp, &env); err != nil {
		t.Fatalf("unmarshal %q: %v", resp, err)
	}
	if env.Error == nil {
		t.Fatalf("expected error envelope when engine not started, got %s", resp)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd engine && go test ./cmd/linetta-ffi/ -run TestHandleRequest -v`
Expected: FAIL — helpers undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// engine/cmd/linetta-ffi/ffi.go
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"encoding/json"
	"sync"
	"unsafe"

	"github.com/devlikebear/linetta/engine/internal/engineapp"
)

var (
	mu  sync.Mutex
	app *engineapp.App
	ctx = context.Background()
)

func startEngine(home string) error {
	mu.Lock()
	defer mu.Unlock()
	if app != nil {
		return nil // idempotent
	}
	a, err := engineapp.Open(ctx, engineapp.Options{Home: home})
	if err != nil {
		return err
	}
	app = a
	return nil
}

func handleRequest(request []byte) []byte {
	mu.Lock()
	a := app
	mu.Unlock()
	if a == nil {
		return errorEnvelope(request, "engine not started")
	}
	resp, err := a.Handle(ctx, request)
	if err != nil {
		return errorEnvelope(request, err.Error())
	}
	if resp == nil {
		return []byte(`{}`) // no-reply notification: always return a freeable string
	}
	return resp
}

func stopEngine() error {
	mu.Lock()
	defer mu.Unlock()
	if app == nil {
		return nil
	}
	err := app.Close()
	app = nil
	return err
}

func errorEnvelope(request []byte, msg string) []byte {
	var probe struct{ ID json.RawMessage `json:"id"` }
	_ = json.Unmarshal(request, &probe)
	id := probe.ID
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	out, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": -32603, "message": msg},
	})
	return out
}

//export LinettaEngineStart
func LinettaEngineStart(home *C.char) C.int {
	if err := startEngine(C.GoString(home)); err != nil {
		return C.int(1)
	}
	return C.int(0)
}

//export LinettaEngineCall
func LinettaEngineCall(request *C.char) *C.char {
	return C.CString(string(handleRequest([]byte(C.GoString(request))))) // caller frees
}

//export LinettaEngineFreeCString
func LinettaEngineFreeCString(s *C.char) { C.free(unsafe.Pointer(s)) }

//export LinettaEngineStop
func LinettaEngineStop() C.int {
	if err := stopEngine(); err != nil {
		return C.int(1)
	}
	return C.int(0)
}

func main() {} // required for buildmode=c-archive/c-shared, never called
```

NOTE: `-32603` matches the internal-error code in `engine/internal/rpc/server.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd engine && go test ./cmd/linetta-ffi/ -run TestHandleRequest -v`
Expected: PASS.

- [ ] **Step 5: Verify the c-archive builds (desktop host)**

Run: `cd engine && CGO_ENABLED=1 go build -buildmode=c-archive -o /tmp/linetta-ffi.a ./cmd/linetta-ffi && grep -q LinettaEngineCall /tmp/linetta-ffi.h && echo OK`
Expected: prints `OK`; the generated `.h` declares all four exports.

- [ ] **Step 6: Commit**

```bash
git add engine/cmd/linetta-ffi/
git commit -m "feat(engine): add linetta-ffi c-archive ABI"
```

---

### Task 4: Add the notification callback to the C ABI

**Files:**
- Modify: `engine/cmd/linetta-ffi/ffi.go`
- Modify: `engine/cmd/linetta-ffi/ffi_test.go`

**Interfaces:**
- Consumes: `App.SetNotifier` (Task 1).
- Produces: `LinettaEngineSetNotifyCallback(cb C.LinettaNotifyCallback)` where C typedef is `typedef void (*LinettaNotifyCallback)(const char* method, const char* params);`. Plus Go-typed test seam `setGoNotifier(fn)` / `emitNotify(method, params)`. `startEngine` wires `app.SetNotifier(emitNotify)`.

- [ ] **Step 1: Write the failing test**

```go
// add to engine/cmd/linetta-ffi/ffi_test.go
func TestNotifierFanout(t *testing.T) {
	var gotMethod, gotParams string
	setGoNotifier(func(method string, params json.RawMessage) {
		gotMethod = method
		gotParams = string(params)
	})
	defer setGoNotifier(nil)

	emitNotify("ai.delta", json.RawMessage(`{"text":"hi"}`))
	if gotMethod != "ai.delta" {
		t.Fatalf("method = %q, want ai.delta", gotMethod)
	}
	if gotParams != `{"text":"hi"}` {
		t.Fatalf("params = %q", gotParams)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd engine && go test ./cmd/linetta-ffi/ -run TestNotifierFanout -v`
Expected: FAIL — `setGoNotifier`/`emitNotify` undefined.

- [ ] **Step 3: Write minimal implementation**

Replace the cgo preamble and add the notifier bridge:

```go
/*
#include <stdlib.h>
typedef void (*LinettaNotifyCallback)(const char* method, const char* params);
static void linetta_invoke_notify(LinettaNotifyCallback cb, const char* m, const char* p) { cb(m, p); }
*/
import "C"
```

```go
var (
	notifyMu sync.Mutex
	cNotify  C.LinettaNotifyCallback
	goNotify func(method string, params json.RawMessage) // test seam
)

func setGoNotifier(fn func(method string, params json.RawMessage)) {
	notifyMu.Lock(); goNotify = fn; notifyMu.Unlock()
}

func emitNotify(method string, params json.RawMessage) {
	notifyMu.Lock(); gn, cb := goNotify, cNotify; notifyMu.Unlock()
	if gn != nil {
		gn(method, params)
	}
	if cb != nil {
		cm, cp := C.CString(method), C.CString(string(params))
		C.linetta_invoke_notify(cb, cm, cp)
		C.free(unsafe.Pointer(cm)); C.free(unsafe.Pointer(cp))
	}
}

//export LinettaEngineSetNotifyCallback
func LinettaEngineSetNotifyCallback(cb C.LinettaNotifyCallback) {
	notifyMu.Lock(); cNotify = cb; notifyMu.Unlock()
}
```

In `startEngine`, after `app = a`, add: `app.SetNotifier(emitNotify)`.

NOTE: the C callback runs on a Go-owned thread; the Rust trampoline (Phase 3) must be `extern "C"` and must not unwind. `params` is always valid JSON (possibly `null`).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd engine && go test ./cmd/linetta-ffi/ -run TestNotifierFanout -v`
Expected: PASS.

- [ ] **Step 5: Re-verify c-archive builds with callback typedef**

Run: `cd engine && CGO_ENABLED=1 go build -buildmode=c-archive -o /tmp/linetta-ffi.a ./cmd/linetta-ffi && grep -q LinettaEngineSetNotifyCallback /tmp/linetta-ffi.h && echo OK`
Expected: prints `OK`.

- [ ] **Step 6: Commit**

```bash
git add engine/cmd/linetta-ffi/
git commit -m "feat(engine): add notification callback to ffi ABI"
```

---

## Phase 3 — Desktop integration + verify (the mechanism proof)

### Task 5: `build.rs` compiles and links the c-archive (desktop)

**Files:**
- Modify: `apps/desktop/src-tauri/build.rs` (already exists; currently compiles `macos/bookmarks.m` under `mas`)
- Reference: `engine/internal/settings/secrets_darwin.go` (frameworks: Security, CoreFoundation)

**Interfaces:**
- Consumes: `engine/cmd/linetta-ffi`.
- Produces: `liblinetta_engine_ffi.a` in `OUT_DIR` + `cargo:rustc-link-*` directives.

- [ ] **Step 1: Extend `build.rs`**

Add `build_go_engine()` and call it from `main()` before `tauri_build::build()`:

```rust
fn build_go_engine() {
    use std::{env, path::PathBuf, process::Command};
    let out_dir = PathBuf::from(env::var("OUT_DIR").unwrap());
    let manifest = PathBuf::from(env::var("CARGO_MANIFEST_DIR").unwrap());
    let engine_dir = manifest.join("../../../engine");
    let archive = out_dir.join("liblinetta_engine_ffi.a");

    let mut tags: Vec<&str> = Vec::new();
    if env::var("CARGO_FEATURE_MAS").is_ok() { tags.push("mas"); }

    let mut cmd = Command::new("go");
    cmd.current_dir(&engine_dir)
        .env("CGO_ENABLED", "1")
        .arg("build").arg("-buildmode=c-archive").arg("-o").arg(&archive);
    if !tags.is_empty() { cmd.arg(format!("-tags={}", tags.join(","))); }
    cmd.arg("./cmd/linetta-ffi");
    assert!(cmd.status().expect("spawn go build").success(), "go c-archive build failed");

    println!("cargo:rustc-link-search=native={}", out_dir.display());
    println!("cargo:rustc-link-lib=static=linetta_engine_ffi");
    let target_os = env::var("CARGO_CFG_TARGET_OS").unwrap_or_default();
    if target_os == "macos" {
        println!("cargo:rustc-link-lib=framework=Security");
        println!("cargo:rustc-link-lib=framework=CoreFoundation");
    }
    println!("cargo:rerun-if-changed={}", engine_dir.display());
}
```

- [ ] **Step 2: Verify the archive builds via Cargo**

Run: `cd apps/desktop/src-tauri && cargo build && find target -name 'liblinetta_engine_ffi.a' | head -1`
Expected: prints a path to the archive (the symbols are unused until Task 6 — that's fine).

- [ ] **Step 3: Commit**

```bash
git add apps/desktop/src-tauri/build.rs
git commit -m "build(desktop): compile and link go engine c-archive"
```

---

### Task 6: Rust `ffi` module — safe wrapper + notification trampoline

**Files:**
- Create: `apps/desktop/src-tauri/src/ffi.rs`
- Modify: `apps/desktop/src-tauri/src/lib.rs` (add `mod ffi;`)

**Interfaces:**
- Consumes: linked C symbols (Task 5).
- Produces: `pub struct Engine`; `pub fn start(app: &AppHandle, home: Option<&str>) -> Result<Engine, String>`; `pub fn start_raw(home: &str) -> Result<Engine, String>` (no Tauri notifier, for tests); `pub fn call(&self, method: &str, params: Option<Value>) -> Result<Value, String>`; `impl Drop` → `LinettaEngineStop`.

- [ ] **Step 1: Write the failing test**

```rust
// bottom of apps/desktop/src-tauri/src/ffi.rs
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn ping_round_trips() {
        let tmp = std::env::temp_dir().join(format!("linetta-ffi-test-{}", std::process::id()));
        std::fs::create_dir_all(&tmp).unwrap();
        let eng = Engine::start_raw(tmp.to_str().unwrap()).expect("start");
        assert_eq!(eng.call("ping", None).expect("ping"), serde_json::json!("pong"));
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/desktop/src-tauri && cargo test --lib ffi::tests::ping_round_trips 2>&1 | tail -30`
Expected: FAIL — `ffi`/`Engine` undefined.

- [ ] **Step 3: Write minimal implementation**

```rust
// apps/desktop/src-tauri/src/ffi.rs
use std::ffi::{c_char, c_int, CStr, CString};
use std::sync::OnceLock;
use serde_json::Value;
use tauri::{AppHandle, Emitter};

extern "C" {
    fn LinettaEngineStart(home: *const c_char) -> c_int;
    fn LinettaEngineCall(request: *const c_char) -> *mut c_char;
    fn LinettaEngineFreeCString(s: *mut c_char);
    fn LinettaEngineStop() -> c_int;
    fn LinettaEngineSetNotifyCallback(cb: extern "C" fn(*const c_char, *const c_char));
}

static NOTIFY_APP: OnceLock<AppHandle> = OnceLock::new();

extern "C" fn notify_trampoline(method: *const c_char, params: *const c_char) {
    let _ = std::panic::catch_unwind(|| {
        if method.is_null() { return; }
        let method = unsafe { CStr::from_ptr(method) }.to_string_lossy().into_owned();
        let params_str = if params.is_null() {
            "null".to_string()
        } else {
            unsafe { CStr::from_ptr(params) }.to_string_lossy().into_owned()
        };
        let payload: Value = serde_json::from_str(&params_str).unwrap_or(Value::Null);
        if let Some(app) = NOTIFY_APP.get() {
            let _ = app.emit(&method, payload); // SAME event-name scheme as the stdout reader (see NOTE)
        }
    });
}

pub struct Engine;

impl Engine {
    pub fn start(app: &AppHandle, home: Option<&str>) -> Result<Engine, String> {
        let _ = NOTIFY_APP.set(app.clone());
        unsafe { LinettaEngineSetNotifyCallback(notify_trampoline) };
        Self::start_raw(home.unwrap_or(""))
    }
    pub fn start_raw(home: &str) -> Result<Engine, String> {
        let c_home = CString::new(home).map_err(|e| e.to_string())?;
        let rc = unsafe { LinettaEngineStart(c_home.as_ptr()) };
        if rc != 0 { return Err(format!("LinettaEngineStart failed (code {rc})")); }
        Ok(Engine)
    }
    pub fn call(&self, method: &str, params: Option<Value>) -> Result<Value, String> {
        let request = serde_json::json!({"jsonrpc":"2.0","id":1,"method":method,"params":params});
        let c_req = CString::new(serde_json::to_string(&request).map_err(|e| e.to_string())?)
            .map_err(|e| e.to_string())?;
        let ptr = unsafe { LinettaEngineCall(c_req.as_ptr()) };
        if ptr.is_null() { return Err("engine returned null".into()); }
        let resp_str = unsafe { CStr::from_ptr(ptr) }.to_string_lossy().into_owned();
        unsafe { LinettaEngineFreeCString(ptr) };
        let resp: Value = serde_json::from_str(&resp_str).map_err(|e| e.to_string())?;
        if let Some(err) = resp.get("error") {
            let msg = err.get("message").and_then(|m| m.as_str()).unwrap_or("engine error");
            return Err(msg.to_string());
        }
        Ok(resp.get("result").cloned().unwrap_or(Value::Null))
    }
}

impl Drop for Engine {
    fn drop(&mut self) { unsafe { LinettaEngineStop() }; }
}
```

NOTE: read the CURRENT stdout-reader notification routing in `apps/desktop/src-tauri/src/engine.rs` (the `on_notification` closure maps `ai.delta` → Tauri event `ai-delta`, etc.). Replicate that SAME mapping here (e.g. translate `.`→`-` and apply the same allow-list) so existing frontend listeners keep working. Do NOT emit raw `method` names if the current code remapped them.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/desktop/src-tauri && cargo test --lib ffi::tests::ping_round_trips 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src-tauri/src/ffi.rs apps/desktop/src-tauri/src/lib.rs
git commit -m "feat(desktop): add ffi engine wrapper and notify trampoline"
```

---

### Task 7: Switch `lib.rs` from sidecar to in-process FFI

**Files:**
- Modify: `apps/desktop/src-tauri/src/lib.rs`

**Interfaces:**
- Consumes: `ffi::Engine`, `ffi::start`.
- Produces: `EngineState { engine: Option<Arc<ffi::Engine>>, startup_error: Option<String> }`; commands call `Engine::call` via `spawn_blocking`. Signatures unchanged.

- [ ] **Step 1: Add a Rust test for diagnostics.version**

```rust
// add to ffi.rs tests
#[test]
fn diagnostics_version_present() {
    let tmp = std::env::temp_dir().join(format!("linetta-ffi-ver-{}", std::process::id()));
    std::fs::create_dir_all(&tmp).unwrap();
    let eng = Engine::start_raw(tmp.to_str().unwrap()).unwrap();
    let v = eng.call("diagnostics.version", None).unwrap();
    assert!(v.get("version").and_then(|x| x.as_str()).is_some(), "version missing: {v}");
}
```

- [ ] **Step 2: Run it**

Run: `cd apps/desktop/src-tauri && cargo test --lib ffi::tests::diagnostics_version_present 2>&1 | tail -20`
Expected: PASS (handler reachable through `engineapp`). If FAIL, fix Task 1 registration.

- [ ] **Step 3: Rewire `lib.rs`**

Replace `engine::spawn` with `ffi::start`; make commands call the in-process engine on a blocking thread.

```rust
pub(crate) struct EngineState {
    pub(crate) engine: Option<Arc<ffi::Engine>>,
    pub(crate) startup_error: Option<String>,
}

// in setup()
let state = match ffi::start(&handle, None) {
    Ok(engine) => EngineState { engine: Some(Arc::new(engine)), startup_error: None },
    Err(e) => { eprintln!("[linetta] failed to start engine: {e}");
        EngineState { engine: None, startup_error: Some(e) } }
};
handle.manage(state);

fn engine_handle(state: &EngineState) -> Result<Arc<ffi::Engine>, String> {
    state.engine.clone().ok_or_else(|| {
        state.startup_error.clone().unwrap_or_else(|| "engine unavailable".to_string())
    })
}

#[tauri::command]
async fn engine_call(state: tauri::State<'_, EngineState>, method: String, params: Option<Value>)
    -> Result<Value, String> {
    let engine = engine_handle(&state)?;
    tauri::async_runtime::spawn_blocking(move || engine.call(&method, params))
        .await.map_err(|e| e.to_string())?
}
```

Update `engine_ping` and `engine_status` the same way (`spawn_blocking(move || engine.call(...))`). `engine_status` keeps its `ping` → `diagnostics.version` sequence and the `EngineStatus` struct. To preserve the 2s `ENGINE_STATUS_TIMEOUT`, wrap each `spawn_blocking` in `tokio::time::timeout`. Remove the old `engine_client`/`jsonrpc::Client` references from `lib.rs`; leave `mod engine; mod jsonrpc;` for now (removed in Task 8).

- [ ] **Step 4: Build + manual streaming smoke**

Run: `cd apps/desktop/src-tauri && cargo build 2>&1 | tail -20`
Expected: builds clean.

Manual (document result, do not automate): launch the dev app, confirm the status panel shows `ok: true` + version, then run an AI action and confirm incremental deltas render. **This validates the cgo→Rust→Tauri notification path on a non-main thread — the single most important behavioral check in the plan.** If deltas don't render, the cause is almost certainly the event-name mapping flagged in Task 6 Step 3.

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src-tauri/src/lib.rs
git commit -m "feat(desktop): use in-process ffi engine instead of sidecar"
```

---

### Task 8: Remove sidecar packaging (desktop)

**Files:**
- Modify: `apps/desktop/src-tauri/tauri.conf.json` (remove `externalBin` entry ~line 44)
- Delete: `apps/desktop/src-tauri/src/engine.rs`, `apps/desktop/src-tauri/src/jsonrpc.rs`
- Modify: `apps/desktop/src-tauri/src/lib.rs` (remove `mod engine; mod jsonrpc;`)

- [ ] **Step 1: Remove the externalBin entry** from `tauri.conf.json` (drop the `linetta-engine` sidecar; remove `externalBin` key if it becomes empty).

- [ ] **Step 2: Delete dead modules**

```bash
git rm apps/desktop/src-tauri/src/engine.rs apps/desktop/src-tauri/src/jsonrpc.rs
```
Remove `mod engine;` and `mod jsonrpc;` from `lib.rs`.

- [ ] **Step 3: Build with no sidecar**

Run: `cd apps/desktop/src-tauri && cargo build 2>&1 | tail -20`
Expected: clean.

- [ ] **Step 4: Sidecar still builds independently (retained CLI)**

Run: `cd engine && go build ./cmd/linetta-engine && go test ./...`
Expected: PASS.

- [ ] **Step 5: Both feature sets link**

Run: `cd apps/desktop/src-tauri && cargo build && cargo build --features mas 2>&1 | tail -20`
Expected: both default and `mas` builds succeed (correct Go tag per feature).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore(desktop): drop sidecar packaging, link engine in-process"
```

---

## Phase 4 — `mobile` build-tag profile

Goal: make `cmd/linetta-ffi` (and everything it imports) compile AND run safely under a `mobile` build tag by gating the two `os/exec` features that cannot run on iOS. The `mobile` tag is orthogonal to `mas`.

### Task 9: Gate git-sync behind `!mobile`

**Files:**
- Modify: `engine/internal/gitsync/gitsync.go` (the `exec.CommandContext(..., "git", ...)` path at ~line 220)
- Likely create: `engine/internal/gitsync/gitsync_mobile.go` (build tag `mobile`) + add `//go:build !mobile` to the existing exec-using file
- Test: `engine/internal/gitsync/gitsync_mobile_test.go`

**Interfaces:**
- Produces: a `mobile`-tagged stub for whatever exported function currently shells out to `git`, returning a clear "not supported on this platform" error with the SAME signature as the desktop impl. Find the exact exported entrypoint (the function containing line 220) before splitting.

- [ ] **Step 1: Write the failing test (mobile tag)**

```go
//go:build mobile
// engine/internal/gitsync/gitsync_mobile_test.go
package gitsync

import "testing"

func TestGitSyncUnsupportedOnMobile(t *testing.T) {
	// Call the same exported entrypoint the desktop path exposes; expect a
	// not-supported error rather than an exec attempt.
	err := Sync( /* minimal args matching the real signature */ )
	if err == nil {
		t.Fatal("expected not-supported error on mobile build")
	}
}
```

- [ ] **Step 2: Run under the mobile tag to verify it fails**

Run: `cd engine && go test -tags mobile ./internal/gitsync/ -run TestGitSyncUnsupportedOnMobile -v`
Expected: FAIL — mobile stub doesn't exist yet (or the exec impl is still compiled in).

- [ ] **Step 3: Split the file by build tag**

Add `//go:build !mobile` to the top of the existing `gitsync.go` (the exec-using file). Create `gitsync_mobile.go`:

```go
//go:build mobile
package gitsync

import "errors"

var errUnsupported = errors.New("git sync is not supported on mobile builds")

// Mirror the EXACT exported signature from the !mobile file.
func Sync(/* same params */) error { return errUnsupported }
```

NOTE: replace `Sync(...)` with the real exported function name(s)/signature(s) found in the current `gitsync.go`. If multiple exported functions touch git, stub each. Keep types/structs that are shared (and don't shell out) in a tag-neutral file so both builds see them.

- [ ] **Step 4: Run both tag variants**

Run: `cd engine && go test -tags mobile ./internal/gitsync/ -run TestGitSyncUnsupportedOnMobile -v && go build ./internal/gitsync/ && go build -tags mobile ./internal/gitsync/`
Expected: PASS; both default and `mobile` builds compile.

- [ ] **Step 5: Commit**

```bash
git add engine/internal/gitsync/
git commit -m "feat(engine): gate git-sync behind !mobile build tag"
```

---

### Task 10: Gate clidetect behind `!mobile`

**Files:**
- Modify: `engine/internal/clidetect/clidetect.go` (the `exec.CommandContext(..., "command -v claude")` at ~line 88)
- Create: `engine/internal/clidetect/clidetect_mobile.go`
- Test: `engine/internal/clidetect/clidetect_mobile_test.go`

**Interfaces:**
- Produces: a `mobile`-tagged stub for the exported detect function, returning "not detected / not supported" with the SAME signature as the desktop impl.

- [ ] **Step 1: Write the failing test (mobile tag)**

```go
//go:build mobile
// engine/internal/clidetect/clidetect_mobile_test.go
package clidetect

import (
	"context"
	"testing"
)

func TestDetectUnsupportedOnMobile(t *testing.T) {
	// Same exported entrypoint as desktop; expect "not available", never an exec.
	got := Detect(context.Background()) // match real signature/return
	if got.Available { // match real return-type field
		t.Fatalf("expected unavailable on mobile, got %+v", got)
	}
}
```

- [ ] **Step 2: Run under mobile tag to verify it fails**

Run: `cd engine && go test -tags mobile ./internal/clidetect/ -run TestDetectUnsupportedOnMobile -v`
Expected: FAIL.

- [ ] **Step 3: Split by build tag**

Add `//go:build !mobile` to the existing `clidetect.go`; create `clidetect_mobile.go` returning the zero/"unavailable" result with the matching signature.

NOTE: match the real exported function name and return type (e.g. a struct with an `Available bool`) found in current `clidetect.go`.

- [ ] **Step 4: Run both tag variants**

Run: `cd engine && go test -tags mobile ./internal/clidetect/ -run TestDetectUnsupportedOnMobile -v && go build ./internal/clidetect/ && go build -tags mobile ./internal/clidetect/`
Expected: PASS; both build.

- [ ] **Step 5: Commit**

```bash
git add engine/internal/clidetect/
git commit -m "feat(engine): gate clidetect behind !mobile build tag"
```

---

### Task 11: Decide secrets behavior on mobile (explicit, not accidental)

**Files:**
- Reference: `engine/internal/settings/secrets.go` (`SecretStore` interface: Get/Exists/Set/Delete), `secrets_other.go` (`unsupportedSecretStore`), `secrets_darwin.go` (`//go:build darwin`, includes iOS)
- Possibly create: `engine/internal/settings/secrets_mobile_other.go` or a doc note

**Decision to encode (no new platform impl in this plan):**
- iOS: KEEPS `secrets_darwin.go` (darwin tag includes ios). It compiles. Runtime Keychain correctness under the iOS sandbox is verified later in Phase 6 (needs `keychain-access-groups` entitlement). No code change here.
- Android: currently `secrets_other.go` → `unsupportedSecretStore` (Set/Get error). This means API keys won't persist on Android. That is ACCEPTABLE for this plan's scope but must be EXPLICIT, not a silent surprise.

- [ ] **Step 1: Add a regression test documenting the contract**

```go
// engine/internal/settings/secrets_unsupported_test.go  (no special tag; tests the fallback type directly)
package settings

import "testing"

func TestUnsupportedSecretStoreErrors(t *testing.T) {
	var s SecretStore = unsupportedSecretStore{}
	if err := s.Set("k", "v"); err == nil {
		t.Fatal("Set should error on unsupported store")
	}
	if _, ok, _ := s.Get("k"); ok {
		t.Fatal("Get should report not-found on unsupported store")
	}
}
```

- [ ] **Step 2: Run it**

Run: `cd engine && go test ./internal/settings/ -run TestUnsupportedSecretStore -v`
Expected: PASS (documents that the fallback errors cleanly rather than panicking).

- [ ] **Step 3: Record the gap**

Add a short doc comment at the top of `secrets_other.go` stating that Android selects this and a native Android Keystore impl is a tracked follow-up (out of scope here). Optionally add to the plan's tracking memory.

- [ ] **Step 4: Commit**

```bash
git add engine/internal/settings/
git commit -m "test(engine): document unsupported secret store contract for mobile"
```

---

### Task 12: Verify the FFI cmd builds under the `mobile` tag (host)

**Files:** none (verification task).

- [ ] **Step 1: Build the FFI c-archive with `-tags mobile` on the host**

Run: `cd engine && CGO_ENABLED=1 go build -tags mobile -buildmode=c-archive -o /tmp/linetta-ffi-mobile.a ./cmd/linetta-ffi && echo OK`
Expected: `OK` — proves the `mobile` profile compiles the full FFI surface with the gated features excluded.

- [ ] **Step 2: Full engine suite under mobile tag**

Run: `cd engine && go test -tags mobile ./...`
Expected: PASS (the gated packages use their mobile stubs).

- [ ] **Step 3: Commit (if any incidental fixes were needed)**

```bash
git add -A && git commit -m "build(engine): verify ffi builds under mobile tag" || echo "nothing to commit"
```

---

## Phase 5 — Mobile build verification (iOS xcframework + Android .so)

Goal: produce the actual mobile artifacts from `cmd/linetta-ffi` under `-tags mobile`, reusing the verified spike commands. Output: a build script the Tauri mobile targets consume.

### Task 13: iOS device + simulator c-archives → `xcframework`

**Files:**
- Create: `apps/desktop/src-tauri/scripts/build-engine-ios.sh`
- Reference: verified spike clang wrappers (iphoneos / iphonesimulator SDKs)

**Interfaces:**
- Produces: `LinettaEngine.xcframework` containing a device slice (`platform IOS`, arm64) and a simulator slice (`platform IOSSIMULATOR`, arm64), plus the generated `.h`.

- [ ] **Step 1: Write the build script**

```bash
#!/bin/sh
set -e
ENGINE_DIR="$(cd "$(dirname "$0")/../../../../engine" && pwd)"
OUT="${1:-/tmp/linetta-ios}"
mkdir -p "$OUT/device" "$OUT/sim"

mk_wrap() { # $1=sdk  $2=minflag  -> prints wrapper path
  W="$OUT/clangwrap-$1.sh"
  cat > "$W" <<EOF
#!/bin/sh
exec "\$(xcrun --sdk $1 --find clang)" -isysroot "\$(xcrun --sdk $1 --show-sdk-path)" -arch arm64 $2 "\$@"
EOF
  chmod +x "$W"; echo "$W"
}

DEV_CC=$(mk_wrap iphoneos "-miphoneos-version-min=13.0")
SIM_CC=$(mk_wrap iphonesimulator "-mios-simulator-version-min=13.0")

cd "$ENGINE_DIR"
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 CC="$DEV_CC" \
  go build -tags mobile -buildmode=c-archive -o "$OUT/device/liblinetta.a" ./cmd/linetta-ffi
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 CC="$SIM_CC" \
  go build -tags mobile -buildmode=c-archive -o "$OUT/sim/liblinetta.a" ./cmd/linetta-ffi
cp "$OUT/device/liblinetta.h" "$OUT/" 2>/dev/null || true

rm -rf "$OUT/LinettaEngine.xcframework"
xcodebuild -create-xcframework \
  -library "$OUT/device/liblinetta.a" -headers "$OUT" \
  -library "$OUT/sim/liblinetta.a" -headers "$OUT" \
  -output "$OUT/LinettaEngine.xcframework"
echo "built: $OUT/LinettaEngine.xcframework"
```

- [ ] **Step 2: Run it and verify both slices**

Run: `sh apps/desktop/src-tauri/scripts/build-engine-ios.sh /tmp/linetta-ios && ls /tmp/linetta-ios/LinettaEngine.xcframework`
Expected: the `.xcframework` directory exists with `ios-arm64` and `ios-arm64-simulator` subdirs.

Verify platforms (regression of the spike):
Run: `cd /tmp/linetta-ios && mkdir -p chk && cd chk && ar x ../device/liblinetta.a && vtool -show "$(ls *.o | head -1)" | grep -i platform`
Expected: `platform IOS`.

- [ ] **Step 3: Confirm `.h` carries all five exports**

Run: `grep -E 'LinettaEngine(Start|Call|Stop|FreeCString|SetNotifyCallback)' /tmp/linetta-ios/liblinetta.h | wc -l`
Expected: `5`.

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src-tauri/scripts/build-engine-ios.sh
git commit -m "build(ios): script to build engine xcframework under mobile tag"
```

---

### Task 14: Android per-ABI `.so` (NDK)

> **PRECONDITION:** Android NDK must be installed (it was NOT during the spike). Install via Android Studio SDK Manager or `sdkmanager "ndk;<version>"`, then set `ANDROID_NDK_HOME`. The Android engine path is cgo-free (verified), so this is the standard gomobile `c-shared` link — low risk, but it needs the NDK clang present.

**Files:**
- Create: `apps/desktop/src-tauri/scripts/build-engine-android.sh`

**Interfaces:**
- Produces: `liblinetta.so` per Android ABI (`arm64-v8a`, `armeabi-v7a`, `x86_64`) laid out for Tauri/Gradle `jniLibs`.

- [ ] **Step 1: Write the build script**

```bash
#!/bin/sh
set -e
: "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to your NDK path}"
ENGINE_DIR="$(cd "$(dirname "$0")/../../../../engine" && pwd)"
OUT="${1:-/tmp/linetta-android}"
API="${ANDROID_API:-24}"
TC="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/darwin-x86_64/bin"

# ABI -> (GOARCH, NDK clang prefix)
build() { # $1=abi $2=goarch $3=cc
  mkdir -p "$OUT/$1"
  cd "$ENGINE_DIR"
  CGO_ENABLED=1 GOOS=android GOARCH=$2 CC="$TC/$3" \
    go build -tags mobile -buildmode=c-shared -o "$OUT/$1/liblinetta.so" ./cmd/linetta-ffi
}
build arm64-v8a   arm64 "aarch64-linux-android${API}-clang"
build armeabi-v7a arm   "armv7a-linux-androideabi${API}-clang"
build x86_64      amd64 "x86_64-linux-android${API}-clang"
echo "built .so for arm64-v8a, armeabi-v7a, x86_64 under $OUT"
```

NOTE: the `prebuilt/darwin-x86_64` path is correct even on Apple Silicon (NDK ships an x86_64 toolchain that runs under Rosetta). Verify the exact clang triple names against the installed NDK version.

- [ ] **Step 2: Run it and verify the ELF**

Run: `ANDROID_NDK_HOME=<path> sh apps/desktop/src-tauri/scripts/build-engine-android.sh /tmp/linetta-android && file /tmp/linetta-android/arm64-v8a/liblinetta.so`
Expected: `ELF 64-bit LSB shared object, ARM aarch64 ...`.

- [ ] **Step 3: Confirm exports present in the `.so`**

Run: `nm -D /tmp/linetta-android/arm64-v8a/liblinetta.so 2>/dev/null | grep -c LinettaEngine`
Expected: ≥ 5.

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src-tauri/scripts/build-engine-android.sh
git commit -m "build(android): script to build engine .so under mobile tag"
```

---

## Phase 6 — Tauri mobile wiring

Phase 6 was executed after the spike:

- `pnpm tauri ios init --ci --skip-targets-install` and `pnpm tauri android init --ci --skip-targets-install` generated the ignored native projects under `apps/desktop/src-tauri/gen/`.
- `apps/desktop/src-tauri/build.rs` now selects the desktop archive, iOS archive, or Android shared library path based on the Cargo target, and passes the `mobile` Go build tag for iOS/Android.
- The Tauri shell has `#[cfg_attr(mobile, tauri::mobile_entry_point)]` and routes mobile engine data through the app data directory via `LINETTA_HOME`.
- iOS simulator no-sign app build/install/launch is verified by `make smoke-mobile-ios-sim`; the app executable exports the five `LinettaEngine*` symbols and creates `library.db` in the simulator container.
- Android arm64 debug APK generation and temporary-keystore release APK/AAB signing are verified locally; CI covers Android debug packaging and the manual release workflow covers signed APK/AAB artifact generation with real secrets.
- Android secrets are accepted as explicit no-persistence for this plan. A native Android Keystore backend is scheduled as a later product/security task.

Remaining Phase 6 release QA:

- iOS signed `.ipa` export, real-device launch, TestFlight smoke, and Keychain entitlement behavior require Apple team/signing/provisioning credentials.
- Android production upload signing, Play Console internal track upload, and physical/emulator store-build smoke require real upload-keystore secrets and store access.

---

## Self-Review

**Spec coverage (vs the goal: embed Go engine in-process for desktop + iOS + Android):**
- In-process C ABI replacing sidecar → Phases 1–3 (engineapp, FFI cmd, desktop integration).
- iOS build (the platform that makes the sidecar impossible) → verified foundation + Phase 5 Task 13.
- Android build → verified source compat + Phase 5 Task 14.
- Desktop-only feature gating for mobile (git-sync, clidetect) → Phase 4 Tasks 9–10.
- Secrets reality on mobile → Phase 4 Task 11 (explicit), Phase 6 (iOS Keychain runtime).
- Streaming notifications over FFI → Phase 2 Task 4 (Go), Phase 3 Task 6 (Rust trampoline), proven by Phase 3 Task 7 Step 4 manual smoke.
- Same frontend command surface → Phase 3 Task 7 (signatures preserved).
- Tauri mobile packaging → Phase 6 (spike-gated, deliberately not faked).

**Placeholder scan:** The Go RPC API names in Task 1 (`rpc.NewServer`/`handlers.Register`/`HandleMessage`/`SetNotifier`) and the exact exported signatures in Tasks 9–10 (`Sync`, `Detect`) are explicitly flagged as placeholders to replace with the real symbols — these depend on reading current source the executor must open first. Phase 6 is intentionally left as a spike rather than fake-concrete tasks (writing guessed Tauri-mobile steps would be the exact anti-pattern to avoid). Everything in Phases 1–5 carries concrete code/commands.

**Type consistency:** C ABI names identical across Go (Phases 2/4) and Rust `extern "C"` (Phase 3) and the verification greps (Phase 5): `LinettaEngineStart/Call/Stop/FreeCString/SetNotifyCallback`. `Engine::start`/`start_raw`/`call` consistent across Tasks 6/7. `EngineState` field rename (`client`→`engine`) contained to Tasks 7/8. The `mobile` stub functions in Tasks 9–10 must match the real desktop signatures (flagged).

**Biggest residual risk:** The local build chain now proves the embedded engine through desktop, iOS simulator, Android debug APK, and Android temporary-signed release APK/AAB artifacts. The remaining risk is store-grade mobile release QA because it depends on real signing credentials, store accounts, and device/TestFlight/Play internal-track validation.
