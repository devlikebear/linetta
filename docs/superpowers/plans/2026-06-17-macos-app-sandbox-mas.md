# macOS App Sandbox (MAS Prep) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Linetta build and run correctly under macOS App Sandbox (Mac App Store prerequisite), verified locally with the existing Developer ID certificate + sandbox entitlements.

**Architecture:** Add a dedicated `mas` build variant. The Go engine excludes Git Sync (which shells out to `git`, impossible in the sandbox) via a `mas` build tag. The macOS Keychain integration moves from the legacy `SecKeychain*` API (login-keychain, sandbox-hostile) to the modern `SecItem*` API (data-protection keychain, works in the sandbox). A Tauri config overlay applies App Sandbox entitlements; a build script signs the sidecar (`com.apple.security.inherit`) and the app (sandbox) explicitly.

**Tech Stack:** Go (cgo + Security.framework), Tauri v2 (Rust), `tauri-plugin-opener`, `codesign`, bash.

**Spec:** `docs/superpowers/specs/2026-06-17-macos-app-sandbox-mas-design.md`

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `engine/cmd/linetta-engine/sync.go` | Tag-agnostic `dailySyncer` interface + `syncResult` type | Create |
| `engine/cmd/linetta-engine/gitsync_enabled.go` | `//go:build !mas` — real git syncer + handler registration | Create |
| `engine/cmd/linetta-engine/gitsync_disabled.go` | `//go:build mas` — no-op syncer + "unavailable" handlers | Create |
| `engine/cmd/linetta-engine/gitsync_disabled_test.go` | `//go:build mas` — locks no-op contract | Create |
| `engine/cmd/linetta-engine/main.go` | Wire git sync via `setupGitSync` instead of direct `gitsync.New` | Modify |
| `engine/internal/rpc/handlers/gitsync.go` | Git sync RPC handlers (Developer ID build only) | Add build tag |
| `engine/internal/gitsync/gitsync_test.go` | Git sync tests (Developer ID build only) | Add build tag |
| `scripts/build-engine.sh` | Pass `LINETTA_BUILD_TAGS` to `go build` | Modify |
| `engine/internal/settings/secrets_darwin.go` | Keychain via modern `SecItem*` API | Rewrite |
| `engine/internal/settings/secrets_darwin_test.go` | Keychain round-trip test | Create |
| `apps/desktop/src-tauri/entitlements/linetta.entitlements` | Main app sandbox entitlements | Create |
| `apps/desktop/src-tauri/entitlements/linetta-sidecar.entitlements` | Sidecar inherit entitlements | Create |
| `apps/desktop/src-tauri/tauri.mas.conf.json` | MAS config overlay (entitlements) | Create |
| `apps/desktop/src-tauri/src/lib.rs` | `open_path` via opener plugin (sandbox-safe) | Modify |
| `apps/desktop/src-tauri/Cargo.toml` | Add `tauri-plugin-opener` | Modify |
| `scripts/build-mas-local.sh` | Build engine (`mas`) + Tauri (overlay) + sign sidecar/app | Create |
| `Makefile` | `build-mas-local` target | Modify |

---

## Task 1: Engine — exclude Git Sync behind the `mas` build tag

**Files:**
- Create: `engine/cmd/linetta-engine/sync.go`
- Create: `engine/cmd/linetta-engine/gitsync_enabled.go`
- Create: `engine/cmd/linetta-engine/gitsync_disabled.go`
- Create: `engine/cmd/linetta-engine/gitsync_disabled_test.go`
- Modify: `engine/cmd/linetta-engine/main.go`
- Modify: `engine/internal/rpc/handlers/gitsync.go` (add tag)
- Modify: `engine/internal/gitsync/gitsync_test.go` (add tag)

- [ ] **Step 1: Add the tag-agnostic interface**

Create `engine/cmd/linetta-engine/sync.go`:

```go
package main

import "context"

// syncResult is the tag-agnostic subset of gitsync.ResultSummary that main.go's
// backup retention loop needs. Keeping it free of any gitsync import lets the
// mas build omit the gitsync package entirely.
type syncResult struct {
	Error string
}

// dailySyncer is the daily git-sync hook invoked by the backup retention loop.
// The !mas build backs it with a real gitsync.Syncer; the mas build supplies a
// no-op.
type dailySyncer interface {
	RunOnce(ctx context.Context) (syncResult, error)
}
```

