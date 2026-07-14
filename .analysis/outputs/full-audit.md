# Linetta 코드베이스 전면 감사

> 기준 커밋: `1020096a14b65c7bd21ca081947f1747978e3b93`
> 감사일: 2026-07-12
> 범위: 커밋된 파일만, 제품 코드 수정 없음
> 판정: **Blocking** — 사용자 데이터 덮어쓰기 경로 1건과 다음 App Store 업데이트 전에 정리해야 할 개인정보 고지 공백이 있다.

## 0. 검증 요약

- 시작/종료 작업 트리: `main...origin/main`, 제품 변경 없음
- 전체 CI: 통과
- 프론트: 50개 test file, 281 tests 통과; TypeScript/Vite production build 통과
- Go: `go test ./...`, `go test -race ./...`, MAS build-tag suite 통과
- Rust: 기본/MAS feature test 각 3건 통과
- 배포 메타데이터/Plist: `make validate-distribution`, `plutil -lint` 통과
- 의존성 감사:
  - `govulncheck`: 실제 호출 가능 Go 표준 라이브러리 취약점 5건
  - `pnpm audit --prod`: moderate 1건
  - `cargo audit`: 취약점 2건, unsound/unmaintained 경고 별도

기존 테스트 통과는 안전 판정이 아니다. 아래 위험 경로는 현재 suite에 재현 테스트가 없다.

---

# 워크스트림 A — 아키텍처, IPC, SQLite, 로컬 데이터 안전성

## A-1. 디렉터리와 모듈 의존성

```text
React routes/components
  ├─ Tiptap editor state
  └─ apps/desktop/src/lib/rpc.ts
       │ invoke("engine_call")
       ▼
apps/desktop/src-tauri/src/lib.rs
       │ spawn_blocking
       ▼
apps/desktop/src-tauri/src/ffi.rs
       │ JSON-RPC envelope + C ABI
       ▼
engine/cmd/linetta-ffi/ffi.go
       ▼
engine/internal/engineapp.App
  ├─ rpc.Server → rpc/handlers → domain repos/services
  ├─ backup scheduler
  ├─ summarizer
  └─ companion service
       ▼
engine/internal/store.Store → SQLite library.db
```

근거: `apps/desktop/src/App.tsx:35-53`, `apps/desktop/src/lib/rpc.ts:84-106`, `apps/desktop/src-tauri/src/lib.rs:55-64`, `apps/desktop/src-tauri/src/ffi.rs:228-293`, `engine/cmd/linetta-ffi/ffi.go:29-69,125-158`, `engine/internal/engineapp/engineapp.go:82-113,115-211,217-328`.

역방향 알림은 Go notifier → C callback → Rust `notification_event` → Tauri event → `useEngineEvent`/Companion 순이다.

## A-2. IPC 전수 대조

- TypeScript JSON-RPC method: 110
- Go build-variant 합집합: 115
- 프런트에서 호출하지만 Go registry에 없는 method: 0
- Go 전용: `ping`, `diagnostics.version`, `folder_sync.run`, `folder_sync.stage`, `folder_sync.report`
- 직접 Tauri command 5개도 모두 등록: `engine_ping`, `engine_status`, `open_path`, `set_folder_sync_dir`, `folder_sync_now`

method 누락은 없지만 오류 구조와 일부 wire type은 드리프트한다.

## Critical

### A-C01. 지연 자동 저장이 다른 씬 또는 복원 직후 원고를 덮어쓴다

- 위치: `apps/desktop/src/hooks/useDebouncedCallback.ts:9-19`, `apps/desktop/src/routes/Workspace.tsx:686-700,800-815,1735-1744`, `engine/internal/node/repo.go:107-111`
- 문제: timer 인자는 이전 씬의 `doc`인데 실행 함수는 `ref.current`의 최신 `saveNow`다. 씬 A 입력 후 800ms 안에 B로 이동하면 A 문서가 B ID로 저장된다. restore/contextual edit/Companion apply 뒤 pending save도 새 결과를 재덮을 수 있다.
- 근거: debounce에 scene change/unmount cancel이 없고 백엔드 update에 기대 `content_version` 조건도 없다.
- 수정 방향: 먼저 실패 테스트를 추가하고 `{nodeId, doc, expectedContentVersion}`를 예약 시점에 고정한다. debounce에 `cancel/flush`와 cleanup을 추가하고, node 이동·restore·export·Companion apply 전 flush 완료를 기다린다. 백엔드는 낙관적 잠금을 추가한다.
- 예상 작업량: 1~2일
- 확신도: 매우 높음. fake timer로 결정적 재현 가능.

## High

### A-H01. 실패한 일일 백업이 하루 동안 완료로 간주된다

- 위치: `engine/internal/backup/backup.go:21-44`, `engine/internal/backup/backup_test.go:57-72`
- 문제/근거: 날짜 디렉터리를 먼저 만들고 directory 존재만으로 skip한다. `VACUUM INTO` 실패나 중단 뒤 빈 directory가 남으면 그날 재시도하지 않는다.
- 수정 방향: 임시 DB → `PRAGMA quick_check` → fsync → atomic rename → success manifest 순으로 바꾸고 실패 artifact를 정리한다.
- 작업량: 0.5~1일

### A-H02. backup scheduler가 앱 종료를 최대 하루 막을 수 있다

- 위치: `engine/internal/backup/scheduler.go:84-102`, `engine/internal/engineapp/engineapp.go:191-199,341-350`, `engine/cmd/linetta-ffi/ffi.go:61-69`, `apps/desktop/src-tauri/src/ffi.rs:296-299`
- 문제/근거: `time.Sleep`은 context cancel로 깨지지 않는데 stop은 `<-done`을 무기한 기다린다.
- 수정 방향: `time.NewTimer`와 `select`로 cancel-aware 대기, shutdown deadline과 sleep-branch test 추가.
- 작업량: 0.5일

