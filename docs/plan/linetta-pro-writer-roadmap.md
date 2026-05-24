# Linetta Pro Writer Roadmap

## 제품 정의

Linetta는 장편 웹소설 작가가 여러 작품을 동시에 관리하면서, 세계관/캐릭터/연표/복선의 일관성을 유지하고, Tessera 에이전트 팀을 뮤즈/비평가/팩트체커/편집자로 사용해 출판 가능한 수준까지 원고를 다듬는 Mac-native 창작 스튜디오다.

사람은 소재, 주제, 상황, 독창적 아이디어, 에피소드 얼개를 제공한다. AI는 그 얼개를 구체화하고 강화하며, 리뷰/비평/팩트체크/일관성 검사를 수행한다. 최종 창작 결정과 Canon memory 반영은 사람이 승인한다.

## 핵심 가치

- 여러 작품을 안전하게 분리해서 관리한다.
- 장편 연재 중에도 캐릭터, 세계관, 연표, 복선, 문체가 흔들리지 않는다.
- AI가 작가의 창작 주도권을 빼앗지 않고, 작업을 확장/비평/검증하는 협업자로 작동한다.
- Tessera run event를 통해 어떤 에이전트가 어떤 근거로 어떤 제안을 했는지 추적할 수 있다.

## 최종 아키텍처 방향

- `Go/Tessera engine`: 작품 데이터, Canon memory, Tessera 에이전트 실행, run event stream, 설정 파일 관리.
- `SwiftUI macOS app`: 작품 갤러리, 원고 편집, 협업 패널, Canon diff 승인, 설정 UI.
- `SQLite`: 로컬 작품 저장소. 작품별 데이터를 분리하되 하나의 앱 라이브러리에서 검색/관리한다.
- `HTTP + SSE`: SwiftUI 앱과 Go engine 간 통신. SwiftUI는 AI provider나 DB를 직접 호출하지 않는다.
- `Tessera config`: 작품별 또는 앱 기본 `tessera.yaml`을 지원한다.

## MVP 범위

1. 작품 갤러리와 새 작품 생성
2. 작품별 Canon memory 저장
3. 에피소드 작업대에서 사람이 얼개를 입력하고 Tessera 에이전트 실행
4. 실행 이벤트와 산출물 확인
5. Canon 충돌/변경 제안을 사람이 승인하는 흐름

## Out of Scope

- 클라우드 동기화
- 협업/공동 편집
- 모바일 앱
- 결제/계정/구독
- 웹 배포판
- 완전 자동 연재 업로드
- 상용 출판 포맷 전체 지원
- 벡터 검색/임베딩 기반 semantic memory 고도화

벡터 검색과 자동 출판은 나중에 강력한 기능이 될 수 있지만, MVP에서는 Canon memory의 구조와 승인 흐름을 먼저 완성한다.

## 주요 사용자 저니

```text
[앱 실행]
  -> [작품 갤러리]
  -> [새 작품 생성]
  -> [작품 작업실 열기]
  -> [캐릭터/세계관/연표 Canon 입력]
  -> [에피소드 작업대에서 얼개 작성]
  -> [Tessera 에이전트 실행]
  -> [초안/비평/팩트체크/일관성 결과 검토]
  -> [원고 일부 채택]
  -> [Canon diff 승인]
  -> [다음 에피소드로 진행]
```

## 핵심 화면

| 화면 | 목적 | 핵심 요소 |
|---|---|---|
| 작품 갤러리 | 여러 작품을 만들고 다시 여는 첫 화면 | 작품 카드, 새 작품 버튼, 최근 작업, 상태 필터 |
| 새 작품 생성 | 작품의 기본 방향과 저장소 생성 | 제목, 장르, 연재 플랫폼 메모, 기본 톤, 저장 위치 |
| 작품 작업실 | 선택한 작품의 메인 화면 | 에피소드 목록, Canon memory, 원고 편집기, AI 패널 |
| Canon Memory | 작품 전체의 뼈대 관리 | 캐릭터, 세계관, 연표, 복선, 스타일, 자료 |
| Episode Workbench | 한 화를 기획/작성/검토 | 사람의 얼개 입력, AI 실행 버튼, 산출물 비교, 원고 채택 |
| Run Timeline | Tessera 실행 과정 추적 | 에이전트 상태, 이벤트, 실패/재시도, 산출물 링크 |
| Memory Diff | AI가 제안한 Canon 변경 승인 | 추가/수정/충돌/보류, 승인/거절 |
| Settings | Tessera/모델/저장소 설정 | config 경로, engine 상태, provider 설정 |

