# 동기화 Phase 2 — 항목 선택과 서재 항목

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_목표: 옮길 항목을 작가가 고르고, 전역 스킬과 작가 프로필이 기기 사이를 오간다_
_예상 소요: 3~4일_
_전제: Phase 1 완료_
_상태: 계획_

## Goal

설정에서 항목을 체크하면 모든 기기가 그 선택을 따른다. 끈 항목은 올라가지 않고, 받을 때 지워지지도 않는다.
새 기기는 작품을 받기 전에 전역 스킬, 작가 프로필, 항목 선택부터 받는다.

## Non-goals

- 자동 실행. 이 단계도 수동 올리기·받기다
- 에이전트 대화와 사용 내역
- GitHub와 폴더

## 항목

| 범주 ID | 화면 이름 | 내용 | 기본 |
|---|---|---|---|
| (필수) | 원고와 설정 | 원고, 세계관, 구성, 스토리라인, 자료집, 메모, 문체, 연속성 점검, 스토리 제안, 통계 | 항상 켜짐 |
| `images` | 삽화와 참조 이미지 | `illustrations`, `node_representative_illustrations`, `illustration_requests`, `entity_reference_images` | 켜짐 |
| `history` | 버전 이력 | `node_snapshots` | 켜짐 |
| `work_agent` | 작품 스킬과 메모리 | `files/skills/`, `files/memory/`, `agent_memory`의 `work_notes` | 켜짐 |
| `global_skills` | 전역 스킬 | `<home>/skills/<name>/` (`works/` 제외) | 켜짐 |
| `writer_profile` | 작가 프로필 | `agent_memory`의 `writer_profile` | 켜짐 |

화면에는 항목별 현재 크기를 함께 보여 준다. 삽화가 대부분을 차지한다는 것을 작가가 눈으로 보고 고르게 한다.

## 설계

### 번들 형식 2

```go
const FormatVersion = 2

type Manifest struct {
    // ...기존 필드
    Omitted []string `json:"omitted,omitempty"` // 이 번들이 싣지 않은 범주
}
```

- `tables.go`의 `Table`에 `Category` 필드를 더한다. 값이 없으면 필수다. 모든 표가 분류돼 있어야 한다는 `TestEveryTableIsClassified`를 유지한다
- `Export(ctx, db, home, projectID, now, omit)`는 끈 범주의 표와 파일을 싣지 않는다. 같은 이유로 해시도 바뀌므로, 선택을 바꾸면 작품마다 새 리비전이 한 번 올라간다
- `Apply`는 `Manifest.Omitted`에 있는 범주의 표를 `DELETE`하지 않고 쓰지도 않는다. `ApplyFiles`도 같다
- 형식 1 번들은 `Omitted`가 비어 있는 것으로 읽는다. 읽는 코드를 지우지 않는다

### 끈 항목이 남아야 하는 경로

`applyNodes`와 `applyEntities`가 지운 노드나 인물을 참조하는 행은 외래 키로 함께 지워질 수 있다. 끈 범주의 행은 부모가 사라졌을 때만 함께 사라지고, 그 밖에는 그대로여야 한다. 이것을 시험으로 고정한다.

### 삽화를 끈 기기

본문의 `illustration` 노드는 id만 갖는다. 삽화가 없는 새 기기에서는 지금 "삭제된 삽화" 자리표시가 나오는데 사실과 다르다. `images`가 꺼져 있으면 "이 기기에 없는 삽화"로 그린다. 다시 켜면 다음 리비전에서 돌아온다.

### 서재 항목

```go
// 복구 키에서 유도한다. 두 기기가 서로 묻지 않고 같은 항목을 찾는다.
func (k *Keys) LibraryItemID() string

type LibraryBundle struct {
    Format   int             `json:"format"`
    Settings LibrarySettings `json:"settings"` // omit 목록
    // 파트: profile, skills/<name>/...
}
```

- `vaultbundle/library.go`에 `ExportLibrary`, `ApplyLibrary`를 둔다. 작품 번들과 파트 방식이 같다
- `revisionManifest`에 `Kind` 필드를 더한다. `""`는 작품, `"library"`는 서재다. `Status`의 작품 목록에는 나오지 않는다
- 받을 때 충돌을 묻지 않는다. 파일별로 시각이 새로운 쪽이 이긴다. 덮어쓰기 전 현재 스킬은 `skill_snapshots`에 남겨 되돌릴 수 있게 한다
- 작가 프로필은 `agentmemory`의 예산을 넘으면 받은 값을 잘라 쓰지 않고 거절한 뒤 알린다
- 프로바이더 키, 설정, 약관 동의는 절대 싣지 않는다

### 새 기기

`vault.setup`이 복구 키를 저장한 직후 서재 항목을 먼저 받는다. 서재 항목이 없으면 첫 올리기가 만든다.

## Touch points

