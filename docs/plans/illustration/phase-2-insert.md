# 삽화 Phase 2 — 씬 본문 삽입

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_목표: 갤러리의 삽화를 씬 본문에 넣고, 본문을 읽고 고치는 모든 경로에서 삽화가 사라지거나 깨지지 않게 하는 수직 슬라이스_
_예상 소요: 3~4일_
_전제: Phase 1 완료 (`illustrations` 테이블, 갤러리, `illustrations.get`)_
_상태: 완료 (2026-10-24)_

## Goal

작가는 갤러리나 생성 결과에서 `씬에 넣기`를 눌러 커서 위치에 삽화를 넣는다. 본문에는 삽화 id만 저장된다.
저장, 스냅샷 복원, 찾기/바꾸기, 맥락 편집, 에이전트의 씬 다시 쓰기, 병합 복원을 거쳐도 삽화가 그 자리에 남는다.
글자 수와 검색은 삽화를 세지 않는다.

Markdown 내보내기와 Works 게시는 이 단계에서 삽화를 싣지 않는다. 대신 빠진다는 것을 작가에게 알린다(Phase 3에서 싣는다).

## Non-goals

- Markdown 내보내기·폴더/Git 동기화·게시에 이미지 파일 싣기 (Phase 3)
- 작가가 가진 이미지 파일을 직접 본문에 넣기 (Later. 갤러리에 "파일에서 추가"를 두는 방식이 자연스럽다)
- 이미지 크기 조절, 정렬, 여러 장 나란히 배치

## 노드 설계

```json
{"type": "illustration", "attrs": {"id": "<illustration id>", "caption": "<본문용 캡션>"}}
```

- 블록 수준 atom 노드. 텍스트 자식이 없다.
- `caption`은 넣을 때 갤러리 캡션으로 채우고, 본문에서 따로 고칠 수 있다(갤러리 캡션은 바뀌지 않는다).
- id가 가리키는 삽화가 없으면 "삭제된 삽화" 자리표시로 그린다. 문서는 그대로 둔다.

## Touch points

- `apps/desktop/src/components/editor/IllustrationExtension.ts` 신규, `IllustrationNodeView.tsx` 신규
- `apps/desktop/src/components/editor/Tiptap.tsx`, `apps/desktop/src/routes/Workspace.tsx` (확장 등록, 넣기 명령)
- `apps/desktop/src/components/editor/editorSchema.test.ts`, `engine/internal/export/testdata/editor_schema.json`
- `apps/desktop/src/components/illustration/IllustrationGallery.tsx`, `IllustrationDialog.tsx` (`씬에 넣기`)
- `apps/desktop/src/hooks/useIllustrationImage.ts` 신규 (id → object URL 캐시)
- `engine/internal/export/markdown.go`, `text.go`, `episode.go`
- `engine/internal/node/repo.go` (글자 수 계산 경로 확인)
- `engine/internal/manuscript/indexer.go`
- `engine/internal/manuscriptedit/replace.go`, `engine/internal/contextualedit/contextualedit.go`
- `engine/internal/mcphost/tools_read.go`, `tools_write.go` (본문 텍스트 ↔ 문서 변환)
- `engine/internal/illustration/repo.go` (사용처 조회)
- `engine/internal/restore/restore.go`

## Desktop tasks

- [x] **2.1 `illustration` 노드와 NodeView** — Zen 모드 에디터에서도 빠지지 않도록 `Tiptap.tsx`의 기본 확장에 넣었다
  - `IllustrationExtension.ts`: `Node.create({ name: "illustration", group: "block", atom: true, draggable: true })`,
    attrs `id`, `caption`. `NoteMarkerExtension.ts`의 구성 방식을 따른다
  - `IllustrationNodeView.tsx`: `useIllustrationImage(id)`로 그린다. 로딩, 없음, 오류 상태. 캡션은 그 자리에서 고친다
  - `insertIllustration(id, caption)` 명령을 Tiptap imperative handle에 더한다
  - 테스트: `IllustrationExtension.test.ts` — JSON 왕복, 없는 id 자리표시, 캡션 수정이 attrs에 반영

- [x] **2.2 스키마 패리티 갱신**
  - `editor_schema.json`의 `nodes`에 `illustration` 추가, `editorSchema.test.ts`의 확장 목록에 등록
  - 이 변경으로 Go 쪽 패리티 테스트가 실패하는 것이 정상이다. 2.4에서 내보내기가 이 노드를 어떻게 다룰지 정하면 통과한다

- [x] **2.3 넣기 진입점**
  - 갤러리 항목과 생성 결과에 `씬에 넣기`. 열린 씬이 없으면 버튼을 막고 이유를 보인다
  - 갤러리에서 에디터로 끌어 놓기
  - 갤러리 항목에 `사용 중` 표시(어느 씬에 쓰였는지 목록, 클릭하면 이동)

## Engine tasks

