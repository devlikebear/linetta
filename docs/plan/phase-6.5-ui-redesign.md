# Phase 6.5: UI Redesign — Codex / Claude Desktop Style

_Linetta macOS 앱의 UI를 4단 중첩 SwiftUI(NavigationSplitView→TabView→HSplitView→TabView)에서 flat한 3-column "AI 워크플로우 러너" 레이아웃으로 전면 재설계._

_작성일: 2026-05-24_
_속한 로드맵: [`linetta-macos-app-completion-roadmap.md`](./linetta-macos-app-completion-roadmap.md)_
_예상 소요: 10~16시간_
_선행: Phase 6 (Embedded Engine Lifecycle) 완료_
_후속: Phase 7 (Settings Studio)_

---

## 1. 페이즈 목표

현재 UI는 `NavigationSplitView` 안에 `TabView`(Overview/Memory/Workbench)가 들어가고, 그 안에 또 `HSplitView`(Blueprint/RunPanel)가, 또 그 안에 `TabView`(Artifacts/Manuscript/Review)가 들어가는 4단 중첩 구조다. 어떤 윈도우 너비에서도 한쪽이 밀려 깨진다 — toolbar `>>` 오버플로우, footer 오버랩, 사이드바 텍스트 잘림, 메모리 폼의 Save 버튼 잘림.

이 페이즈는 그 구조를 버리고 **AI 워크플로우 러너**(Claude Desktop / Codex 스타일)로 재설계한다. flat한 3-column 셸 · warm dark 테마 · 중첩 탭 0개 · 시스템 native `.inspector()` 활용 · 명시적 theme token. 결과적으로 윈도우 너비와 무관하게 레이아웃이 안 깨지고, 키보드만으로 모든 주요 작업이 가능하며, 작가가 "AI에게 시키고 결과를 채택"하는 핵심 workflow가 화면에서 즉시 보인다.

---

## 2. Brainstorming 결과 (확정 결정)

다음 7개 결정은 brainstorming 단계에서 사용자 승인을 거쳤다. 변경 시 재논의 필요.

| # | 결정 |
|---|---|
| 1 | **도구 정체성**: AI 워크플로우 러너 (글 에디터 아님). 핵심 = Run → Artifacts → Adopt 흐름 |
| 2 | **Thread 단위**: Episode 1개 = 대화 1개 (Claude Desktop의 conversation에 해당). Work는 project 폴더, Run은 메시지 교환 |
| 3 | **Canon Memory 위치**: 사이드바의 Work 하위 별도 섹션 (`📓 Memory`). 클릭 시 메인 패널 전체 점유로 전환 |
| 4 | **Run timeline 표현**: Workspace 상단 고정 Blueprint + 하단 접힌 Run 목록 (한 번에 1개만 expanded) |
| 5 | **Manuscript 위치**: 우측 inspector 패널 (toggle). 기본 닫힘 |
| 6 | **Review queue 위치**: 메인 패널 하단 고정 섹션, **work-level** 집계 (전체 에피소드의 pending decisions 모음) |
| 7 | **시각 톤**: Claude Desktop dark warm — `#1b1a17` 배경, `#d97757` coral accent, soft corners, 여백 후함 |

---

## 3. 전체 아키텍처 (Section 1)

### 3.1 셸 구조

```
┌──────────────┬────────────────────────────────────┬──────────────────┐
│  SIDEBAR     │  MAIN PANE                         │  INSPECTOR       │
│  (230pt)     │  (flex, min 540pt)                 │  (320pt, toggle) │
│              │                                    │                  │
│  Works tree  │  Toolbar (sticky)                  │  Manuscript      │
│  + Memory    │  ──────────                        │  (TextEditor)    │
│  + Settings  │  Scrollable content:               │                  │
│   footer     │  · Blueprint card                  │                  │
│              │  · Run history card                │                  │
│              │  · Review queue card               │                  │
│              │                                    │                  │
├──────────────┴────────────────────────────────────┴──────────────────┤
│ ● Engine · localhost:54321 · Synced 12s ago             status footer │
└──────────────────────────────────────────────────────────────────────┘
```

### 3.2 셸 동작

