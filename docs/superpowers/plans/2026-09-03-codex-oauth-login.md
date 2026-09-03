# Codex OAuth(PKCE) 앱 내 로그인 (#92) — 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> 커밋은 기능 단위로, `feat/fix/chore/test/docs` 메시지로. 동작이 바뀌는 작업은 실패하는 테스트를 먼저 쓴다. 각 작업 종료 시 해당 패키지 테스트를 통과시키고, 마지막 작업에서 `make test`를 통과시킨다.
>
> **설계 문서:** `docs/superpowers/specs/2026-09-02-builtin-agent-byok-design.md` 5.3절. **이슈:** #92 (에픽 #90). **선행:** #91 (머지 완료 — `internal/provider`, `providers.*` RPC, 이유 코드 체계가 이미 있다).

**Goal:** 외부 CLI 없이 Linetta 안에서 ChatGPT(Codex) 계정으로 로그인해, 구독으로 프론티어 모델을 쓸 수 있게 한다.

**Architecture:** 새 패키지 `internal/codexauth`가 OAuth PKCE 흐름 전체를 담당한다. 인가 URL을 만들어 셸이 OS 브라우저로 열고, `127.0.0.1`의 일회용 콜백 리스너가 코드를 받아 토큰으로 교환한 뒤 **Codex CLI와 같은 형식의 `auth.json`** 을 Linetta 데이터 디렉터리에 0600으로 쓴다. 그 뒤로는 tars가 그 파일을 읽고 만료 시 갱신까지 알아서 한다 — tars는 건드리지 않는다.

**Tech Stack:** Go 1.26 엔진 (`net/http`, `crypto/sha256`, `crypto/rand` — 새 의존성 없음), Tauri 2 / Rust 셸, React 18 + TypeScript + Vitest.

## Global Constraints

- 엔진 모듈은 `github.com/devlikebear/linetta/engine`. 빌드 태그 `mas`와 `mobile`은 독립이며 둘 다 계속 빌드되어야 한다: `make test`, `make test-mobile-engine`, `cd engine && go build -tags mas ./...`, `cd engine && GOOS=windows GOARCH=amd64 go build ./...`.
- **프로토콜 상수는 아래 표의 값을 그대로 쓰고, 환경 변수 오버라이드를 두지 않는다.** Codex CLI 소스(`openai/codex`, `codex-rs/login/src/{server.rs, auth/manager.rs, token_data.rs}`, 2026-09-02 확인)에서 가져온 값이다.

  | 항목 | 값 |
  | --- | --- |
  | 발급자 | `https://auth.openai.com` |
  | 인가 | `{발급자}/oauth/authorize` |
  | 토큰 교환 | `{발급자}/oauth/token` (POST, `grant_type=authorization_code`) |
  | 클라이언트 id | `app_EMoamEEZ73f0CkXaXp7hrann` |
  | redirect_uri | `http://localhost:1455/auth/callback`, 1455가 막히면 `1457` |
  | scope | `openid profile email offline_access api.connectors.read api.connectors.invoke` |
  | 추가 쿼리 | `id_token_add_organizations=true`, `codex_cli_simplified_flow=true`, `originator=linetta` |
  | PKCE | S256 |

