# Mac App Store Submission Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **NOTE — this plan has two kinds of tasks.** "Build" tasks create/verify scripts with NO external side effects (safe for subagents). "Live" and "Manual" tasks create REAL Apple resources, sign, and upload to App Store Connect — they are operator-run (the human + controller together), one-time, and require the user's Apple account and decisions. Do not delegate Live/Manual tasks to an unattended subagent.

**Goal:** Build a Mac App Store `.pkg` (Apple Distribution–signed app + embedded MAS provisioning profile + Mac Installer Distribution–signed pkg) locally and upload it successfully to App Store Connect.

**Architecture:** A tiny Go ASC-API helper (stdlib ES256 JWT + authenticated HTTP) provides the auth primitive; bash scripts orchestrate cert/App-ID/profile creation (openssl + jq) and the build→sign→package→upload pipeline. Apple-side setup is hybrid: API for certs/App ID/profile, manual for the App Store Connect app record.

**Tech Stack:** Go (stdlib only), bash, openssl, `xcrun altool`, `productbuild`, App Store Connect API.

**Spec:** `docs/superpowers/specs/2026-06-18-mac-app-store-submission-design.md`

**Credentials (already present, outside repo):** `~/.linetta/apple/config.env` defines `APP_STORE_CONNECT_KEY_ID=Z8W67QU9X9`, `APP_STORE_CONNECT_ISSUER_ID`, `APPLE_TEAM_ID=2QW8S2B594`, `AUTH_KEY_PATH=$HOME/.linetta/apple/AuthKey_Z8W67QU9X9.p8`.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `scripts/ascapi/go.mod` | Isolated module for the helper (no deps) | Create |
| `scripts/ascapi/main.go` | ES256 JWT + authenticated ASC API request (`ascapi METHOD PATH [bodyFile]`) | Create |
| `scripts/setup-mas-apple.sh` | Orchestrate App ID + Apple Distribution cert + Mac Installer cert + MAC_APP_STORE profile via the helper | Create |
| `apps/desktop/src-tauri/entitlements/linetta-mas.entitlements` | MAS app entitlements (sandbox + application-identifier + team-identifier) | Create |
| `scripts/release-mas-local.sh` | Build (mas) → embed profile → sign (Apple Distribution) → productbuild (Mac Installer) → validate | Create |
| `Makefile` | `release-mas-local` target | Modify |
| `~/.linetta/apple/config.env` | Add cert identities + profile path (OUTSIDE repo, not committed) | Modify (live) |

---

## Task 1: ASC API helper (Go, stdlib ES256 JWT)

**Files:**
- Create: `scripts/ascapi/go.mod`
- Create: `scripts/ascapi/main.go`

- [ ] **Step 1: Create the module file**

Create `scripts/ascapi/go.mod`:

```
module ascapi

go 1.26
```

- [ ] **Step 2: Write the helper**

Create `scripts/ascapi/main.go`:

```go
// Command ascapi makes one authenticated App Store Connect API request.
//
// Usage: ascapi METHOD PATH [bodyFile]
// Reads credentials from env: APP_STORE_CONNECT_KEY_ID, APP_STORE_CONNECT_ISSUER_ID,
// AUTH_KEY_PATH (path to the .p8). Prints the response body to stdout; exits 1 on
// HTTP >= 300 (after still printing the body, so callers can inspect errors).
package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 3 {
		fail("usage: ascapi METHOD PATH [bodyFile]")
	}
	method, path := os.Args[1], os.Args[2]
	var body io.Reader
	if len(os.Args) > 3 {
		b, err := os.ReadFile(os.Args[3])
		check(err)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "https://api.appstoreconnect.apple.com"+path, body)
	check(err)
	req.Header.Set("Authorization", "Bearer "+token())
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	check(err)
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	os.Stdout.Write(out)
	if resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "\nascapi: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}
}

func token() string {
	kid := must("APP_STORE_CONNECT_KEY_ID")
	iss := must("APP_STORE_CONNECT_ISSUER_ID")
	keyPath := expand(must("AUTH_KEY_PATH"))
	pemBytes, err := os.ReadFile(keyPath)
	check(err)
	blk, _ := pem.Decode(pemBytes)
	if blk == nil {
		fail("AUTH_KEY_PATH is not a PEM .p8 file")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	check(err)
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		fail("p8 key is not ECDSA")
	}

	now := time.Now().Unix()
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": kid, "typ": "JWT"})
	payload, _ := json.Marshal(map[string]any{
		"iss": iss, "iat": now, "exp": now + 1200, "aud": "appstoreconnect-v1",
	})
	signing := b64(header) + "." + b64(payload)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	check(err)
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + b64(sig)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func must(name string) string {
	v := os.Getenv(name)
	if v == "" {
		fail("missing env: " + name)
	}
	return v
}

func check(err error) {
	if err != nil {
		fail(err.Error())
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "ascapi: "+msg)
	os.Exit(2)
}
```

- [ ] **Step 3: Build it**

Run: `cd scripts/ascapi && go build ./... && go vet ./...`
Expected: no output (compiles + vets clean).

- [ ] **Step 4: Smoke-test auth against the live API (read-only)**

This GET is side-effect-free — it only verifies the JWT works.

Run:
```bash
set -a; . "$HOME/.linetta/apple/config.env"; set +a
( cd scripts/ascapi && go run . GET "/v1/apps?limit=1" ) | jq '.data | length, .errors // empty'
```
Expected: prints a number (0 or more) and no `errors` — i.e. HTTP 200. If it prints `HTTP 401`, the credentials/JWT are wrong; stop and fix before proceeding.

- [ ] **Step 5: Commit**

```bash
git add scripts/ascapi/go.mod scripts/ascapi/main.go
git commit -m "feat(scripts): add App Store Connect API helper (ES256 JWT)"
```

---

## Task 2: MAS entitlements file

**Files:**
- Create: `apps/desktop/src-tauri/entitlements/linetta-mas.entitlements`

- [ ] **Step 1: Create the entitlements**

Create `apps/desktop/src-tauri/entitlements/linetta-mas.entitlements` (Team ID `2QW8S2B594`, bundle id `com.devlikebear.linetta`):

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
	<key>com.apple.application-identifier</key>
	<string>2QW8S2B594.com.devlikebear.linetta</string>
	<key>com.apple.developer.team-identifier</key>
	<string>2QW8S2B594</string>
</dict>
</plist>
```

- [ ] **Step 2: Validate it parses**

Run: `plutil -lint apps/desktop/src-tauri/entitlements/linetta-mas.entitlements`
Expected: `OK`.

- [ ] **Step 3: Commit**

```bash
git add apps/desktop/src-tauri/entitlements/linetta-mas.entitlements
git commit -m "feat(macos): add Mac App Store entitlements (application-identifier)"
```

---

## Task 3: Apple setup orchestration script

**Files:**
- Create: `scripts/setup-mas-apple.sh`

This script is written now (Build task) but RUN in Task 4 (Live). It is idempotent-friendly: it skips creating a bundle ID / cert that already exists by id-lookup, but always (re)creates the profile.

- [ ] **Step 1: Write the script**

Create `scripts/setup-mas-apple.sh`:

```bash
#!/usr/bin/env bash
# One-time Mac App Store Apple-side setup via the App Store Connect API:
#   - register the App ID (bundle id) if missing
#   - create the Apple Distribution + Mac Installer Distribution certificates
#   - create a MAC_APP_STORE provisioning profile
# Private keys and the profile are written to ~/.linetta/apple/ (never committed).
#
# If any API call is rejected (some cert types can be account-holder-only), the
# script prints the error and the manual web-portal fallback for that step.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIR="${HOME}/.linetta/apple"
CONFIG="${LINETTA_APPLE_CONFIG:-${DIR}/config.env}"
# shellcheck disable=SC1090
set -a; . "${CONFIG}"; set +a
: "${APPLE_TEAM_ID:?}" "${APP_STORE_CONNECT_KEY_ID:?}" "${APP_STORE_CONNECT_ISSUER_ID:?}"

