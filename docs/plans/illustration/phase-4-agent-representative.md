# 삽화 Phase 4 — 에이전트 삽화 도구와 대표 삽화

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_목표: 에이전트(내장 에이전트와 외부 MCP 클라이언트)가 삽화를 요청하고, 작가가 카드에서 승인하면 그려지며, 씬과 화(또는 장)마다 대표 삽화를 둘 수 있는 수직 슬라이스_
_예상 소요: 3~4일_
_전제: Phase 1~3 완료. linetta-artworks Phase 5(회차 삽화 게시) 배포와 앱 v1.12.0 릴리스 완료_

## 배경

- 지금 에이전트는 삽화 **프롬프트**만 만든다(`linetta_build_illustration_prompt`, "Generates no image"). 작가가 "네가 직접 삽화를 그려 달라"고 해도 할 수 없다.
- 그림은 작가가 `IllustrationDialog`에서 버튼을 눌러야만 생긴다(Phase 1 결정 "자동 생성은 없다").
- 씬·화·작품에는 대표 이미지가 없다. 삽화와 노드의 관계는 `illustrations.source_node_id`(어디서 그렸나)와 본문 안의 삽화 노드(`UsedIn`)뿐이다.

## 결정 (2026-10-25 사용자)

- **생성마다 승인 카드**: 에이전트는 그리지 않고 "그려 달라"는 **요청**을 남긴다. 작가가 카드에서 프롬프트를 보고(고칠 수 있다) "생성"을 눌러야 이미지 공급자로 간다.
  - Phase 1의 "자동 생성은 없다"는 그대로 지킨다. 비용과 한도는 작가가 쓴다.
  - 기존 승인 구조(제안함, 문체 초안, 갈래 예측)처럼 도구는 대기 기록만 남기고 바로 돌아온다. 에이전트 턴을 멈추지 않는다.
- **대표 삽화는 노드 하나에 하나**: 씬(leaf)이든 화·장(container)이든 노드에 `representative_illustration_id`를 둔다. 작품 대표 이미지는 Phase 5에서 작품에 둔다.
- **썸네일은 대표 이미지에서 자른다** (Phase 5): 따로 생성하지 않는다.

## Goal

- 에이전트가 `linetta_request_illustration`으로 씬·화를 골라 삽화를 요청하면, 에이전트 패널과 갤러리에 승인 카드가 뜬다.
- 작가가 카드에서 "생성"을 누르면 그려져 갤러리에 들어가고, 요청의 목적에 따라 씬에 넣거나 그 노드의 대표 삽화가 된다.
- 에이전트는 요청과 갤러리의 상태를 읽을 수 있다(승인됐는지, 어떤 그림이 대표인지).
- 작가는 갤러리에서 아무 삽화나 씬·화의 대표로 지정하고 뗄 수 있다. 개요(아웃라인)와 씬 머리에 대표 삽화가 작게 보인다.

## Non-goals

- 승인 없는 생성, 여러 장 뽑아 고르기 (로드맵 Later)
- 에이전트가 그림 바이트를 직접 넣기(외부 이미지 업로드). 참조 이미지 도구(`linetta_add_reference_image`)는 그대로 둔다
- 대표 삽화의 Works 게시 (회차 대표 그림은 사이트에 자리가 없다. 필요하면 linetta-artworks 계획으로)

## Touch points

- `engine/internal/store/migrations/0030_illustration_requests.sql`(새), `0031_node_representative.sql`(새) — 번호는 그때의 main 기준
- `engine/internal/illustration/` (`requests.go` 새, `service.go`, `repo.go`)
- `engine/internal/node/` (`node.go`, `repo.go`): 대표 삽화 필드
- `engine/internal/mcphost/tools_visuals.go`, `tools_visuals_write.go`(또는 `tools_illustrations.go` 새), `budget.go`, `budget_test.go`, `tools.go`(`ToolDeps`)
- `engine/internal/agent/prompt.go`(삽화 요청 안내 한 단락), `apps/desktop/src/lib/agentTools.ts`(도구 이름 대칭)
- `engine/internal/rpc/handlers/illustrations.go`, `engine/internal/engineapp/engineapp.go`, `mcp_enabled.go`, `agent_enabled.go`
- `apps/desktop/src/components/agent/AgentPanel.tsx`, `components/illustration/IllustrationRequestCard.tsx`(새), `IllustrationGallery.tsx`, 개요 트리, `lib/rpc.ts`, `lib/types.ts`, `lib/i18n.tsx`

## Tasks