- **`auth.json` 형식은 Codex CLI와 같아야 한다.** `{"auth_mode":"chatgpt","tokens":{"id_token","access_token","refresh_token","account_id"},"last_refresh":"<RFC3339>"}`. tars는 `tokens.access_token` / `tokens.refresh_token` / `tokens.account_id`만 요구하지만 나머지도 함께 쓴다 — Codex CLI와 같은 파일을 공유할 수 있어야 하기 때문이다.
- **`account_id`는 `id_token`의 `https://api.openai.com/auth` 클레임 안 `chatgpt_account_id`에서 꺼낸다.** JWT는 검증하지 않고 페이로드만 디코드한다. 서명 검증은 토큰 엔드포인트를 TLS로 직접 호출해 받은 값이므로 불필요하고, 검증하려면 JWKS 의존성이 생긴다.
- 쓰기 대상은 **항상** `$LINETTA_HOME/codex/auth.json`, 파일 권한 0600, 디렉터리 0700.
- 포트 1455와 1457이 모두 막히면 **조용히 다른 포트로 가지 않는다.** redirect_uri가 등록된 값이라 다른 포트는 어차피 실패한다. `codex_port_in_use`로 보고한다.
- 로그인 대기는 5분에 끊는다. 새 로그인을 시작하면 이전 대기는 취소된다. **타임아웃은 조용하다** — `Start`는 즉시 반환하고 타임아웃은 리스너를 닫을 뿐이므로 엔진이 보고할 대상이 없다. 로그인이 비동기이고 설정 화면이 `login_status`를 폴링하므로, 시간 초과 판단은 `login_start`를 부른 시각을 아는 클라이언트(#94)가 한다.
- 콜백은 `state`가 일치할 때만 받는다. 성공 뒤 브라우저에는 **로컬 성공 페이지**를 보여준다 — Codex의 호스트된 페이지로 리다이렉트하지 않는다.
- 새 이유 코드는 `engine/internal/rpc/reason.go`의 기존 블록에 추가하고, `apps/desktop/src/lib/rpcMessage.ts`의 `REASON_MESSAGE_KEYS`와 `apps/desktop/src/lib/i18n.tsx`의 ko/en/ja 카탈로그에 **같은 커밋에서** 추가한다. 빠뜨리면 `String(error)`로 새어나간다(#91에서 고친 그 문제다).
- UI가 부르는 새 엔진 메서드는 `apps/desktop/src-tauri/src/lib.rs`의 `RENDERER_ENGINE_METHODS`에 **정렬 순서를 지켜** 추가하고(이진 탐색), 같은 커밋에서 `apps/desktop/src/lib/rpc.ts` 래퍼를 붙인다. `rpcAllowlist.test.ts`가 양방향 일치를 요구한다.
- `internal/codexauth`는 `tars/pkg/llm`을 import하지 않는다. 순수 HTTP다. `scripts/validate-story-core-deps.sh`가 이미 이를 강제한다.
- 이 계획은 UI를 만들지 않는다. 설정 화면의 Codex 로그인 버튼은 #94다.

---

## 파일 구조

| 파일 | 책임 |
| --- | --- |
| `engine/internal/codexauth/oauth.go` (신규) | 프로토콜 상수, PKCE 생성, state 생성, 인가 URL 조립 |
| `engine/internal/codexauth/oauth_test.go` (신규) | 위 순수 함수 검증 |
| `engine/internal/codexauth/authfile.go` (신규) | `auth.json` 읽기·쓰기·삭제, `id_token` 클레임 파싱 |
| `engine/internal/codexauth/authfile_test.go` (신규) | 파일 형식·권한·클레임 파싱 |
| `engine/internal/codexauth/login.go` (신규) | 콜백 리스너, 토큰 교환, 로그인 수명주기(`Service`) |
| `engine/internal/codexauth/login_test.go` (신규) | `httptest`로 토큰 엔드포인트를 흉내낸 종단 로그인 |
| `engine/internal/codexauth/success_page.go` (신규) | 브라우저에 보여줄 로컬 성공/실패 페이지 |
| `engine/internal/provider/codexhome.go` (신규) | 읽기 경로 결정: Linetta 우선, 없으면 `~/.codex` |
| `engine/internal/provider/codexhome_mas.go` (신규) | MAS 빌드는 폴백 없음 |
| `engine/internal/provider/codexhome_test.go` (신규) | 폴백 우선순위 |
| `engine/internal/provider/provider.go` (수정) | `Resolve`가 폴백을 거쳐 `CodexHome`을 채운다 |
| `engine/internal/rpc/reason.go` (수정) | `codex_port_in_use`, `codex_login_failed` |
| `engine/internal/rpc/handlers/codex.go` (신규) | `CodexService` 인터페이스와 세 핸들러 |
| `engine/internal/rpc/handlers/codex_test.go` (신규) | 가짜 서비스로 핸들러 검증 |
| `engine/internal/engineapp/codex.go` (신규) | `codexauth.Service` → `handlers.CodexService` 어댑터 |
| `engine/internal/engineapp/engineapp.go` (수정) | 서비스 생성, 핸들러 3개 등록, 종료 시 정리 |
| `engine/internal/engineapp/codex_wiring_test.go` (신규) | 등록 확인 |
| `apps/desktop/src-tauri/src/lib.rs` (수정) | 허용 목록 3개, `EXTERNAL_URL_HOSTS` 교체 |
| `apps/desktop/src/lib/types.ts`, `rpc.ts` (수정) | `CodexLoginStart`, `CodexStatus`, `codex.*` 래퍼 |
| `apps/desktop/src/lib/rpcMessage.ts`, `i18n.tsx` (수정) | 새 이유 코드 3개 번역 |
| `docs/privacy-policy.md` ko/en/ja (수정) | 토큰 파일 보관 위치 |

---

### Task 1: `codexauth` — 프로토콜 상수, PKCE, 인가 URL

**Files:**
- Create: `engine/internal/codexauth/oauth.go`
- Create: `engine/internal/codexauth/oauth_test.go`

**Interfaces:**
- Consumes: 없음 (이 계획의 첫 작업).
- Produces (후속 작업이 쓰는 이름):
  - 상수 `Issuer`, `ClientID`, `CallbackPath`, `PrimaryPort`, `FallbackPort`, `Scope`
  - `func newPKCE() (verifier, challenge string, err error)`
  - `func newState() (string, error)`
  - `func authorizeURL(challenge, state string, port int) string`
  - `func redirectURI(port int) string`

- [ ] **Step 1: 실패하는 테스트 작성** — `engine/internal/codexauth/oauth_test.go`

```go
package codexauth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestNewPKCE_challengeIsS256OfVerifier(t *testing.T) {
	verifier, challenge, err := newPKCE()
	if err != nil {
		t.Fatalf("newPKCE: %v", err)
	}
	// RFC 7636: the verifier is 43-128 chars from the unreserved set, and the
	// challenge is the base64url-without-padding SHA-256 of the verifier.
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("verifier length %d out of the RFC 7636 range", len(verifier))
	}
	if strings.ContainsAny(verifier, "+/=") {
		t.Errorf("verifier %q is not base64url without padding", verifier)
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != want {
		t.Errorf("challenge = %q, want %q", challenge, want)
	}
}

func TestNewPKCE_isDifferentEveryCall(t *testing.T) {
	a, _, err := newPKCE()
	if err != nil {
		t.Fatalf("newPKCE: %v", err)
	}
	b, _, err := newPKCE()
	if err != nil {
		t.Fatalf("newPKCE: %v", err)
	}
	if a == b {
		t.Fatal("two verifiers were identical; the source is not random")
	}
}

func TestNewState_isRandomAndURLSafe(t *testing.T) {
	a, err := newState()
	if err != nil {
		t.Fatalf("newState: %v", err)
	}
	b, _ := newState()
	if a == b {
		t.Fatal("two states were identical")
	}
	if a == "" || strings.ContainsAny(a, "+/=") {
		t.Errorf("state %q is not base64url without padding", a)
	}
}

func TestRedirectURI_usesLocalhostAndTheCodexPath(t *testing.T) {
	if got := redirectURI(1455); got != "http://localhost:1455/auth/callback" {
		t.Errorf("redirectURI(1455) = %q", got)
	}
	if got := redirectURI(1457); got != "http://localhost:1457/auth/callback" {
		t.Errorf("redirectURI(1457) = %q", got)
	}
}

func TestAuthorizeURL_carriesEveryParameterCodexSends(t *testing.T) {
	raw := authorizeURL("test-challenge", "test-state", 1455)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Scheme != "https" || u.Host != "auth.openai.com" || u.Path != "/oauth/authorize" {
		t.Fatalf("endpoint = %s://%s%s, want https://auth.openai.com/oauth/authorize", u.Scheme, u.Host, u.Path)
	}
	q := u.Query()
	want := map[string]string{
		"response_type":              "code",
		"client_id":                  "app_EMoamEEZ73f0CkXaXp7hrann",
		"redirect_uri":               "http://localhost:1455/auth/callback",
		"scope":                      "openid profile email offline_access api.connectors.read api.connectors.invoke",
		"code_challenge":             "test-challenge",
		"code_challenge_method":      "S256",
		"state":                      "test-state",
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow":  "true",
		"originator":                 "linetta",
	}
	for k, v := range want {
		if got := q.Get(k); got != v {
			t.Errorf("query %s = %q, want %q", k, got, v)
		}
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/codexauth/ 2>&1 | head`
Expected: 패키지가 없어 컴파일 실패.

- [ ] **Step 3: `engine/internal/codexauth/oauth.go` 생성**

```go
// Package codexauth logs a writer into their ChatGPT account so the built-in
// agent can call Codex on their subscription (#92).
//
// tars already reads the credential file and refreshes an expired token by
// itself; the only thing missing was the first login. So this package performs
// the OAuth PKCE dance and writes the same auth.json the Codex CLI writes —
// deliberately the same format, so the two tools can share one file.
//
// The protocol constants below come from the Codex CLI's own source
// (openai/codex, codex-rs/login), read on 2026-09-02. They are constants on
// purpose: an env-var override here would be a way to point Linetta's login at
// somebody else's endpoint.
package codexauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
)

const (
	// Issuer is OpenAI's OAuth host.
	Issuer = "https://auth.openai.com"
	// ClientID is the Codex CLI's registered public client.
	ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	// CallbackPath is the redirect path registered with that client.
	CallbackPath = "/auth/callback"
	// PrimaryPort and FallbackPort are the only two ports the registered
	// redirect URIs allow. A third port would be rejected by the issuer, so a
	// busy pair is an error the writer has to resolve, not something to route
	// around.
	PrimaryPort  = 1455
	FallbackPort = 1457
	// Scope is what the Codex CLI requests.
	Scope = "openid profile email offline_access api.connectors.read api.connectors.invoke"

	authorizeEndpoint = Issuer + "/oauth/authorize"
	tokenEndpoint     = Issuer + "/oauth/token"
)

// randomURLSafe returns n random bytes as base64url text without padding.
func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("codexauth: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// newPKCE returns a fresh verifier and its S256 challenge (RFC 7636). 32 bytes
// encode to 43 characters, the shortest length the RFC allows.
func newPKCE() (verifier, challenge string, err error) {
	verifier, err = randomURLSafe(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// newState returns the CSRF value the callback must echo back.
func newState() (string, error) { return randomURLSafe(32) }

// redirectURI is the address the issuer sends the browser back to. It says
// "localhost" rather than 127.0.0.1 because that is the string registered with
// the client, and the issuer compares it literally.
func redirectURI(port int) string {
	return fmt.Sprintf("http://localhost:%d%s", port, CallbackPath)
}

// authorizeURL builds the address the OS browser opens.
func authorizeURL(challenge, state string, port int) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", ClientID)
	q.Set("redirect_uri", redirectURI(port))
	q.Set("scope", Scope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	// The Codex CLI sends these three; the issuer's simplified consent screen
	// and the organization claims in the id_token depend on them.
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("originator", "linetta")
	return authorizeEndpoint + "?" + q.Encode()
}
```

- [ ] **Step 4: 통과 확인**

Run: `cd engine && go test ./internal/codexauth/ && go vet ./internal/codexauth/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add engine/internal/codexauth/oauth.go engine/internal/codexauth/oauth_test.go
git commit -m "feat(codexauth): the Codex OAuth constants, PKCE, and the authorize URL (#92)"
```

---

### Task 2: `codexauth` — `auth.json` 읽기·쓰기·삭제와 id_token 클레임

**Files:**
- Create: `engine/internal/codexauth/authfile.go`
- Create: `engine/internal/codexauth/authfile_test.go`

**Interfaces:**
- Consumes: Task 1의 패키지.
- Produces:
  - `type Tokens struct { IDToken, AccessToken, RefreshToken, AccountID string }` (JSON: `id_token`, `access_token`, `refresh_token`, `account_id`)
  - `type authFile struct { AuthMode string; Tokens Tokens; LastRefresh string }` (JSON: `auth_mode`, `tokens`, `last_refresh`)
  - `func AuthPath(codexHome string) string`
  - `func writeAuthFile(codexHome string, tok Tokens, now time.Time) error`
  - `func readAuthFile(codexHome string) (authFile, error)`
  - `func Logout(codexHome string) error`
  - `func claimsFromIDToken(idToken string) (email string, accountID string, expiresAt int64)`

- [ ] **Step 1: 실패하는 테스트 작성** — `engine/internal/codexauth/authfile_test.go`

```go
package codexauth

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fakeIDToken builds an unsigned JWT whose payload carries the claims the
// Codex issuer sends. Only the payload segment is ever read.
func fakeIDToken(t *testing.T, email, accountID string, exp int64) string {
	t.Helper()
	payload := map[string]any{
		"email": email,
		"exp":   exp,
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": accountID,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return "ignored-header." + base64.RawURLEncoding.EncodeToString(body) + ".ignored-signature"
}

func TestWriteAuthFile_matchesTheCodexCLIShape(t *testing.T) {
	home := t.TempDir()
	tok := Tokens{
		IDToken:      "id-token",
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		AccountID:    "acct-123",
	}
	at := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	if err := writeAuthFile(home, tok, at); err != nil {
		t.Fatalf("writeAuthFile: %v", err)
	}

	raw, err := os.ReadFile(AuthPath(home))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["auth_mode"] != "chatgpt" {
		t.Errorf("auth_mode = %v, want chatgpt", got["auth_mode"])
	}
	if got["last_refresh"] != "2026-09-03T10:00:00Z" {
		t.Errorf("last_refresh = %v", got["last_refresh"])
	}
	tokens, ok := got["tokens"].(map[string]any)
	if !ok {
		t.Fatalf("tokens is not an object: %v", got["tokens"])
	}
	for k, want := range map[string]string{
		"id_token": "id-token", "access_token": "access-token",
		"refresh_token": "refresh-token", "account_id": "acct-123",
	} {
		if tokens[k] != want {
			t.Errorf("tokens.%s = %v, want %q", k, tokens[k], want)
		}
	}
}

func TestWriteAuthFile_permissionsKeepTheTokenPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits do not apply on Windows")
	}
	home := filepath.Join(t.TempDir(), "codex")
	if err := writeAuthFile(home, Tokens{AccessToken: "a"}, time.Now()); err != nil {
		t.Fatalf("writeAuthFile: %v", err)
	}
	fi, err := os.Stat(AuthPath(home))
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("auth.json mode = %o, want 600", perm)
	}
	di, err := os.Stat(home)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("codex dir mode = %o, want 700", perm)
	}
}

func TestWriteAuthFile_replacesAnExistingLogin(t *testing.T) {
	home := t.TempDir()
	if err := writeAuthFile(home, Tokens{AccessToken: "first"}, time.Now()); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := writeAuthFile(home, Tokens{AccessToken: "second"}, time.Now()); err != nil {
		t.Fatalf("second write: %v", err)
	}
	got, err := readAuthFile(home)
	if err != nil {
		t.Fatalf("readAuthFile: %v", err)
	}
	if got.Tokens.AccessToken != "second" {
		t.Errorf("access token = %q, want the second login", got.Tokens.AccessToken)
	}
}

func TestReadAuthFile_missingFileIsNotFound(t *testing.T) {
	if _, err := readAuthFile(t.TempDir()); !os.IsNotExist(err) {
		t.Fatalf("err = %v, want a not-exist error", err)
	}
}

func TestLogout_removesTheFileAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if err := writeAuthFile(home, Tokens{AccessToken: "a"}, time.Now()); err != nil {
		t.Fatalf("writeAuthFile: %v", err)
	}
	if err := Logout(home); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := os.Stat(AuthPath(home)); !os.IsNotExist(err) {
		t.Errorf("auth.json survived logout: %v", err)
	}
	// Logging out twice is what a writer does when they are not sure it took.
	if err := Logout(home); err != nil {
		t.Errorf("second Logout: %v", err)
	}
}

func TestClaimsFromIDToken_readsEmailAccountAndExpiry(t *testing.T) {
	token := fakeIDToken(t, "writer@example.com", "acct-999", 1788000000)
	email, accountID, exp := claimsFromIDToken(token)
	if email != "writer@example.com" {
		t.Errorf("email = %q", email)
	}
	if accountID != "acct-999" {
		t.Errorf("accountID = %q", accountID)
	}
	if exp != 1788000000 {
		t.Errorf("exp = %d", exp)
	}
}

func TestClaimsFromIDToken_malformedInputYieldsEmptyValues(t *testing.T) {
	for _, tc := range []struct{ name, token string }{
		{"empty", ""},
		{"not a jwt", "nonsense"},
		{"two segments", "a.b"},
		{"payload is not base64", "a.!!!.c"},
		{"payload is not json", "a." + base64.RawURLEncoding.EncodeToString([]byte("nope")) + ".c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			email, accountID, exp := claimsFromIDToken(tc.token)
			if email != "" || accountID != "" || exp != 0 {
				t.Errorf("got (%q, %q, %d), want zero values", email, accountID, exp)
			}
		})
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/codexauth/ -run 'TestWriteAuthFile|TestReadAuthFile|TestLogout|TestClaims' 2>&1 | head`
Expected: 컴파일 실패 — `undefined: Tokens`, `undefined: writeAuthFile` 등.

- [ ] **Step 3: `engine/internal/codexauth/authfile.go` 생성**

```go
package codexauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// authFileName is what the Codex CLI calls its credential file, and what tars
// looks for under CodexHome.
const authFileName = "auth.json"

// Tokens is the credential set the issuer returns. The JSON names match the
// Codex CLI's file exactly so the two tools can read one another's login.
type Tokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

// authFile is the whole on-disk document.
type authFile struct {
	AuthMode    string `json:"auth_mode"`
	Tokens      Tokens `json:"tokens"`
	LastRefresh string `json:"last_refresh"`
}

// AuthPath is where the credential lives under a Codex home directory.
func AuthPath(codexHome string) string {
	return filepath.Join(codexHome, authFileName)
}

// writeAuthFile stores a completed login. The write goes to a temporary file
// first and is then renamed, so a crash mid-write cannot leave a half-written
// credential that reads as a broken login.
func writeAuthFile(codexHome string, tok Tokens, now time.Time) error {
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return fmt.Errorf("codexauth: create %s: %w", codexHome, err)
	}
	body, err := json.MarshalIndent(authFile{
		AuthMode:    "chatgpt",
		Tokens:      tok,
		LastRefresh: now.UTC().Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("codexauth: encode auth file: %w", err)
	}
	tmp := AuthPath(codexHome) + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return fmt.Errorf("codexauth: write auth file: %w", err)
	}
	if err := os.Rename(tmp, AuthPath(codexHome)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("codexauth: install auth file: %w", err)
	}
	return nil
}

// readAuthFile loads the stored login. A missing file returns an error that
// satisfies os.IsNotExist, which is how callers tell "not logged in" apart
// from "the file is broken".
func readAuthFile(codexHome string) (authFile, error) {
	raw, err := os.ReadFile(AuthPath(codexHome))
	if err != nil {
		return authFile{}, err
	}
	var f authFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return authFile{}, fmt.Errorf("codexauth: parse auth file: %w", err)
	}
	return f, nil
}

// Logout deletes the stored login. Removing a file that is not there is a
// success: the writer asked to end up logged out, and they are.
func Logout(codexHome string) error {
	err := os.Remove(AuthPath(codexHome))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("codexauth: remove auth file: %w", err)
	}
	return nil
}

// idClaims is the subset of the id_token payload worth reading.
type idClaims struct {
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
	Auth  struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
	} `json:"https://api.openai.com/auth"`
}

// claimsFromIDToken decodes the payload segment of the id_token. The signature
// is not verified and does not need to be: this token came back over TLS from
// the token endpoint we called ourselves, so there is no untrusted hop to
// guard against, and verifying would drag in a JWKS client for nothing.
//
// Anything malformed yields zero values rather than an error — these claims
// are for display, and a login is not worth failing over an unreadable email.
func claimsFromIDToken(idToken string) (email string, accountID string, expiresAt int64) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", "", 0
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", 0
	}
	var c idClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return "", "", 0
	}
	return c.Email, c.Auth.ChatGPTAccountID, c.Exp
}
```

- [ ] **Step 4: 통과 확인**

Run: `cd engine && go test ./internal/codexauth/ && go vet ./internal/codexauth/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add engine/internal/codexauth/authfile.go engine/internal/codexauth/authfile_test.go
git commit -m "feat(codexauth): read, write and clear the Codex credential file (#92)"
```

---

### Task 3: `codexauth` — 콜백 리스너, 토큰 교환, 로그인 수명주기

**Files:**
- Create: `engine/internal/codexauth/success_page.go`
- Create: `engine/internal/codexauth/login.go`
- Create: `engine/internal/codexauth/login_test.go`

**Interfaces:**
- Consumes: Task 1의 `newPKCE`/`newState`/`authorizeURL`/`redirectURI`/상수, Task 2의 `Tokens`/`writeAuthFile`/`readAuthFile`/`Logout`/`claimsFromIDToken`/`AuthPath`.
- Produces:
  - `var ErrPortInUse`, `var ErrLoginFailed`
  - `type Status struct { LoggedIn bool; Email string; AccountID string; ExpiresAt int64 }` (JSON: `logged_in`, `email`, `account_id`, `expires_at`)
  - `type Service struct{ ... }`, `func NewService(codexHome string) *Service`
  - `(*Service).WithTokenURL(string) *Service`, `(*Service).WithHTTPClient(*http.Client) *Service`, `(*Service).WithClock(func() time.Time) *Service`
  - `(*Service).Start(ctx context.Context) (authURL string, err error)`
  - `(*Service).Status() Status`
  - `(*Service).Logout() error`
  - `(*Service).Close() error`

- [ ] **Step 1: 실패하는 테스트 작성** — `engine/internal/codexauth/login_test.go`

```go
package codexauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// tokenServer stands in for https://auth.openai.com/oauth/token. It records
// the form it was posted so the test can assert the PKCE verifier travelled.
type tokenServer struct {
	*httptest.Server
	gotForm url.Values
	status  int
	body    string
}

func newTokenServer(t *testing.T, idToken string) *tokenServer {
	t.Helper()
	ts := &tokenServer{status: http.StatusOK}
	ts.body = `{"access_token":"acc","refresh_token":"ref","id_token":"` + idToken + `"}`
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts.gotForm, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(ts.status)
		_, _ = io.WriteString(w, ts.body)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// follow performs the browser's half: GET the callback with the code and state
// the issuer would have sent back.
func follow(t *testing.T, authURL, code string, overrideState string) *http.Response {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	q := u.Query()
	state := q.Get("state")
	if overrideState != "" {
		state = overrideState
	}
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		t.Fatalf("parse redirect uri: %v", err)
	}
	cb := *redirect
	cbq := url.Values{}
	cbq.Set("code", code)
	cbq.Set("state", state)
	cb.RawQuery = cbq.Encode()

	res, err := http.Get(cb.String())
	if err != nil {
		t.Fatalf("callback GET: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func newService(t *testing.T, idToken string) (*Service, *tokenServer, string) {
	t.Helper()
	home := t.TempDir()
	ts := newTokenServer(t, idToken)
	svc := NewService(home).WithTokenURL(ts.URL).WithHTTPClient(ts.Client())
	t.Cleanup(func() { _ = svc.Close() })
	return svc, ts, home
}

func TestStart_completesTheLoginAndWritesTheCredential(t *testing.T) {
	idToken := fakeIDToken(t, "writer@example.com", "acct-42", 1788000000)
	svc, ts, home := newService(t, idToken)

	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	res := follow(t, authURL, "the-code", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %d, want 200", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "Linetta") {
		t.Errorf("success page does not mention Linetta: %s", body)
	}

	// The credential must land before Status reports a login.
	deadline := time.Now().Add(5 * time.Second)
	var st Status
	for time.Now().Before(deadline) {
		if st = svc.Status(); st.LoggedIn {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !st.LoggedIn {
		t.Fatal("Status still reports logged out after a successful callback")
	}
	if st.Email != "writer@example.com" || st.AccountID != "acct-42" || st.ExpiresAt != 1788000000 {
		t.Errorf("status = %+v", st)
	}

	// The exchange must carry the PKCE verifier and the registered redirect.
	if got := ts.gotForm.Get("grant_type"); got != "authorization_code" {
		t.Errorf("grant_type = %q", got)
	}
	if got := ts.gotForm.Get("code"); got != "the-code" {
		t.Errorf("code = %q", got)
	}
	if got := ts.gotForm.Get("client_id"); got != ClientID {
		t.Errorf("client_id = %q", got)
	}
	verifier := ts.gotForm.Get("code_verifier")
	if len(verifier) < 43 {
		t.Errorf("code_verifier = %q, want the PKCE verifier", verifier)
	}
	authQuery, _ := url.Parse(authURL)
	if got, want := ts.gotForm.Get("redirect_uri"), authQuery.Query().Get("redirect_uri"); got != want {
		t.Errorf("redirect_uri = %q, want the one sent to authorize (%q)", got, want)
	}

	// And the file must be the Codex CLI's shape, with the account id lifted
	// out of the id_token.
	f, err := readAuthFile(home)
	if err != nil {
		t.Fatalf("readAuthFile: %v", err)
	}
	if f.Tokens.AccessToken != "acc" || f.Tokens.RefreshToken != "ref" || f.Tokens.AccountID != "acct-42" {
		t.Errorf("stored tokens = %+v", f.Tokens)
	}
}

func TestStart_rejectsACallbackWithTheWrongState(t *testing.T) {
	svc, _, home := newService(t, fakeIDToken(t, "a@b.c", "acct", 1))
	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	res := follow(t, authURL, "the-code", "forged-state")
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a mismatched state", res.StatusCode)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := readAuthFile(home); err == nil {
		t.Fatal("a forged callback wrote a credential")
	}
	if svc.Status().LoggedIn {
		t.Fatal("a forged callback produced a login")
	}
}

func TestStart_reportsAnIssuerRejection(t *testing.T) {
	svc, ts, home := newService(t, "")
	ts.status = http.StatusBadRequest
	ts.body = `{"error":"invalid_grant"}`

	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	res := follow(t, authURL, "stale-code", "")
	if res.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 when the exchange fails", res.StatusCode)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := readAuthFile(home); err == nil {
		t.Fatal("a failed exchange wrote a credential")
	}
}

func TestStart_bothPortsBusyIsAnError(t *testing.T) {
	// Occupy both registered ports so no listener can bind.
	for _, port := range []int{PrimaryPort, FallbackPort} {
		ln, err := net.Listen("tcp", "127.0.0.1:"+itoa(port))
		if err != nil {
			t.Skipf("port %d is already taken by something else; cannot run this case", port)
		}
		defer ln.Close() //nolint:revive // held for the length of the test on purpose
	}
	svc, _, _ := newService(t, "")
	_, err := svc.Start(context.Background())
	if !errors.Is(err, ErrPortInUse) {
		t.Fatalf("err = %v, want ErrPortInUse", err)
	}
}

func TestStart_secondCallCancelsTheFirst(t *testing.T) {
	svc, _, _ := newService(t, fakeIDToken(t, "a@b.c", "acct", 1))
	first, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	second, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if first == second {
		t.Fatal("the second login reused the first state; each attempt needs its own")
	}
	// The first attempt's callback must no longer be honoured.
	res := follow(t, first, "code", "")
	if res.StatusCode == http.StatusOK {
		t.Error("a superseded login attempt still accepted its callback")
	}
}

func TestStatus_reportsLoggedOutWithoutAFile(t *testing.T) {
	svc, _, _ := newService(t, "")
	if st := svc.Status(); st.LoggedIn || st.Email != "" {
		t.Errorf("status = %+v, want logged out", st)
	}
}

func TestLogoutMethod_clearsAStoredLogin(t *testing.T) {
	svc, _, home := newService(t, "")
	if err := writeAuthFile(home, Tokens{AccessToken: "a", IDToken: fakeIDToken(t, "x@y.z", "acct", 5)}, time.Now()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !svc.Status().LoggedIn {
		t.Fatal("seeded credential did not read as a login")
	}
	if err := svc.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if svc.Status().LoggedIn {
		t.Error("Logout left a login behind")
	}
}

func TestStart_timesOutAndStopsListening(t *testing.T) {
	svc, _, _ := newService(t, "")
	svc.loginWindow = 50 * time.Millisecond
	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	u, _ := url.Parse(authURL)
	redirect, _ := url.Parse(u.Query().Get("redirect_uri"))
	if _, err := http.Get(redirect.String()); err == nil {
		t.Error("the callback listener is still accepting connections after the window closed")
	}
}

// itoa avoids pulling strconv into the test's import list for one call.
func itoa(v int) string { return strings.TrimSpace(jsonNumber(v)) }

func jsonNumber(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/codexauth/ -run TestStart 2>&1 | head`
Expected: 컴파일 실패 — `undefined: NewService`, `undefined: ErrPortInUse` 등.

- [ ] **Step 3: `engine/internal/codexauth/success_page.go` 생성**

```go
package codexauth

// successPage is what the writer's browser shows when the login lands. It is
// served from the local callback rather than redirecting to OpenAI's hosted
// page: the writer's next step is in Linetta, and a hosted page would leave
// them looking at somebody else's brand wondering whether it worked.
const successPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>Signed in</title>
<style>
  body { font: 16px/1.6 -apple-system, "Segoe UI", system-ui, sans-serif;
         display: grid; place-items: center; min-height: 100vh; margin: 0;
         color: #2b2722; background: #f7f4ee; }
  main { text-align: center; padding: 2rem; }
  h1 { font-size: 1.25rem; font-weight: 600; margin: 0 0 .5rem; }
  p { margin: 0; color: #6b635a; }
</style></head>
<body><main>
  <h1>Signed in to ChatGPT</h1>
  <p>You can close this tab and return to Linetta.</p>
</main></body></html>`

// failurePage explains a callback Linetta refused or could not complete.
const failurePage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>Sign-in failed</title>
<style>
  body { font: 16px/1.6 -apple-system, "Segoe UI", system-ui, sans-serif;
         display: grid; place-items: center; min-height: 100vh; margin: 0;
         color: #2b2722; background: #f7f4ee; }
  main { text-align: center; padding: 2rem; }
  h1 { font-size: 1.25rem; font-weight: 600; margin: 0 0 .5rem; }
  p { margin: 0; color: #6b635a; }
</style></head>
<body><main>
  <h1>Sign-in did not complete</h1>
  <p>Close this tab and try again from Linetta.</p>
</main></body></html>`
```

- [ ] **Step 4: `engine/internal/codexauth/login.go` 생성**

```go
package codexauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ErrPortInUse means neither registered callback port could be bound. The
// redirect URI is registered per port, so there is no third port to try — a
// running Codex CLI login is the usual cause.
var ErrPortInUse = errors.New("codexauth: both callback ports are in use")

// ErrLoginFailed means the issuer refused the exchange.
var ErrLoginFailed = errors.New("codexauth: login failed")

// defaultLoginWindow bounds how long a callback listener stays open. Long
// enough to find a password, short enough that a forgotten attempt does not
// hold a port all day.
const defaultLoginWindow = 5 * time.Minute

// exchangeTimeout bounds the token request itself.
const exchangeTimeout = 30 * time.Second

// Status is what the settings pane shows about the stored login. It never
// carries a token — only whether one exists and whose account it is.
type Status struct {
	LoggedIn  bool   `json:"logged_in"`
	Email     string `json:"email,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

// attempt is one in-flight login: the listener, the secrets that tie the
// callback to the authorize request, and the cancel that tears it all down.
type attempt struct {
	state    string
	verifier string
	srv      *http.Server
	cancel   context.CancelFunc
}

// Service owns the login flow for one Codex home directory.
type Service struct {
	codexHome string
	tokenURL  string
	client    *http.Client
	now       func() time.Time

	loginWindow time.Duration

	mu      sync.Mutex
	current *attempt
}

// NewService returns a Service writing to codexHome. Nothing binds or dials
// until Start is called.
func NewService(codexHome string) *Service {
	return &Service{
		codexHome:   codexHome,
		tokenURL:    tokenEndpoint,
		client:      &http.Client{Timeout: exchangeTimeout},
		now:         time.Now,
		loginWindow: defaultLoginWindow,
	}
}

// WithTokenURL points the exchange somewhere else (tests).
func (s *Service) WithTokenURL(u string) *Service { s.tokenURL = u; return s }

// WithHTTPClient replaces the exchange client (tests).
func (s *Service) WithHTTPClient(c *http.Client) *Service { s.client = c; return s }

// WithClock replaces the clock stamped into the credential file (tests).
func (s *Service) WithClock(fn func() time.Time) *Service { s.now = fn; return s }

// Start opens a callback listener and returns the URL the OS browser should
// open. It returns as soon as the listener is up: the exchange happens on the
// callback, and the writer learns the outcome from Status.
//
// A second Start cancels the first. A writer who abandoned one attempt and
// clicked again should not have a stale listener holding the port, nor two
// listeners racing for one callback.
func (s *Service) Start(ctx context.Context) (string, error) {
	verifier, challenge, err := newPKCE()
	if err != nil {
		return "", err
	}
	state, err := newState()
	if err != nil {
		return "", err
	}

	ln, port, err := listenOnRegisteredPort()
	if err != nil {
		return "", err
	}

	s.stopCurrent()

	runCtx, cancel := context.WithTimeout(context.Background(), s.loginWindow)
	att := &attempt{state: state, verifier: verifier, cancel: cancel}

	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, s.callbackHandler(att, port))
	att.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	s.mu.Lock()
	s.current = att
	s.mu.Unlock()

	go func() {
		if err := att.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("codexauth: callback server: %v", err)
		}
	}()
	go func() {
		<-runCtx.Done()
		// Closing the listener is what makes the timeout observable: an
		// abandoned attempt must not hold a registered port.
		_ = att.srv.Close()
	}()

	return authorizeURL(challenge, state, port), nil
}

// callbackHandler completes the login. It is deliberately strict: the state
// must match this attempt, and anything else is refused without touching the
// stored credential.
func (s *Service) callbackHandler(att *attempt, port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		s.mu.Lock()
		superseded := s.current != att
		s.mu.Unlock()
		if superseded || q.Get("state") != att.state {
			writePage(w, http.StatusBadRequest, failurePage)
			return
		}
		if e := q.Get("error"); e != "" {
			log.Printf("codexauth: issuer refused the authorization: %s", e)
			writePage(w, http.StatusBadRequest, failurePage)
			return
		}
		code := q.Get("code")
		if code == "" {
			writePage(w, http.StatusBadRequest, failurePage)
			return
		}

		tok, err := s.exchange(r.Context(), code, att.verifier, port)
		if err != nil {
			log.Printf("codexauth: %v", err)
			writePage(w, http.StatusBadGateway, failurePage)
			return
		}
		_, accountID, _ := claimsFromIDToken(tok.IDToken)
		tok.AccountID = accountID
		if err := writeAuthFile(s.codexHome, tok, s.now()); err != nil {
			log.Printf("codexauth: %v", err)
			writePage(w, http.StatusInternalServerError, failurePage)
			return
		}
		writePage(w, http.StatusOK, successPage)

		// The writer has their answer; the listener's job is done.
		go s.stopCurrent()
	}
}

func writePage(w http.ResponseWriter, status int, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, page)
}