BUNDLE_ID="com.devlikebear.linetta"
api() { ( cd "${ROOT}/scripts/ascapi" && go run . "$@" ); }

echo "== 1. App ID =="
existing="$(api GET "/v1/bundleIds?filter[identifier]=${BUNDLE_ID}" | jq -r '.data[0].id // empty')"
if [ -n "${existing}" ]; then
  echo "bundle id already registered: ${existing}"
  BID="${existing}"
else
  cat > /tmp/bundleid.json <<JSON
{"data":{"type":"bundleIds","attributes":{"identifier":"${BUNDLE_ID}","name":"Linetta","platform":"MAC_OS","seedId":"${APPLE_TEAM_ID}"}}}
JSON
  BID="$(api POST /v1/bundleIds /tmp/bundleid.json | jq -r '.data.id')"
  echo "registered bundle id: ${BID}"
fi

# create_cert <DISTRIBUTION|MAC_INSTALLER_DISTRIBUTION> <basename> <p12-pass>
create_cert() {
  local ctype="$1" base="$2" pass="$3"
  local key="${DIR}/${base}.key" csr="${DIR}/${base}.csr" cer="${DIR}/${base}.cer" p12="${DIR}/${base}.p12"
  if [ ! -f "${key}" ]; then
    openssl req -new -newkey rsa:2048 -nodes -keyout "${key}" \
      -out "${csr}" -subj "/CN=Linetta ${ctype}/O=${APPLE_TEAM_ID}/C=US"
    chmod 600 "${key}"
  fi
  # csrContent must be the full CSR PEM as a JSON string; jq handles newline escaping.
  jq -n --arg t "${ctype}" --arg csr "$(cat "${csr}")" \
    '{data:{type:"certificates",attributes:{certificateType:$t,csrContent:$csr}}}' > /tmp/cert.json
  echo "creating ${ctype} certificate..."
  local content
  content="$(api POST /v1/certificates /tmp/cert.json | jq -r '.data.attributes.certificateContent')"
  printf '%s' "${content}" | base64 --decode > "${cer}.der"
  openssl x509 -inform der -in "${cer}.der" -out "${cer}"
  openssl pkcs12 -export -legacy -macalg sha1 -inkey "${key}" -in "${cer}" \
    -out "${p12}" -passout "pass:${pass}"
  security import "${p12}" -P "${pass}" -T /usr/bin/codesign -T /usr/bin/productbuild
  echo "imported ${ctype}: ${p12}"
}

echo "== 2. Apple Distribution cert =="
create_cert DISTRIBUTION mas_app_distribution linetta-mas-app
echo "== 3. Mac Installer Distribution cert =="
create_cert MAC_INSTALLER_DISTRIBUTION mas_installer_distribution linetta-mas-installer

echo "== 4. Provisioning profile =="
# Apple Distribution cert id (most recent DISTRIBUTION cert for this team)
CERTID="$(api GET "/v1/certificates?filter[certificateType]=DISTRIBUTION&sort=-createdDate&limit=1" | jq -r '.data[0].id')"
cat > /tmp/profile.json <<JSON
{"data":{"type":"profiles","attributes":{"name":"Linetta MAS","profileType":"MAC_APP_STORE"},"relationships":{"bundleId":{"data":{"type":"bundleIds","id":"${BID}"}},"certificates":{"data":[{"type":"certificates","id":"${CERTID}"}]}}}}
JSON
api POST /v1/profiles /tmp/profile.json | jq -r '.data.attributes.profileContent' \
  | base64 --decode > "${DIR}/linetta-mas.provisionprofile"