### A-H03. 폴더/Git 동기화가 누락·충돌·부분 복사를 성공으로 보고한다

- 위치: `engine/internal/foldersync/foldersync.go:66-89,92-123`, `engine/internal/gitsync/gitsync.go:114-132`, `engine/internal/export/project.go:126-129,189-217`, `engine/internal/store/migrations/0001_init.sql:2-14`, `apps/desktop/src-tauri/src/folder_sync.rs:8-16,125-149`
- 문제: project별 오류를 log 후 계속하고, 중복 가능한 title slug만 filename으로 쓴다. direct write/copy라 중단 시 partial file이 남으며 MAS 중간 실패는 이미 복사한 수도 0으로 보고한다.
- 수정 방향: ID 포함 filename과 manifest, expected/actual count 검증, temp+fsync+rename, 하나라도 실패하면 job 실패, partial count/failed filename 보고.
- 작업량: 2~3일

### A-H04. UI의 “작품 백업 (.md)”은 무손실 복구 포맷이 아니다

- 위치: `apps/desktop/src/routes/Library.tsx:220-228,433-437`, `engine/internal/export/project.go:65-93,132-164`, `engine/internal/export/markdown.go:43-125`, `engine/internal/mdmeta/metadata.go:18-23`, `engine/internal/importmd/tree.go:97-109`, `engine/internal/importmd/builder.go:45-53`
- 문제: 3단계 이상 outline 평탄화, 다수 project/node/domain metadata와 note/fact/thread/beat/snapshot/Companion 기록 손실, unknown Tiptap 구조와 Markdown 특수문자 손실 가능성이 있다.
- 수정 방향: 즉시 “읽기용 Markdown 내보내기”로 명칭 변경. 중기에는 versioned `.linetta` archive/JSON/SQLite 복구 포맷 추가.
- 작업량: 2~4일

### A-H05. DB/migration 실패 시 backup이 있어도 앱 내부 복구 진입점이 없다

- 위치: `engine/internal/store/store.go:18-46`, `engine/internal/engineapp/engineapp.go:96-111,191-199`, `engine/cmd/linetta-ffi/ffi.go:125-130`, `apps/desktop/src-tauri/src/ffi.rs:255-260`, `apps/desktop/src/components/EngineGate.tsx:36-59`
- 문제: engine 시작 전에 실패하면 오류가 code 1로 축약되고 EngineGate는 retry/diagnostics copy만 제공한다. pre-migration backup도 없다.
- 수정 방향: ABI에 상세 start error, Rust 독립 recovery screen, `quick_check` 기반 backup 선택 복원, 원본 보존, migration 전 snapshot.
- 작업량: 3~5일

## Medium

| ID | 위치 | 문제/근거 | 수정 방향 | 작업량 | 확신도 |
|---|---|---|---|---:|---|
| A-M01 | `engine/internal/store/store.go:23-35`, `engine/internal/backup/backup.go:36-44`, `apps/desktop/src-tauri/src/lib.rs:190-209` | 단일 SQLite connection에서 `VACUUM INTO`가 일반 IPC를 막을 수 있고 일반 `engine_call`에는 timeout이 없다 | DB 크기별 p95/p99 측정 후 writer queue/read/backup 정책 분리 | 2~3일 | 중간; benchmark 필요 |
| A-M02 | `engine/internal/importmd/builder.go:37-87,92-139,168-223`, `engine/internal/rpc/handlers/imports.go:43-53` | project/node/entity/relationship import가 하나의 transaction이 아니어서 중간 실패 시 partial project가 남는다 | 단일 `sql.Tx` 또는 staging project 후 activate | 2~3일 | 높음 |
| A-M03 | `engine/internal/rpc/server.go:62-69,187-198`, `apps/desktop/src-tauri/src/ffi.rs:284-290`, `apps/desktop/src/lib/rpc.ts:104-106` | Go error code/data가 Rust에서 message string으로 소실 | `{code,message,data,method,requestId}` 보존, TS guard | 1~2일 | 매우 높음 |
| A-M04 | `apps/desktop/src/lib/types.ts:272-279,658-663`, `engine/internal/entity/entity.go:69-78`, `engine/internal/entity/repo.go:79-86`, `engine/internal/thread/thread.go:22-28`, `engine/internal/thread/repo.go:93-105` | TS optional patch가 Go value field로 들어가 누락 필드를 empty overwrite할 수 있다 | pointer patch 또는 PUT/PATCH 분리, absent/empty/null contract test | 1일 | 높음 |
| A-M05 | `engine/internal/node/repo.go:156-164,541-556`, `engine/internal/manuscript/indexer.go:19-29`, `engine/internal/manuscript/searcher.go:69-81` | 원문 commit 후 best-effort FTS DELETE/INSERT가 별도 transaction이며 일부 누락은 rebuild되지 않는다 | FTS transaction, content version, reconcile | 1~2일 | 높음 |
| A-M06 | `apps/desktop/src-tauri/src/folder_sync.rs:31-57` | settings path를 먼저 바꾸고 bookmark 생성/직접 write를 나중에 해 실패 시 불일치 | bookmark temp+검증+rename 후 settings, rollback | 1일 | 매우 높음 |
| A-M07 | `engine/internal/node/repo.go:156-165`, `engine/internal/rpc/handlers/nodes.go:45-53`, `apps/desktop/src/routes/Workspace.tsx:803-811` | 원고 commit 뒤 mention resync 실패가 전체 save 실패처럼 보인다 | 원고 저장 성공과 derived warning 분리, retry marker | 1일 | 높음 |

## Low / 확인 필요