## Phase 구성

### Phase 1: 작품 갤러리와 로컬 엔진

목표: 여러 작품을 만들고 열 수 있는 Mac 앱 shell과 Go local engine을 만든다.

완료 신호:
- 앱을 열면 작품 갤러리가 보인다.
- 새 작품을 만들면 SQLite에 저장된다.
- 작품을 열면 빈 작업실로 들어간다.
- Go engine의 `/health`, `/api/works`가 동작한다.

### Phase 2: Canon Memory Core

목표: 작품별 Canon memory를 저장하고 편집할 수 있게 한다.

완료 신호:
- 캐릭터/세계관/연표/복선/스타일/자료를 작품별로 저장한다.
- 메모리 변경은 `Decision` 기록을 남긴다.
- SwiftUI에서 Canon memory를 읽고 수정한다.

### Phase 3: Episode Workbench와 Tessera 협업 실행

목표: 사람이 제공한 에피소드 얼개를 Tessera 에이전트 팀이 확장/초안/비평하도록 한다.

완료 신호:
- 에피소드 작업대에서 얼개를 입력한다.
- Tessera run이 시작되고 SSE로 이벤트가 표시된다.
- 산출물은 에피소드에 연결되어 저장된다.

### Phase 4: 일관성 검수와 Canon Diff 승인

목표: AI 산출물이 기존 Canon과 충돌하는지 검사하고, 새 설정은 사람이 승인하게 한다.

완료 신호:
- AI가 제안한 Canon 변경이 diff로 표시된다.
- 충돌 경고와 근거가 보인다.
- 승인된 변경만 Canon memory에 반영된다.

### Phase 5: 연재/출판 작업 흐름과 프로 사용성

목표: 실제 연재 작가가 계속 쓸 수 있는 편집/버전/내보내기/품질 개선 흐름을 완성한다.

완료 신호:
- 에피소드 상태와 버전 히스토리를 관리한다.
- Markdown/TXT 내보내기를 지원한다.
- 작업실 단축키, 메뉴, 설정, 백업 UX가 정리된다.

## 데이터 모델 초안

```text
works
episodes
episode_versions
canon_items
canon_decisions
characters
world_facts
timeline_events
plot_threads
style_rules
sources
agent_runs
agent_run_events
artifacts
canon_change_proposals
```

원칙:
- `works`가 모든 도메인 데이터의 최상위 소유자다.
- 모든 메모리/에피소드/실행 결과는 반드시 `work_id`를 가진다.
- Canon 변경은 `canon_change_proposals`를 거쳐 승인되어야 한다.
- 원고 버전은 덮어쓰지 않고 `episode_versions`로 남긴다.

## 기술 결정

- Go package 방향:
  - `internal/store`: SQLite 연결, migration, transaction helper
  - `internal/work`: 작품/에피소드 도메인
  - `internal/memory`: Canon memory 도메인
  - `internal/agent`: Tessera run orchestration
  - `internal/server`: HTTP/SSE API
- CLI 방향:
  - 기존 `cmd/linetta` 유지
  - `cmd/linetta serve` 추가
  - 기존 one-shot novel generation은 smoke/debug 용도로 유지
- macOS app 방향:
  - `macos/Linetta` 아래 SwiftUI app
  - SwiftUI는 API client만 가진다.
  - 긴 원고 편집기는 SwiftUI + AppKit `NSTextView` wrapper로 구현한다.

## 공통 검증 명령

```sh
git status --short --branch
go test ./...
go run ./cmd/linetta --config examples/tessera.yaml --goal "Draft a smoke test" --title "Smoke"
```

SwiftUI phase 이후:

```sh
xcodebuild -project macos/Linetta/Linetta.xcodeproj -scheme Linetta -destination 'platform=macOS' test
```

## 진행 원칙

- phase마다 최소 하나의 사용자-visible vertical slice를 만든다.
- 문서와 테스트를 먼저 업데이트하고 구현한다.
- 기능 단위로 commit한다.
- 각 checkpoint마다 사용자에게 확인받고 다음 phase로 넘어간다.
