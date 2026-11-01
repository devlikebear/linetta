# 동기화 Phase 1 — 용어와 화면 통합

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_목표: 동작은 그대로 두고 이름과 자리만 바꿔, 작가가 한 곳에서 세 가지 동기화를 보게 한다_
_예상 소요: 1~2일_
_전제: 없음_
_상태: 계획_

## Goal

갤러리의 "보관함"이 "아카이브"가 되고, 설정의 "비공개 보관함"이 사라져 "동기화" 안의 "Linetta Works"가 된다.

## Non-goals

- 동기화 동작 변경. GitHub와 폴더는 이 단계에서도 Markdown 내보내기다
- 코드, RPC, DB, URL의 `vault` 이름 변경
- 약관 문구 변경. `vault-terms.ts`를 바꾸면 재동의가 걸리므로 Phase 2에서 한 번에 한다

## 문구 표

| 키 | 지금 (ko) | 바꾼 뒤 ko | en | ja |
|---|---|---|---|---|
| `library.archiveBox` | 보관함 | 아카이브 | Archive | アーカイブ |
| `library.archive` | 보관 | 아카이브 | Archive | アーカイブ |
| `library.archived` | 보관됨 | 아카이브됨 | Archived | アーカイブ済み |
| `library.toast.projectArchiveSuccess` | 작품을 보관했습니다 | 작품을 아카이브했습니다 | 그대로 | 作品をアーカイブしました |
| `library.toast.projectArchiveFailed` | 작품 보관 실패 | 작품 아카이브 실패 | 그대로 | 作品のアーカイブに失敗 |
| `workspace.vault` | 비공개 보관함 | Linetta Works 동기화 | Linetta Works sync | Linetta Works 同期 |
| `settings.vault.title` | 비공개 보관함 | Linetta Works | Linetta Works | Linetta Works |

`settings.vault.*`와 `vault.*`의 "보관함"은 문맥에 맞게 "Linetta Works" 또는 "동기화"로 고친다.
일본어는 지시가 없었지만 같은 개념이 한 언어 안에서 두 이름이 되지 않도록 함께 맞춘다.

## Touch points

- `apps/desktop/src/lib/i18n.tsx`
- `apps/desktop/src/routes/Settings.tsx`, `apps/desktop/src/routes/Workspace.tsx`
- `apps/desktop/src/components/settings/VaultSection.tsx`, `VaultDialog.tsx`, `VaultNotice.tsx`
- `apps/site/src/lib/content.ts`, `README.md`, `docs/privacy-policy.md`
- `linetta-artworks`: `frontend/src/app/me/vault/page.tsx`, `frontend/src/app/admin/`의 보관함 탭, 내 계정 메뉴, `docs/api.md`

## Tasks

- [ ] **1.1 아카이브 문구**
  - 위 표대로 세 언어를 고친다
  - `rg '보관' apps/desktop/src/lib/i18n.tsx`로 전수 조사한다. archive를 뜻하는 곳만 바꾼다. `thread.closeArchive`도 포함한다. "함께 보관합니다" 같은 일반 동사는 둔다
  - 검증: `cd apps/desktop && pnpm test`, 문구를 찾는 시험을 함께 고친다

- [ ] **1.2 설정 합치기**
  - `Settings.tsx`의 `settings.nav.vault` 항목을 없애고 `sync` 화면 맨 위에 `VaultSection`을 둔다. 순서는 Linetta Works, GitHub, 폴더다
  - `vault` 카테고리로 들어오는 경로(`VaultDialog`의 "설정 열기", `VaultNotice`)는 `sync`로 보낸다. 저장된 카테고리 값이 `vault`면 `sync`로 읽는다
  - `vaultAvailable`이 거짓이면 Linetta Works 영역만 숨긴다. GitHub와 폴더는 그대로다
  - 작품 화면의 올리기·받기 창과 버튼은 자리를 유지하고 이름만 바꾼다
  - 검증: `VaultSection.test.tsx`와 Settings 시험 통과, 카테고리 이동 시험 추가

- [ ] **1.3 지금 동작을 사실대로 적기**
  - Linetta Works 영역 위에 "기기 사이를 오갑니다", GitHub와 폴더 영역에 "내보내기만 합니다"를 적는다. Phase 4에서 이 문구를 고친다
  - 검증: 세 언어 키 누락 시험 통과

- [ ] **1.4 사이트 이름** (`linetta-artworks`)
  - `/me/vault` 제목과 안내, 내 계정 메뉴, 관리자 탭, 알림 문구의 "보관함"을 "Linetta Works 동기화"로 고친다. URL과 API 경로는 그대로다
  - 약관과 동의 문구는 건드리지 않는다
  - 검증: `pnpm --dir frontend lint`, `pnpm --dir frontend test`, `make test-backend`(알림 문구 시험이 있으면 함께 고친다)

- [ ] **1.5 문서**
  - `README.md`, `apps/site/src/lib/content.ts`, 세 언어 개인정보 문서, `linetta-artworks/docs/api.md`의 이름을 고친다
  - 검증: `make site-check`

---

### ✅ Checkpoint: Phase 1 완료 확인

**구현 확인:**
- [ ] 갤러리에 "아카이브"가 보이고, 앱 어디에도 archive를 뜻하는 "보관함"이 없다
- [ ] 설정의 데이터 그룹에 "비공개 보관함"이 없고, 동기화 안에 Linetta Works, GitHub, 폴더가 있다
- [ ] 내부 이름 `vault`, RPC, DB, URL은 바뀌지 않았다

**실행 확인:**
- [ ] `. "$HOME/.cargo/env"; CGO_ENABLED=0 make ci` 통과
- [ ] `cd engine && CGO_ENABLED=0 go test -tags mobile ./...` 통과
- [ ] `linetta-artworks`에서 `make test` 통과

**수동 확인:**
- [ ] 세 언어로 갤러리와 설정을 열어 본다
- [ ] 작품 화면의 동기화 창에서 설정으로 가는 링크가 동기화 화면을 연다

**이 체크포인트를 통과하면 사용자에게 확인을 요청한 뒤 다음으로 진행한다.**
실패하면 실패 항목을 보고하고 원인을 찾아 고친 뒤 다시 검증한다.

---