- [x] **2.4 내보내기·평문·글자 수·검색**
  - `export/markdown.go` `renderBlock`에 `case "illustration"`: 이 단계에서는 아무것도 내보내지 않는다(`nil`).
    Phase 3에서 이미지 링크로 바꾼다
  - `export/text.go`: 평문에서 빠진다(현재 기본 분기가 자식 없는 노드를 버리므로 테스트로 고정만 한다)
  - `PublishDocToMarkdown`: 빠진 삽화 수를 돌려주도록 해 게시 화면이 "삽화 N장은 아직 게시되지 않습니다"를 보이게 한다
  - 글자 수(`node.Repo.UpdateContent`의 계산 경로)와 `manuscript/indexer.go`가 삽화를 세지 않음을 테스트로 고정
  - 테스트: `markdown_test.go`, `text_test.go`, `episode_test.go`, 스키마 패리티 테스트 통과

- [x] **2.5 찾기/바꾸기·맥락 편집이 삽화를 보존**
  - `manuscriptedit/replace.go`, `contextualedit`은 텍스트 노드만 고친다. 삽화가 있는 문서로 일괄 바꾸기 후 노드가 그대로인지 테스트

- [x] **2.6 에이전트 읽기/쓰기 왕복** — 자리표시·id 다루기는 `engine/internal/illustref` 한 곳에 있다. story-ops `set_scene_text`도 같은 길을 탄다
  - 문제: `linetta_write_scene`은 씬 본문을 통째로 바꾼다. 삽화를 모르는 에이전트가 다시 쓰면 삽화가 사라진다
  - `tools_read.go`의 씬 본문 텍스트에 삽화를 한 줄 자리표시로 넣는다: `[[illustration:<id>|<caption>]]`
  - `tools_write.go`의 텍스트 → 문서 변환에서 그 줄을 `illustration` 노드로 되살린다. 모르는 id는 일반 텍스트로 둔다
  - 툴 설명에 "자리표시 줄은 그대로 두면 삽화가 유지되고, 지우면 본문에서 빠진다(갤러리에는 남는다)"를 더한다
  - 내장 에이전트도 같은 툴 경로를 쓰는지 확인하고, 따로 쓰면 같은 변환 함수를 쓰게 한다
  - 테스트: 읽기 → 그대로 쓰기 왕복에서 문서 동일, 자리표시 삭제 시 노드 제거, 가짜 id는 텍스트

- [x] **2.7 사용처 조회와 삭제 보호**
  - `illustration.Repo.UsedIn(ctx, projectID, id) ([]NodeRef, error)`: 작품 씬들의 `content_doc`을 `LIKE '%"illustration"%'`로 거른 뒤 JSON으로 확인
  - `illustrations.list`가 항목마다 `used_in`을 돌려준다
  - `illustrations.delete`는 사용 중이면 새 reason `rpc.ReasonIllustrationInUse`와 씬 목록으로 거절. 스냅샷 안의 참조는 막지 않는다(복원하면 자리표시로 보인다)
  - 테스트: 사용 중 삭제 거절, 본문에서 뺀 뒤 삭제 성공

- [x] **2.8 병합 복원 시 본문 안 id 다시 쓰기**
  - Phase 1의 삽화 id 재매핑 표로, 복원한 노드의 `content_doc`과 스냅샷 문서 안 `illustration` attrs의 `id`를 바꾼다
  - 테스트: 삽화를 넣은 씬이 있는 작품을 병합 복원하면 새 작품의 씬이 새 삽화 id를 가리키고 이미지가 열린다

---

### ✅ Checkpoint: Phase 2 완료 확인

**구현 확인:**
- [x] 씬에 넣은 삽화가 저장 후 다시 열어도 보이고, `content_doc`에는 id와 캡션만 있다
- [x] 글자 수, 원고 검색, 평문 내보내기에 삽화가 끼지 않는다
- [x] 에이전트가 씬을 읽고 그대로 다시 쓰면 삽화가 남는다

**실행 확인:**
- [x] `cd engine && CGO_ENABLED=0 go test ./...` 통과 (스키마 패리티 포함), `make test-mobile-engine` 통과
- [x] `make test-desktop` 통과

**수동 확인:**
- [ ] 삽화를 넣은 씬에서 스냅샷을 복원하고, 작품 전체 바꾸기를 하고, 에이전트에게 문장 다듬기를 시켜도 삽화가 자리에 있다 — 앱에서 손으로는 하지 않았다. 바꾸기(`manuscriptedit`)와 에이전트 왕복(`engineapp` MCP 통합 테스트)은 테스트로 확인. 스냅샷 복원은 저장된 문서를 그대로 되돌리므로 따로 시험하지 않았다
- [x] 사용 중인 삽화는 갤러리에서 지울 수 없고, 쓰인 씬을 보여 준다 (임시 `LINETTA_HOME`의 웹 UI로 확인). 그 씬으로 이동하는 링크는 만들지 않았다
- [x] 게시 화면에 "삽화 N장은 아직 게시되지 않습니다"가 보인다 — `PublishDialog` 테스트로 확인, 실제 Works 연결로는 보지 않았다

**이 체크포인트를 통과하면 사용자에게 확인 요청 후 Phase 3로 진행.**
실패하면 실패 항목을 보고하고, 사용자와 원인을 파악해 고친 뒤 다시 확인한다.