// exchange trades the authorization code for tokens.
func (s *Service) exchange(ctx context.Context, code, verifier string, port int) (Tokens, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI(port))
	form.Set("client_id", ClientID)
	form.Set("code_verifier", verifier)

	ctx, cancel := context.WithTimeout(ctx, exchangeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: build request: %v", ErrLoginFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := s.client.Do(req)
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: %v", ErrLoginFailed, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: read response: %v", ErrLoginFailed, err)
	}
	if res.StatusCode != http.StatusOK {
		// The body can carry the issuer's own wording; keep only the status so
		// a provider message never rides into a log verbatim.
		return Tokens{}, fmt.Errorf("%w: token endpoint returned %d", ErrLoginFailed, res.StatusCode)
	}
	var tok Tokens
	if err := json.Unmarshal(body, &tok); err != nil {
		return Tokens{}, fmt.Errorf("%w: parse response: %v", ErrLoginFailed, err)
	}
	if tok.AccessToken == "" || tok.RefreshToken == "" {
		return Tokens{}, fmt.Errorf("%w: response was missing a token", ErrLoginFailed)
	}
	return tok, nil
}

// Status reports the stored login without reading any token value out to the
// caller.
func (s *Service) Status() Status {
	f, err := readAuthFile(s.codexHome)
	if err != nil {
		return Status{}
	}
	if strings.TrimSpace(f.Tokens.AccessToken) == "" {
		return Status{}
	}
	email, accountID, exp := claimsFromIDToken(f.Tokens.IDToken)
	if accountID == "" {
		accountID = f.Tokens.AccountID
	}
	return Status{LoggedIn: true, Email: email, AccountID: accountID, ExpiresAt: exp}
}