- [x] **4.1 삽화 요청 저장과 서비스**
  - 구현(2026-10-25): `0031_illustration_requests.sql`, `illustration/requests.go`(`RequestService`), RPC `illustration_requests.list|get|approve|reject`, 알림 `illustrations.changed`(`ChangedEvent`), 사유 코드 3개와 세 언어 문구
  - 계획과 다른 점
    - `negative_prompt`는 두지 않았다. `Generate`는 프롬프트를 그대로 보내고, 대화상자도 조립된 프롬프트 하나만 쓴다
    - 대상 노드 삭제는 트리거 대신, 요청을 읽거나 승인할 때 사라진 노드의 대기·실패 요청을 `cancelled`로 바꾼다(`cancelOrphans`)
    - 목적 `representative`는 표에는 있지만, 채울 노드 필드가 4.3에서 생기므로 그때까지 `Submit`이 거절한다
    - 씬에 넣기는 읽은 버전 위에만 쓰고(`UpdateContentIfVersion`) 충돌하면 세 번까지 다시 시도한다. 넣지 못하면 요청은 `done`이고 `placed_at` 없이 `error`에 사유를 남긴다
    - 승인은 `pending`·`failed` → `generating`을 조건부로 바꿔 두 번 눌러도 한 번만 그린다. 실패한 요청은 다시 승인할 수 있다
  - 새 표 `illustration_requests`:
    - `id`, `project_id`, `target_node_id`(씬 또는 화·장, 노드가 지워지면 요청은 `cancelled`)
    - `purpose`: `insert`(씬에 넣기), `representative`(그 노드의 대표), `gallery`(갤러리에만)
    - `prompt`, `negative_prompt`, `character_ids_json`, `include_references`, `aspect`, `caption`
    - `source`: `agent` | `external`, `note`(에이전트가 작가에게 하는 한 줄 설명)
    - `status`: `pending` | `generating` | `done` | `rejected` | `failed` | `cancelled`, `illustration_id`, `error`, `created_at`, `decided_at`
  - `illustration.RequestService`:
    - `Submit`: 프로젝트·노드·인물 확인. 프로젝트당 대기 요청 20개 상한(넘으면 거절해 에이전트가 쏟아내지 못하게)
    - `Approve(id, edits)`: 작가가 고친 프롬프트·비율·캡션을 반영해 `Service.Generate`를 부른다. 성공하면 목적대로 처리한다
      - `insert`: 씬 끝이 아니라 **요청 때 에이전트가 고른 위치**에 넣을 수 없으므로(에이전트는 본문 위치를 모른다), 씬 본문 끝에 삽화 노드를 덧붙이고 작가가 옮긴다. 화·장 대상이면 `insert`를 받지 않는다
      - `representative`: 그 노드의 대표로 지정(4.3)
    - `Reject`, `Cancel`, `List(projectID, status?)`
  - `illustrations.changed` 알림으로 화면이 다시 읽는다. 생성 중 앱을 끄면 `generating`을 `failed`(사유 "앱이 닫혀 중단됨")로 되돌린다
  - 테스트: 요청·승인·거절, 상한, 노드 삭제 시 취소, 목적별 처리(씬 끝에 넣기, 대표 지정), 생성 실패 시 `failed`와 사유

- [ ] **4.2 MCP·에이전트 도구**
  - `linetta_request_illustration` (쓰기 도구, 기본 켜짐. `DisableVisualWrites`처럼 끌 수 있는 그룹에 넣는다)
    - 입력: `project_id`, `node_id`, `purpose`, `scene`(장면 설명) 또는 `prompt`(완성된 프롬프트), `entity_ids`/`names`, `aspect`, `caption`, `note`
    - `scene`만 주면 `visual.Repo.Compose`로 비주얼 시트와 화풍을 실은 프롬프트를 만든다(`linetta_build_illustration_prompt`와 같은 조립)
    - 출력: `request_id`, `status: "waiting_for_writer"`, 조립된 `prompt`(에이전트가 작가에게 설명할 수 있게)
    - 설명 문구에 "이미지는 작가가 승인해야 생성된다"를 넣는다
  - `linetta_list_illustrations` (읽기 도구)
    - 갤러리: `id`, `caption`, `source_node_id`, `used_in`(씬 id), `representative_of`(노드 id), 크기
    - 요청: `request_id`, `node_id`, `purpose`, `status`, `illustration_id`, `error`
    - 에이전트가 승인 결과를 알 수 있는 첫 경로다(기존 제안함은 결과를 읽는 도구가 없다)
  - `linetta_set_representative_illustration` (쓰기 도구, 되돌리기 가능)
    - 이미 갤러리에 있는 삽화를 노드의 대표로 지정하거나 뗀다. 생성이 없어 비용이 들지 않으므로 승인 카드 없이 바로 하고 `undo_batch_id`를 준다
  - 씬에 기존 삽화를 넣는 것은 지금처럼 `linetta_write_scene`의 `[[illustration:id|caption]]` 줄로 한다. 도구 설명과 에이전트 안내에 이 방법을 적는다
  - 도구 스키마 예산: 새 세 도구에 몫을 따로 주고 `budget_test.go`에 상한을 더한다(시각 읽기 도구 3,500B처럼). `agentTools.ts`의 읽기·쓰기 목록과 `agentToolParity.test.ts`를 맞춘다
  - 에이전트 안내(`prompt.go`): 삽화를 부탁받으면 요청을 남기고 "카드에서 승인해 달라"고 말한다. 승인 전에 됐다고 말하지 않는다
  - 테스트: 도구 입력 검증, `scene`만 준 요청의 프롬프트 조립, 목록 출력, 대표 지정과 되돌리기, 예산

