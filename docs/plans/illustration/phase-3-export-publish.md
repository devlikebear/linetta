# 삽화 Phase 3 — 내보내기·동기화·게시

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_목표: 본문에 넣은 삽화가 Markdown 내보내기, 폴더/Git 동기화, Linetta Works 게시까지 따라가게 하는 수직 슬라이스_
_예상 소요: Linetta 2~3일 + Linetta Works 2일_
_전제: Phase 2 완료. Works 쪽은 `linetta-works` 저장소에 별도 계획과 PR이 필요하다_

## Goal

- Markdown으로 내보내거나 폴더/Git으로 동기화하면 작품 파일 옆 이미지 폴더에 삽화가 파일로 나가고, 본문은 상대 경로로 가리킨다.
- Markdown 가져오기는 그 상대 경로 이미지를 갤러리에 넣고 `illustration` 노드로 되살린다.
- Works에 회차를 게시하면 그 회차에 쓰인 삽화가 함께 올라가고, 독자 화면에 보인다.

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

- [ ] **3.1 내보내기에 이미지 싣기**
  - `DocToMarkdown`에 이미지 해석기를 넘기는 옵션을 더한다: `illustration` 노드 → `![caption](<dir>/<id>.<ext>)`
  - `ExportProject`·`ExportNode`가 쓰인 삽화 목록을 `Payload`에 함께 돌려주고, 저장하는 쪽이 파일로 쓴다
  - 이미지 폴더 이름은 `SyncFilename` 기반으로 정해 작품끼리 겹치지 않게 한다 (예: `<작품 파일 이름>.images/`)
  - 해석기를 넘기지 않는 호출(클립보드 복사 등)은 Phase 2처럼 삽화를 뺀다
  - 테스트: `markdown_test.go`, `project_test.go` — 링크 형식, 쓰인 삽화만 나감, 캡션의 `]` 이스케이프

- [ ] **3.2 폴더/Git 동기화**
  - `foldersync.exportAll`이 이미지 폴더를 같이 쓰고, 더는 안 쓰이는 이미지 파일을 지운다
  - 이미지 파일은 id가 같으면 다시 쓰지 않는다(Git diff가 매번 생기지 않게)
  - 테스트: 두 번 동기화해도 이미지 파일이 바뀌지 않음, 본문에서 뺀 삽화의 파일이 지워짐

- [ ] **3.3 Markdown 가져오기**
  - `importmd`가 상대 경로 이미지 링크를 찾아 파일을 읽고 갤러리에 추가(`provider`는 `import`, 프롬프트는 빈 값)한 뒤 노드로 바꾼다
  - 파일이 없거나 상한을 넘으면 링크를 텍스트로 남기고 가져오기 결과에 경고를 더한다
  - 테스트: 내보내기 → 가져오기 왕복에서 삽화 수와 위치가 같음

## Publish tasks (Linetta + Linetta Works)

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
- [ ] 내보낸 Markdown을 다른 편집기로 열면 삽화가 보인다
- [ ] 동기화를 두 번 해도 Git에 이미지 변경이 생기지 않는다
- [ ] 게시한 회차를 Works에서 열면 삽화가 본문 위치에 보인다

**실행 확인:**
- [ ] `cd engine && CGO_ENABLED=0 go test ./...` 통과, `make test-desktop` 통과
- [ ] `linetta-works`: `make test` 통과

**수동 확인:**
- [ ] 삽화가 있는 작품을 Markdown으로 내보내고 새 작품으로 가져오면 같은 위치에 삽화가 있다
- [ ] 로컬 Works(`make dev`)에 회차를 게시하고, 삽화 하나를 다시 생성해 바꾼 뒤 재게시하면 독자 화면에 바뀐 그림이 보인다

**이 체크포인트를 통과하면 사용자에게 확인을 받고 로드맵의 Now/Next를 완료로 표시한다.**
