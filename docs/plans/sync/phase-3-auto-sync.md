# 동기화 Phase 3 — 자동 동기화

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_목표: Linetta Works 동기화를 켜 두면 전체 작품이 알아서 오간다_
_예상 소요: 4~5일_
_전제: Phase 2 완료_
_상태: 계획_

## Goal

작가가 자동 동기화를 한 번 켜면, 고친 작품은 올라가고 다른 기기의 변경은 들어온다.
멈추거나 충돌이 나면 화면에 보인다.

## Non-goals

- GitHub와 폴더의 자동 양방향 (Phase 4)
- 행 단위 병합, 열려 있는 편집기에 실시간 반영
- 작품별 제외

## 동작

### 언제 도는가

| 시점 | 받기 | 올리기 |
|---|---|---|
| 앱 시작, 동기화 연결 직후 | 서재 항목, 전체 작품 | 변경된 작품 |
| 작품을 열 때 | 그 작품 | |
| 작품을 닫을 때, 앱을 내릴 때, 종료할 때 | | 그 작품 |
| 마지막 편집 뒤 3분 | | 그 작품 |
| 5분마다(지터 ±60초), 앱에 포커스가 돌아올 때 | 머리 리비전 확인 뒤 필요한 작품 | 변경된 작품 |

주기를 도는 주체는 엔진이다. 앱 창이 닫혀 있어도 백그라운드 실행이 켜져 있으면 돈다.

### 작품 상태별 처리

| 상태 | 자동 처리 |
|---|---|
| `not_uploaded` | 올린다 |
| `local_changes` | 올린다. 편집이 3분 이상 멈춘 뒤에만 |
| `remote_changes` | 작품이 열려 있지 않으면 받는다. 열려 있으면 "새 버전" 표시를 띄우고 한 번 눌러 받는다 |
| `conflict` | 아무것도 하지 않고 표시만 한다. 고를 때까지 그 작품의 자동 동기화를 멈춘다 |
| 서버에만 있음 | 받는다 |
| 삭제 표시 있음 | 아카이브로 옮기고 알린다 |

에이전트가 작품을 고치는 중이면 그 작품은 건너뛴다. 받기가 작품을 통째로 바꾸기 때문이다.

### 받기 전 백업

지금 받기는 매번 `RunBeforeVaultPull`로 서재 전체를 복사한다. 자동 받기에 그대로 쓰면 `backups/`가 빠르게 는다.
자동 받기는 로컬이 기준 리비전과 같을 때만 일어나므로 로컬 상태는 서버 복원 지점에 있다. 그래서 자동 받기의 전체 백업은 하루 한 번으로 줄인다. 수동 받기와 충돌 처리는 지금처럼 매번 백업한다.

### 복원 지점

서버는 지금 항목마다 최근 5개만 남긴다(`RESTORE_POINTS`). 자동 올리기에 그대로 두면 한두 시간 전 상태도 사라진다.
최근 5개에 더해 지난 7일의 날짜별 마지막 리비전을 남기도록 바꾼다. 조각은 중복 저장되지 않아 쿼터에 주는 부담이 작다. 서버를 먼저 배포한다.

### 삭제

- 동기화된 작품을 지우면 그 항목에 삭제 표시 리비전을 커밋한다. 봉인된 매니페스트의 `Deleted: true`이고, 조각은 없다
- 다른 기기는 받으면 작품을 아카이브로 옮기고 삭제 표시를 알린다. 원고는 지우지 않는다
- 삭제 표시가 있는 항목은 자동으로 다시 받지 않는다
- 서버의 데이터를 완전히 지우는 것은 지금처럼 설정의 항목 삭제로 한다

### 멈춤을 알리는 방법

아래 경우 자동 동기화를 멈추고 동기화 화면과 갤러리에 표시한다.

- `terms_outdated`, `read_only`, 허가 회수
- 쿼터 초과, 항목 수 초과
- 복구 키 없음, 키 불일치
- 사이트 연결 해제, 토큰 거절
- 오프라인은 조용히 기다리되 마지막 성공 시각을 보여 준다