- [ ] **Step 2: Add the enabled (Developer ID) implementation**

Create `engine/cmd/linetta-engine/gitsync_enabled.go`:

```go
//go:build !mas

package main

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/gitsync"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// setupGitSync constructs the real git syncer, registers its RPC handlers, and
// returns the daily syncer used by the backup retention loop.
func setupGitSync(
	s *rpc.Server,
	settingsStore *settings.Store,
	projects *project.Repo,
	nodes *node.Repo,
	entities *entity.Repo,
	relationships *relationship.Repo,
	ops *opsstatus.Repo,
) dailySyncer {
	syncer := gitsync.New(settingsStore, projects, nodes, entities, relationships)
	syncer.Ops = ops
	s.Handle("git_sync.run", handlers.RunGitSync(syncer))
	s.Handle("git_sync.init", handlers.InitGitSync(syncer))
	return realSyncer{syncer}
}

type realSyncer struct{ s *gitsync.Syncer }

func (r realSyncer) RunOnce(ctx context.Context) (syncResult, error) {
	res, err := r.s.RunOnce(ctx)
	return syncResult{Error: res.Error}, err
}
```

- [ ] **Step 3: Add the disabled (App Store) implementation**

Create `engine/cmd/linetta-engine/gitsync_disabled.go`:

```go
//go:build mas

package main

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// setupGitSync registers git_sync handlers that report the feature is
// unavailable and returns a no-op syncer. The gitsync package (which shells out
// to git) is never compiled into the mas build.
func setupGitSync(
	s *rpc.Server,
	_ *settings.Store,
	_ *project.Repo,
	_ *node.Repo,
	_ *entity.Repo,
	_ *relationship.Repo,
	_ *opsstatus.Repo,
) dailySyncer {
	unavailable := func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, &rpc.MethodError{
			Code:    rpc.CodeMethodNotFound,
			Message: "git sync is not available in the App Store build",
		}
	}
	s.Handle("git_sync.run", unavailable)
	s.Handle("git_sync.init", unavailable)
	return noopSyncer{}
}

type noopSyncer struct{}

func (noopSyncer) RunOnce(context.Context) (syncResult, error) { return syncResult{}, nil }
```

- [ ] **Step 4: Add the mas-build contract test**

Create `engine/cmd/linetta-engine/gitsync_disabled_test.go`:

```go
//go:build mas

package main

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func TestSetupGitSyncDisabledIsNoop(t *testing.T) {
	s := rpc.NewServer()
	syncer := setupGitSync(s, nil, nil, nil, nil, nil, nil)
	res, err := syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("RunOnce returned non-empty error: %q", res.Error)
	}
}
```

- [ ] **Step 5: Tag the Developer-ID-only gitsync files**

Add this as the FIRST line of `engine/internal/rpc/handlers/gitsync.go` (blank line after, before `package handlers`):

```go
//go:build !mas

```

Add this as the FIRST line of `engine/internal/gitsync/gitsync_test.go` (blank line after, before the package clause):

```go
//go:build !mas

```

- [ ] **Step 6: Rewire main.go**

In `engine/cmd/linetta-engine/main.go`, delete the import line:

```go
	"github.com/devlikebear/linetta/engine/internal/gitsync"
```

Replace these two lines (currently ~152-153):

```go
	syncer := gitsync.New(settingsStore, projects, nodes, entities, relationships)
	syncer.Ops = ops
```

with:

```go
	syncer := setupGitSync(s, settingsStore, projects, nodes, entities, relationships, ops)
```

Delete these two lines (currently ~291-292):

```go
	s.Handle("git_sync.run", handlers.RunGitSync(syncer))
	s.Handle("git_sync.init", handlers.InitGitSync(syncer))
```

(The `retentionFn` call `res, err := syncer.RunOnce(ctx)` at ~158 now uses the `dailySyncer` interface and needs no change — `res.Error` is still valid.)

- [ ] **Step 7: Verify the default (Developer ID) build still works**

Run: `cd engine && go build ./... && go test ./...`
Expected: build succeeds, all tests PASS (gitsync tests still run because the default build has no `mas` tag).

- [ ] **Step 8: Verify the mas build compiles and excludes gitsync**

Run: `cd engine && go build -tags mas ./... && go test -tags mas ./cmd/linetta-engine/`
Expected: build + test PASS.