- **Native macOS 15**: `NavigationSplitView`(2-column) + `.inspector()` modifier. 시스템이 사이드바 collapse·inspector animation·toolbar 통합을 처리
- **윈도우 minWidth 1080pt, minHeight 720pt** (현재 화면 잘림 방지)
- **Manuscript inspector**: `⌘⇧M` 또는 toolbar 토글 버튼. inspector 너비 280~480pt 사용자 drag 조절, 마지막 너비 디스크 저장
- **사이드바 토글**: macOS 표준 `View → Show/Hide Sidebar` (`⌘⌃S`). 윈도우 작아지더라도 자동 collapse 안 함 (예측 가능성)
- **Footer (status bar)**: `.safeAreaInset(edge: .bottom)`로 항상 보이되 inner content 위 오버랩 없음 (Phase 6에서 깨졌던 부분)
- **Dark mode 강제**: `.preferredColorScheme(.dark)` + 모든 색은 `LinettaTheme` 토큰만 사용. 시스템 light 모드 추종 안 함

---

## 4. Sidebar 디테일 (Section 2)

### 4.1 구조

```
┌─ Linetta ────────────── ＋ ┐
│                            │
│ WORKS                      │
│ ▾ Mira's Forest            │
│   📓 Memory          [42]  │
│   ● Episode 4              │
│     Episode 3              │
│     Episode 2              │
│     Episode 1              │
│   ＋ New episode           │
│                            │
│ ▸ Glass Harbor             │
│ ▸ Lantern Catalogue        │
│                            │
│ ─────────────────────────  │
│ ⚙  ＋                       │
└────────────────────────────┘
```

### 4.2 동작

| 항목 | 결정 |
|---|---|
| Episode 정렬 | 생성 순 (oldest-first, 연재 순서) |
| Episode 상태 표시 | dot 색만 — idea(gray) / drafting(yellow) / polishing(coral) / ready(green) / published(blue). 풀 라벨은 main toolbar의 status chip |
| Work 펼침/접힘 | `@AppStorage("linetta.ui.sidebar.expanded.<workID>")` 디스크 저장 |
| `📓 Memory` 클릭 | 메인 패널을 그 Work의 Canon 관리 화면으로 전환. 사이드바 상태 그대로 유지 |
| 검색 | `⌘L`로 toggle (보통 숨김). episode 제목 · work 제목 · premise 본문 매칭 |
| 새 work | 사이드바 헤더 ＋ 또는 `⌘N` → modal sheet |
| 새 episode | Work 아래 placeholder `＋ New episode` 클릭 또는 `⇧⌘N` → inline 제목 입력 즉시 생성 |
| 빈 상태 (works=0) | 사이드바 + 메인 모두 onboarding wizard 1페이지 |
| Drag reorder | **P2 (이 페이즈 X)** |
| 너비 | 기본 230pt, 사용자 220~320pt drag 조절. `linetta.ui.sidebar.width` 저장 |

---

## 5. Episode Workspace 메인 패널 (Section 3)

### 5.1 Main toolbar (sticky top)

```
[Mira's Forest › Episode 4 — Echo in the Well]    [Idea ▾]    [📄 Manuscript]   [⋯]
```

- **Breadcrumb**: Work › Episode 제목. 클릭 시 부모로 이동
- **Status chip**: 현재 episode 상태 (Idea/Drafting/Polishing/Ready/Published). 클릭 시 dropdown으로 전이
- **Manuscript 토글**: `⌘⇧M`. on/off 색으로 표시
- **⋯ menu**: Export TXT · Delete episode · …
- **Episode 화면에서만 보임**. Memory 페이지·빈 상태에서는 다른 toolbar (또는 숨김)

### 5.2 Blueprint card

```
┌─ Blueprint                                    [draft]    ⌃ Collapse ─┐
│ Premise        Mira finds a well that whispers...                    │
│ Theme          Inheritance · grief · listening                       │
│ Situation      Late autumn, abandoned Ueno parcel                    │
│ Must include   A black crow, a clay cup, "you came back."            │
│ Must avoid     Direct supernatural reveal in this episode            │
│                                                                      │
│ [▶ Run Agents]  [💾 Save]              last run · 2 hours ago        │
└──────────────────────────────────────────────────────────────────────┘
```

- 접힘/펼침 토글 (per-episode 저장). 접혔을 때는 한 줄 premise 요약 + Run 버튼만
- premise가 비었으면 Run 버튼 disabled
- Save 버튼은 디바운스 자동 저장도 함께 — 명시 클릭은 즉시 확정

### 5.3 Run history card