### 켜기

- 기본은 꺼져 있다. 동의한 작가가 "자동 동기화 켜기"를 누를 때 시작한다
- 켤 때 대상 작품 수와 예상 크기를 보여 주고 확인을 받는다. 그 전에는 아무것도 보내지 않는다는 지금의 약속을 지킨다
- 끄면 진행 중인 작업을 마치고 멈춘다

## 설계

```go
// engine/internal/syncauto (go:build !mobile)
type Runner struct {
    Vault  *vault.Service
    Active ActiveWorks // 열린 작품과 에이전트 실행 상태
    Now    func() time.Time
    Sleep  func(ctx context.Context, d time.Duration) error
    Notify func(Status)
}

func (r *Runner) Start(ctx context.Context) (stop func())
func (r *Runner) Tick(ctx context.Context) (Status, error)
func (r *Runner) Touch(projectID string) // 편집 시각
```

- `backup.Start`가 시각과 대기 함수를 주입하는 방식을 따른다
- `vault.Service.begin()`의 한 번에 하나 규칙을 지킨다. 수동 작업이 돌면 그 주기는 건너뛴다
- 서버 호출은 `Items` 한 번으로 머리 리비전을 보고, 바뀐 작품만 더 읽는다
- 변경 판정은 `export` 해시다. 첫 작업에서 잰 비용이 크면 `PRAGMA data_version`이 그대로인 주기는 건너뛴다
- 설정 `SyncAutoEnabled`는 `engine/internal/settings/settings.go`에 둔다. 서재 항목에는 넣지 않는다. 기기마다 켠다
- 알림은 `sync.status`다. `vault.progress`와 같은 방식으로 보낸다

## Touch points

- `engine/internal/syncauto/` 신규, `engine/internal/engineapp/engineapp.go`, `vault_enabled.go`, `vault_disabled.go`
- `engine/internal/vault/service.go`, `manifest.go`, `repo.go`
- `engine/internal/store/migrations/` (삭제 대기열)
- `engine/internal/rpc/handlers/vault.go`, `projects.go`
- `engine/internal/settings/settings.go`
- `apps/desktop/src-tauri/src/lib.rs`
- `apps/desktop/src/routes/Workspace.tsx`, `Library.tsx`, `apps/desktop/src/components/settings/VaultSection.tsx`, `VaultNotice.tsx`, `apps/desktop/src/lib/rpc.ts`, `types.ts`, `i18n.tsx`
- `linetta-artworks`: `backend/pb_hooks/lib/vault.js`, `backend/tests/`

## Tasks

- [ ] **3.1 측정**
  - 임시 서재에 큰 작품을 만들어 `export` 시간과 한 번의 올리기 전송량을 잰다. 씬 하나를 고친 경우를 잰다
  - 결과를 이 문서에 적고, `data_version` 건너뛰기와 `node_snapshots` 파트 분할 필요 여부를 정한다
  - 검증: 측정값이 문서에 있음

- [ ] **3.2 복원 지점 보존 규칙** (`linetta-artworks`, 먼저 배포)
  - `vault.js`의 커밋 뒤 삭제를 최근 5개와 날짜별 마지막 리비전 유지로 바꾼다. `limits`에 `daily_points: 7`을 더한다
  - 시험: 하루 안의 20개 리비전, 여러 날에 걸친 리비전, 사용량 계산, 정리 작업이 살아 있는 조각을 지우지 않음
  - 검증: `make test-backend`

- [ ] **3.3 서비스 분리**
  - `vault.Service`에 `SyncAll(ctx, policy)`을 더한다. `Pull`의 백업을 정책으로 나눈다
  - 시험: 상태별 처리 표의 모든 줄, 열린 작품, 에이전트 실행 중인 작품, 충돌 작품은 건드리지 않음
  - 검증: `go test ./internal/vault/...`

