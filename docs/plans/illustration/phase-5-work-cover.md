# 삽화 Phase 5 — 작품 대표 이미지와 썸네일

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_목표: 작품마다 대표 이미지(표지)를 정하고, 거기서 잘라 만든 썸네일을 서재와 Works 표지에 쓰는 수직 슬라이스_
_예상 소요: 2~3일_
_전제: Phase 4 완료. linetta-artworks Phase 8(세계관·인물 시트와 표지)의 사이트 쪽(작업 그룹 A~C) 완료_

## 결정 (2026-10-25 사용자)

- **썸네일은 대표 이미지에서 자른다**: 썸네일용 그림을 따로 생성하지 않는다. 작가가 대표 이미지 위에서 자를 영역을 고른다.
- **작품 대표 이미지는 Works 표지와 같은 것이다**: linetta-artworks Phase 8은 앱이 "삽화 갤러리에서 고른 그림"을 표지로 올리게 정했다(`PUT …/cover?illustration_id=`, 작업 그룹 D에서 `publish_works.cover_illustration_id`). 둘을 따로 두면 작가가 같은 일을 두 번 한다. 그래서 대표 이미지는 작품(`projects`)에 두고, Works 게시는 그것을 표지로 올린다.
  - Phase 8 작업 그룹 D가 먼저 진행되면, 그쪽이 `projects.cover_illustration_id`를 쓰도록 맞춘다(두 계획 중 먼저 하는 쪽이 필드를 만든다).

## Goal

- 작가가 갤러리나 작품 설정에서 삽화 하나를 작품 대표 이미지로 정한다. 에이전트는 `linetta_request_illustration`의 목적 `cover`로 대표 이미지를 요청할 수 있다(Phase 4의 승인 카드를 그대로 탄다).
- 작가가 대표 이미지에서 썸네일 영역(정사각형과 세로 3:4)을 고른다. 고르지 않으면 가운데를 자른다.
- 서재(작품 목록)에 썸네일이 보인다. Works 게시는 대표 이미지를 표지로 올린다.

## Non-goals

- 썸네일 따로 생성, 여러 크기 묶음(파비콘·SNS 카드 크기 등)
- 대표 이미지 없이 글자로 만든 기본 표지 디자인
- 이미지 편집(필터, 글자 얹기)

## Touch points

- `engine/internal/store/migrations/00xx_project_cover.sql`(새)
- `engine/internal/project/` (`project.go`, `repo.go`), `engine/internal/illustration/` (자르기, `InUseError`)
- `engine/internal/rpc/handlers/projects.go`, `illustrations.go`
- `engine/internal/publish/` (표지 올리기. linetta-artworks Phase 8 작업 그룹 D와 함께)
- `engine/internal/mcphost/` (`linetta_request_illustration`의 목적 `cover`, `linetta_list_illustrations`의 `cover_of`)
- `apps/desktop/src/components/` 서재 카드, 작품 설정, 갤러리 상세, `CoverCropDialog.tsx`(새)

## Tasks

- [ ] **5.1 작품 대표 이미지**
  - `projects.cover_illustration_id`(삽화가 지워지면 NULL)와 `projects.cover_crop_json`(`{"square": {x,y,w,h}, "portrait": {x,y,w,h}}`, 0~1 비율 좌표)
  - 대표 이미지로 쓰는 삽화는 지울 수 없게 막는다(Phase 4의 대표 삽화와 같은 방식)
  - RPC: `projects.set_cover`(삽화 id 또는 null), `projects.set_cover_crop`
  - `linetta_request_illustration`의 목적에 `cover`를 더한다(대상 노드 없이 작품). 승인해서 그리면 작품 대표 이미지가 된다. 비율 기본값은 세로
  - `linetta_set_representative_illustration`에 작품 대상을 더하거나 같은 모양의 작품 도구를 둔다(예산을 보고 정한다)
  - 테스트: 지정·해제, 삭제 막기, 다른 작품 삽화 거절, 요청 목적 `cover`

- [ ] **5.2 썸네일 자르기**
  - `illustration.Thumbnail(id, crop, size)`: Go `image`로 영역을 잘라 줄여 JPEG로 만든다. 저장하지 않고 요청 때 만들고 메모리에 잠깐 둔다(대표 이미지와 자르기 값의 해시로 캐시)
  - 썸네일 크기: 정사각형 256px, 세로 3:4 240×320px (서재 카드 기준. Works는 사이트가 원본에서 따로 줄인다)
  - RPC: `projects.cover_thumbnail(project_id, shape)` → 바이트(base64)와 MIME
  - `CoverCropDialog`: 대표 이미지 위에서 정사각형·세로 영역을 끌어 고른다. 고르지 않으면 가운데
  - WebP 원본은 Go 표준 라이브러리로 디코드할 수 없으므로 `golang.org/x/image/webp`로 읽는다(의존성 추가). 인코딩은 JPEG
  - 테스트: 자르기 좌표 경계, 기본 가운데, WebP·PNG·JPEG 입력, 캐시 무효화

- [ ] **5.3 서재와 Works**
  - 서재(작품 목록) 카드에 세로 썸네일. 없으면 지금 모양 그대로
  - Works 게시: 작품을 등록·갱신할 때 대표 이미지를 `PUT …/cover?illustration_id=`로 올리고, 대표 이미지를 떼면 `DELETE …/cover`. linetta-artworks Phase 8 작업 그룹 D와 같은 일이므로, 그 작업에서 하거나 이 단계에서 하되 한쪽에서만 한다
  - 게시 창에 "표지: 작품 대표 이미지" 줄과 표지 검사 상태
  - 세 언어 문구
  - 테스트: 서재 카드, 게시 시 표지 올리기(가짜 사이트), 표지 변경·삭제

---

### ✅ Checkpoint: Phase 5 완료 확인

**자동 확인:**
- [ ] 엔진 전체, `-tags mobile`, 의존성 검사, `make test-desktop`

**수동 확인 (사용자):**
- [ ] 갤러리에서 삽화를 작품 대표 이미지로 정하면 서재 카드에 세로 썸네일이 보인다
- [ ] 썸네일 영역을 바꾸면 카드가 바로 바뀐다
- [ ] 에이전트에게 "표지를 그려 줘"라고 하면 승인 카드가 뜨고, 승인하면 대표 이미지가 된다
- [ ] 작품을 Works에 게시하면 대표 이미지가 표지로 올라가고, 검사를 통과하면 작품 페이지와 링크 미리보기에 보인다

**이 체크포인트를 통과하면 사용자에게 확인을 받고 로드맵에 완료를 표시한다.**
