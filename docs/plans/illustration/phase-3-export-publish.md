# 삽화 Phase 3 — 내보내기·동기화·게시

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_목표: 본문에 넣은 삽화가 Markdown 내보내기, 폴더/Git 동기화, Linetta Works 게시까지 따라가게 하는 수직 슬라이스_
_예상 소요: Linetta 2~3일 + Linetta Works 2일_
_전제: Phase 2 완료. Works 쪽은 `linetta-works` 저장소에 별도 계획과 PR이 필요하다_

_상태: 3.1~3.3 완료(2026-10-24). 3.4~3.5(Works 게시)는 별도 계획으로 미룸_

## Goal

- Markdown으로 내보내면 삽화가 그 `.md` 파일 하나에 담겨 나간다.
- 폴더/Git으로 동기화하면 작품 파일 옆 이미지 폴더에 삽화가 파일로 나가고, 본문은 상대 경로로 가리킨다.
- Markdown 가져오기는 파일에 담긴 삽화를 갤러리에 넣고 `illustration` 노드로 되살린다.
- (미룸) Works에 회차를 게시하면 그 회차에 쓰인 삽화가 함께 올라가고, 독자 화면에 보인다.

## 결정 (2026-10-24)

- **내보내기·가져오기는 파일 하나**: 데스크톱 내보내기와 가져오기는 `.md` 한 개만 저장하고 연다(MAS 샌드박스에서 옆 폴더를 쓰거나 읽을 권한이 없다). 그래서 내보낸 파일은 본문에 참조형 이미지 `![캡션][illustration-<id>]`를 두고, 파일 끝에 그림마다 한 번 `[illustration-<id>]: data:<mime>;base64,...` 정의를 둔다. 본문은 읽기 쉽게 남고, 같은 그림을 두 번 써도 한 번만 담긴다. 가져오기는 이 정의만 읽는다(상대 경로 파일은 읽지 않는다).
- **동기화는 이미지 파일**: 동기화 폴더는 엔진(또는 MAS에서는 Rust 쪽 복사)이 쓸 수 있으므로 `<작품 파일 이름>.images/<id>.<ext>`에 파일로 쓰고 본문은 `![캡션](<폴더>/<id>.<ext>)`로 가리킨다. Git diff에 base64가 쌓이지 않는다.
- **Works 게시(3.4~3.5)는 미룸**: `linetta-works` 저장소 쪽 API 합의가 먼저 필요해서 별도 계획(linetta-works 쪽)으로 진행한다. Phase 2의 "삽화는 아직 게시되지 않습니다" 안내는 그대로 둔다.

## Non-goals

- Works에서 이미지 크기 변환·CDN 최적화 (Works 쪽 판단)
- 게시하지 않은 회차의 삽화 업로드
- 외부 URL 이미지 가져오기

## Touch points

- `engine/internal/export/markdown.go`, `project.go`, `episode.go`
- `engine/internal/foldersync/foldersync.go`, `engine/internal/gitsync/`
- `engine/internal/importmd/`
- `engine/internal/publish/client.go`, `service.go`
- `apps/desktop/src/components/settings/PublishSection.tsx`, 게시 미리보기 UI
- `linetta-works`: `backend/`(PocketBase 컬렉션·훅), `frontend/`(회차 본문 렌더러)

## Engine tasks

- [x] **3.1 내보내기에 이미지 싣기**
  - `export/illustrations.go`: `IllustrationSource`(갤러리 바이트), `imageRenderer`가 `illustration` 노드를 참조형 이미지(내장) 또는 `<dir>/<id>.<ext>` 링크(동기화)로 쓴다
  - `Sources.Illustrations`·`Sources.ImageDir`, `ExportNode(..., illustrations)`. 내장 모드는 정의를 부록 뒤 파일 끝에, 파일 모드는 `Payload.Images`로 돌려준다
  - 갤러리에서 지운 그림은 빠지고, 소스를 넘기지 않는 호출(클립보드 복사 등)은 Phase 2처럼 삽화를 뺀다
  - 이미지 폴더 이름은 `SyncImageDir` = `SyncFilename`에서 `.md`를 뺀 뒤 `.images`
  - 테스트: `export/illustrations_test.go` — 위치, 한 번만 정의, 캡션 `[ ]` 이스케이프, 지운 그림 제외, 파일 모드, `ExportNode`