// Logout deletes the stored login and drops any attempt in flight.
func (s *Service) Logout() error {
	s.stopCurrent()
	return Logout(s.codexHome)
}

// Close releases a listener still waiting on a callback.
func (s *Service) Close() error {
	s.stopCurrent()
	return nil
}

func (s *Service) stopCurrent() {
	s.mu.Lock()
	att := s.current
	s.current = nil
	s.mu.Unlock()
	if att == nil {
		return
	}
	att.cancel()
	_ = att.srv.Close()
}

// listenOnRegisteredPort binds the primary callback port, falling back to the
// secondary one. It never picks a third: the redirect URI is registered per
// port, so any other port would be refused by the issuer, and silently binding
// one would turn a clear error into a mystery in the browser.
func listenOnRegisteredPort() (net.Listener, int, error) {
	var lastErr error
	for _, port := range []int{PrimaryPort, FallbackPort} {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			return ln, port, nil
		}
		lastErr = err
		if !isAddrInUse(err) {
			return nil, 0, fmt.Errorf("codexauth: listen on 127.0.0.1:%d: %w", port, err)
		}
	}
	return nil, 0, fmt.Errorf("%w (%d, %d): %v", ErrPortInUse, PrimaryPort, FallbackPort, lastErr)
}

// isAddrInUse recognises a taken port across platforms. Windows reports
// WSAEADDRINUSE (10048) rather than EADDRINUSE, and some wrappers only leave
// the text behind.
func isAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && uintptr(errno) == 10048 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "only one usage of each socket address")
}