| ID | 위치 | 판정 | 문제/근거와 수정 방향 | 작업량 |
|---|---|---|---|---:|
| A-L01 | `apps/desktop/src/lib/types.ts:364-372,560-581,621-626`, `apps/desktop/src-tauri/src/lib.rs:72-81`, `engine/internal/settings/settings.go:97-120`, `engine/internal/rpc/handlers/imports.go:21-28` | Low | Rust Option은 null인데 TS는 optional only, settings/import result 필드 drift. fixture/schema contract test 추가 | 1일 |
| A-L02 | `apps/desktop/src-tauri/src/ffi.rs:182-204` | Low | notification invalid JSON과 emit 실패를 조용히 버림. parse/emit log와 run ID 추가 | 0.5일 |
| A-L03 | `engine/internal/store/migrations.go:15-52,55-78` | Low | migration checksum과 newer-schema guard 없음. checksum 검증과 read-only recovery guard | 1일 |
| A-Q01 | `engine/internal/store/store.go:32-36` | 확인 필요 | WAL `synchronous=NORMAL`과 `FULL`의 Mac 전원 손실/latency 비교 후 정책 결정 | 0.5일 측정 |
| A-Q02 | `engine/internal/store/store.go:31`, `engine/internal/backup/backup.go:40-43` | 확인 필요 | 100MB/500MB/1GB DB backup 중 autosave p50/p95/p99 측정 | 0.5일 측정 |

---

# 워크스트림 B — 리팩터링 로드맵

## 발견

| ID | 근거 | 독립 PR 방향 |
|---|---|---|
| TIDY-001 | `apps/desktop/src/routes/Workspace.historyPanel.test.ts:1-18`, `Workspace.responsive.test.ts:1-35`, `Workspace.ipadInput.test.ts:1-20`, `Settings.iosReduction.test.ts:1-40` | TSX 원문 문자열 검사를 사용자 관찰 기반 테스트로 전환 |
| DUP-001 | `engine/internal/ai/context.go:558-611`, `summarizer/summarizer.go:299-340`, `snapshot/repo.go:134-181`, `contextualedit/contextualedit.go:563-604`, `manuscriptedit/replace.go:369-405` | 동일 출력 정책 5개만 `internal/tiptapdoc` walker로 통합 |
| DUP-002 | `apps/desktop/src/routes/Settings.tsx:110-124,336-455`, `components/companion/CompanionPanel.tsx:575-587,924-1026` | API/state transition만 `useOpenRouterSetup`으로 추출; OAuth/UI는 호출자 유지 |
| TIDY-002 | `apps/desktop/src/routes/Workspace.tsx:1-310,428-704,800-927,960-1444,1589-2060` | 테스트 seam 후 outline → persistence → render-only 순차 추출. 전면 재작성 금지 |
| TIDY-003 | `apps/desktop/src/App.tsx:65-84`, `Settings.tsx:199-215,355-379`, `CompanionPanel.tsx:912-948`, `NoteMarkerExtension.ts:43-80`, `MentionExtension.ts:101-108`, `Workspace.tsx:281-309,728-754` | `lib/appEvents.ts` typed dispatch/subscribe |
| TIDY-004 | `engine/internal/rpc/handlers/beats.go:75-89`, `threads.go:71-125`, `engine/internal/companion/tools.go:747-752` | update 후 Get error를 `CodeInternalError`로 전파하고 fake seam 테스트 |
| TIDY-005 | `apps/desktop/package.json:7-13,32-45`, `FocusExtension.ts:21-29,84-85`, `MentionExtension.ts:48-62,113-131`, `SearchHighlightExtension.ts:44-58`, `Tiptap.tsx:229-234`, `autoMention.ts:39-75,118-129`, `useDebouncedCallback.ts:5-18`, `useThrottledCallback.ts:5-37` | ESLint/React Hooks CI와 Tiptap/JSON `any` 축소 |
| DUP-003 | `apps/desktop/src/routes/Library.tsx:406-450`, `LibraryAll.tsx:108-147`, `components/ProjectCard.tsx:1-14` | 현재 book UI 공용 카드 추출 |
| DUP-004 | `engine/internal/node/repo.go:281-340,343-393,396-443` | 위치 계산은 유지하고 `insertNodeTx`, `initialContentDoc`만 추출 |
| TIDY-006 | `engine/internal/companion/companion.go:143-227,367-559,705-739` | 기능 수정과 맞닿을 때만 package 내부 파일 분리 |
| TIDY-007 | `relationshipPresets.ts:1-22`, `rpc.ts:84-86`, `ProjectCard.tsx:1-14`, `companion.go:163-225`, `runner.go:181-193` | build graph/계획 확인 후 dead code 제거 |
| TIDY-008 | `engine/internal/node/repo.go:291-297,353-359,488-493,582-598,691-705,776-782,822-828,983-1028` | `errors.Is(err, sql.ErrNoRows)` 기계 통일; DUP-004에 포함 |

## 독립 머지 PR 순서

```text
HOTFIX-A-C01 (새 선행 작업)
  └─ B01 행동 테스트 seam ──> B09 Workspace outline ──> B10 persistence 추출

B03 typed events ──> B07 OpenRouter controller
B02 Tiptap walker, B04 RPC error, B05 node helper,
B06 lint/types, B08 book card는 독립 실행 가능
```

중재: 원래 B10 persistence 추출을 B01/B09 뒤에 두는 로드맵은 구조 정리에는 맞지만 A-C01을 기다리게 하면 안 된다. 먼저 최소 핫픽스와 회귀 테스트를 머지하고, 이후 동작 보존 리팩터링을 한다.

### 주말 단위 PR 로드맵