- `engine/internal/vaultbundle/bundle.go`, `tables.go`, `library.go` 신규
- `engine/internal/vault/service.go`, `manifest.go`, `crypto.go`
- `engine/internal/agentmemory/agentmemory.go`, `engine/internal/agentskills/store.go`, `history.go`
- `engine/internal/rpc/handlers/vault.go`, `engine/internal/engineapp/vault_enabled.go`, `vault_disabled.go`
- `apps/desktop/src-tauri/src/lib.rs` (`RENDERER_ENGINE_METHODS`와 그 시험)
- `apps/desktop/src/components/settings/VaultSection.tsx`, `apps/desktop/src/lib/rpc.ts`, `types.ts`, `i18n.tsx`
- `apps/desktop/src/components/editor/IllustrationNodeView.tsx`
- `linetta-artworks`: `frontend/src/content/vault-terms.ts`, `terms.ts`, `privacy.ts`, `backend/pb_hooks/lib/vault-access.js`

## Tasks

- [ ] **2.1 범주 분류와 형식 2**
  - `tables.go`에 `Category`, `bundle.go`에 `Omitted`와 `FormatVersion = 2`
  - 시험: 범주별로 끈 왕복, 형식 1 번들 읽기, 분류 누락 실패
  - 검증: `cd engine && CGO_ENABLED=0 go test ./internal/vaultbundle/...`

- [ ] **2.2 끈 항목 보존**
  - `Apply`, `ApplyFiles` 수정
  - 시험: 삽화를 끄고 올린 리비전을 삽화가 있는 서재에 받아도 삽화와 대표 삽화 지정이 남는다. 받은 뒤 작품이 변경으로 보이지 않는다
  - 시험: 옛 복원 지점으로 되돌릴 때도 같다
  - 검증: `go test ./internal/vaultbundle/... ./internal/vault/...`

- [ ] **2.3 서재 항목**
  - `LibraryItemID`, `ExportLibrary`, `ApplyLibrary`, `vault.Service`의 서재 올리기·받기
  - 시험: 두 서재 왕복, 새 것이 이김, 진 스킬이 `skill_snapshots`에 남음, `works/`가 섞이지 않음, 비밀 값이 번들에 없음
  - 검증: `go test ./internal/vault/... ./internal/agentskills/...`

- [ ] **2.4 선택 저장과 RPC**
  - RPC `vault.items_get`(범주별 크기와 켜짐), `vault.items_set`을 더하고 `RENDERER_ENGINE_METHODS`에 넣는다
  - 선택은 서재 항목에 봉인한다. 받는 쪽은 서재 항목의 선택을 따른다
  - 필수 범주를 끄는 요청은 거절한다
  - 검증: `go test ./internal/rpc/handlers/...`, `make test-tauri`

- [ ] **2.5 설정 화면과 자리표시**
  - Linetta Works 영역에 체크 목록과 크기, 끄면 다음 올리기부터 빠진다는 안내를 넣는다
  - `IllustrationNodeView`에 "이 기기에 없는 삽화" 상태를 더한다
  - 검증: `VaultSection.test.tsx`, 세 언어 문구

- [ ] **2.6 약관 v2** (`linetta-artworks`, 앱보다 먼저 배포)
  - `vault-terms.ts`의 이름을 "Linetta Works 동기화"로 바꾼다. 전역 스킬과 작가 프로필이 함께 전송될 수 있다는 것, 자동 동기화를 켜면 변경이 자동으로 전송된다는 것을 적는다
  - `VAULT_TERMS_VERSION`, `TERMS_VERSION`, `TERMS_SHA256`을 올린다. `terms.ts` 7조의2와 `privacy.ts`도 맞춘다
  - 앱의 `settings.vault.consent`와 세 언어 개인정보 문서를 고친다
  - 문구는 작가가 확인한 뒤 확정한다
  - 검증: `vault-terms.test.ts`, `make test`

---

### ✅ Checkpoint: Phase 2 완료 확인

**구현 확인:**
- [ ] 끈 범주가 서버로 나가지 않는다 (파트 목록으로 확인)
- [ ] 끈 범주가 받기로 지워지지 않는다
- [ ] 프로바이더 키와 토큰이 어떤 번들에도 없다

**실행 확인:**
- [ ] `. "$HOME/.cargo/env"; CGO_ENABLED=0 make ci` 통과
- [ ] `cd engine && CGO_ENABLED=0 go test -tags mobile ./...` 통과
- [ ] `linetta-artworks`에서 `make test` 통과

**수동 확인:**
- [ ] 임시 서재 둘로 운영 환경에서 전역 스킬과 작가 프로필이 오가는지 본다. 임시 서재는 `CGO_ENABLED=0` 엔진과 `LINETTA_HOME`을 쓴다
- [ ] 삽화를 끄면 사용량이 다음 리비전들이 밀려난 뒤 줄어든다
- [ ] 약관 v2 재동의 전에는 올리기가 막히고 받기는 된다

**이 체크포인트를 통과하면 사용자에게 확인을 요청한 뒤 다음으로 진행한다.**
실패하면 실패 항목을 보고하고 원인을 찾아 고친 뒤 다시 검증한다.

---