Run: `cd engine && go list -tags mas -deps ./cmd/linetta-engine | grep gitsync || echo "gitsync excluded"`
Expected: prints `gitsync excluded` (the gitsync package is NOT in the mas dependency graph).

- [ ] **Step 9: Commit**

```bash
git add engine/cmd/linetta-engine/sync.go \
  engine/cmd/linetta-engine/gitsync_enabled.go \
  engine/cmd/linetta-engine/gitsync_disabled.go \
  engine/cmd/linetta-engine/gitsync_disabled_test.go \
  engine/cmd/linetta-engine/main.go \
  engine/internal/rpc/handlers/gitsync.go \
  engine/internal/gitsync/gitsync_test.go
git commit -m "feat(engine): exclude git sync from the mas build via build tag"
```

---

## Task 2: Build script — accept `LINETTA_BUILD_TAGS`

**Files:**
- Modify: `scripts/build-engine.sh:73-76`

- [ ] **Step 1: Thread build tags into `go build`**

In `scripts/build-engine.sh`, replace this block (currently ~72-77):

```bash
echo "Building engine -> ${OUT}"
(
  cd "${ROOT}/engine"
  GOOS="${GOOS}" GOARCH="${GOARCH}" go build -o "${OUT}" ./cmd/linetta-engine
)
echo "ok"
```

with:

```bash
TAGS="${LINETTA_BUILD_TAGS:-}"
echo "Building engine -> ${OUT}${TAGS:+ (tags: ${TAGS})}"
(
  cd "${ROOT}/engine"
  if [ -n "${TAGS}" ]; then
    GOOS="${GOOS}" GOARCH="${GOARCH}" go build -tags "${TAGS}" -o "${OUT}" ./cmd/linetta-engine
  else
    GOOS="${GOOS}" GOARCH="${GOARCH}" go build -o "${OUT}" ./cmd/linetta-engine
  fi
)
echo "ok"
```

- [ ] **Step 2: Verify default build path unchanged**

Run: `bash scripts/build-engine.sh`
Expected: builds the sidecar into `apps/desktop/src-tauri/binaries/`, prints `ok`.

- [ ] **Step 3: Verify mas tag path**

Run: `LINETTA_BUILD_TAGS=mas bash scripts/build-engine.sh`
Expected: prints `Building engine -> ... (tags: mas)` then `ok`.

- [ ] **Step 4: Commit**

```bash
git add scripts/build-engine.sh
git commit -m "feat(build): let build-engine.sh pass LINETTA_BUILD_TAGS to go build"
```

---

## Task 3: Engine — migrate Keychain to the modern `SecItem*` API

