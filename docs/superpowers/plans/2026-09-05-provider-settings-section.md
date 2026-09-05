# 설정 → 연결 → AI 프로바이더 섹션 (#94)

에픽 #90 서브프로젝트 1의 네 번째 단계. 설계 문서
`docs/superpowers/specs/2026-09-02-builtin-agent-byok-design.md` 8.1절.

앞선 세 단계(#91 프로바이더 계층, #92 Codex 로그인, #93 에이전트 루프)가 엔진을
완성했다. 작가가 그것에 닿는 첫 화면이 이 이슈다.

## 조사에서 확인된 사실 — 이슈 본문과 다른 것들

이슈 본문은 `types.ts`/`rpc.ts`에 `ProviderConfig`, `providers.*`, `codex.*`를
**추가**하라고 적었다. 실제로는 전부 이미 있다. 확인한 것:

| 이슈가 가정한 것 | 실제 |
| --- | --- |
| `ProviderConfig` 등 타입 추가 필요 | `types.ts`에 `ProviderID`, `ProviderConfig`, `ProviderPatch`, `ProviderStatus`, `CodexLoginStart`, `CodexStatus` 전부 존재 |
| `rpc.ts`에 `providers.*`/`codex.*` 추가 필요 | `providers.list/listModels/test`, `codex.loginStart/loginStatus/logout` 전부 존재 |
| `rpcAllowlist.test.ts` 확장 필요 | 6개 메서드 모두 `RENDERER_ENGINE_METHODS`에 이미 등재. **새 래퍼를 만들지 않는 한 확장 불필요** |
| 활동 로그에 출처 열 추가 = UI 작업 + 엔진 작업 | 엔진은 이미 보낸다. `mcphost.ActivityEntry`에 `Source`/`RunID`가 있다(#93 Task 1). **TS 타입과 렌더만 고치면 된다** |

그러니 이 계획서의 대부분은 **그린필드 UI**다. 배관은 전부 깔려 있다.

### 함정 네 개

**1. `providers.test`는 동의 게이트 뒤에 있다.** 자격증명만으로는 안 된다.
`Source.Client()`가 `Configured()`와 `Consented()`를 **둘 다** 요구한다. "키가
맞는지 먼저 확인하고 동의하겠다"는 순서는 서버에서 `provider_consent_required`로
막힌다. 그래서 이 화면은 **동의 → 테스트** 순서를 강제한다. 이슈 본문의
"체크 전엔 [연결 테스트] 비활성"이 정확히 이것이고, UX 취향이 아니라 서버 계약이다.

`providers.list_models`는 다르다. 자격증명만 요구하고 동의는 요구하지 않는다 —
모델 목록에는 원고가 실리지 않기 때문이다. 그래서 동의 전에도 모델을 고를 수 있다.

**2. 죽은 동의 필드가 따로 있다.** `Settings.ai_data_sharing_consent_version` /
`ai_data_sharing_consented_at`와 이유 코드 `ai_data_sharing_consent_required`가
엔진과 TS 양쪽에 선언돼 있지만 **어디서도 읽거나 쓰지 않는다.** 살아 있는 동의는
프로바이더별 `ProviderConfig.consented_at`다. 이름이 그럴듯해서 잘못 잡기 쉽다.

**3. `unavailable_providers`도 죽어 있다.** `DiagnosticsSnapshot`에 타입만 있고
엔진에 생산자가 없다. 이걸로 UI 로직을 짜면 안 된다.

**4. `Settings.css`에 옛 `.provider-test`, `.provider-test-ok`,
`.provider-test-error`가 남아 있다.** #82 사이드바 개편 이전, 1.0 이전 AI 설정
플로의 잔재이고 지금 아무도 안 쓴다. 현재 디자인의 `.settings-section`/`.modal-field`
간격 체계와 맞는지 확인하지 않은 채 재사용하지 말 것. 이 계획서는 재사용하지 않는다.

## Global Constraints

이 계획서 전체에 걸리는 규칙. 각 작업의 리뷰가 이것들을 렌즈로 쓴다.

- **`apps/desktop`만 건드린다.** 엔진은 이미 필요한 것을 전부 제공한다. 엔진 변경이
  필요해 보이면 그건 계획서의 오류이니 멈추고 보고할 것.
- **`McpSection`이 본이다.** 플랫, 중첩 없음, 여백. 옛 온보딩 마법사와 프로바이더
  카탈로그는 돌아오지 않는다. 새 CSS 클래스를 만들기 전에 `settings-section`,
  `sd`, `modal-field`로 되는지 먼저 볼 것.
- **동의는 체크하는 순간 저장한다.** `McpSection`의 동의 게이트와 같은 방식이다.
  폼 제출까지 모아두지 않는다.
- **i18n 키는 세 카탈로그(ko/en/ja)에 같은 커밋에서 들어간다.** `MessageKey`가
  `keyof typeof messages.ko`라 둘에만 넣으면 타입 검사에서 걸리지만, 그건 안전망이지
  절차가 아니다. `settings.providers.*` 네임스페이스를 쓴다.
- **비밀은 화면에 다시 뜨지 않는다.** `settings.get`은 `api_key_set` 불리언만 준다.
  저장된 키를 되읽어 입력란을 채우는 코드는 존재할 수 없다.
- **테스트는 i18n 키로 단언한다.** `McpSection.test.tsx`처럼 `i18n`을 키 에코로
  모킹한다. 문구가 아니라 키를 단언해야 번역이 바뀌어도 테스트가 안 깨진다.
- **`data-testid`로 단언한다.** 이 저장소의 설정 테스트 관습이다.
- **새 `rpcCall` 래퍼를 만들면 `RENDERER_ENGINE_METHODS`에 정렬된 위치로 넣는다.**
  그 배열은 런타임에 이진 탐색되고, `rpcAllowlist.test.ts`가 정렬·양방향 일치를
  검사한다. 이 계획서는 새 래퍼를 만들지 않으므로 해당 없음이어야 한다.
- **`make test-desktop`이 통과해야 한다.** 프로덕션 빌드까지 포함이다.

## 검증되지 않은 위험 하나

**`originator=linetta`가 실제 서버에서 통하는지 아직 모른다.** #92의 프로토콜
상수는 openai/codex 소스에서 읽은 값이지 실제 응답으로 확인한 게 아니다. CLI의
값을 일부러 바꿨기 때문에, OpenAI가 이 값을 검증한다면 모든 로그인이 동의 화면에서
실패한다.

**이 계획서는 그것에 막히지 않는다.** 실패할 경우 고칠 곳은
`engine/internal/codexauth/oauth.go`의 상수 하나이지 이 화면의 설계가 아니다.
로그인 버튼, 상태 줄, 폴링, 로그아웃은 그대로다. 그래서 검증을 기다리지 않고
진행하되, 실전 로그인은 별도로 한 번 돌려봐야 한다는 사실을 여기 남긴다.

## 작업 목록

1. 활동 로그 출처 열 — 가장 작고, 엔진이 이미 주는 데이터를 쓴다
2. `ProviderSection` 뼈대 — 프로바이더 4개 세그먼트와 가용성 게이트
3. 자격증명 필드 — Codex 로그인과 API 키
4. 모델 콤보박스
5. 동의와 연결 테스트
6. `Settings.tsx` 배선과 i18n 마감

---

## Task 1: 활동 로그 출처 열

가장 작은 작업이고, 엔진이 이미 보내는 데이터를 화면에 꺼내는 것뿐이다.

**왜 먼저인가.** #93이 내장 에이전트를 외부 클라이언트와 같은 툴 레이어에 붙였다.
활동 로그는 작가가 "무엇이 내 원고를 건드렸나"를 확인하는 유일한 증거인데, 지금은
둘이 구분 없이 한 줄로 섞여 나온다. 프로바이더 섹션이 생기기 전에도 이건 이미
틀린 화면이다.

### 파일

- `apps/desktop/src/lib/types.ts` — `McpActivityEntry`에 두 필드
- `apps/desktop/src/components/settings/McpSection.tsx` — 렌더 한 곳
- `apps/desktop/src/components/settings/McpSection.test.tsx` — 테스트 추가
- `apps/desktop/src/lib/i18n.tsx` — 키 두 개 × 세 카탈로그

### 1-1. 타입

`McpActivityEntry`에 추가한다. 엔진의 `mcphost.ActivityEntry`가 이미 이 두 필드를
보낸다 — `Source`는 `json:"source"`(항상 존재), `RunID`는
`json:"run_id,omitempty"`(에이전트 호출에만 존재).

```ts
export interface McpActivityEntry {
  id: string;
  at: number;
  tool: string;
  project_id?: string;
  target_id?: string;
  ok: boolean;
  detail?: string;
  /** Who called the tool: "agent" for Linetta's built-in writing agent,
   *  "external" for a client connected over the MCP port. The engine stamps
   *  this from a static field on the tool deps, never from the wire, so an
   *  external client cannot claim to be the agent. */
  source?: string;
  /** Which agent turn the call belonged to. Only the built-in agent sets it. */
  run_id?: string;
}
```

`source`를 옵셔널로 두는 이유: 이 필드가 생기기 전에 기록된 행이 데이터베이스에
남아 있다. 마이그레이션 `0017`은 `source`를 `DEFAULT 'external'`로 채웠으므로 실제로는 그
문자열로 오지만, 타입이 그 사실에 기대지 않게 한다.

### 1-2. 렌더

`McpSection.tsx`의 활동 목록(현재 430–440행)을 바꾼다.

```tsx
<ul>
  {activity.map((entry) => (
    <li key={entry.id} data-testid={`mcp-activity-${entry.id}`}>
      <span data-testid={`mcp-activity-source-${entry.id}`}>
        {entry.source === "agent"
          ? t("settings.mcp.activity.sourceAgent")
          : t("settings.mcp.activity.sourceExternal")}
      </span>{" "}
      {entry.ok ? "✓" : "✕"} {entry.tool}
      {entry.detail ? ` — ${entry.detail}` : ""}
    </li>
  ))}
</ul>
```

`source === "agent"`만 에이전트로 읽고 나머지는 전부 외부로 읽는다. 빈 문자열도,
장래에 생길 모르는 값도 외부로 떨어진다 — 내장 에이전트가 한 일을 외부가 한 것으로
보여주는 쪽이 그 반대보다 안전하기 때문이다. 작가가 "이건 내가 시킨 게 아닌데"라고
확인하러 오는 화면이다.

### 1-3. i18n

세 카탈로그 모두에 넣는다. `settings.mcp.activity.title` 옆이다.

```
"settings.mcp.activity.sourceAgent": "내장 에이전트"      / "Built-in agent"  / "内蔵エージェント"
"settings.mcp.activity.sourceExternal": "외부 클라이언트" / "External client" / "外部クライアント"
```

### 1-4. 테스트

`McpSection.test.tsx`에 추가한다. 기존 파일의 모킹 방식(`vi.hoisted` + `vi.mock`,
i18n 키 에코)을 그대로 쓴다.

```tsx
it("labels each activity row with who called the tool", async () => {
  mcpStatus.mockResolvedValue({ running: true, mode: "full", port: 7391, token_set: true });
  settingsGet.mockResolvedValue({ mcp_mode: "full", mcp_port: 7391, mcp_consent_version: 1 });
  mcpActivity.mockResolvedValue([
    { id: "a1", at: 1, tool: "linetta_write_scene", ok: true, source: "agent", run_id: "r1" },
    { id: "e1", at: 2, tool: "linetta_read_scene", ok: true, source: "external" },
    // A row written before the source column existed.
    { id: "old", at: 3, tool: "linetta_read_scene", ok: true },
  ]);
  render(<McpSection bridgePath="/bridge" />);

  expect(await screen.findByTestId("mcp-activity-source-a1")).toHaveTextContent(
    "settings.mcp.activity.sourceAgent",
  );
  expect(screen.getByTestId("mcp-activity-source-e1")).toHaveTextContent(
    "settings.mcp.activity.sourceExternal",
  );
  // An unstamped legacy row must not read as the built-in agent.
  expect(screen.getByTestId("mcp-activity-source-old")).toHaveTextContent(
    "settings.mcp.activity.sourceExternal",
  );
});
```

세 번째 케이스가 이 작업의 핵심이다. `source`가 없는 옛 행이 내장 에이전트로
표시되면 작가에게 거짓을 말하는 것이고, 그건 이 로그의 존재 이유를 깨뜨린다.

### 검증

```
cd apps/desktop && pnpm test McpSection
make test-desktop
```

### 커밋

```
feat(desktop): the activity log says who called each tool (#94)
```

---

## Task 2: `ProviderSection` 뼈대

프로바이더 4개 세그먼트와, 이 섹션이 이 빌드에 존재하는지 판단하는 게이트.

**이 작업이 끝나면** 섹션이 렌더되고, 세그먼트를 누르면 활성 프로바이더가 바뀌고,
현재 상태(설정됨/동의함)가 보인다. 자격증명 입력은 아직 없다.

### 파일

- `apps/desktop/src/components/settings/ProviderSection.tsx` — 신규
- `apps/desktop/src/components/settings/ProviderSection.test.tsx` — 신규
- `apps/desktop/src/lib/i18n.tsx` — `settings.providers.*` 키 블록 시작

### 2-1. 컴포넌트

`McpSection`의 구조를 그대로 따른다: `useState` 여러 개, `refresh` 한 개,
`guard` 헬퍼, 원시 에러를 담아 렌더 시점에 번역.

```tsx
import { useCallback, useEffect, useState } from "react";

import { useI18n } from "../../lib/i18n";
import { providers as providersApi, settings as settingsApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { ProviderID, ProviderStatus } from "../../lib/types";

/** Connect an AI provider (BYOK).
 *
 *  The pane's job is to make one decision legible: from here on, the scenes
 *  the writer asks the built-in agent about leave this machine and reach a
 *  company the writer chose. So consent is a per-provider checkbox rather
 *  than something choosing a provider implies, and it is what unlocks the
 *  connection test — because the test itself sends a prompt.
 *
 *  The four ids are fixed by the engine's whitelist. There is no catalogue
 *  and no wizard: the 1.0 onboarding flow is not coming back.
 */

/** The order the engine's providers.list returns, kept explicit so the
 *  segmented control does not silently reorder when a response is cached. */
const PROVIDER_ORDER: ProviderID[] = ["openai-codex", "anthropic", "gemini-native", "openai"];

export function ProviderSection() {
  const { t } = useI18n();
  const [list, setList] = useState<ProviderStatus[]>([]);
  const [active, setActive] = useState<ProviderID>("openai-codex");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    const rows = await providersApi.list();
    setList(rows);
    const chosen = rows.find((r) => r.active);
    if (chosen) setActive(chosen.id);
  }, []);

  useEffect(() => {
    void refresh().catch(setError);
  }, [refresh]);

  const guard = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  // Choosing a provider is itself a saved setting, not a local tab. The agent
  // reads settings.provider at the start of every turn, so a writer who picks
  // Anthropic here and opens the panel gets Anthropic without a save button.
  const choose = (id: ProviderID) =>
    guard(async () => {
      await settingsApi.set({ provider: id });
      setActive(id);
      await refresh();
    });

  const current = list.find((r) => r.id === active);

  return (
    <section className="settings-section" id="provider-settings" data-testid="provider-section">
      <h3>{t("settings.providers.title")}</h3>
      <p className="sd">{t("settings.providers.description")}</p>

      <div className="modal-field" data-testid="provider-choices">
        {PROVIDER_ORDER.map((id) => (
          <button
            key={id}
            type="button"
            disabled={busy}
            aria-pressed={id === active}
            onClick={() => void choose(id)}
            data-testid={`provider-choice-${id}`}
          >
            {t(`settings.providers.name.${id}` as never)}
          </button>
        ))}
      </div>

      {current ? (
        <p className="sd" data-testid="provider-state">
          {current.configured
            ? t("settings.providers.state.configured")
            : t("settings.providers.state.notConfigured")}
          {" · "}
          {current.consented
            ? t("settings.providers.state.consented")
            : t("settings.providers.state.notConsented")}
        </p>
      ) : null}

      {error ? (
        <p className="sd" role="alert" data-testid="provider-error">
          {rpcErrorMessage(error, t)}
        </p>
      ) : null}
    </section>
  );
}
```

**`t(...as never)`에 대해.** `MessageKey`는 리터럴 유니온이라 템플릿 문자열이
그대로 안 들어간다. 구현자는 두 가지 중 하나를 고른다: (a) `PROVIDER_ORDER`를
`{ id, labelKey }` 배열로 바꿔 키를 리터럴로 적는다, (b) `as never` 대신 명시적
맵을 둔다. **(a)를 권한다** — 캐스팅이 없고, 키가 존재하지 않으면 타입 검사가
잡는다. 위 코드는 형태를 보이려는 것이고, 구현할 때 (a)로 바꿀 것.

### 2-2. 왜 세그먼트가 버튼인가

`<select>`가 아니라 버튼 4개인 이유는 이슈가 "세그먼트"라고 적어서만이 아니다.
프로바이더마다 아래 필드가 통째로 달라진다(Codex는 로그인 버튼, 나머지는 키 입력,
openai는 base URL 추가). 선택이 화면을 바꾼다는 걸 컨트롤 모양이 미리 말해주는
편이 낫다. `aria-pressed`로 선택 상태를 접근성 트리에 남긴다.

### 2-3. i18n

```
settings.providers.title            "AI 프로바이더" / "AI provider" / "AI プロバイダー"
settings.providers.description      한 줄. 원고가 작가가 고른 회사로 간다는 사실.
settings.providers.name.openai-codex   "ChatGPT (Codex)"
settings.providers.name.anthropic      "Anthropic"
settings.providers.name.gemini-native  "Google Gemini"
settings.providers.name.openai         "OpenAI 호환" / "OpenAI-compatible" / "OpenAI 互換"
settings.providers.state.configured    "설정됨" / "Configured" / "設定済み"
settings.providers.state.notConfigured "설정 안 됨" / "Not configured" / "未設定"
settings.providers.state.consented     "동의함" / "Consented" / "同意済み"
settings.providers.state.notConsented  "동의 안 함" / "Not consented" / "未同意"
```

프로바이더 이름은 세 언어에서 같아도 된다(고유명사). `openai`만 "OpenAI 호환"으로
번역한다 — 그 항목은 OpenAI만이 아니라 OpenRouter 등 호환 엔드포인트 전부를
가리키기 때문이다.

### 2-4. 테스트

```tsx
it("renders the four providers and marks the active one", async () => { ... });
it("saves the chosen provider and does not keep it local", async () => {
  // settingsSet must be called with { provider: "anthropic" }
});
it("shows configured and consent state for the active provider", async () => { ... });
it("renders a reason code as a translated message", async () => {
  providersList.mockRejectedValue(Object.assign(new Error("x"), {
    data: { reason: "provider_not_configured" },
  }));
  // expect the errors.providerNotConfigured key
});
```

### 검증

```
cd apps/desktop && pnpm test ProviderSection
make test-desktop
```

### 커밋

```
feat(desktop): the AI provider section and its four choices (#94)
```

---

## Task 3: 자격증명 필드

프로바이더에 따라 다른 것을 묻는다. Codex는 로그인, 나머지 셋은 API 키,
`openai`는 base URL이 하나 더.

### 파일

- `ProviderSection.tsx` — 필드 블록 추가
- `ProviderSection.test.tsx` — 테스트 추가
- `i18n.tsx` — 키

### 3-1. Codex: 로그인 버튼과 상태 줄

```tsx
const [codexStatus, setCodexStatus] = useState<CodexStatus | null>(null);
// The poll handle, so a second click cannot start a second loop and so the
// interval dies with the component. A login the writer abandons must not
// leave a timer calling the engine forever.
const pollRef = useRef<number | null>(null);

const stopPolling = useCallback(() => {
  if (pollRef.current !== null) {
    window.clearInterval(pollRef.current);
    pollRef.current = null;
  }
}, []);

useEffect(() => stopPolling, [stopPolling]);

const startCodexLogin = () =>
  guard(async () => {
    const { auth_url } = await codexApi.loginStart();
    await openExternalUrl(auth_url);
    stopPolling();
    pollRef.current = window.setInterval(() => {
      void codexApi
        .loginStatus()
        .then((s) => {
          setCodexStatus(s);
          // Both outcomes end the poll. login_failed is how the engine
          // reports a failed exchange — it is a status field, not an RPC
          // error, because the failure happens after login_start returned.
          if (s.logged_in || s.login_failed) {
            stopPolling();
            void refresh();
          }
        })
        .catch(() => stopPolling());
    }, 1500);
  });
```

**폴링 간격 1.5초.** 로그인은 브라우저를 다녀오는 동안 수십 초가 걸릴 수 있고,
그 사이 화면은 "기다리는 중"만 보여준다. 더 촘촘히 물어봐야 할 이유가 없다.

**`login_failed`도 폴링을 끝낸다.** #92에서 확인했듯 실패는 RPC 에러가 아니라
`Status.LoginFailed` 필드로 온다 — `login_start`가 이미 반환한 뒤에 벌어지는
일이기 때문이다. 이 분기를 빠뜨리면 실패한 로그인이 영원히 폴링된다.

상태 줄:

```tsx
{active === "openai-codex" ? (
  <div className="modal-field" data-testid="provider-codex">
    {codexStatus?.logged_in ? (
      <>
        <span data-testid="provider-codex-email">
          {codexStatus.email ?? t("settings.providers.codex.signedIn")}
        </span>
        <button type="button" disabled={busy} onClick={() => void logoutCodex()}
                data-testid="provider-codex-logout">
          {t("settings.providers.codex.logout")}
        </button>
      </>
    ) : (
      <button type="button" disabled={busy} onClick={() => void startCodexLogin()}
              data-testid="provider-codex-login">
        {t("settings.providers.codex.login")}
      </button>
    )}
    {codexStatus?.login_failed ? (
      <p className="sd" role="alert" data-testid="provider-codex-failed">
        {t("settings.providers.codex.failed")}
      </p>
    ) : null}
  </div>
) : null}
```

`email`이 없을 수 있다 — `id_token`에 이메일 클레임이 없는 계정이 있다. 그때는
"로그인됨"으로 떨어뜨린다. 빈 줄을 보여주면 로그인이 안 된 것처럼 읽힌다.

### 3-2. API 키 필드 (anthropic / gemini-native / openai)

```tsx
{active !== "openai-codex" ? (
  <div className="modal-field" data-testid="provider-key">
    <label htmlFor="provider-api-key">{t("settings.providers.apiKey")}</label>
    <input
      id="provider-api-key"
      type="password"
      value={keyDraft}
      placeholder={current?.configured
        ? t("settings.providers.apiKey.stored")
        : t("settings.providers.apiKey.placeholder")}
      disabled={busy}
      onChange={(e) => setKeyDraft(e.target.value)}
      data-testid="provider-key-input"
    />
    <button type="button" disabled={busy || !keyDraft.trim()}
            onClick={() => void saveKey()} data-testid="provider-key-save">
      {t("settings.providers.apiKey.save")}
    </button>
    {current?.configured ? (
      <button type="button" disabled={busy} onClick={() => void clearKey()}
              data-testid="provider-key-clear">
        {t("settings.providers.apiKey.clear")}
      </button>
    ) : null}
  </div>
) : null}
```

**저장된 키는 절대 입력란에 안 들어간다.** `settings.get`은 `api_key_set`
불리언만 준다. 저장 여부는 **placeholder**로만 말한다. 입력란은 항상 비어서
시작하고, 작가가 새 키를 넣으면 그때만 값이 생긴다.

```ts
const saveKey = () =>
  guard(async () => {
    await settingsApi.set({ providers: { [active]: { api_key: keyDraft.trim() } } });
    setKeyDraft("");
    await refresh();
  });

// An empty string is not a no-op: the engine deletes the stored secret.
// That is the only way to clear a key, and it is why this is a separate
// button rather than "save an empty field".
const clearKey = () =>
  guard(async () => {
    await settingsApi.set({ providers: { [active]: { api_key: "" } } });
    setKeyDraft("");
    await refresh();
  });
```

빈 문자열이 삭제라는 계약이 위험하다 — 실수로 빈 값을 저장하면 키가 사라진다.
그래서 저장 버튼은 `!keyDraft.trim()`일 때 비활성이고, 삭제는 별도 버튼이다.

### 3-3. base URL (`openai`만)

```tsx
{active === "openai" ? (
  <div className="modal-field" data-testid="provider-base-url">
    <label htmlFor="provider-base-url-input">{t("settings.providers.baseUrl")}</label>
    <input
      id="provider-base-url-input"
      type="url"
      value={baseUrlDraft}
      placeholder="https://openrouter.ai/api/v1"
      disabled={busy}
      onChange={(e) => setBaseUrlDraft(e.target.value)}
      onBlur={() => void saveBaseUrl()}
      data-testid="provider-base-url-input"
    />
    <p className="sd">{t("settings.providers.baseUrl.hint")}</p>
  </div>
) : null}
```

엔진은 `openai` 이외의 id에 `base_url`을 실으면 **하드 에러**를 낸다. 그래서 이
필드는 `openai`일 때만 존재해야 하고, 다른 프로바이더로 옮겨갈 때 초안을 비워야
한다. 안 그러면 다음 저장이 통째로 거절된다 — `Set()`은 전부 아니면 전무다.

```tsx
// Every draft is scoped to the provider it belongs to. Carrying an openai
// base URL into an anthropic patch is not a cosmetic bug: the engine rejects
// base_url on any id but openai, and Set() is all-or-nothing, so the whole
// patch — including the key the writer just typed — would be refused.
useEffect(() => {
  setKeyDraft("");
  setBaseUrlDraft(list.find((r) => r.id === active)?.base_url ?? "");
}, [active, list]);
```

### 3-4. 테스트

```tsx
it("never puts a stored key back into the input", async () => {
  // configured: true → the input is empty and the placeholder says stored
});
it("clears a key by sending an empty string", async () => {
  // settingsSet called with { providers: { anthropic: { api_key: "" } } }
});
it("offers base URL only for the openai-compatible provider", async () => { ... });
it("drops the base URL draft when the provider changes", async () => {
  // switch openai → anthropic, save a key, assert the patch has no base_url
});
it("opens the browser and polls until Codex reports signed in", async () => {
  // openExternalUrl called with auth_url; loginStatus polled; poll stops
});
it("stops polling when Codex reports a failed login", async () => { ... });
```

네 번째와 여섯 번째가 핵심이다. 넷째는 전부-아니면-전무 계약을 지키는지, 여섯째는
실패한 로그인이 무한 폴링으로 남지 않는지 본다.

**타이머 테스트.** `vi.useFakeTimers()`를 쓰고 `vi.advanceTimersByTimeAsync`로
민다. 실제 1.5초를 기다리는 테스트를 쓰지 말 것.

### 검증

```
cd apps/desktop && pnpm test ProviderSection
make test-desktop
```

### 커밋

```
feat(desktop): credential fields per provider — Codex login, API key, base URL (#94)
```

---

## Task 4: 모델 콤보박스

목록은 거들 뿐이고, 자유 입력이 본체다.

**왜 자유 입력이 본체인가.** 새 모델은 프로바이더가 발표한 날부터 쓸 수 있어야
하는데, 목록은 그때 낡아 있다. 그리고 `providers.list_models`는 자격증명을
요구하므로, 키를 넣기 전에는 목록 자체가 없다. 목록이 비어도 화면이 잠기면 안 된다.

### 4-1. 컴포넌트

`<datalist>`로 붙인 `<input>`을 쓴다. `<select>`가 아니다 — 목록에 없는 값을
적을 수 있어야 한다.

```tsx
const [models, setModels] = useState<string[]>([]);
const [modelsError, setModelsError] = useState<unknown>(null);
const [modelDraft, setModelDraft] = useState("");

const loadModels = () =>
  guard(async () => {
    setModelsError(null);
    try {
      const { models } = await providersApi.listModels(active);
      setModels(models);
    } catch (e) {
      // A model list that will not load is an inconvenience, not a failure of
      // the pane: the writer can still type a model name. So this error lands
      // on its own line and never in `error`, which drives the section-level
      // alert.
      setModelsError(e);
      setModels([]);
    }
  });
```

```tsx
<div className="modal-field" data-testid="provider-model">
  <label htmlFor="provider-model-input">{t("settings.providers.model")}</label>
  <input
    id="provider-model-input"
    list="provider-model-list"
    value={modelDraft}
    placeholder={t("settings.providers.model.default")}
    disabled={busy}
    onChange={(e) => setModelDraft(e.target.value)}
    onBlur={() => void saveModel()}
    data-testid="provider-model-input"
  />
  <datalist id="provider-model-list">
    {models.map((m) => <option key={m} value={m} />)}
  </datalist>
  <button type="button" disabled={busy || !current?.configured}
          onClick={() => void loadModels()} data-testid="provider-model-refresh">
    {t("settings.providers.model.refresh")}
  </button>
  {modelsError ? (
    <p className="sd" data-testid="provider-model-error">
      {rpcErrorMessage(modelsError, t)}
    </p>
  ) : null}
</div>
```

**새로고침 버튼은 자격증명이 있을 때만 활성.** `list_models`는 `Configured()`를
요구한다. 키가 없는데 누르면 `provider_not_configured`가 뜰 뿐이고, 그건 작가가
이미 아는 사실이다.

**동의는 요구하지 않는다.** 이건 계획서 앞부분에 적은 함정 1의 반대편이다.
`list_models`는 원고를 보내지 않으므로 동의 전에도 부를 수 있고, 그래서 작가는
동의하기 전에 어떤 모델이 있는지 볼 수 있다.

```ts
// Empty means "the provider's own default", which is what #91 chose over
// shipping a model catalogue that ages. Saving "" is meaningful, not a skip.
const saveModel = () =>
  guard(async () => {
    await settingsApi.set({ providers: { [active]: { model: modelDraft.trim() } } });
    await refresh();
  });
```

### 4-2. 테스트

```tsx
it("keeps the model input usable when the list fails to load", async () => {
  listModels.mockRejectedValue(Object.assign(new Error("x"), {
    data: { reason: "provider_unreachable" },
  }));
  // the inline error renders, provider-error (section alert) does NOT,
  // and the input is still enabled and typeable
});
it("saves an empty model as the provider default", async () => {
  // settingsSet with { providers: { anthropic: { model: "" } } }
});
it("disables refresh until a credential is stored", async () => { ... });
```

첫 번째가 핵심이다. 목록 실패가 섹션 전체 에러로 번지면 작가는 아무것도 못 한다.

### 커밋

```
feat(desktop): a model box that types as well as it lists (#94)
```

---

## Task 5: 동의와 연결 테스트

이 화면이 존재하는 이유다.

### 5-1. 동의 체크박스

```tsx
<label className="modal-field">
  <input
    type="checkbox"
    checked={Boolean(current?.consented)}
    disabled={busy}
    onChange={(e) => void saveConsent(e.target.checked)}
    data-testid="provider-consent"
  />
  {t("settings.providers.consent", { provider: t(nameKeyFor(active)) })}
</label>
```

**문장에 프로바이더 이름이 들어간다.** 설계 문서 5.4절이 요구하는 것이고, 이유가
있다. "AI 기능을 사용하는 데 동의합니다"는 아무것도 말하지 않는다. "씬 원문이
**Anthropic**으로 전송되는 데 동의합니다"는 어디로 가는지를 말한다. 동의는
프로바이더별이므로 문장도 프로바이더별이어야 한다.

```ts
const saveConsent = (consented: boolean) =>
  guard(async () => {
    await settingsApi.set({
      providers: { [active]: { consented_at: consented ? Date.now() : 0 } },
    });
    await refresh();
  });
```

`0`이 동의 철회다 — 엔진의 `consented_at`은 평범한 int64 덮어쓰기이고, 0은
"동의 없음"을 뜻한다.

### 5-2. 연결 테스트

```tsx
<div className="modal-field">
  <button
    type="button"
    disabled={busy || !current?.consented || !current?.configured}
    onClick={() => void runTest()}
    data-testid="provider-test"
  >
    {t("settings.providers.test")}
  </button>
  {testResult === "ok" ? (
    <span data-testid="provider-test-ok">{t("settings.providers.test.ok")}</span>
  ) : null}
  {testError ? (
    <span role="alert" data-testid="provider-test-error">
      {rpcErrorMessage(testError, t)}
    </span>
  ) : null}
</div>
```

**버튼이 동의와 자격증명 둘 다에 걸린다.** 동의는 서버 계약이고(함정 1), 자격증명은
그것 없이 누르면 확실히 실패하기 때문이다. 둘 중 하나라도 없으면 누를 수 없는 편이,
누르고 나서 이유 코드를 읽는 것보다 낫다.

```ts
const runTest = () =>
  guard(async () => {
    setTestResult(null);
    setTestError(null);
    try {
      await providersApi.test(active);
      setTestResult("ok");
    } catch (e) {
      // Same reasoning as the model list: a failed test is information about
      // the provider, not a broken pane.
      setTestError(e);
    }
  });
```

**테스트 결과는 프로바이더가 바뀌면 지운다.** Anthropic이 통과했다는 초록 표시가
Gemini 화면에 남아 있으면 거짓말이다.

```tsx
useEffect(() => {
  setTestResult(null);
  setTestError(null);
}, [active]);
```

### 5-3. 테스트

```tsx
it("disables the connection test until consent is given", async () => { ... });
it("disables the connection test until a credential is stored", async () => { ... });
it("writes consent as providers[id].consented_at", async () => {
  // the exact patch shape, since a wrong field name silently does nothing
});
it("revokes consent by writing zero", async () => { ... });
it("clears a passing test result when the provider changes", async () => { ... });
it("renders a failed test as a translated reason, not a raw message", async () => {
  // provider_auth_failed → errors.providerAuthFailed
});
```

세 번째가 중요하다. 앞에서 적었듯 죽은 `ai_data_sharing_consent_*` 필드가 따로
있어서, 잘못 잡으면 화면은 멀쩡히 동작하는데 엔진은 영원히 동의를 못 본다.

### 커밋

```
feat(desktop): per-provider consent, and the test it unlocks (#94)
```

---

## Task 6: `Settings.tsx` 배선과 마감

### 6-1. 카테고리 등록

`Settings.tsx`에서 네 곳을 고친다:

1. `SETTINGS_CATEGORIES`에 `"providers"` 추가
2. `agentAvailable` 상태를 `mcpAvailable`과 같은 방식으로 추가 —
   `diag.agent_available ?? false`. **이 필드는 엔진이 이미 채우고 있지만
   `Settings.tsx`가 읽지 않는다.** #93이 넣었고 아무도 안 쓴다.
3. 기존 `groupConnect` 그룹의 `items`에 두 번째 항목 추가
4. `{category === "providers" && agentAvailable && <ProviderSection />}` 렌더 분기

```tsx
...(mcpAvailable || agentAvailable
  ? [{
      label: t("settings.nav.groupConnect"),
      items: [
        ...(agentAvailable ? [{ id: "providers" as const, label: t("settings.nav.providers") }] : []),
        ...(mcpAvailable ? [{ id: "mcp" as const, label: t("settings.nav.mcp") }] : []),
      ],
    }]
  : []),
```

**프로바이더가 MCP보다 위에 온다.** 작가가 먼저 하는 일이 프로바이더 연결이고,
MCP는 외부 도구를 쓰는 사람만 건드린다.

**`agent_available`로 게이트하는 이유.** 모바일 빌드는 `internal/agent`를
링크하지 않으므로 `agent.*`가 없다. 프로바이더만 설정할 수 있고 쓸 데가 없는
화면을 보여주는 건 거짓 약속이다.

### 6-2. i18n 마감

`settings.nav.providers` 키를 세 카탈로그에 추가하고, 앞선 작업들에서 쓴 모든
`settings.providers.*` 키가 세 곳에 다 있는지 확인한다.

### 6-3. 테스트

`Settings.test.tsx`에 추가:

```tsx
it("shows the provider item in the connect group when the agent is available", async () => { ... });
it("hides it on a build without the agent", async () => {
  // diagnostics agent_available: false
});
```

### 6-4. 전체 검증

```
cd apps/desktop && pnpm test
make test-desktop
make test-tauri
```

`rpcAllowlist.test.ts`가 통과해야 한다. 새 래퍼를 만들지 않았다면 자동으로 통과한다 —
통과하지 않는다면 어딘가에서 새 `rpcCall`을 추가한 것이고, 그건 `RENDERER_ENGINE_METHODS`에
정렬된 위치로 넣어야 한다는 뜻이다.

### 커밋

```
feat(desktop): put the AI provider section in Settings → Connections (#94)
```

---

## 이 계획서가 일부러 남기는 것

- **에이전트 패널은 #95다.** 이 화면은 프로바이더를 연결할 뿐, 그것으로 무엇을
  하는지는 다음 이슈다. 그래서 이 브랜치가 머지돼도 작가가 에이전트를 부를 방법은
  아직 없다.
- **문서 문구는 #96이다.** README와 개인정보 문서가 "Linetta는 모델을 호출하지
  않는다"고 말하는 상태는 패널이 나가기 전까지 참이다.
- **실전 Codex 로그인 검증.** 위 "검증되지 않은 위험"을 볼 것. 이 계획서는 그것에
  막히지 않지만, 릴리스 전에 한 번은 돌려야 한다.
