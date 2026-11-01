# 동기화 Phase 4 — GitHub·폴더 합류

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_목표: GitHub와 폴더가 Linetta Works와 같은 항목을 같은 방식으로 저장하고, 기본 대상이나 미러가 된다_
_예상 소요: 4~5일_
_전제: Phase 3 완료, 며칠 실제 사용_
_상태: 계획_

## Goal

작가는 Linetta Works, GitHub, 폴더 중 하나를 기본 동기화로 고른다. 나머지는 미러로 켤 수 있다.
어느 대상이든 같은 항목이 실리고, 저장된 내용에서 새 기기로 복원할 수 있다.

## Non-goals

- 여러 대상의 동시 양방향
- 평문으로 된 전체 상태 저장. 사람이 읽는 것은 Markdown이 맡는다
- GitHub API 연동. 인증은 지금처럼 시스템 git에 맡긴다
- GitHub와 폴더를 쓰기 위한 운영자 허가

## 저장 형식

GitHub와 폴더는 같은 배치를 쓴다.

```text
<대상 폴더>/
  <작품>.md, <작품> 그림 폴더/     지금의 Markdown 내보내기. 그대로 둔다
  .linetta-sync.json               지금의 목록
  .linetta/
    format.json                    {"format":1}
    blobs/ab/<blob id>             봉인된 조각. 내용 주소라 덮어쓰지 않는다
    items/<item id>/
      00000012-<기기>-<매니페스트 앞 8자>.json
```

리비전 파일은 `{revision, manifest_blob, blob_ids, created, bytes}`다.

서버가 없으므로 비교 후 쓰기를 파일 이름으로 대신한다.

- 파일 이름에 기기와 매니페스트가 들어가, 두 기기가 같은 번호를 써도 파일이 겹치지 않는다. Dropbox와 Git 모두 충돌 사본이나 병합 충돌이 생기지 않는다
- 머리는 `(revision, 파일 이름)`이 가장 큰 파일이다. 모든 기기가 같은 답을 낸다
- 같은 번호에서 진 기기는 머리가 자기 해시와 달라 `conflict`가 된다. 판정 코드는 지금 것을 그대로 쓴다
- 조각을 먼저 쓰고 리비전 파일을 마지막에 쓴다. 중간에 끊겨도 리비전이 없는 조각만 남는다
- 동기화 앱이 조각보다 리비전 파일을 먼저 옮겨 올 수 있다. 조각이 없으면 받기를 미루고 다음 주기에 다시 본다. 오류로 다루지 않는다
- 정리는 복원 지점 규칙을 따른다. 어느 리비전도 쓰지 않고 하루 이상 지난 조각을 지운다

## Store 인터페이스

`vault.Service`가 `*Client`와 토큰을 직접 쓰는 부분을 인터페이스 뒤로 옮긴다.

```go
type Store interface {
    Account(ctx context.Context) (Account, error)
    Items(ctx context.Context) ([]RemoteItem, error)
    Revisions(ctx context.Context, itemID string) ([]Revision, error)
    PutBlob(ctx context.Context, id string, sealed []byte) error
    GetBlob(ctx context.Context, id string) ([]byte, error)
    Commit(ctx context.Context, itemID string, base int, manifest string, blobs []string) (int, error)
    DeleteItem(ctx context.Context, itemID string) error
}
```

- `worksStore`는 지금의 `Client`와 `PublishToken`을 감싼다
- `dirStore`는 위 배치를 읽고 쓴다. `Account`는 `ModeFull`이고 쿼터가 없다
- `gitStore`는 `dirStore`에 Git 왕복을 더한다
- 조각 누락과 충돌은 지금과 같은 오류로 돌려준다. 재개와 충돌 흐름을 다시 쓰지 않는다

연결 표 `vault_items`에 `target` 열을 더해, 작품 하나가 대상마다 기준 리비전을 갖게 한다. 기존 행은 `works`다.

## Git 순서

지금 `gitsync.RunOnce`는 `pull`을 하지 않아 두 번째 기기의 `push`가 거절된다. 순서를 이렇게 바꾼다.

1. 작업 폴더가 깨끗하지 않으면 지금까지처럼 add와 commit을 먼저 한다
2. `git pull --ff-only`. 실패하면 `git pull --rebase`. 충돌이 Markdown뿐이면 원격을 받고 다음 단계에서 다시 만든다. 그 밖의 충돌은 `git rebase --abort`로 되돌리고 멈춘다
3. `.linetta/`로 동기화한다
4. Markdown을 내보낸다
5. `git add`, `commit`, `push`. 거절되면 한 번만 2번부터 다시 한다

`excludeFromStaging`의 `.mcp.json` 제외를 유지한다. 작가가 직접 넣은 파일을 지우는 명령(`reset --hard`, `clean`)은 쓰지 않는다.

## 주기

| 대상 | 기본 대상일 때 | 미러일 때 |
|---|---|---|
| Linetta Works | Phase 3 그대로 | 작품을 닫을 때와 앱 종료 때 올리기 |
| 폴더 | Phase 3 그대로 | 작품을 닫을 때와 앱 종료 때 올리기 |
| GitHub | 30분마다, 앱 시작과 종료 때 | 하루 한 번 |