// compile-time assurance that os is used for the not-exist check in Status's
// helpers; keeps the import honest if readAuthFile's contract ever changes.
var _ = os.IsNotExist
```

- [ ] **Step 5: 통과 확인**

Run: `cd engine && go test ./internal/codexauth/ && go vet ./internal/codexauth/`
Expected: PASS. `TestStart_bothPortsBusyIsAnError`는 개발 머신에서 1455/1457이 이미 점유돼 있으면 skip된다 — skip은 정상이며, 그 경우 보고서에 적는다.

만약 `var _ = os.IsNotExist` 줄이 불필요하다고 판단되면(그 파일에서 `os`를 달리 쓰지 않는다면) `os` import와 함께 지운다. 컴파일러가 알려준다.

- [ ] **Step 6: 커밋**

```bash
git add engine/internal/codexauth/login.go engine/internal/codexauth/success_page.go engine/internal/codexauth/login_test.go
git commit -m "feat(codexauth): the browser login round trip, from authorize to auth.json (#92)"
```

---

### Task 4: `provider` — 읽기 경로 폴백 (`~/.codex`)

**Files:**
- Create: `engine/internal/provider/codexhome.go`
- Create: `engine/internal/provider/codexhome_mas.go`
- Create: `engine/internal/provider/codexhome_test.go`
- Modify: `engine/internal/provider/provider.go` (`Resolve`의 `CodexHome` 대입부)

**Interfaces:**
- Consumes: 기존 `Source.codexHome`(= `$LINETTA_HOME/codex`), `Resolved.CodexHome`, `codexAuthFile` 상수.
- Produces: `func resolveCodexHome(linettaCodexHome string) string` — 두 빌드 태그 파일이 각자 정의한다.

**배경:** 이미 Codex CLI로 로그인한 사람은 `~/.codex/auth.json`을 갖고 있다. 그걸 읽어주면 클릭 하나를 덜 수 있다. MAS 빌드는 샌드박스 컨테이너 밖을 못 읽으므로 폴백이 없다. 쓰기는 **언제나** Linetta 자신의 경로다(Task 3) — 폴백은 읽기 전용 편의다.

- [ ] **Step 1: 실패하는 테스트 작성** — `engine/internal/provider/codexhome_test.go`

```go
//go:build !mas