echo "wrote profile: ${DIR}/linetta-mas.provisionprofile"

rm -f /tmp/bundleid.json /tmp/cert.json /tmp/profile.json
echo ""
echo "Done. Verify identities:"
echo "  security find-identity -v -p codesigning | grep -E 'Apple Distribution|Mac Installer|3rd Party Mac'"
```

- [ ] **Step 2: Make executable + syntax-check**

Run: `chmod +x scripts/setup-mas-apple.sh && bash -n scripts/setup-mas-apple.sh`
Expected: no output (valid syntax). Do NOT run it yet — that's Task 4 (it creates real Apple resources).

- [ ] **Step 3: Commit**

```bash
git add scripts/setup-mas-apple.sh
git commit -m "feat(scripts): add Mac App Store Apple-side setup (certs, App ID, profile)"
```

---

## Task 4: LIVE — run Apple setup

**Operator-run. Creates real Apple resources. Not for unattended subagents.**

- [ ] **Step 1: Run the setup**

Run: `bash scripts/setup-mas-apple.sh`
Expected: prints registered bundle id, "imported DISTRIBUTION", "imported MAC_INSTALLER_DISTRIBUTION", and "wrote profile: …/linetta-mas.provisionprofile".

If a `POST /v1/certificates` returns `HTTP 403`/`409`: a cert of that type may already exist or be account-holder-restricted. Fallback: create it manually at https://developer.apple.com/account/resources/certificates/add using the CSR the script already wrote (`~/.linetta/apple/mas_app_distribution.csr` / `mas_installer_distribution.csr`), download the `.cer`, then re-run from the `openssl pkcs12`/`security import` lines (or import the downloaded cert with the matching `.key`).

- [ ] **Step 2: Verify signing identities**

Run: `security find-identity -v -p codesigning | grep -E 'Apple Distribution|Mac Installer|3rd Party Mac'`
Expected: two identities — one application ("Apple Distribution: Changheon Shin (2QW8S2B594)" or "3rd Party Mac Developer Application: …") and one installer ("Mac Installer Distribution: …" or "3rd Party Mac Developer Installer: …").

- [ ] **Step 3: Verify the profile**

Run: `security cms -D -i "$HOME/.linetta/apple/linetta-mas.provisionprofile" | plutil -p - | grep -E 'application-identifier|TeamIdentifier|com.devlikebear.linetta'`
Expected: shows `2QW8S2B594.com.devlikebear.linetta` (matching the entitlements from Task 2).

- [ ] **Step 4: Record identities in config.env (outside repo, not committed)**

Append to `~/.linetta/apple/config.env` the resolved identity strings + profile path (use the exact common names from Step 2):
```bash
MAS_APP_IDENTITY="Apple Distribution: Changheon Shin (2QW8S2B594)"
MAS_INSTALLER_IDENTITY="Mac Installer Distribution: Changheon Shin (2QW8S2B594)"
MAS_PROFILE_PATH="$HOME/.linetta/apple/linetta-mas.provisionprofile"
```
(There is nothing to commit — this file is outside the repo.)

---

## Task 5: MANUAL — create the App Store Connect app record

**User action in the App Store Connect portal. Required before upload (Task 8).**

- [ ] **Step 1: Create the app**

In App Store Connect → Apps → "+" → New App:
- Platform: macOS
- Name: a unique App Store name (e.g. "Linetta" — if taken, choose a variant)
- Primary language, Bundle ID: select `com.devlikebear.linetta`, SKU: any unique string (e.g. `linetta-macos`)
- Set up at least the minimum required fields. Pricing can be set later.

- [ ] **Step 2: Confirm**

Confirm the app appears in App Store Connect with bundle id `com.devlikebear.linetta`. (No command — this is portal state. The upload in Task 8 matches the build to this record by bundle id.)

---

## Task 6: MAS release pipeline script

**Files:**
- Create: `scripts/release-mas-local.sh`
- Modify: `Makefile`

- [ ] **Step 1: Write the release script**

Create `scripts/release-mas-local.sh`:

```bash
#!/usr/bin/env bash
# Build, sign, and package the Mac App Store .pkg locally (no upload).
#   engine (mas tag) -> Tauri build (mas config) -> embed provisioning profile ->
#   sign sidecar+app (Apple Distribution) -> productbuild (Mac Installer) -> validate
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIR="${HOME}/.linetta/apple"
CONFIG="${LINETTA_APPLE_CONFIG:-${DIR}/config.env}"
# shellcheck disable=SC1090
set -a; . "${CONFIG}"; set +a