Git 이력은 지워지지 않고 봉인된 조각은 압축되지 않는다. 그래서 GitHub만 주기를 길게 둔다. 기본 대상으로 고를 때 저장소가 커질 수 있다고 알리고, `.linetta/`의 크기를 동기화 화면에 보여 준다.

## 제약

- MAS 빌드는 엔진이 사용자 폴더를 읽지 못하고 Tauri가 북마크로 복사만 한다(`folder_sync.rs`). MAS에서 폴더는 올리기만 하는 미러로 두고 기본 대상은 고를 수 없게 한다
- 모바일은 지금처럼 제외한다
- 기본 대상을 바꾸면 새 대상에 전체를 올린 뒤 바꾼다. 옛 대상의 데이터는 지우지 않는다

## Touch points

- `engine/internal/vault/store.go`, `store_works.go`, `store_dir.go`, `store_git.go` 신규, `service.go`, `repo.go`
- `engine/internal/gitsync/gitsync.go`, `engine/internal/foldersync/foldersync.go`
- `engine/internal/syncauto/`
- `engine/internal/store/migrations/`, `engine/internal/settings/settings.go`
- `apps/desktop/src-tauri/src/folder_sync.rs`, `apps/desktop/src-tauri/src/lib.rs`
- `apps/desktop/src/routes/Settings.tsx`, `apps/desktop/src/components/settings/VaultSection.tsx`, `i18n.tsx`

## Tasks

- [ ] **4.1 Store 인터페이스 추출**
  - 동작 변화 없이 `worksStore`로 옮긴다
  - 검증: 기존 `service_test.go` 전부 통과, 가짜 사이트 시험 그대로

- [ ] **4.2 `dirStore`**
  - 배치, 커밋, 머리 판정, 조각 누락, 정리
  - 시험: 공통 시험표를 `worksStore`와 `dirStore`에 함께 돌린다. 같은 번호 동시 커밋, 리비전이 조각보다 먼저 도착, 끊긴 커밋
  - 검증: `go test ./internal/vault/...`

- [ ] **4.3 대상별 연결**
  - `vault_items.target` 마이그레이션. 지금 `project_id`가 기본 키라 `(project_id, target)` 기본 키로 표를 다시 만든다. 기존 행 보존 시험
  - 설정 `SyncPrimaryTarget`, `SyncMirrors`
  - 복구 키 만들기를 Linetta Works 허가 뒤가 아니라 동기화 공통 단계로 옮긴다. GitHub와 폴더만 쓰는 작가도 만들 수 있어야 한다
  - 검증: 마이그레이션 시험, `go test ./internal/vault/... ./internal/settings/...`

- [ ] **4.4 `gitStore`와 순서 변경**
  - 위 Git 순서를 구현한다. `CmdRunner` 시험 기록을 쓴다
  - 시험: 실제 `git` 저장소 둘과 bare 원격으로 두 기기 왕복, push 거절 재시도, Markdown 충돌, 사용자 파일 보존, `.mcp.json` 제외
  - `gitTimeout` 60초가 첫 올리기에 모자라면 Git 왕복만 늘린다
  - 검증: `go test ./internal/gitsync/... ./internal/vault/...`

- [ ] **4.5 Runner 합류**
  - 기본 대상과 미러를 주기표대로 돌린다. 하루 한 번 도는 `retentionFn`의 동기화를 Runner로 옮긴다
  - 검증: `go test ./internal/syncauto/... ./internal/engineapp/...`

- [ ] **4.6 화면**
  - 동기화 화면에 기본 대상 선택, 미러 켜기, 대상별 상태와 크기
  - Phase 1의 "내보내기만 합니다" 문구를 고친다
  - 검증: 컴포넌트 시험, 세 언어 문구

- [ ] **4.7 MAS와 문서**
  - MAS에서 기본 대상 선택을 막고 미러만 둔다
  - `README.md`, 사이트, 개인정보 문서를 고친다. GitHub와 폴더에도 암호화된 전체 사본이 실린다는 것을 적는다
  - 검증: `make site-check`

---

### ✅ Checkpoint: Phase 4 완료 확인

**구현 확인:**
- [ ] 세 대상에 같은 항목이 실린다. 선택을 바꾸면 세 대상 모두 따른다
- [ ] `.linetta/` 안에 평문 제목과 원고가 없다
- [ ] `.mcp.json`과 작가의 파일이 건드려지지 않는다

**실행 확인:**
- [ ] `. "$HOME/.cargo/env"; CGO_ENABLED=0 make ci` 통과
- [ ] `cd engine && CGO_ENABLED=0 go test -tags mobile ./...` 통과

**수동 확인:**
- [ ] 비공개 GitHub 저장소를 기본 대상으로, 두 기기에서 오가며 쓴다
- [ ] Dropbox 같은 동기화 폴더를 기본 대상으로 같은 시험을 한다
- [ ] Linetta Works를 기본 대상으로, GitHub 미러에서 새 서재로 복원한다
- [ ] 복구 키가 없는 상태에서도 Markdown은 읽힌다

**이 체크포인트를 통과하면 사용자에게 보고하고 동기화 통합 완료를 승인받는다.**
실패하면 실패 항목을 보고하고 원인을 찾아 고친 뒤 다시 검증한다.

---