| PR | 작업 | 선행 | 리스크 | 예상 | 실패 테스트/회귀 검증 |
|---|---|---|---|---:|---|
| B01 | TSX 원문 검사 → 행동 테스트 | 없음 | 중간 | 0.5~1일 | 새 layout seam import red → render/interaction green; responsive/history/iOS reduction tests + build |
| B02 | 공용 Go Tiptap walker | 없음 | 중간 | 1일 | text/mention/paragraph/heading/blockquote/nested/malformed table test; 관련 5 package + full Go/MAS/mobile |
| B03 | typed app event API | 없음 | 낮음 | 0.5일 | dispatch payload/unsubscribe test; editor/Settings/Companion tests |
| B04 | RPC 재조회 error 전파 | 없음 | 낮음~중간 | 0.5일 | update 성공+Get 실패 fake로 `CodeInternalError`; handler race/full Go |
| B05 | node insert helper + `errors.Is` | 없음 | 중간 | 0.5~1일 | leaf/container content, ordinal, project timestamp table test; node race |
| B06 | ESLint/hooks gate + 타입 정리 | 없음 | 중간 | 1일 | lint config로 현재 violation red → explicit any/hooks 정리; test/build/lint |
| B07 | OpenRouter setup controller | B03 | 중간 | 1~1.5일 | key save/delete, keyInfo, model order, draft-before-test, failure preservation |
| B08 | 공용 project book card | 없음 | 낮음 | 0.5~1일 | title/count/action/navigation component test; Library/LibraryAll tests |
| B09 | Workspace outline coordinator | B01, B03, HOTFIX | 중간 | 1~1.5일 | create/move/delete fallback/repair/undo hook test |
| B10 | Workspace persistence hook | B01, B09, HOTFIX | 중간 | 1일 | debounce/manual/idle snapshot, Companion flush, node switch cancel |

각 PR은 구조 변경과 동작 변경을 분리한다. 대표 검증은 `pnpm test && pnpm build`, `go test ./...`, 변경 패키지 `go test -race`다.

---

# 워크스트림 C — ko/en/ja 로컬라이제이션 QA

## 자동 대조

| 검사 | 결과 |
|---|---:|
| ko / en / ja 키 | 각각 1,014 |
| 언어별 누락 키 | 0 |
| placeholder 불일치 | 0 |
| 정적 참조 키 / 사전에 없는 참조 | 806 / 0 |
| 최초 미사용 후보 / 동적 경로 해소 / 확정 고아 | 137 / 117 / 20 |

동적 참조 근거: `ContextPanel.tsx:153`, `OutlinePanel.tsx:362-366`, `VersionSheet.tsx:114-117`, `AIContextChecklist.tsx:55-57`, `CompanionPanel.tsx:263-267`, `i18n.tsx:3382-3506`.

`MessageKey`가 `string | ...`로 시작해 리터럴 유니언이 무력화된다(`apps/desktop/src/lib/i18n.tsx:14-312`). 기존 테스트는 노드 label 6건뿐이다(`apps/desktop/src/lib/i18n.displayNodeLabel.test.ts:1-33`). 키/placeholder/동적 allowlist 검사를 CI에 추가해야 한다.

## High: “외부 전송 없음”이 실제 AI 동작과 모순된다

- 절대 표현: `apps/desktop/src/lib/i18n.tsx:1019-1021,2035-2037,3051-3053`; 노출 `apps/desktop/src/routes/Library.tsx:363-373`
- 실제 AI 전송 안내: `apps/desktop/src/lib/i18n.tsx:816,1128,1832,2144,2848,3160`

| 현재 값 | 제안 값 | 이유 |
|---|---|---|
| 외부 전송 없음 | 원고는 기본적으로 로컬 저장 | AI 사용 예외 반영 |
| No external transfer | Manuscripts are stored locally by default | 절대 주장 제거 |
| 外部送信なし | 原稿は基本的にローカル保存 | 전송 예외 반영 |
| Manuscripts and settings stay on this computer. | Manuscripts and settings are stored locally. AI features send only the context you choose to the connected provider. | 실제 동작 명시 |
| 原稿と設定はこのコンピュータ内に保管されます。 | 原稿と設定はローカルに保存されます。AI機能では、選択したコンテキストのみ接続先へ送信されます。 | 실제 동작 명시 |

## 사용자 노출 하드코딩

| 위치 | 현재 | 제안 |
|---|---|---|
| `routes/Library.tsx:495` | `Git sync` | `library.safety.gitSync` |
| `components/companion/CompanionPanel.tsx:1156,1176,1208` | `companion` | `companion.speaker` |
| `CompanionPanel.tsx:1454-1456` | `{count} chars` | locale 숫자 + `companion.reference.charCount` |
| `routes/Workspace.tsx:175-180` | `(예:)` | `workspace.outlineStructureFormat` |

## 영어 수정표

| 현재 값 / 위치 | 제안 값 | 이유 |
|---|---|---|
| `…so older drafts can keep moving.` `i18n.tsx:1349` | `…so you can continue working on existing drafts.` | 직역투 |
| `The center is the manuscript surface.` `:1357` | `Write your manuscript in the center editor.` | 부자연스러운 명사화 |
| `Published {published} eps · stock {stock} eps` `:1383` | `{published} published · {stock} ready to publish` | 한국식 용어 직역 |
| `Work total {count}` `:1388` | `Total: {count} chars` | 단위/어순 |
| `Beat in next scene` `:1461` | `Add beat to next scene` | 동작 명확화 |
| `Hand off a stuck point…` `:1676` | `Bring me a sticking point, or explore what comes next in this scene.` | 자연스러운 안내 |
| `Plan plot, outline, settings…` `:1683` | `Plan the plot, outline, story world, and other work-wide changes.` | 앱 설정 오독 방지 |
| `Choose a safe starter path…` `:1746` | `Choose a setup option to connect an AI provider.` | 근거 없는 safe 제거 |
| `Save to dossier after search` `:1794` | `Check sources, then save to Fact Book` | 기능명 통일 |
| `Fact dossier` `:1860` | `Fact Book` | 기능명 통일 |
| `All library →` `:2043` | `View all projects →` | 자연스러운 탐색 |
| `Claude subscription harnesses…` `:2317` | `Claude subscription sign-in is not supported in Linetta. Linetta supports Claude through API keys only.` | 명백한 오역 |