- [ ] **3.4 Runner**
  - `syncauto` 패키지, 주기, 지터, 대기 중 취소, 연속 실패 시 간격 늘리기. 최대 30분이다
  - 시험: 가짜 시계로 주기, 3분 대기, 멈춤 조건
  - 검증: `go test ./internal/syncauto/...`

- [ ] **3.5 앱 연결**
  - RPC `sync.status`, `sync.run_now`, `sync.set_auto`, `sync.touch`, `sync.set_active`를 더하고 `RENDERER_ENGINE_METHODS`에 넣는다
  - `Workspace.tsx`는 저장할 때 `sync.touch`, 열고 닫을 때 `sync.set_active`를 보낸다. 닫기 전에 `flushPendingSave`를 기다린다
  - 작품을 열 때 `remote_changes`면 받은 뒤 연다. 실패하면 로컬로 열고 표시한다
  - 검증: `pnpm test`, `make test-tauri`

- [ ] **3.6 삭제 전파**
  - `projects.delete`가 연결된 항목을 삭제 대기열에 넣고 Runner가 삭제 표시를 커밋한다. `vault_items`는 작품을 지우면 `ON DELETE CASCADE`로 함께 사라지므로, 항목 ID는 지우기 전에 읽어 대기열에 옮긴다
  - 시험: 받는 쪽 아카이브 이동, 재다운로드 없음, 오프라인 삭제 뒤 연결
  - 검증: `go test ./internal/vault/... ./internal/rpc/handlers/...`

- [ ] **3.7 화면**
  - 동기화 화면: 켜기 확인, 상태, 마지막 동기화, 작품별 상태, "지금 동기화"
  - 갤러리와 작품 화면: 멈춤과 충돌 표시, 새 버전 표시
  - 수동 올리기·받기는 고급 영역에 남긴다
  - 검증: 컴포넌트 시험, 세 언어 문구

- [ ] **3.8 문서와 릴리스**
  - `README.md`, 사이트, `linetta-artworks/deploy/README.md`, Phase 11 계획의 조건을 고친다
  - 서버 배포 뒤 앱을 릴리스한다

---

### ✅ Checkpoint: Phase 3 완료 확인

**구현 확인:**
- [ ] 끈 동안에는 어떤 요청도 사이트로 나가지 않는다
- [ ] 충돌 작품, 열린 작품, 에이전트 실행 중인 작품이 자동으로 바뀌지 않는다
- [ ] 멈춤 조건마다 화면에 이유가 나온다
- [ ] 서버, DB, R2, 로그에 원고 평문, 제목, 복구 키가 없다

**실행 확인:**
- [ ] `. "$HOME/.cargo/env"; CGO_ENABLED=0 make ci` 통과
- [ ] `cd engine && CGO_ENABLED=0 go test -tags mobile ./...` 통과
- [ ] `linetta-artworks`에서 `make test` 통과

**수동 확인 (실제 앱, 두 기기):**
- [ ] 기기 A에서 고치고 멈추면 기기 B에 몇 분 안에 들어온다. 비공개 보관함 Phase 11의 이어 쓰기 항목을 겸한다
- [ ] 새 기기에서 연결과 복구 키만으로 전체 작품, 전역 스킬, 작가 프로필이 들어온다
- [ ] 두 기기에서 오프라인으로 같은 작품을 고치면 충돌 화면이 나오고 어느 쪽을 골라도 양쪽 원고가 남는다
- [ ] 받은 작품을 게시하면 사이트의 같은 작품과 회차가 갱신된다
- [ ] 운영자가 회수하면 자동 동기화가 멈추고 이유가 보이며, 받기는 된다
- [ ] 약관 버전을 올리면 다시 동의할 때까지 올리기가 멈춘다
- [ ] 한 기기에서 작품을 지우면 다른 기기에서 아카이브로 옮겨지고 원고가 남는다
- [ ] 해지하면 R2 객체가 함께 사라진다

**이 체크포인트를 통과하면 사용자에게 확인을 요청한다. 며칠 실제로 쓴 뒤 Phase 4를 시작한다.**
실패하면 실패 항목을 보고하고 원인을 찾아 고친 뒤 다시 검증한다.

---