- [x] **3.2 폴더/Git 동기화**
  - `export.WriteImageDir`: 같은 이름·같은 크기 파일은 다시 쓰지 않고, 안 쓰는 파일과 빈 폴더는 지운다. `foldersync`·`gitsync`가 작품마다 호출한다
  - MAS(스테이징): `StageResult.image_dirs`와 `<폴더>.images/<파일>` 이름을 Rust `folder_sync.rs`가 받아 복사하고(`valid_staged_name`로 경로 밖 이름 거부), `prune_image_dirs`로 안 쓰는 그림을 지운다
  - 테스트: `export`(`TestWriteImageDir`), `foldersync/illustrations_test.go`, `gitsync/illustrations_test.go`, Rust `folder_sync` 테스트 3개

- [x] **3.3 Markdown 가져오기**
  - `importmd/illustrations.go`: `ParseDocument`가 data URI 정의를 본문에서 떼어 `Document.Images`로, 한 줄짜리 `![캡션][illustration-<id>]`를 `illustration` 블록으로 읽는다
  - `RestoreIllustrations`가 쓰인 그림만 새 작품 갤러리에 넣고(`provider`는 `import`) 씬의 id를 새 id로 바꾼다
  - 정의가 없는 그림(`import.illustrations_missing`)이나 갤러리가 거부한 그림(`import.illustrations_rejected`)은 `[[illustration:<id>|캡션]]` 줄로 남기고 경고한다. 데스크톱 `importWarnings.ts`와 ko/en/ja 문구에 반영
  - 테스트: `importmd/illustrations_test.go`, `rpc/handlers/illustrations_roundtrip_test.go` — 내보내기 → 가져오기 왕복에서 바이트·위치·캡션 유지, 복원 못 한 그림의 경고와 표시 줄

## Publish tasks (Linetta + Linetta Works) — 미룸

_별도 linetta-works 계획으로 옮긴다. 아래는 그때 참고할 초안이다._

- [ ] **3.4 Works API 합의 (linetta-works 저장소)**
  - 회차 이미지 업로드 엔드포인트를 정한다. 예: `PUT works/{project_id}/episodes/{node_id}/images/{illustration_id}` (바이트, MIME)
  - 응답은 공개 URL. 같은 id 재업로드는 덮어쓰기, 회차 게시 취소 시 이미지도 비공개
  - Works 쪽 상한(장당 크기, 회차당 장수)을 Linetta 상한(5 MiB)과 맞춘다
  - Works 저장소의 계획 문서와 PR로 진행하고, 여기에는 합의한 계약만 적는다

- [ ] **3.5 Linetta 게시에 이미지 싣기**
  - `publish.Client`에 이미지 업로드 메서드, `Service`가 회차 게시 전에 쓰인 삽화를 올리고 `body_md`의 이미지 링크를 받은 URL로 쓴다
  - `ContentHash`에 삽화 id와 바이트 해시를 넣어 이미지만 바뀌어도 다시 게시되게 한다
  - Phase 2의 "삽화 N장은 아직 게시되지 않습니다" 안내를 지우고, 게시 미리보기에 올라갈 삽화 수를 보인다
  - 게시 동의 문장(`settings.publish.consent`)에 "회차에 넣은 삽화도 함께 전송된다"를 더한다
  - 테스트: `fake_site_test.go`에 업로드 경로를 더해 업로드 순서, 실패 시 회차 게시 중단, 재게시 시 바뀐 이미지만 업로드

---

### ✅ Checkpoint: Phase 3 완료 확인

**구현 확인:**
- [ ] 내보낸 Markdown을 다른 편집기로 열면 삽화가 보인다 (data URI 참조형을 그리는 편집기 기준, 수동 확인 필요)
- [x] 동기화를 두 번 해도 이미지 파일이 다시 쓰이지 않는다 (테스트로 확인)
- [ ] (미룸) 게시한 회차를 Works에서 열면 삽화가 본문 위치에 보인다

**실행 확인:**
- [ ] `cd engine && CGO_ENABLED=0 go test ./...` 통과, `make test-desktop` 통과
- [ ] (미룸) `linetta-works`: `make test` 통과

**수동 확인:**
- [ ] 삽화가 있는 작품을 Markdown으로 내보내고 새 작품으로 가져오면 같은 위치에 삽화가 있다
- [ ] (미룸) 로컬 Works(`make dev`)에 회차를 게시하고, 삽화 하나를 다시 생성해 바꾼 뒤 재게시하면 독자 화면에 바뀐 그림이 보인다

**이 체크포인트를 통과하면 사용자에게 확인을 받고 로드맵의 Now/Next를 완료로 표시한다.**