package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func writeAuth(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"tokens":{}}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
}

func TestResolveCodexHome_prefersLinettasOwnLogin(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	linetta := filepath.Join(t.TempDir(), "codex")
	writeAuth(t, linetta)
	writeAuth(t, filepath.Join(fakeHome, ".codex"))

	if got := resolveCodexHome(linetta); got != linetta {
		t.Errorf("resolveCodexHome = %q, want Linetta's own %q", got, linetta)
	}
}

func TestResolveCodexHome_fallsBackToTheCodexCLI(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	linetta := filepath.Join(t.TempDir(), "codex") // no auth.json written
	cli := filepath.Join(fakeHome, ".codex")
	writeAuth(t, cli)

	if got := resolveCodexHome(linetta); got != cli {
		t.Errorf("resolveCodexHome = %q, want the CLI's %q", got, cli)
	}
}

func TestResolveCodexHome_withNoLoginAnywhereReturnsLinettas(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	linetta := filepath.Join(t.TempDir(), "codex")

	// A future login has to land in Linetta's own directory, so that is the
	// answer when neither side has a credential yet.
	if got := resolveCodexHome(linetta); got != linetta {
		t.Errorf("resolveCodexHome = %q, want %q", got, linetta)
	}
}

func TestResolveCodexHome_emptyInputStaysEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if got := resolveCodexHome(""); got != "" {
		t.Errorf("resolveCodexHome(\"\") = %q, want empty so Configured() stays false", got)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/provider/ -run TestResolveCodexHome 2>&1 | head`
Expected: 컴파일 실패 — `undefined: resolveCodexHome`.

- [ ] **Step 3: `engine/internal/provider/codexhome.go` 생성**

```go
//go:build !mas

package provider

import (
	"os"
	"path/filepath"
)

// resolveCodexHome decides which directory tars should read Codex's auth.json
// from. Linetta's own login always wins; a Codex CLI login is honoured only
// when Linetta has none, so somebody who already ran `codex login` does not
// have to log in twice.
//
// Writes never come here — the login (#92) always stores into Linetta's own
// directory. This is a read-side convenience, and it is re-evaluated on every
// Resolve so a login performed mid-session takes effect immediately.
func resolveCodexHome(linettaCodexHome string) string {
	if linettaCodexHome == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(linettaCodexHome, codexAuthFile)); err == nil {
		return linettaCodexHome
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return linettaCodexHome
	}
	cli := filepath.Join(home, ".codex")
	if _, err := os.Stat(filepath.Join(cli, codexAuthFile)); err == nil {
		return cli
	}
	return linettaCodexHome
}
```

- [ ] **Step 4: `engine/internal/provider/codexhome_mas.go` 생성**

```go
//go:build mas

package provider

// resolveCodexHome has no fallback in the App Store build. The sandbox cannot
// read ~/.codex — the path resolves inside the app container, where the Codex
// CLI never wrote anything — so pretending to look there would only produce a
// confusing "logged in" that tars could not use.
func resolveCodexHome(linettaCodexHome string) string { return linettaCodexHome }
```

- [ ] **Step 5: `provider.go`의 `Resolve` 수정** — `CodexHome: s.codexHome,` 한 줄을 다음으로 바꾼다.

```go
		CodexHome:   resolveCodexHome(s.codexHome),
```

- [ ] **Step 6: 통과 확인**

Run: `cd engine && go test ./internal/provider/ && go build -tags mas ./... && go vet ./internal/provider/`
Expected: PASS. 기존 `TestConfigured_codexMeansTheAuthFileExists`와 `TestResolve_emptyMeansTheActiveProvider`가 계속 통과해야 한다. 후자는 `CodexHome`이 Linetta 경로와 같기를 기대하는데, 그 테스트의 임시 홈에는 `~/.codex`가 없으므로 폴백이 걸리지 않는다 — 다만 **개발 머신의 실제 `~/.codex`를 읽지 않도록** 그 테스트가 `HOME`을 격리하는지 확인하고, 아니라면 `t.Setenv("HOME", t.TempDir())`를 추가한다. (#91에서 키체인이 `LINETTA_HOME`으로 격리되지 않아 겪은 것과 같은 종류의 함정이다.)

- [ ] **Step 7: 커밋**

```bash
git add engine/internal/provider/codexhome.go engine/internal/provider/codexhome_mas.go engine/internal/provider/codexhome_test.go engine/internal/provider/provider.go
git commit -m "feat(provider): read an existing Codex CLI login when Linetta has none (#92)"
```

---

### Task 5: `codex.*` RPC — 이유 코드, 핸들러, 엔진 배선, 렌더러, 번역

**Files:**
- Modify: `engine/internal/rpc/reason.go`
- Create: `engine/internal/rpc/handlers/codex.go`
- Create: `engine/internal/rpc/handlers/codex_test.go`
- Create: `engine/internal/engineapp/codex.go`
- Modify: `engine/internal/engineapp/engineapp.go`
- Create: `engine/internal/engineapp/codex_wiring_test.go`
- Modify: `apps/desktop/src-tauri/src/lib.rs`
- Modify: `apps/desktop/src/lib/types.ts`, `apps/desktop/src/lib/rpc.ts`
- Modify: `apps/desktop/src/lib/rpcMessage.ts`, `apps/desktop/src/lib/i18n.tsx`
- Modify: `apps/desktop/src/lib/rpcMessage.test.ts`

**Interfaces:**
- Consumes: Task 3의 `codexauth.NewService`, `(*Service).Start/Status/Logout/Close`, `codexauth.Status`, `ErrPortInUse`/`ErrLoginFailed`; 기존 `rpc.ReasonError`, `rpc.MethodErrorFrom`.
- Produces:
  - 이유 코드 `rpc.ReasonCodexPortInUse` = `"codex_port_in_use"`, `rpc.ReasonCodexLoginFailed` = `"codex_login_failed"`
  - `type CodexService interface { LoginStart(ctx) (json.RawMessage, error); LoginStatus(ctx) (json.RawMessage, error); Logout(ctx) error }`
  - 핸들러 `CodexLoginStart`, `CodexLoginStatus`, `CodexLogout`
  - RPC `codex.login_start` → `{auth_url}`; `codex.login_status` → `Status`; `codex.logout` → `{ok:true}`
  - 프론트 `codex.loginStart()`, `codex.loginStatus()`, `codex.logout()`; 타입 `CodexLoginStart`, `CodexStatus`

- [ ] **Step 1: 실패하는 핸들러 테스트 작성** — `engine/internal/rpc/handlers/codex_test.go`

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type fakeCodexService struct {
	start   json.RawMessage
	status  json.RawMessage
	err     error
	loggedOut bool
}

func (f *fakeCodexService) LoginStart(context.Context) (json.RawMessage, error) {
	return f.start, f.err
}
func (f *fakeCodexService) LoginStatus(context.Context) (json.RawMessage, error) {
	return f.status, f.err
}
func (f *fakeCodexService) Logout(context.Context) error {
	f.loggedOut = true
	return f.err
}

func TestCodexLoginStart_returnsTheAuthURL(t *testing.T) {
	svc := &fakeCodexService{start: json.RawMessage(`{"auth_url":"https://auth.openai.com/oauth/authorize?x=1"}`)}
	res, err := CodexLoginStart(svc)(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if string(res) != `{"auth_url":"https://auth.openai.com/oauth/authorize?x=1"}` {
		t.Errorf("payload = %s", res)
	}
}

func TestCodexLoginStart_portInUseKeepsItsReasonCode(t *testing.T) {
	svc := &fakeCodexService{err: &rpc.ReasonError{
		Reason: rpc.ReasonCodexPortInUse, Err: errors.New("1455, 1457 busy"),
	}}
	_, err := CodexLoginStart(svc)(context.Background(), nil)
	var me *rpc.MethodError
	if !errors.As(err, &me) {
		t.Fatalf("want MethodError, got %v", err)
	}
	if me.Code != rpc.CodeInvalidParams {
		t.Errorf("code = %d", me.Code)
	}
	var data struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(me.Data, &data)
	if data.Reason != "codex_port_in_use" {
		t.Errorf("reason = %q", data.Reason)
	}
}

func TestCodexLoginStatus_passesTheServicePayload(t *testing.T) {
	svc := &fakeCodexService{status: json.RawMessage(`{"logged_in":true,"email":"w@example.com"}`)}
	res, err := CodexLoginStatus(svc)(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if string(res) != `{"logged_in":true,"email":"w@example.com"}` {
		t.Errorf("payload = %s", res)
	}
}

func TestCodexLogout_callsTheServiceAndReportsOK(t *testing.T) {
	svc := &fakeCodexService{}
	res, err := CodexLogout(svc)(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !svc.loggedOut {
		t.Error("the service was never asked to log out")
	}
	if string(res) != `{"ok":true}` {
		t.Errorf("payload = %s", res)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/rpc/handlers/ -run TestCodex 2>&1 | head`