ENT_APP="${ROOT}/apps/desktop/src-tauri/entitlements/linetta-mas.entitlements"
ENT_SIDECAR="${ROOT}/apps/desktop/src-tauri/entitlements/linetta-sidecar.entitlements"
PROFILE="${MAS_PROFILE_PATH:-${DIR}/linetta-mas.provisionprofile}"

# Resolve identities (override via config.env MAS_APP_IDENTITY / MAS_INSTALLER_IDENTITY).
APP_ID="${MAS_APP_IDENTITY:-}"
if [ -z "${APP_ID}" ]; then
  APP_ID="$(security find-identity -v -p codesigning | awk -F '"' '/Apple Distribution|3rd Party Mac Developer Application/ {print $2; exit}')"
fi
INST_ID="${MAS_INSTALLER_IDENTITY:-}"
if [ -z "${INST_ID}" ]; then
  INST_ID="$(security find-identity -v | awk -F '"' '/Mac Installer Distribution|3rd Party Mac Developer Installer/ {print $2; exit}')"
fi
[ -n "${APP_ID}" ] || { echo "no Apple Distribution identity" >&2; exit 1; }
[ -n "${INST_ID}" ] || { echo "no Mac Installer identity" >&2; exit 1; }
[ -f "${PROFILE}" ] || { echo "missing profile: ${PROFILE}" >&2; exit 1; }
echo "App identity:       ${APP_ID}"
echo "Installer identity: ${INST_ID}"

echo "Building engine (mas) + Tauri app"
LINETTA_BUILD_TAGS=mas bash "${ROOT}/scripts/build-engine.sh"
cd "${ROOT}/apps/desktop"
pnpm tauri build --config src-tauri/tauri.mas.conf.json --bundles app

APP="${ROOT}/apps/desktop/src-tauri/target/release/bundle/macos/Linetta.app"
SIDECAR="${APP}/Contents/MacOS/linetta-engine"
PKG="${ROOT}/apps/desktop/src-tauri/target/release/bundle/macos/Linetta.pkg"

echo "Embedding provisioning profile"
cp "${PROFILE}" "${APP}/Contents/embedded.provisionprofile"

echo "Signing sidecar then app (Apple Distribution, MAS entitlements)"
codesign --force --timestamp --sign "${APP_ID}" --entitlements "${ENT_SIDECAR}" "${SIDECAR}"
codesign --force --timestamp --sign "${APP_ID}" --entitlements "${ENT_APP}" "${APP}"

echo "Building installer package"
rm -f "${PKG}"
productbuild --component "${APP}" /Applications --sign "${INST_ID}" "${PKG}"

echo "=== Verification ==="
codesign --verify --deep --strict --verbose=2 "${APP}"
codesign -d --entitlements - "${APP}" 2>&1 | grep -E 'application-identifier|app-sandbox' || true
pkgutil --check-signature "${PKG}"
echo ""
echo "Built: ${PKG}"
echo "Upload with: make ... (see release notes) or scripts/upload step."
```

- [ ] **Step 2: Make executable + syntax-check**

Run: `chmod +x scripts/release-mas-local.sh && bash -n scripts/release-mas-local.sh`
Expected: no output. (Do not run the full build here — that's Task 7.)

- [ ] **Step 3: Add the Makefile target**

In `Makefile`, add `release-mas-local` to the `.PHONY` line (after `build-mas-local`):

```make
.PHONY: help dev test test-go test-desktop test-tauri validate-distribution build-engine build-desktop release-macos-local build-mas-local release-mas-local bump-version ci
```

Add the target after the `build-mas-local` target block:

```make
release-mas-local: ## Build + sign + package the Mac App Store .pkg locally
	bash scripts/release-mas-local.sh