```
┌─ Run History                                            [3]   latest first ─┐
│                                                                              │
│ ● 11:45 today    Run #3 · draft + scene-list      [adopted v3]      ⌃       │
│   ┌─ ARTIFACTS ─────────────────────────────────────────────────────────┐   │
│   │ [draft · 1,840w]  [scene-list · 6]  [character-arc · Mira]          │   │
│   │                                                                     │   │
│   │ DECISIONS  2 canon proposals · 1 continuity issue · jump to ↓       │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│ ○ 10:32 today    Run #2 · draft only              [3 artifacts]      ⌄      │
│ ○ yesterday      Run #1 · discarded               [archived]         ⌄      │
└──────────────────────────────────────────────────────────────────────────────┘
```

- 최근 5개 기본 (finished + running 모두), "Show all (N)" 푸터로 확장
- 한 번에 1개만 expanded
- expanded 안의 artifact pill 클릭 → Manuscript inspector가 열리며 artifact preview 모드로 본문 표시
- expanded 안에 "Adopt as version" 버튼 (아직 채택 안 된 경우만)
- **Running 상태의 run row**: status dot이 spinner로 대체 + 시간 자리에 "Running…" + tag 자리에 elapsed time. expand 가능하지만 artifacts 자리에 progress 텍스트만

### 5.4 Review queue card

- **work-level 집계**. 빈 경우 카드 자체 숨김
- 한 row = 1개 proposal 또는 1개 continuity issue
- 각 row 우측에 액션 (canon: ✓ approve / ✗ reject / ⏸ defer · continuity: accept / resolve / ignore)
- 출처 표기 (`Run #3 from Ep 4`)
- 5+ 이상이면 첫 3개만 보이고 "Show all" 확장
- Batch action (Approve all canon)은 **P2**

### 5.5 다른 main pane 모드

| 모드 | 트리거 | 내용 |
|---|---|---|
| Memory | 사이드바의 `📓 Memory` 클릭 | toolbar = kind/status filter + search. 본문 = Canon item list (kind별 묶음). inline edit + `＋ New item`. 사이드바에서는 해당 Work의 `📓 Memory` 행이 active 표시, episode active 표시는 해제 |
| Work Overview | Work 선택, episode 미선택 | 제목 · genre · premise · 통계 · 마지막 episode `Open` CTA |
| Onboarding | works=0 | brand + "Create your first work" CTA. 사이드바도 비워두고 onboarding 강조 |

### 5.6 Run 진행 / 에러 상태

- **Run 진행 중**: Blueprint card에 spinner + "Running…" chip. 새 Run row가 timeline 상단에 inserting 애니메이션과 함께. SSE live progress는 **Phase 9**에서, 이 페이즈는 정적 표시만
- **Engine offline**: toolbar 옆에 ⚠ 빨간 chip "Engine offline · Start" — 클릭 시 Settings. 메인 컨텐츠는 마지막 캐시된 데이터 보여주되 액션 버튼 disable

---

## 6. Manuscript Inspector (Section 4)

### 6.1 기본

- **기본 visibility**: 닫혀 있음. `⌘⇧M` 또는 toolbar 토글로 열기
- **너비**: 기본 320pt, 사용자 280~480pt drag 조절. `linetta.ui.inspector.width` 저장. 마지막 visibility는 episode마다 `linetta.ui.inspector.open.<episodeID>` 저장

### 6.2 헤더 (sticky)

```
📄 MANUSCRIPT          v3 · adopted ▾                   ⋯
```

- 좌측 라벨 + 버전 dropdown (v1/v2/v3 + "From artifact: …" 항목들)
- 우측 ⋯ menu: Export TXT · Copy all · Open in full editor (**Phase 9 대기**, 현재 disabled + tooltip)

### 6.3 모드

| 모드 | 트리거 | 동작 |
|---|---|---|
| Adopted version (기본) | 인스펙터 열기 | 메타 `1,840 words · adopted 2h ago · v3` + `TextEditor` inline editable. 1.5초 디바운스 → 새 version 자동 저장 (`note: "auto-save"`) |
| Artifact preview | Run history에서 artifact pill 클릭 | 헤더에 azure 띠 "Preview · Run #3 draft" + **read-only** + 하단 `Adopt as v(N+1)` 버튼 |
| Version diff | (P2 — 이 페이즈 X) | 두 버전 plain text 비교. 별도 spec |

### 6.4 빈 상태

adopted 버전 0개 → "No manuscript yet · Adopt an artifact to start." + 최근 run으로 가는 링크