Expected: 컴파일 실패 — `undefined: CodexLoginStart`, `undefined: ReasonCodexPortInUse` 등.

- [ ] **Step 3: `reason.go`에 이유 코드 추가** — 기존 provider 블록 바로 아래에 붙인다.

```go
	// The Codex login (#92). port_in_use is the only one the writer can fix
	// themselves, and the message has to name the two ports, because the fix
	// is closing whatever holds them — usually a Codex CLI login.
	ReasonCodexPortInUse     = "codex_port_in_use"
	ReasonCodexLoginFailed   = "codex_login_failed"
```

- [ ] **Step 4: `engine/internal/rpc/handlers/codex.go` 생성**

```go
package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// CodexService is the slice of internal/codexauth the RPC layer needs. It
// speaks JSON and errors only, so the handler package stays free of the login
// flow's HTTP machinery.
type CodexService interface {
	LoginStart(ctx context.Context) (json.RawMessage, error)
	LoginStatus(ctx context.Context) (json.RawMessage, error)
	Logout(ctx context.Context) error
}

// CodexLoginStart returns a handler for codex.login_start. It answers with the
// authorize URL the shell opens in the OS browser; the login itself completes
// on the loopback callback, and the pane learns the outcome from
// codex.login_status.
func CodexLoginStart(svc CodexService) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		out, err := svc.LoginStart(ctx)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return out, nil
	}
}

// CodexLoginStatus returns a handler for codex.login_status.
func CodexLoginStatus(svc CodexService) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		out, err := svc.LoginStatus(ctx)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return out, nil
	}
}

// CodexLogout returns a handler for codex.logout.
func CodexLogout(svc CodexService) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		if err := svc.Logout(ctx); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
```

- [ ] **Step 5: 핸들러 테스트 통과 확인**

Run: `cd engine && go test ./internal/rpc/ ./internal/rpc/handlers/`
Expected: PASS

- [ ] **Step 6: `engine/internal/engineapp/codex.go` 생성**

```go
package engineapp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/codexauth"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// codexService adapts *codexauth.Service to handlers.CodexService, and is the
// one place that turns a login failure into a reason code the pane can
// translate.
type codexService struct {
	svc *codexauth.Service
}

func (c codexService) LoginStart(ctx context.Context) (json.RawMessage, error) {
	url, err := c.svc.Start(ctx)
	if err != nil {
		return nil, codexReason(err)
	}
	return json.Marshal(struct {
		AuthURL string `json:"auth_url"`
	}{url})
}

func (c codexService) LoginStatus(context.Context) (json.RawMessage, error) {
	return json.Marshal(c.svc.Status())
}

func (c codexService) Logout(context.Context) error {
	if err := c.svc.Logout(); err != nil {
		return codexReason(err)
	}
	return nil
}

// codexReason maps the login package's sentinels onto reason codes. Anything
// unrecognised stays an internal error rather than being dressed up as a
// failure the writer can act on.
func codexReason(err error) error {
	switch {
	case errors.Is(err, codexauth.ErrPortInUse):
		return &rpc.ReasonError{Reason: rpc.ReasonCodexPortInUse, Err: err}
	case errors.Is(err, codexauth.ErrLoginFailed):
		return &rpc.ReasonError{Reason: rpc.ReasonCodexLoginFailed, Err: err}
	default:
		return err
	}
}
```

- [ ] **Step 7: `engineapp.go` 배선** — import에 `"github.com/devlikebear/linetta/engine/internal/codexauth"`를 추가한다. `providerSrc := provider.NewSource(...)` 줄 **바로 위**에 codexHome을 뽑아내고, 서비스를 만든 뒤 closer에 등록한다.

```go
	// The Codex login (#92) writes into Linetta's own directory; the provider
	// decides which directory to read from, and may prefer an existing Codex
	// CLI login when Linetta has none.
	codexHome := filepath.Join(home, "codex")
	codexSvc := codexauth.NewService(codexHome)
	a.closers = append(a.closers, codexSvc.Close)

	providerSrc := provider.NewSource(settingsStore, codexHome)
```

기존 `providerSrc := provider.NewSource(settingsStore, filepath.Join(home, "codex"))` 줄은 위 마지막 줄로 대체된다.

그리고 `s.Handle("providers.test", ...)` 바로 뒤에 등록한다.

```go
	codex := codexService{svc: codexSvc}
	s.Handle("codex.login_start", handlers.CodexLoginStart(codex))
	s.Handle("codex.login_status", handlers.CodexLoginStatus(codex))
	s.Handle("codex.logout", handlers.CodexLogout(codex))
```

- [ ] **Step 8: 배선 테스트 작성** — `engine/internal/engineapp/codex_wiring_test.go`. 헬퍼 `call(t, app, method, params)`와 `openApp(t)`는 `engine/internal/engineapp/mcp_wiring_test.go`에 있고 같은 패키지·같은 빌드 태그(`!mobile`)이므로 그대로 쓴다. `providers_wiring_test.go`가 같은 헬퍼를 쓰는 선례다.

```go
//go:build !mobile

package engineapp

import (
	"encoding/json"
	"testing"
)

// A fresh library has no Codex login, and the three methods must all be
// reachable. A typo in a Handle string compiles and passes every unit test;
// only a call through the real dispatcher catches it.
func TestCodexMethodsAreRegistered(t *testing.T) {
	app := openApp(t)

	res, rpcErr := call(t, app, "codex.login_status", "")
	if rpcErr != nil {
		t.Fatalf("codex.login_status: %v", rpcErr)
	}
	var st struct {
		LoggedIn bool   `json:"logged_in"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(res, &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if st.LoggedIn || st.Email != "" {
		t.Errorf("fresh library reports a login: %+v", st)
	}

	// Logging out with nothing stored is a success, and proves the method is
	// wired rather than merely absent.
	if _, rpcErr := call(t, app, "codex.logout", ""); rpcErr != nil {
		t.Fatalf("codex.logout: %v", rpcErr)
	}
}
```

`codex.login_start`는 이 테스트에서 부르지 않는다 — 실제로 1455 포트를 바인딩하고 브라우저를 기다리기 때문이다. 등록 여부는 Step 10의 허용 목록 테스트와 위 두 메서드로 충분하다.

- [ ] **Step 9: 엔진 빌드·테스트**

Run: `cd engine && go build ./... && go test ./internal/engineapp/ ./internal/rpc/... ./internal/codexauth/ ./internal/provider/`
Expected: PASS

- [ ] **Step 10: 렌더러 허용 목록** — `apps/desktop/src-tauri/src/lib.rs`. 정렬 순서가 곧 정확성이다(이진 탐색). `codex.*`는 `beats.*`와 `contextual.*` 사이에 들어간다.

```rust
    "codex.login_start",
    "codex.login_status",
    "codex.logout",
```

같은 파일의 `EXTERNAL_URL_HOSTS`를 교체한다. 현재 값 `["openrouter.ai"]`는 1.0에서 OpenRouter OAuth가 사라지면서 **호출자가 없는 죽은 값**이다(`openExternalUrl`을 부르는 프로덕션 코드가 없다). Codex 로그인이 이 경로의 유일한 사용자가 된다.

```rust
const EXTERNAL_URL_HOSTS: [&str; 1] = ["auth.openai.com"];
```

같은 파일의 Rust 테스트 `external_url_allows_only_https_openrouter`도 새 호스트로 바꾼다. 테스트 이름과 본문의 `openrouter.ai`를 `auth.openai.com`으로 치환하되, 검사의 의도(https만, 자격증명 삽입 거부, 하위 도메인 처리)는 그대로 둔다. 함수 이름은 `external_url_allows_only_https_openai_auth`로 바꾼다.

- [ ] **Step 11: 프론트 타입** — `apps/desktop/src/lib/types.ts`. 기존 `ProviderStatus` 정의 바로 아래에 추가한다.

```ts
/** codex.login_start — the address the shell opens in the OS browser. */
export interface CodexLoginStart {
  auth_url: string;
}

/** codex.login_status — never carries a token, only who is signed in. */
export interface CodexStatus {
  logged_in: boolean;
  email?: string;
  account_id?: string;
  /** id_token expiry, epoch seconds. */
  expires_at?: number;
}
```

- [ ] **Step 12: 프론트 래퍼** — `apps/desktop/src/lib/rpc.ts`. import 목록에 `CodexLoginStart`, `CodexStatus`를 추가하고, `export const providers = { … };` 바로 뒤에 넣는다.

```ts
export const codex = {
  loginStart: () => rpcCall<CodexLoginStart>("codex.login_start"),
  loginStatus: () => rpcCall<CodexStatus>("codex.login_status"),
  logout: () => rpcCall<{ ok: true }>("codex.logout"),
};
```

- [ ] **Step 13: 이유 코드 번역** — `apps/desktop/src/lib/rpcMessage.ts`의 `REASON_MESSAGE_KEYS`는 `reason 문자열 → "errors.<camelCase>"` 맵이다. 기존 provider 블록(`provider_not_configured: "errors.providerNotConfigured"` 등) 바로 아래에 세 줄을 같은 형식으로 추가한다.

```ts
  // The Codex login (#92). port_in_use is the only one the writer can fix
  // themselves, so its message names the two ports.
  codex_port_in_use: "errors.codexPortInUse",
  codex_login_failed: "errors.codexLoginFailed",