## 일본어 수정표

| 현재 값 / 위치 | 제안 값 | 이유 |
|---|---|---|
| `Linettaを案内` `i18n.tsx:2360` | `Linettaを見てみる` | 비문 수정 |
| `これまでの原稿を続けられます` `:2365` | `既存の原稿もそのまま執筆を続けられます` | 목적 관계 |
| `GitHub sync` `:2369` | `GitHub同期` | 혼용 제거 |
| `再作成中 / 再作成` `:2409-2410` | `書き直し中 / 書き直す` | 집필 용어 |
| `登場` `:2415` | `メンション` | entity 종류 반영 |
| `このシーンに紐づいた人物はありません` `:2418` | `このシーンでメンションされた世界観要素はありません` | 실제 모델 반영 |
| `次のシーンにビート` `:2477` | `次のシーンにビートを追加` | 동작 명확화 |
| `このシーンを新しい Thread としてマーク` `:2479` | `このシーンを新しいストーリーラインとして登録` | 도메인 용어 통일 |
| `Thread View` `:2553` | `ストーリーライン表示` | 번역 누락 |
| `バージョン · 復元 · diff` `:2555` | `バージョン · 復元 · 差分` | 일반 UI 용어 |
| `Focus モード` `:2565-2566` | `集中モード` | 기능 의미 |
| `流れ` `:2885` | `ストーリーライン` | 모호함 제거 |
| `下書き前` `:3040` | `まだ原稿がありません` | 자연스러운 상태 |
| `全ライブラリ →` `:3059` | `すべての作品を見る →` | 자연스러운 탐색 |
| `保管箱` `:3060` | `アーカイブ` | 표준 UI 용어 |
| `provider` `:3137,3156,3160,3209,3216` | `プロバイダー` | 표기 통일 |

설명/완료 문구는 대체로 です/ます로 일관된다. 모델 지시문 `〜して`체는 UI 문구가 아니며 일관돼 있어 결함으로 세지 않았다.

## 고아 키 20개

- `onboarding.workspace.ai.title/body`: `i18n.tsx:344-345,1360-1361,2376-2377`; 실제 tour `Workspace.tsx:1479-1510`
- `workspace.newPart/newChapter/prompt.renameLabel`: `i18n.tsx:397-398,459,1413-1414,1475,2429-2430,2491`
- `workspace.toast.copyTextSuccess`, `workspace.command.restore`: `i18n.tsx:494,524,1510,1540,2526,2556`; 현재 호출 `Workspace.tsx:1110-1113`
- `contextual.projectReplace.disabled`, `contextual.snapshotNotice`: `i18n.tsx:574,583,1590,1599,2606,2615`
- `companion.examplesLabel/examplesTitle/example.{conflict,outline,search,fetch,apply}`: `i18n.tsx:702-708,1718-1724,2734-2740`; 현재 UI `CompanionPanel.tsx:1105-1110`
- `ai.mode.replace`: `i18n.tsx:804,1820,2836`; 현재 `AIPanel.tsx:122-129`
- `settings.git.runNow`, `settings.ops.ok/failed`: `i18n.tsx:1195,1219-1220,2211,2235-2236,3227,3251-3252`

## UI 길이 확인 필요

1. 340px 패널의 3등분 contextual tabs: `App.css:462-466`, `ContextualEditPanel.css:9-28`, `ContextualEditPanel.tsx:280-308`, 영어 `i18n.tsx:1567-1569`.
2. Companion reference 4열/nowrap: `CompanionPanel.css:475-502`, `CompanionPanel.tsx:1448-1456`, 일본어 `i18n.tsx:2680-2688`.

검증: 340px/320px, ko/en/ja, 글자 크기 100%/125% 시각 회귀. 확정 파손으로 단정하지 않는다.

## 제안 diff

```diff
diff --git a/apps/desktop/src/routes/Library.tsx b/apps/desktop/src/routes/Library.tsx
@@
-                <span>Git sync</span>
+                <span>{t("library.safety.gitSync")}</span>
diff --git a/apps/desktop/src/components/companion/CompanionPanel.tsx b/apps/desktop/src/components/companion/CompanionPanel.tsx
@@
-import { useI18n } from "../../lib/i18n";
+import { localeForLanguage, useI18n } from "../../lib/i18n";
@@
-  const { t } = useI18n();
+  const { language, t } = useI18n();
@@
-<span className="msg-who">companion</span>
+<span className="msg-who">{t("companion.speaker")}</span>
@@
-<span>{ref.char_count.toLocaleString()} chars</span>
+<span>{t("companion.reference.charCount", {
+  count: ref.char_count.toLocaleString(localeForLanguage(language)),
+})}</span>
diff --git a/apps/desktop/src/routes/Workspace.tsx b/apps/desktop/src/routes/Workspace.tsx
@@
-return `${t(outlinePreset.nameKey)}: ${part} > ${chapter} > ${scene} (예: ...)`;
+return t("workspace.outlineStructureFormat", {
+  preset: t(outlinePreset.nameKey), part, chapter, scene,
+  partExample: outlineNumberLabel(outlinePreset, "part", 1, t),
+  chapterExample: outlineNumberLabel(outlinePreset, "chapter", 1, t),
+  sceneExample: outlineNumberLabel(outlinePreset, "scene", 1, t),
+});
```

추가 키:

```diff
+ "library.safety.gitSync": "Git 동기화" | "Git sync" | "Git同期"
+ "companion.speaker": "컴패니언" | "Companion" | "コンパニオン"
+ "companion.reference.charCount": "{count}자" | "{count} chars" | "{count}字"
+ "workspace.outlineStructureFormat":
+   "{preset}: {part} > {chapter} > {scene} (예: {partExample} > {chapterExample} > {sceneExample})"
+   "{preset}: {part} > {chapter} > {scene} (example: {partExample} > {chapterExample} > {sceneExample})"
+   "{preset}: {part} > {chapter} > {scene}（例：{partExample} > {chapterExample} > {sceneExample}）"
```

---

# 워크스트림 D — 배포와 보안 체크리스트

현재 정책 판단은 Apple/Tauri 공식 문서와 2026-07-12 취약점 DB를 기준으로 했다.

| 판정 | 항목 | 코드 근거 | 판단/조치 |
|---|---|---|---|
| 통과 | CSP script 경계 | `apps/desktop/src-tauri/tauri.conf.json:24-29` | `script-src 'self'`, eval/inline script 없음. `style-src 'unsafe-inline'`은 CSS에만 한정 |
| 통과 | capability가 main window로 제한 | `apps/desktop/src-tauri/capabilities/default.json:1-15` | local `main`만 대상 |
| 통과 | shell default는 실행/spawn을 허용하지 않음 | `capabilities/default.json:7-14`, `Cargo.toml:16-26` | 공식 Tauri `shell:default`는 URL open만 허용. 현재 JS shell 사용처도 없음 |
| 통과 | FS가 picker 흐름과 결합 | `exportSave.ts:1-14`, `importLoad.ts:1-21`, `CompanionPanel.tsx:726` | dialog가 부여한 runtime scope와 text read/write 사용 |
| 실패 | custom `open_path`가 Tauri opener scope를 우회 | `apps/desktop/src-tauri/src/lib.rs:169-179`, 호출 `Library.tsx:157-175` | command는 임의 path를 받는다. 실제 UI는 engine 제공 home/backup만 쓰지만 XSS 시 기존 파일/app 열기 blast radius. 허용 base path를 Rust에서 검증 |
| 확인 필요 | 단일 동적 `engine_call`이 전체 Go RPC를 노출 | `src-tauri/src/lib.rs:93-101`, `rpc.ts:104-106`, `engineapp.go:217-328` | Tauri custom command는 기본적으로 모든 local window/webview에 허용된다. 현재 1개 local window라 즉시 exploit은 아니나 XSS 시 delete/settings/AI까지 전부 가능. future window 전에 read/write capability 또는 Rust method allowlist 설계 |
| 통과 | MAS sandbox/entitlement | `tauri.mas.conf.json:3-8`, `linetta-mas.entitlements:5-16` | sandbox, outbound network, user-selected RW, app-scoped bookmark 최소 권한 |
| 통과 | bookmark 기반 unattended folder access | `folder_sync.rs:28-57,79-149`, `macos_bookmarks.rs:18-56` | API 선택은 적절. A-M06 원자성 문제는 별도 |
| 통과 | 코드에 분석/광고/크래시 SDK 없음 | `apps/desktop/package.json:14-44`, `engine/go.mod:5-24` | 별도 analytics/tracker 발견 없음 |
| 실패 | 앱 안에서 privacy policy를 쉽게 열 수 없음 | Settings UI `Settings.tsx:520-600`, provider guide `AISetupStart.tsx:300-369`; 저장소 전체 링크 검색 0건 | Apple 5.1.1(i)는 App Store metadata와 앱 내부 접근을 모두 요구. Settings/About에 링크 추가 |
| 실패 | 제3자 AI 전송 전 명시 고지/동의가 없음 | provider 설명 `i18n.tsx:1104-1152`, 선택 UI `AISetupStart.tsx:300-369`, 전송 동작 `CompanionPanel.tsx`/`rpc.ts:310-318` | Apple 5.1.2(i)는 third-party AI 공유 위치 고지와 explicit permission을 요구. 첫 provider 연결/첫 전송 전 공급자·전송 범위·취소 방법 동의 저장 |
| 실패 | privacy policy가 자체 모순이고 필수 항목이 부족 | `docs/privacy-policy.md:9-31,46-68,83-105` | “전송하지 않음/제3자 제공 없음”과 AI provider 전송이 모순. third party 동등 보호 확인, 보존/삭제, 동의 철회 방법도 없음 |
| 확인 필요 | `PrivacyInfo.xcprivacy` 부재 | `tauri.conf.json:31-44`, `build.rs:1-13`, `release-mas-local.sh:32-52`에 resource 포함 없음 | required-reason API 선언 의무는 Apple 문서상 macOS에는 동일 적용되지 않으므로 부재 자체를 즉시 실패로 단정하지 않는다. 그러나 data collection은 모든 platform 대상이다. ASC privacy label과 provider retention을 확인한 후 유효 manifest가 필요한지 판단; 무근거 빈 manifest 금지 |
| 통과 | MAS package signing 기본 검증 | `release-mas-local.sh:34-52`, entitlement plist | app/package 서명과 plist lint 통과 |
| 확인 필요 | App Store server-side preflight 자동화 없음 | `release-mas-local.sh:45-55` | 로컬 codesign/pkgutil까지만 검증. 실제 upload 전 Transporter/App Store Connect validation 기록 확인 |
| 실패 | Go 1.26.2에 실제 호출 가능 stdlib 취약점 5건 | `engine/go.mod:1-9`, CI `ci.yml:25-29`, 호출 `openrouter/oauth.go:97-140`, `keyinfo.go:71-128`, `websearch.go:70-108` | `GO-2026-5856/5039/5037/4971/4918`; Go 1.26.5 이상으로 올리고 재스캔 |
| 실패 | React Router 6.30.3 advisory | `package.json:26-30`, `pnpm-lock.yaml:53-55,1435-1444` | GHSA-2j2x-hqr9-3h42, fix 6.30.4. 현재 navigation은 내부 경로라 exploit 가능성 낮지만 patch update 권장 |
| 확인 필요 | Rust `quick-xml` 0.39.4 advisory 2건 | `Cargo.lock:2654-2661`, path `quick-xml → plist → Tauri` | RUSTSEC-2026-0194/0195. macOS path에 포함되나 Linetta가 untrusted XML을 직접 파싱하는 사용처는 발견되지 않음. Tauri/plist update로 해결하고 직접 우회 금지 |
| 확인 필요 | Rust `anyhow` unsound 경고 | `Cargo.toml:24`, `Cargo.lock:44-48` | RUSTSEC-2026-0190; `downcast_mut` 사용처 0. direct unused dependency 제거 또는 1.0.103+ update |
| 확인 필요 | GTK3 unmaintained 경고 | `Cargo.lock:1298-1315,1362-1375` | Linux target의 Tauri transitive dependency. MAS와 무관하며 Linetta 단독 GTK4 전환은 하지 말고 upstream 추적 |
| 실패 | CI dependency audit gate 없음 | `.github/workflows/ci.yml:43-55`, `Makefile:9-20` | frozen lock/install은 통과하지만 govulncheck/pnpm audit/cargo audit가 없다. OS별 target noise 정책과 allowlist 만료일을 둔다 |
| 통과 | secret literal 노출 없음 | Keychain `engine/internal/settings/secrets_darwin.go:82-155`, CI secret env `build.yml:83-167` | 키는 OS secure store/CI secrets 사용 |
| 통과 | Go race 및 MAS tests | 실행 결과 | `go test -race ./...`, `cargo test --features mas` 통과 |
| 실패 | 공개 배포 문구와 유료 상태 컨텍스트 불일치 | `README.md:44-50` | README가 “free”라고 단정. App Store Connect 가격을 확인하고 유료 배포 상태와 동기화 |