```

- [ ] **Step 4: Commit**

```bash
git add scripts/release-mas-local.sh Makefile
git commit -m "feat(build): add Mac App Store package build script and make target"
```

---

## Task 7: LIVE — build, sign, package the .pkg

**Operator-run. Requires Task 4 (certs/profile) complete.**

- [ ] **Step 1: Run the pipeline**

Run: `make release-mas-local`
Expected (the `=== Verification ===` block):
- `Linetta.app: valid on disk` + `satisfies its Designated Requirement`
- entitlements dump shows `com.apple.application-identifier` and `com.apple.security.app-sandbox`
- `pkgutil --check-signature` reports the package signed by the Mac Installer / "Developer ID Installer"-class chain with status "signed by a certificate trusted for current user".
- prints `Built: …/Linetta.pkg`.

If signing fails with a provisioning-profile / application-identifier mismatch, re-check that `linetta-mas.entitlements` (Task 2) and the embedded profile (Task 4 Step 3) both carry `2QW8S2B594.com.devlikebear.linetta`.

---

## Task 8: LIVE — upload to App Store Connect

**Operator-run. Requires Task 5 (app record) and Task 7 (.pkg) complete.**

- [ ] **Step 1: Place the API key where altool looks for it**

Run:
```bash
mkdir -p "$HOME/.appstoreconnect/private_keys"
cp "$HOME/.linetta/apple/AuthKey_Z8W67QU9X9.p8" "$HOME/.appstoreconnect/private_keys/"
```
Expected: no output. (altool resolves `--apiKey Z8W67QU9X9` to this file.)

- [ ] **Step 2: Validate the package**

Run:
```bash
set -a; . "$HOME/.linetta/apple/config.env"; set +a
PKG="apps/desktop/src-tauri/target/release/bundle/macos/Linetta.pkg"
xcrun altool --validate-app -f "${PKG}" -t macos \
  --apiKey "${APP_STORE_CONNECT_KEY_ID}" --apiIssuer "${APP_STORE_CONNECT_ISSUER_ID}"
```
Expected: ends with no errors (validation success). If it reports asset/entitlement errors, fix and rebuild (Task 7) before uploading.

- [ ] **Step 2 (fallback): Transporter**

If altool is unavailable/broken, upload the same `.pkg` with the Transporter.app (Mac App Store) using the same API key. Note this in the run log.

- [ ] **Step 3: Upload**

Run:
```bash
xcrun altool --upload-app -f "${PKG}" -t macos \
  --apiKey "${APP_STORE_CONNECT_KEY_ID}" --apiIssuer "${APP_STORE_CONNECT_ISSUER_ID}"
```
Expected: "No errors uploading" (or similar success message).

- [ ] **Step 4: Verify in App Store Connect**

Confirm the build appears in App Store Connect (the app record from Task 5 → TestFlight/Builds), initially "Processing". This is the completion criterion.

---

## Done

Spec coverage:
- §1 Apple setup (API certs/App ID/profile + manual app record) → Tasks 1, 3, 4, 5
- §2 MAS entitlements → Task 2
- §3 build/package/sign → Tasks 6, 7
- §4 upload → Task 8
- §5 credentials in `~/.linetta/apple/`, not committed → Tasks 4, 8 (no secrets enter git)
- Verification (identities, entitlements, profile match, validate, upload) → Tasks 4, 7, 8