```

그리고 `apps/desktop/src/lib/i18n.tsx`의 ko/en/ja 카탈로그 **세 곳 모두**에 `errors.codexPortInUse`, `errors.codexLoginFailed` 항목을 추가한다. 기존 `errors.provider*` 항목이 각 카탈로그의 어디에 있는지 찾아 그 옆에 둔다. `i18n.catalog.test.ts`가 세 언어의 키 집합이 같기를 요구하므로 하나라도 빠지면 실패한다.

문구는 소설가가 읽고 무엇을 할지 알 수 있게 쓴다. 에러 용어를 쓰지 않는다.

- `codex_port_in_use` — ko: "로그인 포트(1455, 1457)를 다른 프로그램이 쓰고 있습니다. Codex CLI 로그인 창을 닫고 다시 시도하세요." en/ja도 같은 내용.
- `codex_login_failed` — ko: "ChatGPT 로그인에 실패했습니다. 다시 시도하세요."

`apps/desktop/src/lib/rpcMessage.test.ts`에 세 코드가 번역되는지(그리고 `String(error)`로 새지 않는지) 검증을 추가한다. 기존 provider 코드 다섯 개를 검증하는 케이스가 이미 있으니 그 배열에 세 개를 더한다.

- [ ] **Step 14: 프론트·셸 검증**

Run: `cd apps/desktop && pnpm lint && pnpm test && pnpm build && cd src-tauri && cargo check && cargo test`
Expected: 전부 PASS. `rpcAllowlist.test.ts`의 세 케이스(호출 ⊆ 허용, 허용 ⊆ 호출, 정렬)와 `i18n.catalog.test.ts`의 언어별 키 일치가 통과해야 한다.

- [ ] **Step 15: 커밋**

```bash
git add engine/internal/rpc/reason.go engine/internal/rpc/handlers/codex.go engine/internal/rpc/handlers/codex_test.go engine/internal/engineapp/codex.go engine/internal/engineapp/engineapp.go engine/internal/engineapp/codex_wiring_test.go apps/desktop/src-tauri/src/lib.rs apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts apps/desktop/src/lib/rpcMessage.ts apps/desktop/src/lib/rpcMessage.test.ts apps/desktop/src/lib/i18n.tsx
git commit -m "feat: codex.login_start / login_status / logout over RPC, wired to the renderer (#92)"
```

---

### Task 6: 개인정보 문서와 전체 검증

**Files:**
- Modify: `docs/privacy-policy.md` (ko/en/ja 세 언어 모두)

**배경:** #91은 프로바이더에 원고를 보내는 것을 문서화했다. 이 작업은 **자격증명 파일**을 문서화한다. 리프레시 토큰이 앱 데이터 디렉터리 안 0600 파일에 평문으로 보관된다는 사실은 작가가 알아야 하고, #96의 정책 갱신이 이 문장을 이어받는다.

- [ ] **Step 1: 개인정보 문서 갱신**

`docs/privacy-policy.md`의 데이터 보관 절에 한 문단을 추가한다. 세 언어가 **같은 사실**을 말해야 한다(1.0에서 언어별 불일치를 겪었다).

내용:
- ChatGPT(Codex)로 로그인하면 접근 토큰과 갱신 토큰이 `<앱 데이터>/codex/auth.json`에 소유자만 읽을 수 있는 권한(0600)으로 저장된다.
- Codex CLI가 자기 토큰을 보관하는 방식과 같다.
- Linetta는 이 파일을 어디로도 보내지 않는다. OpenAI에 요청할 때만 쓴다.
- 설정에서 로그아웃하면 파일이 지워진다.
- macOS App Store 빌드를 포함해 모든 빌드가 파일 저장 방식을 쓴다. 키체인을 쓰지 않는다.

- [ ] **Step 2: 문서 커밋**

```bash
git add docs/privacy-policy.md
git commit -m "docs: say where the Codex login token is kept (#92)"
```

- [ ] **Step 3: 전체 검증 계약**

Run: `make test`
Expected: `test-go`(엔진 테스트 + 의존성 게이트), `test-desktop`(lint·vitest·build), `test-tauri`(cargo check·test) 전부 PASS.

- [ ] **Step 4: 모바일·MAS·Windows**

Run: `make test-mobile-engine && cd engine && go build -tags mas ./... && GOOS=windows GOARCH=amd64 go build ./...`
Expected: 전부 성공. `internal/codexauth`는 빌드 태그가 없으므로 모바일에서도 컴파일된다 — 호출부만 데스크톱에 있다. Windows에서 `syscall.EADDRINUSE`가 문제되면 `isAddrInUse`의 숫자 비교(10048) 경로가 받아준다.

- [ ] **Step 5: 실물 확인 (수동, 대화형 Mac 필요)**

이 단계는 자동화할 수 없다. 실제 ChatGPT 계정이 필요하고 브라우저가 열린다.

1. 앱을 띄우고 엔진에 `codex.login_start`를 호출한다(설정 UI는 #94이므로 지금은 개발자 도구나 `linetta-engine --stdio`로).
2. 반환된 URL을 브라우저에서 연다. ChatGPT 로그인 화면이 떠야 한다.
3. 로그인하면 브라우저에 "Signed in to ChatGPT" 페이지가 뜬다.
4. `codex.login_status`가 `logged_in: true`와 이메일을 돌려준다.
5. `$LINETTA_HOME/codex/auth.json`이 0600으로 존재하고 `auth_mode: "chatgpt"`를 담고 있다.
6. `providers.test`(프로바이더 `openai-codex`, 동의 체크 후)가 성공한다 — **이것이 이 이슈의 진짜 완료 조건이다.** tars가 우리가 쓴 파일을 읽어 실제로 모델을 부를 수 있어야 한다.
7. `codex.logout` 후 파일이 사라지고 `providers.test`가 `provider_not_configured`로 실패한다.

결과를 #92에 기록한다. 6번이 실패하면 파일 형식이 tars의 기대와 다르다는 뜻이므로, `auth.json`을 Codex CLI가 만든 것과 직접 비교한다.

- [ ] **Step 6: 이슈 갱신** — #92의 체크리스트를 채우고, 5단계 수동 확인 결과를 적는다. 스펙 5.3절과 달라진 점이 있으면 함께 적는다.

---

## 자기 검토 기록

- **스펙 커버리지 (5.3절):** 흐름 1~5는 Task 1(인가 URL)·Task 3(콜백·교환·파일 쓰기)·Task 2(status/logout 재료)·Task 5(RPC 표면)가 덮는다. 프로토콜 상수 표는 Task 1이 상수로 고정하고 테스트가 값을 하나씩 검증한다. `auth.json` 형식은 Task 2. `account_id`를 id_token 클레임에서 뽑는 것은 Task 2(`claimsFromIDToken`)와 Task 3(콜백에서 대입). 포트 정책·state 검증·로컬 성공 페이지·5분 타임아웃은 Task 3. `TARS_OPENAI_CODEX_REFRESH_TOKEN_STORAGE=file`은 #91의 `provider.NewSource`가 이미 설정하므로 새 작업이 없다. `~/.codex` 폴백과 MAS 예외는 Task 4. 개인정보 문서는 Task 6.
- **이름 일관성:** `codexauth.Tokens`/`Status`/`NewService`/`Start`/`Logout`/`Close`/`AuthPath`(Task 2·3) → `engineapp.codexService`(Task 5) → `handlers.CodexService`(Task 5). `resolveCodexHome`(Task 4)은 `provider.go`의 `Resolve`가 부른다. 이유 코드 `ReasonCodexPortInUse`/`ReasonCodexLoginFailed`(Task 5)는 `codexReason`과 프론트 카탈로그가 같은 문자열을 쓴다.
- **플레이스홀더:** 없음. 유일하게 코드 블록 없이 지시만 있는 곳은 Task 5 Step 13의 번역 문구인데, 세 언어의 실제 문장을 명시했고 "기존 provider 이유 코드의 형식을 따르라"는 구체적 참조를 달았다 — 카탈로그 파일의 구조를 계획서에 복사하는 것보다 그쪽이 정확하다.
- **미해결 위험 하나:** Task 3의 `TestStart_bothPortsBusyIsAnError`는 개발 머신에서 1455/1457이 이미 쓰이고 있으면 skip된다. CI에서는 비어 있으므로 실제로 돈다. skip이 나오면 보고서에 남긴다.