### 6.5 단어 카운트

헤더 메타에 단순 단어 수 (whitespace split). 페이지 footer 별도 없음

---

## 7. Chrome (Section 5)

### 7.1 Titlebar

- 표준 macOS native (traffic lights)
- 타이틀: `Linetta — <Work> · <Episode>` 동적. fallback:
  - work 미선택 → `Linetta`
  - work 선택 + episode 미선택 → `Linetta — <Work>`
  - Memory 페이지 → `Linetta — <Work> · Memory`
- 우측에 `⌘K` hint chip (클릭 가능)

### 7.2 App 메뉴 바 (macOS standard menubar)

| 메뉴 | 항목 |
|---|---|
| **File** | New Work `⌘N` · New Episode `⇧⌘N` · Open Recent ▸ · Export Episode (TXT) · Export Work (Markdown) · Import Backup… · Close Window `⌘W` |
| **Edit** | 표준 (Cut/Copy/Paste/Select All) · Find `⌘F` (manuscript inspector 안에서 동작) |
| **View** | Show/Hide Sidebar `⌘⌃S` · Show/Hide Manuscript Inspector `⌘⇧M` · Toggle Blueprint card `⌘1` · Memory `⌘2` · Settings `⌘,` |
| **Run** (신설) | Run Agents `⇧⌘R` · Save Blueprint `⌘S` · Adopt Latest Artifact · Restart Engine |
| **Window / Help** | macOS 표준 |

### 7.3 Command Palette (⌘K)

- 이 페이즈에 **minimum 구현**
- `.overlay { ... }` + custom Spotlight 스타일 view
- 검색 가능: episodes · works · canon memory items · 정적 command 리스트
- 본격 fuzzy search · shortcut 표시는 Phase 8

### 7.4 Status Footer (`.safeAreaInset(edge: .bottom)`)

```
● Engine  ·  localhost:54321  ·  Synced 12s ago    ·    Run #4 in progress  ·    ETA 12s
```

- 항상 보임. inner content 위 오버랩 없음 (Phase 6 footer 버그 수정)
- 좌→우: engine status dot + 라벨 + (popover로 address/PID/log) · 마지막 sync 시간 · 진행 중 task 개수
- **ETA는 Phase 9 (SSE) 이후 등장**. 6.5에는 표시 안 함 (자리만 비워두지 않음)

### 7.5 Notification / Toast

- 우측 하단 transient toast (Apple HIG)
- autohide 4초
- 이 페이즈는 `ToastCenter` framework만 마련, 본격 메시지는 **Phase 8**

---

## 8. 기술 접근 (Section 6)

### 8.1 SwiftUI 구조

```swift
NavigationSplitView {
  SidebarView()
} detail: {
  MainPaneRouter()
    .inspector(isPresented: $manuscript.isOpen) {
      ManuscriptInspector()
    }
    .toolbar { /* primaryAction placement 강제 */ }
}
.safeAreaInset(edge: .bottom) { StatusFooter() }
.preferredColorScheme(.dark)
```

### 8.2 상태 관리

- Swift 6의 **`@Observable` macro 사용**. 기존 `ObservableObject` 클래스(`AppState` 등) 전부 마이그레이션
- `@EnvironmentObject` → `@Environment(ClassName.self)`
- 서버 데이터는 그대로 `APIClient`로 fetch

### 8.3 신규 상태 객체

| 객체 | 책임 |
|---|---|
| `SidebarState` | 선택된 노드, work 펼침/접힘, 검색 쿼리, 사이드바 너비 |
| `EpisodeState` | per-episode 로컬 편집 버퍼 (blueprint draft, expanded run id, dirty flag) |
| `ManuscriptState` | inspector visibility, draft, dirty, 현재 보고 있는 version, preview artifact |
| `CommandPaletteState` | open/closed, query, results |
| `ToastCenter` | enqueue / dismiss API. 이 페이즈는 framework만 |

### 8.4 파일 트리