- [ ] **4.3 대표 삽화**
  - `nodes.representative_illustration_id`(삽화가 지워지면 NULL). 어떤 종류의 노드든 하나
  - 대표로 지정된 삽화는 본문에 쓰인 삽화처럼 지울 수 없게 막는다(`InUseError`에 "대표 삽화" 사유를 더함)
  - RPC: `nodes.set_representative_illustration`(노드 id, 삽화 id 또는 null), `illustrations.list`의 `Meta`에 `representative_of`
  - 노드 복원·스냅샷: 대표 지정은 본문이 아니라 노드 속성이므로 스냅샷에 넣지 않는다. 버전 복원으로 바뀌지 않는다
  - 내보내기·동기화: 이번에는 싣지 않는다(Markdown 형식이 바뀐다). 로드맵 Later로
  - 테스트: 지정·해제, 삭제 막기, 다른 작품의 삽화 거절, 노드 삭제 시 함께 사라짐

- [ ] **4.4 승인 카드와 화면**
  - `IllustrationRequestCard`: 대상(씬·화 이름), 목적, 에이전트의 한 줄 설명, 고칠 수 있는 프롬프트, 비율, 참조 인물, 공급자·모델 표시와 비용 안내(Codex 사용량·Gemini 과금), "생성"·"거절"
    - 공급자가 설정되지 않았거나 이미지 생성 동의가 없으면 "생성" 대신 설정으로 보내는 버튼
  - 에이전트 패널: 요청 도구 줄 아래에 그 요청의 카드를 바로 보인다(도구 줄의 `request_id`로 찾음). 생성 중에는 진행 표시, 끝나면 썸네일과 "씬에서 보기"
  - 외부 MCP 클라이언트의 요청은 에이전트 패널에 없으므로 갤러리 위쪽 "요청" 구획과 툴바 배지로 보인다
  - 갤러리 상세: "이 씬의 대표로", "이 화의 대표로", "대표에서 빼기"
  - 개요 트리와 씬 머리에 대표 삽화 작은 그림(썸네일 크기 CSS. 별도 썸네일 파일은 Phase 5)
  - 세 언어 문구
  - 테스트: 카드 승인·거절·편집, 동의 없을 때, 외부 요청 배지, 대표 지정 UI

---

### ✅ Checkpoint: Phase 4 완료 확인

**자동 확인:**
- [ ] `cd engine && CGO_ENABLED=0 go test ./...`(`cmd/linetta-ffi` 제외), `-tags mobile`, `scripts/validate-story-core-deps.sh`
- [ ] `make test-desktop`

**수동 확인 (사용자):**
- [ ] 에이전트에게 "씬 1을 그려 줘"라고 하면 카드가 뜨고, 승인 전에는 아무것도 생성되지 않는다
- [ ] 카드에서 프롬프트를 고쳐 "생성"하면 갤러리에 들어가고, `insert`면 씬 끝에, `representative`면 대표로 들어간다
- [ ] 거절하면 에이전트가 다음 턴에 `linetta_list_illustrations`로 거절을 안다
- [ ] 외부 MCP 클라이언트(Claude Code 등)의 요청도 갤러리 "요청"에서 승인할 수 있다
- [ ] 대표 삽화가 개요와 씬 머리에 보이고, 대표 삽화는 지울 수 없다

**이 체크포인트를 통과하면 사용자에게 확인을 받고 Phase 5로 넘어간다.**

## 메모 / 주의

- **비용**: 요청 상한(대기 20개)과 승인 카드가 비용의 문이다. 카드에 공급자와 과금 안내를 늘 보인다.
- **원고 전송**: 요청 자체는 기기 밖으로 나가지 않는다. 승인해서 생성할 때 Phase 1과 같은 범위(프롬프트, 고른 참조 이미지)만 공급자로 간다.
- **외부 클라이언트**: 외부 MCP 클라이언트도 요청만 남길 수 있다. MCP 모드가 읽기 전용이면 요청 도구는 없다.