The legacy `SecKeychain*` API targets the login keychain and is unusable under
App Sandbox. The modern `SecItem*` API uses the data-protection keychain (scoped
to the app's access group when sandboxed) and works in both contexts.

**Files:**
- Rewrite: `engine/internal/settings/secrets_darwin.go`
- Create: `engine/internal/settings/secrets_darwin_test.go`

- [ ] **Step 1: Write the failing round-trip test**

Create `engine/internal/settings/secrets_darwin_test.go`:

```go
//go:build darwin

package settings

import "testing"

func TestKeychainRoundTrip(t *testing.T) {
	k := keychainSecretStore{service: "devlikebear.linetta.test"}
	const name = "roundtrip-key"
	_ = k.Delete(name) // clean any leftover from a prior failed run

	if _, ok, err := k.Get(name); err != nil || ok {
		t.Fatalf("expected absent: ok=%v err=%v", ok, err)
	}
	if err := k.Set(name, "s3cr3t"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, ok, err := k.Get(name)
	if err != nil || !ok || got != "s3cr3t" {
		t.Fatalf("get after set: got=%q ok=%v err=%v", got, ok, err)
	}
	if ok, err := k.Exists(name); err != nil || !ok {
		t.Fatalf("exists: ok=%v err=%v", ok, err)
	}
	if err := k.Set(name, "updated"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got, _, _ := k.Get(name); got != "updated" {
		t.Fatalf("get after update: %q", got)
	}
	if err := k.Delete(name); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if ok, _ := k.Exists(name); ok {
		t.Fatalf("still exists after delete")
	}
}
```

- [ ] **Step 2: Run the test against the legacy implementation**

Run: `cd engine && go test ./internal/settings/ -run TestKeychainRoundTrip -v`
Expected: PASS against the current legacy code (this confirms the test is correct before we rewrite). If macOS prompts to allow keychain access, click "Always Allow".

- [ ] **Step 3: Rewrite `secrets_darwin.go` with `SecItem*`**

Replace the ENTIRE contents of `engine/internal/settings/secrets_darwin.go` with:

```go
//go:build darwin

package settings

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

// linetta_kc_query builds a base generic-password query dict for (service, account).
static CFMutableDictionaryRef linetta_kc_query(const char *service, const char *account) {
    CFMutableDictionaryRef q = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
    CFStringRef s = CFStringCreateWithCString(kCFAllocatorDefault, service, kCFStringEncodingUTF8);
    CFStringRef a = CFStringCreateWithCString(kCFAllocatorDefault, account, kCFStringEncodingUTF8);
    CFDictionarySetValue(q, kSecAttrService, s);
    CFDictionarySetValue(q, kSecAttrAccount, a);
    CFRelease(s);
    CFRelease(a);
    return q;
}

// linetta_kc_get copies the password data. On success mallocs *out (len *outLen); caller frees.
static OSStatus linetta_kc_get(const char *service, const char *account, void **out, int *outLen) {
    CFMutableDictionaryRef q = linetta_kc_query(service, account);
    CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
    CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
    CFTypeRef result = NULL;
    OSStatus status = SecItemCopyMatching(q, &result);
    CFRelease(q);
    if (status != errSecSuccess) return status;
    CFDataRef data = (CFDataRef)result;
    CFIndex len = CFDataGetLength(data);
    void *buf = malloc(len);
    memcpy(buf, CFDataGetBytePtr(data), len);
    CFRelease(result);
    *out = buf;
    *outLen = (int)len;
    return errSecSuccess;
}

// linetta_kc_exists returns errSecSuccess if present, errSecItemNotFound otherwise.
static OSStatus linetta_kc_exists(const char *service, const char *account) {
    CFMutableDictionaryRef q = linetta_kc_query(service, account);
    CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
    OSStatus status = SecItemCopyMatching(q, NULL);
    CFRelease(q);
    return status;
}

// linetta_kc_set updates an existing item or adds a new one.
static OSStatus linetta_kc_set(const char *service, const char *account, const void *value, int valueLen) {
    CFMutableDictionaryRef q = linetta_kc_query(service, account);
    CFDataRef data = CFDataCreate(kCFAllocatorDefault, (const UInt8 *)value, valueLen);
    CFMutableDictionaryRef attrs = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(attrs, kSecValueData, data);
    OSStatus status = SecItemUpdate(q, attrs);
    if (status == errSecItemNotFound) {
        CFDictionarySetValue(q, kSecValueData, data);
        status = SecItemAdd(q, NULL);
    }
    CFRelease(attrs);
    CFRelease(data);
    CFRelease(q);
    return status;
}

// linetta_kc_delete removes the item.
static OSStatus linetta_kc_delete(const char *service, const char *account) {
    CFMutableDictionaryRef q = linetta_kc_query(service, account);
    OSStatus status = SecItemDelete(q);
    CFRelease(q);
    return status;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

const (
	keychainService    = "devlikebear.linetta"
	errSecItemNotFound = -25300
)

func defaultSecretStore() SecretStore {
	return keychainSecretStore{service: keychainService}
}

type keychainSecretStore struct {
	service string
}

func (k keychainSecretStore) Get(name string) (string, bool, error) {
	cService, cAccount := C.CString(k.service), C.CString(name)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cAccount))

	var out unsafe.Pointer
	var outLen C.int
	status := C.linetta_kc_get(cService, cAccount, &out, &outLen)
	if status == errSecItemNotFound {
		return "", false, nil
	}
	if status != 0 {
		return "", false, keychainError("get", status)
	}
	defer C.free(out)
	return string(C.GoBytes(out, outLen)), true, nil
}

func (k keychainSecretStore) Exists(name string) (bool, error) {
	cService, cAccount := C.CString(k.service), C.CString(name)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cAccount))

	status := C.linetta_kc_exists(cService, cAccount)
	if status == errSecItemNotFound {
		return false, nil
	}
	if status != 0 {
		return false, keychainError("exists", status)
	}
	return true, nil
}

func (k keychainSecretStore) Set(name, value string) error {
	if value == "" {
		return k.Delete(name)
	}
	cService, cAccount := C.CString(k.service), C.CString(name)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cAccount))

	cValue := C.CBytes([]byte(value))
	defer C.free(cValue)

	status := C.linetta_kc_set(cService, cAccount, cValue, C.int(len(value)))
	if status != 0 {
		return keychainError("set", status)
	}
	return nil
}

func (k keychainSecretStore) Delete(name string) error {
	cService, cAccount := C.CString(k.service), C.CString(name)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cAccount))

	status := C.linetta_kc_delete(cService, cAccount)
	if status != 0 && status != errSecItemNotFound {
		return keychainError("delete", status)
	}
	return nil
}

func keychainError(op string, status C.OSStatus) error {
	return fmt.Errorf("settings: keychain %s failed: %d", op, int(status))
}
```

- [ ] **Step 4: Run the round-trip test against the new implementation**

Run: `cd engine && go test ./internal/settings/ -run TestKeychainRoundTrip -v`
Expected: PASS. (Allow keychain access if prompted.)

- [ ] **Step 5: Run the full settings package tests**

Run: `cd engine && go test ./internal/settings/`
Expected: PASS (no regression in the rest of the package).

- [ ] **Step 6: Commit**

```bash
git add engine/internal/settings/secrets_darwin.go engine/internal/settings/secrets_darwin_test.go
git commit -m "feat(engine): use modern SecItem keychain API for sandbox compatibility"
```

---

## Task 4: Entitlements files

**Files:**
- Create: `apps/desktop/src-tauri/entitlements/linetta.entitlements`
- Create: `apps/desktop/src-tauri/entitlements/linetta-sidecar.entitlements`

- [ ] **Step 1: Create the main app entitlements**

Create `apps/desktop/src-tauri/entitlements/linetta.entitlements`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>com.apple.security.app-sandbox</key>
	<true/>
	<key>com.apple.security.network.client</key>
	<true/>
	<key>com.apple.security.files.user-selected.read-write</key>
	<true/>
</dict>
</plist>
```

- [ ] **Step 2: Create the sidecar entitlements**

Create `apps/desktop/src-tauri/entitlements/linetta-sidecar.entitlements`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>com.apple.security.app-sandbox</key>
	<true/>
	<key>com.apple.security.inherit</key>
	<true/>
</dict>
</plist>
```

- [ ] **Step 3: Validate the plists parse**

Run: `plutil -lint apps/desktop/src-tauri/entitlements/linetta.entitlements apps/desktop/src-tauri/entitlements/linetta-sidecar.entitlements`
Expected: both print `OK`.

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src-tauri/entitlements/
git commit -m "feat(macos): add App Sandbox entitlements for app and sidecar"
```

---

## Task 5: Tauri MAS config overlay

**Files:**
- Create: `apps/desktop/src-tauri/tauri.mas.conf.json`

- [ ] **Step 1: Create the overlay**

Create `apps/desktop/src-tauri/tauri.mas.conf.json`:

```json
{
  "$schema": "https://schema.tauri.app/config/2",
  "bundle": {
    "macOS": {
      "entitlements": "entitlements/linetta.entitlements"
    }
  }
}
```

- [ ] **Step 2: Validate the overlay JSON**

Run: `python3 -c "import json; json.load(open('apps/desktop/src-tauri/tauri.mas.conf.json')); print('JSON OK')"`
Expected: prints `JSON OK`. (The real config merge is exercised by the build in Task 7.)

- [ ] **Step 3: Commit**

```bash
git add apps/desktop/src-tauri/tauri.mas.conf.json
git commit -m "feat(macos): add Tauri MAS config overlay with sandbox entitlements"
```

---

## Task 6: `open_path` via the opener plugin (sandbox-safe)

App Sandbox blocks spawning `/usr/bin/open`. Replace the subprocess with
`tauri-plugin-opener`, which uses LaunchServices/NSWorkspace.

**Files:**
- Modify: `apps/desktop/src-tauri/Cargo.toml`
- Modify: `apps/desktop/src-tauri/src/lib.rs:147-172` (the `open_path` command) and the plugin registration in `run()`

- [ ] **Step 1: Add the dependency**

Run: `cd apps/desktop/src-tauri && cargo add tauri-plugin-opener`
Expected: `tauri-plugin-opener` added to `Cargo.toml` `[dependencies]`.

- [ ] **Step 2: Register the plugin**

In `apps/desktop/src-tauri/src/lib.rs`, find the `tauri::Builder::default()` chain inside `run()` and add the opener plugin registration. Locate the existing line that registers a plugin (e.g. `.plugin(tauri_plugin_shell::init())`) and add immediately after it:

```rust
        .plugin(tauri_plugin_opener::init())
```

(If unsure where the builder is, run `grep -n "Builder::default\|\.plugin(" apps/desktop/src-tauri/src/lib.rs` to find the chain.)

- [ ] **Step 3: Rewrite the `open_path` command**

In `apps/desktop/src-tauri/src/lib.rs`, replace the entire `open_path` function (currently lines ~147-172) with:

```rust
#[tauri::command]
async fn open_path(app: tauri::AppHandle, path: String) -> Result<(), String> {
    use tauri_plugin_opener::OpenerExt;
    let path = path.trim();
    if path.is_empty() {
        return Err("path required".to_string());
    }
    app.opener()
        .open_path(path, None::<&str>)
        .map_err(|e| e.to_string())
}
```

- [ ] **Step 4: Type-check the Rust shell**

Run: `cd apps/desktop/src-tauri && cargo check`
Expected: compiles with no errors. (If `Command` is now unused, remove its `use` import to clear the warning — run `cargo check` again to confirm.)

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src-tauri/Cargo.toml apps/desktop/src-tauri/Cargo.lock apps/desktop/src-tauri/src/lib.rs
git commit -m "feat(macos): open paths via opener plugin for sandbox compatibility"
```

---

## Task 7: Local MAS build script

**Files:**
- Create: `scripts/build-mas-local.sh`
- Modify: `Makefile`

- [ ] **Step 1: Write the build script**

Create `scripts/build-mas-local.sh`:

```bash
#!/usr/bin/env bash
# Local Mac App Store (sandbox) build: build the engine with the `mas` tag (no
# git sync) + Tauri app with the MAS config overlay, then sign the sidecar with
# inherit entitlements and the app with sandbox entitlements.
#
# This signs with the existing Developer ID identity for LOCAL sandbox
# verification. Submitting to the App Store (Apple Distribution cert + .pkg) is a
# later phase.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_FILE="${LINETTA_APPLE_CONFIG:-${HOME}/.linetta/apple/config.env}"

ENT_APP="${ROOT}/apps/desktop/src-tauri/entitlements/linetta.entitlements"
ENT_SIDECAR="${ROOT}/apps/desktop/src-tauri/entitlements/linetta-sidecar.entitlements"

# Resolve the Developer ID Application identity (override via APPLE_SIGNING_IDENTITY).
SIGNING_IDENTITY="${APPLE_SIGNING_IDENTITY:-}"
if [ -z "${SIGNING_IDENTITY}" ] && [ -f "${CONFIG_FILE}" ]; then
  # shellcheck disable=SC1090
  set -a; . "${CONFIG_FILE}"; set +a
fi
if [ -z "${SIGNING_IDENTITY}" ]; then
  SIGNING_IDENTITY="$(
    security find-identity -v -p codesigning \
      | awk -F '"' '/Developer ID Application/ { print $2; exit }'
  )"
fi
if [ -z "${SIGNING_IDENTITY}" ]; then
  echo "No 'Developer ID Application' signing identity found." >&2
  exit 1
fi
echo "Signing identity: ${SIGNING_IDENTITY}"

echo "Building engine sidecar (mas: git sync excluded)"
LINETTA_BUILD_TAGS=mas bash "${ROOT}/scripts/build-engine.sh"

echo "Building the sandboxed macOS app"
cd "${ROOT}/apps/desktop"
pnpm tauri build --config src-tauri/tauri.mas.conf.json --bundles app

APP="${ROOT}/apps/desktop/src-tauri/target/release/bundle/macos/Linetta.app"
SIDECAR="${APP}/Contents/MacOS/linetta-engine"

echo "Signing sidecar (inherit) then app (sandbox)"
codesign --force --options runtime --timestamp \
  --sign "${SIGNING_IDENTITY}" --entitlements "${ENT_SIDECAR}" "${SIDECAR}"
codesign --force --options runtime --timestamp \
  --sign "${SIGNING_IDENTITY}" --entitlements "${ENT_APP}" "${APP}"

echo "=== Verification ==="
codesign --verify --deep --strict --verbose=2 "${APP}"
echo "--- app entitlements ---"
codesign -d --entitlements - "${APP}"
echo "--- sidecar entitlements ---"
codesign -d --entitlements - "${SIDECAR}"

echo ""
echo "Done."
echo "  App: ${APP}"
echo "  Run it, then check for sandbox violations with:"
echo "    log stream --style compact --predicate 'sender == \"sandboxd\"'"
```

- [ ] **Step 2: Make it executable**

Run: `chmod +x scripts/build-mas-local.sh`
Expected: no output.

- [ ] **Step 3: Add the Makefile target**

In `Makefile`, add `build-mas-local` to the `.PHONY` line (after `release-macos-local`):

```make
.PHONY: help dev test test-go test-desktop test-tauri validate-distribution build-engine build-desktop release-macos-local build-mas-local bump-version ci
```

And add this target immediately after the `release-macos-local` target block:

```make
build-mas-local: ## Build + sign a sandboxed macOS app locally (MAS prep, Developer ID signed)
	bash scripts/build-mas-local.sh
```

- [ ] **Step 4: Run the MAS build**

Run: `make build-mas-local`
Expected: builds, signs, and the verification section prints:
- `Linetta.app: valid on disk` and `satisfies its Designated Requirement`
- app entitlements include `com.apple.security.app-sandbox`, `network.client`, `files.user-selected.read-write`
- sidecar entitlements include `com.apple.security.inherit`

- [ ] **Step 5: Commit**

```bash
git add scripts/build-mas-local.sh Makefile
git commit -m "feat(build): add local MAS sandbox build script and make target"
```

---

## Task 8: Local sandbox verification (completion criteria)

This task has no code — it confirms the sandboxed app actually works. Do it by
hand and record the outcome.

**Files:** none.

- [ ] **Step 1: Confirm entitlements are applied**

Run: `codesign -d --entitlements - "apps/desktop/src-tauri/target/release/bundle/macos/Linetta.app" 2>/dev/null`
Expected: output contains `com.apple.security.app-sandbox` = true.

Run: `codesign -d --entitlements - "apps/desktop/src-tauri/target/release/bundle/macos/Linetta.app/Contents/MacOS/linetta-engine" 2>/dev/null`
Expected: output contains `com.apple.security.inherit` = true.

- [ ] **Step 2: Start a sandbox violation monitor**

In a separate terminal, run: `log stream --style compact --predicate 'sender == "sandboxd"'`
Leave it running during Steps 3-4. Expected at the end: NO `deny` lines mentioning `Linetta` or `linetta-engine`.

- [ ] **Step 3: Launch the app and exercise core features**

Run: `open "apps/desktop/src-tauri/target/release/bundle/macos/Linetta.app"`

In the running app, verify each:
- Create a project, create a node, type and save content → reload the app, content persists (SQLite write/read inside the container).
- Configure an LLM provider, enter an API key, run one AI action → it succeeds (network.client entitlement + keychain).
- Quit and relaunch → the saved API key is still present (keychain `SecItem` persistence).
- Export a node to a `.md` file via the save dialog → file is written to the chosen location.
- Import a `.md` file via the open dialog → content loads.
- Open the backups folder (the "open folder" action) → Finder opens it (opener plugin).

Expected: all succeed.

- [ ] **Step 4: Confirm Git Sync is absent**

In the app, confirm any Git Sync action either is hidden or surfaces a graceful
"not available" message (the engine returns method-not-found). Confirm no `git`
process spawns: `pgrep -fl git` during a sync attempt shows nothing from Linetta.

Expected: no `git` subprocess, no crash.

- [ ] **Step 5: Record the result**

If all pass, the sandbox-compatibility phase is complete. If the violation
monitor showed a `deny`, note the resource it denied — that maps to a missing
entitlement (add it to `linetta.entitlements`) or a path outside the container
(must move into Application Support). Re-run `make build-mas-local` and repeat.

There is nothing to commit for this task unless a violation forced an
entitlement change (commit that change with the message
`fix(macos): add <entitlement> for <reason>`).

---

## Done

All spec requirements are covered:
- §1 Git Sync excluded via `mas` tag → Tasks 1-2
- §2 Keychain `SecItem` migration → Task 3
- §3 Tauri MAS variant (overlay + entitlements + signing order) → Tasks 4, 5, 7
- §4 `open_path` sandbox fix → Task 6
- §5 build script + local verification → Tasks 7, 8