```
macos/Linetta/Sources/Linetta/
  LinettaApp.swift                          # 기존 진입점 유지
  AppDelegate.swift                         # (LinettaApp에서 분리)
  Shell/
    AppShell.swift                          # 최상위 NavigationSplitView
    SidebarView.swift                       # works tree
  MainPane/
    MainPaneRouter.swift                    # episode/memory/overview/onboarding 분기
    EpisodeWorkspaceView.swift
    MemoryPaneView.swift
    WorkOverviewView.swift
    OnboardingView.swift
    Cards/
      BlueprintCard.swift
      RunHistoryCard.swift
      RunRowView.swift
      RunExpandedDetailView.swift
      ReviewQueueCard.swift
      ReviewRowView.swift
  Inspector/
    ManuscriptInspector.swift
    ArtifactPreview.swift
  Chrome/
    MainToolbar.swift
    StatusFooter.swift
    CommandPalette.swift
    AppCommands.swift                       # macOS 메뉴바 (.commands)
  Theme/
    LinettaTheme.swift                      # Color tokens
    LinettaTypography.swift                 # Font tokens
  ToastCenter.swift                         # framework only
  _legacy/                                   # 구 view들 임시 보존
    WorkGalleryView.swift
    WorkspaceView.swift
    EpisodeWorkbenchView.swift
    CanonMemoryView.swift
    ManuscriptVersionView.swift
    MemoryDiffView.swift
    EngineStatusBadge.swift                 # (StatusFooter에 흡수)
```

### 8.5 테마 토큰 (`LinettaTheme.swift`)

```swift
enum LinettaTheme {
  static let background = Color(red: 0.106, green: 0.102, blue: 0.090)        // #1b1a17
  static let surface = Color(red: 0.129, green: 0.118, blue: 0.094)           // #211e18
  static let surfaceElevated = Color(red: 0.086, green: 0.078, blue: 0.059)   // #16140f
  static let border = Color(red: 0.165, green: 0.153, blue: 0.133)            // #2a2722
  static let borderSoft = Color(red: 0.145, green: 0.133, blue: 0.110)        // #25221c
  static let text = Color(red: 0.839, green: 0.827, blue: 0.796)              // #d6d3cb
  static let textSecondary = Color(red: 0.612, green: 0.584, blue: 0.541)
  static let textTertiary = Color(red: 0.431, green: 0.416, blue: 0.376)
  static let accent = Color(red: 0.851, green: 0.467, blue: 0.341)            // #d97757
  static let accentSoft = accent.opacity(0.16)
  static let success = Color(red: 0.435, green: 0.631, blue: 0.463)
  static let warn = Color(red: 0.851, green: 0.667, blue: 0.341).opacity(0.25)
  static let danger = Color.red.opacity(0.85)
}
```

### 8.6 타이포 토큰 (`LinettaTypography.swift`)

| 토큰 | 역할 |
|---|---|
| `.titleLarge` | onboarding 큰 글씨 |
| `.titleSmall` | card 제목 |
| `.body` | 기본 텍스트 (SF Pro Text) |
| `.bodySerif` | manuscript inspector 본문. **New York** 명시 (`Font.system(.body, design: .serif)` — macOS의 system serif) |
| `.bodySmall` | 메타 텍스트 |
| `.caption` | 작은 라벨 |
| `.label` | uppercase 라벨 (letter-spacing 0.7) |

### 8.7 Migration 전략

- **Stop-the-world rewrite**. 한 브랜치에서 작업, 중간 단계에 신구 UI 섞이지 않음
- 페이즈 끝나기 전엔 app이 빌드 안 되는 시점 있을 수 있음 (브랜치 내에서)
- 기존 view 6개 + `EngineStatusBadge` → `_legacy/` 폴더로 이동 (즉시 삭제 X)
- **유지**: `LinettaCore` (Models, APIClient, EngineController, StoragePaths), `LinettaApp.swift` 진입점, `SettingsView.swift` (구조 유지, 색만 새 테마), `NewWorkSheet`, `ExportDocument`
- 페이즈 끝나고 stable 확인되면 `_legacy/` 한 commit으로 삭제

### 8.8 Settings의 위치

Phase 7 "Settings Studio"가 본격 개편 작업. 이번 Phase 6.5는 **기존 `SettingsView`를 새 테마 색만 입혀** 일단 동작하게 두고 Phase 7에서 풀 GUI 작성.

### 8.9 호환성 / 데이터

- 기존 `@AppStorage` 키 그대로: `linetta.engineAddress`, `linetta.useExternalEngine`, `linetta.defaultDBPath`, `linetta.tesseraConfigPath`
- 신규 UI state 키 namespace: `linetta.ui.*`
  - `linetta.ui.sidebar.expanded.<workID>`
  - `linetta.ui.sidebar.width`
  - `linetta.ui.inspector.open.<episodeID>`
  - `linetta.ui.inspector.width`
  - `linetta.ui.blueprint.expanded.<episodeID>`