공식 근거:

- [Apple App Review Guidelines 5.1](https://developer.apple.com/app-store/review/guidelines/)
- [Apple App Privacy Details](https://developer.apple.com/app-store/app-privacy-details/)
- [Apple privacy manifest files](https://developer.apple.com/documentation/bundleresources/privacy-manifest-files)
- [Apple macOS privacy manifest bundle location](https://developer.apple.com/documentation/bundleresources/adding-a-privacy-manifest-to-your-app-or-third-party-sdk)
- [Tauri capabilities](https://v2.tauri.app/security/capabilities/)
- [Tauri file-system permissions](https://v2.tauri.app/plugin/file-system/)
- [Go vulnerability database](https://pkg.go.dev/vuln/)

---

# 통합 교차 검증

## 일치하는 결론

1. A-C01과 B의 `Workspace`/debounce 복잡도는 같은 원인을 가리킨다. 구조 정리 전에 데이터 덮어쓰기 핫픽스가 선행한다.
2. C의 “외부 전송 없음” 모순과 D의 Apple 5.1.2 제3자 AI 고지/명시 동의 공백은 같은 배포 위험이다.
3. A의 IPC error/type drift, B의 strict TS 우회/lint 부재, C의 무력화된 `MessageKey`는 compile-time 계약이 충분하지 않다는 같은 결론이다.
4. A의 Markdown backup/partial sync 문제와 C의 local-first 절대 표현은 “데이터 안전성”을 제품 문구가 실제보다 강하게 약속한다는 같은 결론이다.

## 모순과 중재

| 충돌 | 중재안 |
|---|---|
| B10 persistence refactor가 B01/B09 뒤이지만 A-C01은 즉시 수정 필요 | 최소 hotfix+실패 테스트를 독립 PR로 먼저 머지. 이후 B01/B09/B10은 동작 보존 구조 변경 |
| privacy manifest 부재를 배포 실패로 볼지 여부 | macOS required-reason API 규칙은 iOS 계열과 다르다. 현재는 “확인 필요”. 먼저 ASC privacy label과 provider retention을 확인하고 실제 수집 data type만 선언 |
| single SQLite connection을 늘리면 concurrency가 나아질 수 있음 | 단순 `SetMaxOpenConns` 증가는 write contention을 되살릴 수 있다. benchmark 후 writer queue/maintenance 분리로 해결 |
| 모든 Tiptap text 변환을 하나로 합치면 중복 감소 | 출력 정책이 동일한 5개만 합치고 search/companion 변형은 유지 |
| GTK3 unmaintained를 즉시 제거 | Linux Tauri upstream 경계다. Linetta 단독 대전환은 비용 대비 효과가 낮아 모니터링만 |

---

# 전체 우선순위 큐

정렬은 사용자 데이터 손실 → 심사/배포 차단 → 유지보수 절감 순이다.

| 순위 | 항목 | 이유 | 목표 |
|---:|---|---|---|
| 1 | A-C01 cross-scene stale autosave | 현재 원고 직접 덮어쓰기 | 즉시 hotfix |
| 2 | A-H01 backup 실패 당일 재시도 차단 | 실제 복구본 부재를 성공으로 오인 | 이번 주말 |
| 3 | A-H02 종료 hang | 강제 종료와 사용자 신뢰 저하 | 이번 주말 |
| 4 | A-H03 partial/title-collision sync | 외부 백업의 누락·덮어쓰기·성공 오판 | 이번 달 |
| 5 | A-H04 Markdown backup 오표기 | 사용자가 무손실 복구로 오해 | 명칭 즉시, 포맷 이번 달 |
| 6 | A-H05 startup recovery 부재 | 손상 시 backup에 접근도 못함 | 이번 달 |
| 7 | A-M02 import atomicity | 실패 시 partial project | 이번 달 |
| 8 | A-M05/A-M07 derived index drift | 검색/mention 불일치 | 이번 달 |
| 9 | D privacy link/AI consent/policy + C wording | 다음 심사/업데이트 리젝과 개인정보 고지 위험 | 다음 MAS 제출 전, 가능하면 이번 주말 |
| 10 | Go 1.26.5+와 React Router 6.30.4 | 현재 audit 실패, Go는 실제 호출 가능 | 이번 주말 |
| 11 | CI dependency audit gate | 취약점 재유입 방지 | 이번 달 |
| 12 | A-M03/A-M04/L01 contract tests | 에러/patch/null drift | 이번 달 |
| 13 | C i18n catalog test + 20 orphan 삭제 | 컴파일러가 키를 보호하지 못함 | 이번 달 |
| 14 | B01→B09→B10 Workspace 분리 | 장기 변경 비용/회귀 범위 축소 | 이번 달 이후 순차 |
| 15 | B02 Tiptap walker | 5개 구현 drift 방지 | 이번 달 |
| 16 | B03/B07 typed events/OpenRouter controller | 중복과 문자열 계약 축소 | 이번 달 |
| 17 | C 영어/일본어 표현 및 UI length QA | 품질/레이아웃 | 이번 달 |

---

# 실행 계획

## 이번 주말에 할 것

### 1. `fix: prevent stale scene autosave overwrite` — 최우선, 1~2일

실패 테스트 먼저:

- A 입력 → 800ms 전 B 전환 → timer 진행 시 A/B ID/doc가 섞이지 않음
- restore/Companion apply 전 pending save flush
- unmount/node change cancel
- backend stale `content_version` reject

완료 조건: node ID/version-bound save queue, `cancel/flush`, conflict UI, 전체 frontend/Go/race 통과.

### 2. `fix: make daily backup retryable and shutdown cancellable` — 0.5~1일

실패 테스트 먼저:

- 첫 backup 실패 후 같은 날 재호출은 실행됨
- scheduler가 다음 자정 sleep 중이어도 stop은 1초 내 반환
- temp/invalid DB는 완료 marker가 되지 않음

### 3. `fix: align App Store AI privacy disclosure` — 0.5일

- Settings/About에 privacy policy 링크
- 첫 제3자 AI 전송 전에 provider/전송 context/보존 정책 링크/철회 방법과 명시 동의
- ko/en/ja 절대 문구를 “기본적으로 로컬 저장”으로 수정
- privacy policy에 third-party, retention/deletion, revoke/withdrawal 내용 반영
- App Store Connect privacy answers를 실제 provider 동작과 대조

### 4. `chore: patch audited dependencies` — 0.25일

- Go 1.26.5 이상, React Router 6.30.4 이상
- `govulncheck`, `pnpm audit --prod`, target-aware `cargo audit` 재실행
- Cargo advisories는 reachability를 문서화하고 가능한 Tauri/plist patch update만 수행

주말이 이틀로 엄격히 제한되면 1→2 순으로 끝내고, 3은 다음 MAS 제출을 막는 release gate로 등록한다. 4는 patch update라 1/2 사이에 끼워 넣을 수 있다.

## 이번 달에 할 것

1. sync/export 무결성 PR: ID filename, manifest, atomic write, partial failure reporting.
2. Markdown을 “읽기용 내보내기”로 이름 변경하고 `.linetta` versioned recovery 포맷 설계/구현.
3. startup recovery screen과 pre-migration backup/quick-check restore.
4. atomic import와 FTS/mention reconcile.
5. IPC error envelope, patch semantics, null/field fixture contract tests.
6. i18n catalog/placeholder/dynamic allowlist test, 20 orphan 키 제거, 제안 diff/번역표 반영.
7. B01 테스트 seam 후 `Workspace` outline/persistence를 작은 PR로 분리.
8. 공용 Tiptap walker, typed app events, OpenRouter setup controller, ESLint/React Hooks gate.
9. 340px/320px × ko/en/ja × 100/125% 시각 QA.
10. dependency audit CI. Rust target-specific allowlist에는 근거와 만료일을 둔다.

## 안 해도 되는 것

1. `Workspace` 전면 재작성이나 Redux/Zustand 도입 — 작은 hook 추출로 충분하다.
2. SQLite connection 수를 근거 없이 늘리기 — 먼저 backup latency benchmark가 필요하다.
3. 모든 Tiptap 변환기를 만능 parser 하나로 통합 — 출력 정책 차이를 지운다.
4. Go repository 전체 generic CRUD화 — 현재 중복 helper만 추출한다.
5. Linux GTK3 경고 때문에 Linetta가 독자적으로 GTK4 shell로 전환 — Tauri upstream을 추적한다.
6. 근거 없는 빈 `PrivacyInfo.xcprivacy` 추가 — 잘못된 manifest도 App Store가 거부한다.
7. `Trusted Types`, 대규모 CSP 재설계 — raw HTML/eval sink가 없고 현재 script CSP가 강하다.
8. Companion service를 줄 수만 이유로 전면 분할 — 기능 변경과 맞닿을 때만 파일 이동한다.
9. Rust `engine_ping` command 삭제 — TS wrapper는 미사용이지만 native 상태 점검 경로는 별도 확인 전 유지한다.

---

# 재현/검증 명령

```sh
make ci
cd engine && go test -race ./...
cd engine && go test -tags mas ./...
cd apps/desktop/src-tauri && cargo test --features mas
cd apps/desktop && pnpm audit --prod
cd engine && govulncheck ./...
cd apps/desktop/src-tauri && cargo audit
make validate-distribution
```

감사 종료 시 제품 작업 트리는 clean이었다. `.analysis/` 아래 감사 산출물만 새로 생성했다.
