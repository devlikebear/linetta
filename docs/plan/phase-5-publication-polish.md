# Phase 5: 연재/출판 작업 흐름과 프로 사용성

## 목표

Linetta를 매일 쓰는 작업 도구로 다듬는다. 이 phase는 출판 가능한 수준의 원고 작업을 돕기 위한 편집, 버전, 내보내기, 설정, 백업, 사용성 개선을 포함한다.

## 범위

- episode status workflow
- 원고 버전 히스토리
- Markdown/TXT 내보내기
- 작업 통계
- 앱 설정
- 백업/복구 최소 기능
- Mac-native 사용성 polish

## 작업 목록

### 1. 에피소드 상태 관리

- [x] `episodes.status` 추가
  - idea
  - outlined
  - drafting
  - reviewing
  - ready
  - published
- [x] API 추가
  - `PATCH /api/works/{workID}/episodes/{episodeID}/status`
- [x] SwiftUI episode list에 상태 표시
- [x] 갤러리 카드에 최근 에피소드 상태 요약 표시

검증:

```sh
go test ./internal/work/... ./internal/server/...
```

### 2. 원고 버전 히스토리

- [x] `episode_versions` 구현
  - id
  - work_id
  - episode_id
  - source_artifact_id
  - body
  - note
  - created_at
- [x] 현재 원고 저장 시 version 생성
- [x] artifact 채택 시 version 생성
- [x] SwiftUI에서 version list와 restore 액션 제공
- [x] 덮어쓰기 전에 항상 version을 남긴다.

검증:

```sh
go test ./internal/work/... ./internal/server/...
```

수동 확인:

- [ ] AI draft를 채택하면 새 version이 생긴다.
- [ ] 사람이 수정 후 저장하면 새 version이 생긴다.
- [ ] 이전 version으로 복구할 수 있다.

### 3. 내보내기

- [x] Markdown export
  - 작품 전체
  - 특정 에피소드
  - Canon memory 요약
- [x] TXT export
  - 특정 에피소드 원고
- [x] API 추가
  - `GET /api/works/{workID}/export/markdown`
  - `GET /api/works/{workID}/episodes/{episodeID}/export/txt`
- [x] SwiftUI export sheet 추가

검증:

```sh
go test ./internal/server/...
```

수동 확인:

- [ ] 에피소드 원고를 TXT로 내보낼 수 있다.
- [ ] 작품 설정집을 Markdown으로 내보낼 수 있다.

### 4. 작업 통계와 갤러리 polish

- [x] 작품 갤러리에 표시할 통계 API 추가
  - episode count
  - ready count
  - word count
  - open continuity issue count
  - pending Canon proposal count
- [x] 작품 카드에 진행 상태 표시
- [x] 최근 작업 순 정렬
- [x] 검색/필터 추가
  - title
  - genre
  - status

검증:

```sh
go test ./internal/work/... ./internal/server/...
```

### 5. Settings와 Tessera config 관리

- [x] SwiftUI Settings 화면 추가
  - engine address
  - default DB path
  - default Tessera config path
  - provider config status
- [x] `examples/tessera.yaml` 기반으로 앱 기본 config 생성 기능 추가
- [x] 설정 화면에서 config path를 바꾸고 저장할 수 있게 한다.
- [x] provider secret은 config 파일에 직접 저장하지 않고 env/keychain 전략을 별도 phase 후보로 남긴다.

검증:

```sh
go test ./...
xcodebuild -project macos/Linetta/Linetta.xcodeproj -scheme Linetta -destination 'platform=macOS' test
```

### 6. 백업/복구 최소 기능

- [x] `linetta export-library --db ... --out backup.zip` CLI 추가
- [x] backup에는 SQLite DB와 config snapshot을 포함한다.
- [x] `linetta import-library --in backup.zip --db ...` CLI 추가
- [x] import는 기존 DB를 덮어쓰기 전에 확인 또는 별도 경로를 요구한다.
- [x] 테스트 추가
  - export 후 import하면 works/memory/episodes가 복원되는지

검증:

```sh
go test ./...
```

### 7. Mac-native polish

- [ ] 앱 메뉴 정리
  - New Work
  - Open Work
  - Export
  - Settings
- [x] 주요 단축키
  - 새 작품
  - 새 에피소드
  - 저장
  - AI run 시작
  - Canon search
- [ ] 긴 원고 편집기 개선
  - `NSTextView` wrapper 도입 또는 검증
  - undo/redo
  - find
  - word count
- [x] 빈 상태/에러 상태 다듬기
  - engine offline
  - no works
  - no memory
  - failed run

수동 확인:

- [ ] 키보드만으로 기본 작업 흐름을 수행할 수 있다.
- [ ] engine이 꺼져 있을 때 앱이 명확한 복구 액션을 보여준다.
- [ ] 긴 원고 입력 중 데이터 유실 위험이 없다.

---

### Checkpoint: Phase 5 완료 확인

**구현 확인:**
- [x] 작품 갤러리, 작업실, Canon memory, 에피소드 협업, 내보내기가 하나의 흐름으로 연결된다.
- [x] 원고 version history와 backup/export가 동작한다.
- [x] Settings에서 Tessera config를 관리할 수 있다.

**실행 확인:**
- [x] `go test ./...` 통과
- [ ] `xcodebuild ... test` 통과
- [x] `linetta export-library` 후 `linetta import-library` smoke 통과

메모: 현재 로컬 Xcode 도구가 `IDESimulatorFoundation` 심볼 로딩 문제로 `xcodebuild` 실행 단계에서 막힌다. Swift Package 기준 `swift test`는 통과한다.

**사용자 확인:**
- [ ] 실제 장편 웹소설 프로젝트 하나를 만들어 1화 기획부터 Canon 반영까지 끝까지 진행해본다.
- [ ] 이 흐름이 "계속 연재하면서 쓸 수 있겠다"는 수준인지 확인한다.

이 phase가 끝나면 MVP 완성으로 간주하고, 다음 큰 축은 semantic memory, LLM provider integration, cloud sync 중 하나를 선택한다.