- 사용자 DB 형식 변경 없음

### 8.10 테스트

- LinettaCore 단위 테스트 유지 (Models, APIClient, EngineController)
- 신규 view는 SwiftUI `#Preview` 매크로로 시각 회귀 확인 (각 카드/뷰 1개씩)
- 1개 smoke test: `AppShell이 instantiate되고 crash 안 함`
- E2E UI test 안 함 (Phase 9 이후)

---

## 9. Out of Scope (Section 7)

### 9.1 이 페이즈에서 안 하는 것

| 항목 | 미루는 곳 |
|---|---|
| Settings 전면 GUI 개편 | Phase 7 |
| NSTextView 기반 long-form 에디터 | Phase 9 D그룹 |
| SSE 라이브 run progress | Phase 9 A~C그룹 |
| Backup / Restore GUI | Phase 7 |
| Tessera config 인라인 YAML 에디터 | Phase 7 |
| Engine log tail viewer (full) | Phase 7. 6.5는 StatusFooter popover에서 최근 5줄만 |
| Canon Decisions History 탭 | Phase 8 |
| 정식 onboarding wizard | Phase 8. 6.5는 single-screen CTA만 |
| Toast 메시지 내용 | Phase 8. 6.5는 `ToastCenter` 빈 framework만 |
| Run 실패 재시도 UI | Phase 8 |
| Find/Replace in manuscript | Phase 9 (LongformEditor 포함) |
| Light theme | 영구 미지원 |
| Drag-to-reorder episodes | P2 |
| Manuscript version diff 뷰 | P2 |
| Review queue batch action | P2 |
| Multi-window / 멀티 work 동시 | P3 |
| Memory bulk import/export | P3 |
| Localization (한/영 토큰) | P3 |
| Universal Binary · Notarization · Sandbox | 로드맵 out-of-scope 유지 |

### 9.2 비기능 안전망 (Phase 6.5 내 최소 보장)

- 기존 `@AppStorage` 값 모두 보존
- 사용자 DB 형식 변경 없음
- `go test ./...` + `swift test` 통과
- 각 신규 main view에 `#Preview` 1개씩
- `EngineController`, `APIClient`, `Models` 단위 테스트 유지

---

## 10. 페이즈 의존성 갱신

```
Phase 5 (MVP, 완료)
  └─→ Phase 6 (Embedded Engine, 완료)
        └─→ Phase 6.5 (UI Redesign)  ← 이번 페이즈
              └─→ Phase 7 (Settings Studio)
                    └─→ Phase 8 (App Polish)
                          └─→ Phase 9 (Live + Editor)
```

이전 그래프와 차이: Phase 6 → Phase 7 사이에 6.5 삽입. Phase 7 이후의 모든 페이즈는 새 shell · 새 theme · 새 chrome 위에서 작업하므로 훨씬 깔끔.

---

## 11. 완료 조건 (Checkpoint preview)

세부 task와 검증 항목은 후속 **writing-plans** 스킬이 생성할 implementation plan에 위임. 본 spec의 끝은 다음 한 줄로 요약 가능:

> **Phase 6.5 완료 = 같은 데이터를 새 UI로 보면서 기존 Phase 1~5의 모든 사용자 시나리오(work 생성, episode 생성, blueprint 저장, run 트리거, artifact adopt, manuscript version 저장, memory CRUD, canon proposal 처리, continuity issue 처리, backup/restore CLI 동작)가 단 한 번도 깨지지 않는 상태.**

---

## 12. 후속 단계

이 spec이 승인되면 **`superpowers:writing-plans` 스킬**로 넘어가 implementation plan을 작성한다. 그 plan은:

- 작업 그룹 (Theme/Shell/Sidebar/MainPane/Inspector/Chrome/Migration) 단위 task 분할
- 각 task의 검증 가능한 완료 기준
- 의존성 그래프 (어떤 task가 어떤 task를 차단하는지)
- TDD 친화: 각 신규 view는 `#Preview` 먼저, 그 다음 통합

writing-plans 진입 후에는 implementation skill (frontend-design / mcp-builder 등) 절대 invoke 안 함. brainstorming → writing-plans → 실제 코드 작성(주 conversation)이 정해진 흐름.

---

_본 문서는 brainstorming 스킬을 통해 사용자와 7개 핵심 결정을 합의한 결과의 산물. 변경 시 재논의 필요._
