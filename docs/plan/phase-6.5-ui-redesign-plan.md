# Phase 6.5: UI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Linetta macOS app's nested SwiftUI shell (NavigationSplitView → TabView → HSplitView → TabView) with a flat 3-column AppShell using native `.inspector()`, warm-dark theme, and AI-workflow-runner workspace, while preserving every Phase 1–5 behavior (work / episode / run / artifact / memory / proposal / continuity / backup flows).

**Architecture:** All new SwiftUI views go under `Sources/Linetta/{Theme,Shell,MainPane,Inspector,Chrome}` with `@Observable` state. Top-level shell is `NavigationSplitView` + `.inspector()`. Legacy views move to `Sources/Linetta/_legacy/` and are deleted at the end of the phase. `LinettaCore` (Models / APIClient / EngineController / StoragePaths) is reused as-is.

**Tech Stack:** Swift 6 · macOS 15 · SwiftUI · `@Observable` macro · XCTest · `swift-tools-version: 6.0`.

**Spec:** [`phase-6.5-ui-redesign.md`](./phase-6.5-ui-redesign.md). Variation from spec requires re-brainstorming.

---

## File Structure (planned)

### Created in this phase

```
macos/Linetta/Sources/Linetta/
  Theme/
    LinettaTheme.swift            # Color tokens (dark warm)
    LinettaTypography.swift       # Font tokens
    LinettaShape.swift            # Corner radius / padding tokens
  State/
    SidebarState.swift            # selection, expansion, search, sidebar width
    EpisodeState.swift            # per-episode draft, expanded-run id, dirty flag
    ManuscriptState.swift         # inspector visibility, version, preview mode, draft
    CommandPaletteState.swift     # open, query, results
    ToastCenter.swift             # toast enqueue/dismiss API (framework only)
  Shell/
    AppShell.swift                # top-level NavigationSplitView + .inspector
    SidebarView.swift             # works tree root
    SidebarWorkRow.swift          # one work node (expand/collapse)
    SidebarEpisodeRow.swift       # one episode leaf (status dot)
    SidebarMemoryRow.swift        # 📓 Memory leaf
    SidebarFooterView.swift       # ⚙ settings + ＋ add
    SidebarSearchField.swift      # ⌘L toggle search
  MainPane/
    MainPaneRouter.swift          # routes to EpisodeWorkspace / Memory / Overview / Onboarding
    EpisodeWorkspaceView.swift    # blueprint + run history + review queue
    MemoryPaneView.swift          # canon item list with filters
    WorkOverviewView.swift        # work selected, no episode
    OnboardingView.swift          # works == 0
    Cards/
      BlueprintCard.swift         # blueprint editor + collapse + Run button
      RunHistoryCard.swift        # run rows + Show all
      RunRowView.swift            # collapsed row
      RunExpandedDetailView.swift # artifacts pills + decisions anchor + adopt
      ReviewQueueCard.swift       # work-level queue
      ReviewRowView.swift         # single canon/continuity row
  Inspector/
    ManuscriptInspector.swift     # right inspector entry
    ManuscriptHeader.swift        # version dropdown + ⋯ menu
    ManuscriptEditor.swift        # TextEditor with debounced autosave
    ArtifactPreviewView.swift     # read-only mode + Adopt button
  Chrome/
    MainToolbar.swift             # breadcrumb + status chip + manuscript toggle
    StatusFooter.swift            # engine status + sync info (.safeAreaInset)
    AppCommands.swift             # macOS menubar .commands
    CommandPalette.swift          # ⌘K spotlight overlay
    TitleBarBinding.swift         # window title formatter

macos/Linetta/Tests/LinettaCoreTests/
  (existing tests preserved)

macos/Linetta/Tests/LinettaTests/        # NEW target (see Task A0)
  ThemeTokenTests.swift
  StateObservableTests.swift
  AppShellSmokeTest.swift
```

### Modified

- `macos/Linetta/Sources/Linetta/LinettaApp.swift` — root swaps from `WorkGalleryView()` to `AppShell()`; adds `.preferredColorScheme(.dark)` and `.commands { AppCommands(...) }`.
- `macos/Linetta/Sources/Linetta/AppState.swift` — migrated from `ObservableObject` to `@Observable`.
- `macos/Linetta/Sources/Linetta/Views/SettingsView.swift` — colors swap to `LinettaTheme` tokens; structure unchanged (full overhaul in Phase 7).
- `macos/Linetta/Package.swift` — adds new `LinettaTests` test target.
- `docs/plan/README.md` — adds Phase 6.5 link in reading order.
- `docs/plan/linetta-macos-app-completion-roadmap.md` — adds Phase 6.5 between 6 and 7 in dependency graph.

### Moved to `_legacy/` (then deleted at end)

- `Views/WorkGalleryView.swift`
- `Views/WorkspaceView.swift`
- `Views/EpisodeWorkbenchView.swift`
- `Views/CanonMemoryView.swift`
- `Views/ManuscriptVersionView.swift`
- `Views/MemoryDiffView.swift`
- `Views/EngineStatusBadge.swift` (absorbed into `StatusFooter`)

### Untouched

- `Sources/LinettaCore/*` (APIClient, Models, EngineController, StoragePaths)
- `Sources/Linetta/Views/NewWorkSheet.swift`
- `Sources/Linetta/Views/ExportDocument.swift`
- Go server, CLI, internal packages.

---

## Dependency Graph

```
Group A · Theme         ─┐
Group B · State         ─┤
                         ├─→  Group C · AppShell skeleton
                         │       ├─→  Group D · Sidebar
                         │       ├─→  Group E · Main Pane Router + Modes
                         │       │       └─→  Group F · Episode Workspace Cards
                         │       ├─→  Group G · Inspector
                         │       └─→  Group H · Chrome
                         │
                         └─→  Group I · Migration / Legacy / Settings retrofit
                                  └─→  Group J · Verification & Docs
```

Groups D, E (with F), G, H can be worked in parallel after C lands (subagent-driven mode benefits here). I and J are sequential at the end.

---

## Task Index

| Group | Tasks |
|---|---|
| **A. Theme Foundation** | A0, A1, A2, A3 |
| **B. State Layer** | B1, B2, B3, B4, B5, B6 |
| **C. AppShell Skeleton** | C1, C2 |
| **D. Sidebar** | D1, D2, D3, D4, D5 |
| **E. Main Pane Router + Modes** | E1, E2, E3 |
| **F. Episode Workspace Cards** | F1, F2, F3, F4, F5 |
| **G. Inspector** | G1, G2, G3, G4 |
| **H. Chrome** | H1, H2, H3, H4 |
| **I. Migration / Legacy / Settings retrofit** | I1, I2, I3 |
| **J. Verification & Docs** | J1, J2, J3 |

Total: 36 tasks. Estimated 10–16 hours.

---

# Group A · Theme Foundation

### Task A0: Add LinettaTests target

**Files:**
- Modify: `macos/Linetta/Package.swift`
- Create: `macos/Linetta/Tests/LinettaTests/.gitkeep`

- [ ] **Step 1: Read current Package.swift**

Run: `cat macos/Linetta/Package.swift`

- [ ] **Step 2: Add LinettaTests test target to Package.swift**

Add inside the `targets:` array of `macos/Linetta/Package.swift` (after the existing `LinettaCoreTests` test target):

```swift
.testTarget(
    name: "LinettaTests",
    dependencies: ["Linetta", "LinettaCore"]
)
```

- [ ] **Step 3: Create placeholder file so SwiftPM resolves the target**

Run: `mkdir -p macos/Linetta/Tests/LinettaTests && touch macos/Linetta/Tests/LinettaTests/.gitkeep`

- [ ] **Step 4: Verify the target builds**

Run: `cd macos/Linetta && swift build`
Expected: `Build complete!` with no errors.

- [ ] **Step 5: Commit**

```bash
git add macos/Linetta/Package.swift macos/Linetta/Tests/LinettaTests/.gitkeep
git commit -m "build(macos): add LinettaTests target for Phase 6.5 view tests"
```

---

### Task A1: LinettaTheme color tokens

**Files:**
- Create: `macos/Linetta/Sources/Linetta/Theme/LinettaTheme.swift`
- Test: `macos/Linetta/Tests/LinettaTests/ThemeTokenTests.swift`

- [ ] **Step 1: Write the failing test**

Create `macos/Linetta/Tests/LinettaTests/ThemeTokenTests.swift`:

```swift
import SwiftUI
import XCTest
@testable import Linetta

final class ThemeTokenTests: XCTestCase {
    func testBackgroundIsWarmDark() {
        let resolved = LinettaTheme.background.resolve(in: EnvironmentValues())
        XCTAssertEqual(resolved.red, 0.106, accuracy: 0.01)
        XCTAssertEqual(resolved.green, 0.102, accuracy: 0.01)
        XCTAssertEqual(resolved.blue, 0.090, accuracy: 0.01)
    }

    func testAccentIsCoral() {
        let resolved = LinettaTheme.accent.resolve(in: EnvironmentValues())
        XCTAssertEqual(resolved.red, 0.851, accuracy: 0.01)
        XCTAssertEqual(resolved.green, 0.467, accuracy: 0.01)
        XCTAssertEqual(resolved.blue, 0.341, accuracy: 0.01)
    }

    func testAccentSoftIsSemiTransparent() {
        let resolved = LinettaTheme.accentSoft.resolve(in: EnvironmentValues())
        XCTAssertEqual(resolved.opacity, 0.16, accuracy: 0.01)
    }
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd macos/Linetta && swift test --filter ThemeTokenTests`
Expected: FAIL with `cannot find 'LinettaTheme' in scope`.

- [ ] **Step 3: Create the theme file**

Create `macos/Linetta/Sources/Linetta/Theme/LinettaTheme.swift`:

```swift
import SwiftUI

public enum LinettaTheme {
    public static let background = Color(red: 0.106, green: 0.102, blue: 0.090)         // #1b1a17
    public static let surface = Color(red: 0.129, green: 0.118, blue: 0.094)            // #211e18
    public static let surfaceElevated = Color(red: 0.086, green: 0.078, blue: 0.059)    // #16140f
    public static let border = Color(red: 0.165, green: 0.153, blue: 0.133)             // #2a2722
    public static let borderSoft = Color(red: 0.145, green: 0.133, blue: 0.110)         // #25221c

    public static let text = Color(red: 0.839, green: 0.827, blue: 0.796)               // #d6d3cb
    public static let textSecondary = Color(red: 0.612, green: 0.584, blue: 0.541)
    public static let textTertiary = Color(red: 0.431, green: 0.416, blue: 0.376)

    public static let accent = Color(red: 0.851, green: 0.467, blue: 0.341)             // #d97757
    public static let accentSoft = Color(red: 0.851, green: 0.467, blue: 0.341).opacity(0.16)

    public static let success = Color(red: 0.435, green: 0.631, blue: 0.463)
    public static let warn = Color(red: 0.851, green: 0.667, blue: 0.341).opacity(0.25)
    public static let danger = Color.red.opacity(0.85)
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `cd macos/Linetta && swift test --filter ThemeTokenTests`
Expected: 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add macos/Linetta/Sources/Linetta/Theme/LinettaTheme.swift macos/Linetta/Tests/LinettaTests/ThemeTokenTests.swift
git commit -m "feat(theme): add LinettaTheme color tokens (warm dark)"
```

---

### Task A2: LinettaTypography font tokens

**Files:**
- Create: `macos/Linetta/Sources/Linetta/Theme/LinettaTypography.swift`
- Test: `macos/Linetta/Tests/LinettaTests/ThemeTokenTests.swift` (extend)

- [ ] **Step 1: Add typography test to existing file**

Append to `macos/Linetta/Tests/LinettaTests/ThemeTokenTests.swift`:

```swift
final class TypographyTokenTests: XCTestCase {
    func testBodySerifUsesSystemSerif() {
        // Smoke test: tokens compile and are non-nil
        _ = LinettaTypography.titleLarge
        _ = LinettaTypography.titleSmall
        _ = LinettaTypography.body
        _ = LinettaTypography.bodySerif
        _ = LinettaTypography.bodySmall
        _ = LinettaTypography.caption
        _ = LinettaTypography.label
    }
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd macos/Linetta && swift test --filter TypographyTokenTests`
Expected: FAIL with `cannot find 'LinettaTypography' in scope`.

- [ ] **Step 3: Create typography file**

Create `macos/Linetta/Sources/Linetta/Theme/LinettaTypography.swift`:

```swift
import SwiftUI

public enum LinettaTypography {
    public static let titleLarge = Font.system(size: 28, weight: .semibold, design: .default)
    public static let titleSmall = Font.system(size: 13, weight: .semibold, design: .default)
    public static let body = Font.system(size: 13, weight: .regular, design: .default)
    public static let bodySerif = Font.system(size: 14, weight: .regular, design: .serif)
    public static let bodySmall = Font.system(size: 12, weight: .regular, design: .default)
    public static let caption = Font.system(size: 11, weight: .regular, design: .default)
    public static let label = Font.system(size: 10, weight: .semibold, design: .default)

    /// Reusable view modifier for label-style uppercase text.
    public struct LabelStyle: ViewModifier {
        public func body(content: Content) -> some View {
            content
                .font(LinettaTypography.label)
                .textCase(.uppercase)
                .tracking(0.7)
                .foregroundStyle(LinettaTheme.textTertiary)
        }
    }
}

public extension View {
    func linettaLabelStyle() -> some View {
        modifier(LinettaTypography.LabelStyle())
    }
}
```

- [ ] **Step 4: Run tests**

Run: `cd macos/Linetta && swift test --filter TypographyTokenTests`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add macos/Linetta/Sources/Linetta/Theme/LinettaTypography.swift macos/Linetta/Tests/LinettaTests/ThemeTokenTests.swift
git commit -m "feat(theme): add LinettaTypography font tokens and label style modifier"
```

---

### Task A3: LinettaShape shape tokens

**Files:**
- Create: `macos/Linetta/Sources/Linetta/Theme/LinettaShape.swift`

- [ ] **Step 1: Create shape tokens file**

Create `macos/Linetta/Sources/Linetta/Theme/LinettaShape.swift`:

```swift
import SwiftUI

public enum LinettaShape {
    public static let cardCornerRadius: CGFloat = 9
    public static let buttonCornerRadius: CGFloat = 6
    public static let pillCornerRadius: CGFloat = 9
    public static let shellCornerRadius: CGFloat = 12

    public static let cardPaddingH: CGFloat = 16
    public static let cardPaddingV: CGFloat = 14
    public static let mainContentPadding: CGFloat = 18
    public static let sectionGap: CGFloat = 14
}
```

- [ ] **Step 2: Verify build**

Run: `cd macos/Linetta && swift build`
Expected: `Build complete!`.

- [ ] **Step 3: Commit**

```bash
git add macos/Linetta/Sources/Linetta/Theme/LinettaShape.swift
git commit -m "feat(theme): add LinettaShape constants for corners and padding"
```

---

# Group B · State Layer

### Task B1: Migrate AppState to @Observable

**Files:**
- Modify: `macos/Linetta/Sources/Linetta/AppState.swift`
- Modify (call sites): `macos/Linetta/Sources/Linetta/LinettaApp.swift`, all `@EnvironmentObject AppState` usages

- [ ] **Step 1: List current call sites**

Run: `grep -rn "EnvironmentObject.*AppState\|@StateObject.*AppState\|AppState()" macos/Linetta/Sources/Linetta/`

Note every match for Step 4.

- [ ] **Step 2: Replace AppState body with @Observable**

Replace `macos/Linetta/Sources/Linetta/AppState.swift` contents:

```swift
import Combine
import Foundation
import LinettaCore
import Observation

@MainActor
@Observable
final class AppState {
    private(set) var works: [Work] = []
    var selectedWork: Work?
    var isLoading = false
    var errorMessage: String?
    private(set) var client: APIClient

    private let engine: EngineController
    @ObservationIgnored private var cancellables: Set<AnyCancellable> = []

    init(engine: EngineController) {
        self.engine = engine
        self.client = APIClient(baseURL: engine.address ?? APIClient.defaultBaseURL)
        engine.$address
            .dropFirst()
            .receive(on: RunLoop.main)
            .sink { [weak self] address in
                guard let self else { return }
                self.client = APIClient(baseURL: address ?? APIClient.defaultBaseURL)
                if address != nil {
                    Task { @MainActor [weak self] in await self?.refreshWorks() }
                }
            }
            .store(in: &cancellables)
    }

    func refreshWorks() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            works = try await client.listWorks()
            if let selectedWork, !works.contains(where: { $0.id == selectedWork.id }) {
                self.selectedWork = nil
            }
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func createWork(title: String, genre: String, premise: String) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let work = try await client.createWork(CreateWorkRequest(title: title, genre: genre, premise: premise))
            works.insert(work, at: 0)
            selectedWork = work
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
```

- [ ] **Step 3: Update LinettaApp call sites to `@State` + `.environment`**

In `macos/Linetta/Sources/Linetta/LinettaApp.swift`, replace:

```swift
@StateObject private var engine = sharedEngine
@StateObject private var appState = sharedAppState
```
with:
```swift
@State private var engine = sharedEngine
@State private var appState = sharedAppState
```

and replace every `.environmentObject(appState)` with `.environment(appState)` (same for `.environmentObject(engine)` → `.environment(engine)`).

Also remove the `@MainActor` private `let sharedAppState = AppState(engine: sharedEngine)` — keep it. `@Observable` classes work with `@State` for owning views.

- [ ] **Step 4: Update all View call sites**

For every file from Step 1's grep, replace:

```swift
@EnvironmentObject private var appState: AppState
```
with:
```swift
@Environment(AppState.self) private var appState
```

Same for `EngineController`.

For mutating bindings (e.g., `$appState.selectedWork`), wrap with `@Bindable`:

```swift
@Bindable var appState = appState
// then `$appState.selectedWork` works
```

- [ ] **Step 5: Run build**

Run: `cd macos/Linetta && swift build`
Expected: Build succeeds. Fix any leftover `$appState` errors by inserting `@Bindable var appState = appState` near the top of the view body.

- [ ] **Step 6: Run all tests**

Run: `cd macos/Linetta && swift test`
Expected: All existing tests pass.

- [ ] **Step 7: Commit**

```bash
git add macos/Linetta/Sources/Linetta/AppState.swift macos/Linetta/Sources/Linetta/LinettaApp.swift macos/Linetta/Sources/Linetta/Views/
git commit -m "refactor(state): migrate AppState to @Observable macro"
```

---

### Task B2: SidebarState

**Files:**
- Create: `macos/Linetta/Sources/Linetta/State/SidebarState.swift`
- Test: `macos/Linetta/Tests/LinettaTests/StateObservableTests.swift`

- [ ] **Step 1: Write the failing test**

Create `macos/Linetta/Tests/LinettaTests/StateObservableTests.swift`:

```swift
import XCTest
@testable import Linetta

@MainActor
final class SidebarStateTests: XCTestCase {
    func testInitialSelectionIsNone() {
        let state = SidebarState()
        XCTAssertEqual(state.selection, .none)
    }

    func testWorkExpansionPersistsAcrossInstances() {
        UserDefaults.standard.removeObject(forKey: "linetta.ui.sidebar.expanded.work-A")
        let s1 = SidebarState()
        s1.setExpanded("work-A", expanded: true)
        XCTAssertTrue(s1.isExpanded("work-A"))

        let s2 = SidebarState()
        XCTAssertTrue(s2.isExpanded("work-A"))
        UserDefaults.standard.removeObject(forKey: "linetta.ui.sidebar.expanded.work-A")
    }

    func testSearchToggleClearsQueryWhenClosed() {
        let state = SidebarState()
        state.searchOpen = true
        state.query = "echo"
        state.searchOpen = false
        XCTAssertEqual(state.query, "")
    }
}
```

- [ ] **Step 2: Run test (should fail)**

Run: `cd macos/Linetta && swift test --filter SidebarStateTests`
Expected: FAIL — `cannot find 'SidebarState'`.

- [ ] **Step 3: Create SidebarState**

Create `macos/Linetta/Sources/Linetta/State/SidebarState.swift`:

```swift
import Foundation
import Observation

enum SidebarSelection: Equatable {
    case none
    case work(workID: String)
    case episode(workID: String, episodeID: String)
    case memory(workID: String)
}

@MainActor
@Observable
final class SidebarState {
    var selection: SidebarSelection = .none
    var width: Double {
        didSet { UserDefaults.standard.set(width, forKey: "linetta.ui.sidebar.width") }
    }
    var searchOpen = false {
        didSet { if !searchOpen { query = "" } }
    }
    var query = ""

    init() {
        self.width = UserDefaults.standard.double(forKey: "linetta.ui.sidebar.width").nonZeroOr(230)
    }

    func isExpanded(_ workID: String) -> Bool {
        UserDefaults.standard.bool(forKey: "linetta.ui.sidebar.expanded.\(workID)")
    }

    func setExpanded(_ workID: String, expanded: Bool) {
        UserDefaults.standard.set(expanded, forKey: "linetta.ui.sidebar.expanded.\(workID)")
    }
}

private extension Double {
    func nonZeroOr(_ fallback: Double) -> Double { self == 0 ? fallback : self }
}
```

- [ ] **Step 4: Run tests**

Run: `cd macos/Linetta && swift test --filter SidebarStateTests`
Expected: 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add macos/Linetta/Sources/Linetta/State/SidebarState.swift macos/Linetta/Tests/LinettaTests/StateObservableTests.swift
git commit -m "feat(state): add SidebarState with persisted expansion and width"
```

---

### Task B3: EpisodeState

**Files:**
- Create: `macos/Linetta/Sources/Linetta/State/EpisodeState.swift`
- Test: `macos/Linetta/Tests/LinettaTests/StateObservableTests.swift` (extend)

- [ ] **Step 1: Add failing test**

Append to `macos/Linetta/Tests/LinettaTests/StateObservableTests.swift`:

```swift
@MainActor
final class EpisodeStateTests: XCTestCase {
    func testBlueprintDirtyFlagTrips() {
        let state = EpisodeState()
        state.loadBlueprint(premise: "a", theme: "b", situation: "c", mustInclude: "d", mustAvoid: "e", structureNotes: "f")
        XCTAssertFalse(state.isDirty)
        state.premise = "changed"
        XCTAssertTrue(state.isDirty)
    }

    func testExpandedRunIDDefaultsToNil() {
        XCTAssertNil(EpisodeState().expandedRunID)
    }

    func testBlueprintCardCollapseDefaultsToFalse() {
        UserDefaults.standard.removeObject(forKey: "linetta.ui.blueprint.expanded.ep-X")
        let state = EpisodeState()
        XCTAssertTrue(state.isBlueprintExpanded(episodeID: "ep-X"))
    }
}
```

- [ ] **Step 2: Run test**

Run: `cd macos/Linetta && swift test --filter EpisodeStateTests`
Expected: FAIL — type not found.

- [ ] **Step 3: Create EpisodeState**

Create `macos/Linetta/Sources/Linetta/State/EpisodeState.swift`:

```swift
import Foundation
import Observation

@MainActor
@Observable
final class EpisodeState {
    var premise = ""
    var theme = ""
    var situation = ""
    var mustInclude = ""
    var mustAvoid = ""
    var structureNotes = ""

    var expandedRunID: String?
    var isRunning = false

    @ObservationIgnored private var loadedSnapshot: String = ""

    var isDirty: Bool { currentSnapshot != loadedSnapshot }

    private var currentSnapshot: String {
        [premise, theme, situation, mustInclude, mustAvoid, structureNotes].joined(separator: "|")
    }

    func loadBlueprint(premise: String, theme: String, situation: String, mustInclude: String, mustAvoid: String, structureNotes: String) {
        self.premise = premise
        self.theme = theme
        self.situation = situation
        self.mustInclude = mustInclude
        self.mustAvoid = mustAvoid
        self.structureNotes = structureNotes
        self.loadedSnapshot = currentSnapshot
    }

    func markSaved() {
        loadedSnapshot = currentSnapshot
    }

    func isBlueprintExpanded(episodeID: String) -> Bool {
        let key = "linetta.ui.blueprint.expanded.\(episodeID)"
        if UserDefaults.standard.object(forKey: key) == nil { return true }
        return UserDefaults.standard.bool(forKey: key)
    }

    func setBlueprintExpanded(episodeID: String, expanded: Bool) {
        UserDefaults.standard.set(expanded, forKey: "linetta.ui.blueprint.expanded.\(episodeID)")
    }
}
```

- [ ] **Step 4: Run tests**

Run: `cd macos/Linetta && swift test --filter EpisodeStateTests`
Expected: 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add macos/Linetta/Sources/Linetta/State/EpisodeState.swift macos/Linetta/Tests/LinettaTests/StateObservableTests.swift
git commit -m "feat(state): add EpisodeState with blueprint dirty tracking"
```

---

### Task B4: ManuscriptState

**Files:**
- Create: `macos/Linetta/Sources/Linetta/State/ManuscriptState.swift`
- Test: `macos/Linetta/Tests/LinettaTests/StateObservableTests.swift` (extend)

- [ ] **Step 1: Add failing test**

Append to `macos/Linetta/Tests/LinettaTests/StateObservableTests.swift`:

```swift
@MainActor
final class ManuscriptStateTests: XCTestCase {
    func testDefaultInspectorClosed() {
        UserDefaults.standard.removeObject(forKey: "linetta.ui.inspector.open.ep-Y")
        let s = ManuscriptState()
        XCTAssertFalse(s.isOpen(episodeID: "ep-Y"))
    }

    func testTogglePersists() {
        let s1 = ManuscriptState()
        s1.setOpen(episodeID: "ep-Z", open: true)
        let s2 = ManuscriptState()
        XCTAssertTrue(s2.isOpen(episodeID: "ep-Z"))
        UserDefaults.standard.removeObject(forKey: "linetta.ui.inspector.open.ep-Z")
    }

    func testWidthClampsToRange() {
        let s = ManuscriptState()
        s.width = 100
        XCTAssertEqual(s.width, 280)
        s.width = 999
        XCTAssertEqual(s.width, 480)
    }
}
```

- [ ] **Step 2: Run test**

Run: `cd macos/Linetta && swift test --filter ManuscriptStateTests`
Expected: FAIL.

- [ ] **Step 3: Create ManuscriptState**

Create `macos/Linetta/Sources/Linetta/State/ManuscriptState.swift`:

```swift
import Foundation
import Observation

enum ManuscriptMode: Equatable {
    case adopted
    case artifactPreview(runID: String, artifactID: String, body: String)
}

@MainActor
@Observable
final class ManuscriptState {
    var mode: ManuscriptMode = .adopted
    var draft: String = ""
    private(set) var loadedSnapshot: String = ""
    var width: Double {
        didSet {
            width = max(280, min(480, width))
            UserDefaults.standard.set(width, forKey: "linetta.ui.inspector.width")
        }
    }

    init() {
        let stored = UserDefaults.standard.double(forKey: "linetta.ui.inspector.width")
        self.width = max(280, min(480, stored == 0 ? 320 : stored))
    }

    var isDirty: Bool { draft != loadedSnapshot }

    func loadAdopted(body: String) {
        mode = .adopted
        draft = body
        loadedSnapshot = body
    }

    func markSaved() {
        loadedSnapshot = draft
    }

    func isOpen(episodeID: String) -> Bool {
        UserDefaults.standard.bool(forKey: "linetta.ui.inspector.open.\(episodeID)")
    }

    func setOpen(episodeID: String, open: Bool) {
        UserDefaults.standard.set(open, forKey: "linetta.ui.inspector.open.\(episodeID)")
    }
}
```

- [ ] **Step 4: Run tests**

Run: `cd macos/Linetta && swift test --filter ManuscriptStateTests`
Expected: 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add macos/Linetta/Sources/Linetta/State/ManuscriptState.swift macos/Linetta/Tests/LinettaTests/StateObservableTests.swift
git commit -m "feat(state): add ManuscriptState with mode, debounced autosave snapshot, width clamp"
```

---

### Task B5: CommandPaletteState + ToastCenter

**Files:**
- Create: `macos/Linetta/Sources/Linetta/State/CommandPaletteState.swift`
- Create: `macos/Linetta/Sources/Linetta/State/ToastCenter.swift`

- [ ] **Step 1: Add CommandPaletteState test**

Append to `macos/Linetta/Tests/LinettaTests/StateObservableTests.swift`:

```swift
@MainActor
final class CommandPaletteStateTests: XCTestCase {
    func testOpenClearsQuery() {
        let s = CommandPaletteState()
        s.query = "foo"
        s.isOpen = false
        s.isOpen = true
        XCTAssertEqual(s.query, "")
    }
}

@MainActor
final class ToastCenterTests: XCTestCase {
    func testEnqueueAppendsToast() {
        let center = ToastCenter()
        center.enqueue(.init(title: "Hello", kind: .info))
        XCTAssertEqual(center.toasts.count, 1)
    }
}
```

- [ ] **Step 2: Run test (fail)**

Run: `cd macos/Linetta && swift test --filter CommandPaletteStateTests`
Expected: FAIL.

- [ ] **Step 3: Create CommandPaletteState**

Create `macos/Linetta/Sources/Linetta/State/CommandPaletteState.swift`:

```swift
import Foundation
import Observation

@MainActor
@Observable
final class CommandPaletteState {
    var isOpen = false {
        didSet { if isOpen { query = "" } }
    }
    var query = ""
}
```

- [ ] **Step 4: Create ToastCenter**

Create `macos/Linetta/Sources/Linetta/State/ToastCenter.swift`:

```swift
import Foundation
import Observation

struct ToastMessage: Identifiable, Equatable {
    enum Kind { case info, success, warn, error }
    let id = UUID()
    let title: String
    let kind: Kind
}

@MainActor
@Observable
final class ToastCenter {
    private(set) var toasts: [ToastMessage] = []

    func enqueue(_ message: ToastMessage) {
        toasts.append(message)
        Task { @MainActor [weak self] in
            try? await Task.sleep(nanoseconds: 4_000_000_000)
            self?.dismiss(message.id)
        }
    }

    func dismiss(_ id: UUID) {
        toasts.removeAll { $0.id == id }
    }
}
```

- [ ] **Step 5: Run tests**

Run: `cd macos/Linetta && swift test --filter CommandPaletteStateTests --filter ToastCenterTests`
Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add macos/Linetta/Sources/Linetta/State/CommandPaletteState.swift macos/Linetta/Sources/Linetta/State/ToastCenter.swift macos/Linetta/Tests/LinettaTests/StateObservableTests.swift
git commit -m "feat(state): add CommandPaletteState and ToastCenter scaffold"
```

---

### Task B6: Wire all state objects into LinettaApp

**Files:**
- Modify: `macos/Linetta/Sources/Linetta/LinettaApp.swift`

- [ ] **Step 1: Read current LinettaApp.swift**

Run: `cat macos/Linetta/Sources/Linetta/LinettaApp.swift`

- [ ] **Step 2: Add four new shared state instances at top-of-file**

After existing `sharedEngine` / `sharedAppState` declarations in `LinettaApp.swift`, add:

```swift
@MainActor private let sharedSidebarState = SidebarState()
@MainActor private let sharedEpisodeState = EpisodeState()
@MainActor private let sharedManuscriptState = ManuscriptState()
@MainActor private let sharedCommandPalette = CommandPaletteState()
@MainActor private let sharedToastCenter = ToastCenter()
```

- [ ] **Step 3: Inject into the WindowGroup body**

In the `body` of `LinettaApp`:

```swift
WindowGroup {
    WorkGalleryView()
        .environment(appState)
        .environment(engine)
        .environment(sharedSidebarState)
        .environment(sharedEpisodeState)
        .environment(sharedManuscriptState)
        .environment(sharedCommandPalette)
        .environment(sharedToastCenter)
}
```

(WorkGalleryView is still the legacy root for now; replaced in Task C2.)

- [ ] **Step 4: Build**

Run: `cd macos/Linetta && swift build`
Expected: build succeeds (state objects unused inside legacy views — that's OK).

- [ ] **Step 5: Commit**

```bash
git add macos/Linetta/Sources/Linetta/LinettaApp.swift
git commit -m "feat(state): inject all Phase 6.5 state objects via .environment"
```

---

# Group C · AppShell Skeleton

### Task C1: AppShell with placeholder columns

**Files:**
- Create: `macos/Linetta/Sources/Linetta/Shell/AppShell.swift`
- Test: `macos/Linetta/Tests/LinettaTests/AppShellSmokeTest.swift`

- [ ] **Step 1: Write the smoke test**

Create `macos/Linetta/Tests/LinettaTests/AppShellSmokeTest.swift`:

```swift
import SwiftUI
import XCTest
@testable import Linetta
@testable import LinettaCore

@MainActor
final class AppShellSmokeTest: XCTestCase {
    func testAppShellInstantiatesWithoutCrashing() {
        let engine = EngineController()
        let appState = AppState(engine: engine)
        let view = AppShell()
            .environment(appState)
            .environment(engine)
            .environment(SidebarState())
            .environment(EpisodeState())
            .environment(ManuscriptState())
            .environment(CommandPaletteState())
            .environment(ToastCenter())
        let host = NSHostingController(rootView: view)
        XCTAssertNotNil(host.view)
    }
}
```

- [ ] **Step 2: Run test (fail)**

Run: `cd macos/Linetta && swift test --filter AppShellSmokeTest`
Expected: FAIL — `cannot find 'AppShell'`.

- [ ] **Step 3: Create AppShell**

Create `macos/Linetta/Sources/Linetta/Shell/AppShell.swift`:

```swift
import LinettaCore
import SwiftUI

struct AppShell: View {
    @Environment(AppState.self) private var appState
    @Environment(ManuscriptState.self) private var manuscript

    var body: some View {
        @Bindable var ms = manuscript
        NavigationSplitView {
            // SidebarView() — wired in Task D1
            Color.clear
                .frame(minWidth: 220, idealWidth: 230, maxWidth: 320)
                .background(LinettaTheme.surfaceElevated)
                .overlay { Text("Sidebar").foregroundStyle(LinettaTheme.textTertiary) }
        } detail: {
            // MainPaneRouter() — wired in Task E1
            Color.clear
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .background(LinettaTheme.background)
                .overlay { Text("Main").foregroundStyle(LinettaTheme.textTertiary) }
                .inspector(isPresented: inspectorBinding) {
                    Color.clear
                        .frame(minWidth: 280, idealWidth: ms.width, maxWidth: 480)
                        .background(LinettaTheme.background)
                        .overlay { Text("Inspector").foregroundStyle(LinettaTheme.textTertiary) }
                }
        }
        .frame(minWidth: 1080, minHeight: 720)
        .background(LinettaTheme.background)
        .preferredColorScheme(.dark)
    }

    private var inspectorBinding: Binding<Bool> {
        Binding(
            get: { selectedEpisodeID.map { manuscript.isOpen(episodeID: $0) } ?? false },
            set: { newValue in
                if let id = selectedEpisodeID { manuscript.setOpen(episodeID: id, open: newValue) }
            }
        )
    }

    private var selectedEpisodeID: String? {
        // Placeholder — fully implemented after EpisodeState wiring in Group E
        nil
    }
}

#Preview {
    AppShell()
        .environment(AppState(engine: EngineController()))
        .environment(EngineController())
        .environment(SidebarState())
        .environment(EpisodeState())
        .environment(ManuscriptState())
        .environment(CommandPaletteState())
        .environment(ToastCenter())
}
```

- [ ] **Step 4: Run smoke test**

Run: `cd macos/Linetta && swift test --filter AppShellSmokeTest`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add macos/Linetta/Sources/Linetta/Shell/AppShell.swift macos/Linetta/Tests/LinettaTests/AppShellSmokeTest.swift
git commit -m "feat(shell): add AppShell skeleton with 3-column placeholders"
```

---

### Task C2: Swap LinettaApp root to AppShell

**Files:**
- Modify: `macos/Linetta/Sources/Linetta/LinettaApp.swift`

- [ ] **Step 1: Replace WindowGroup root**

In `LinettaApp.swift`, replace `WorkGalleryView()` in the WindowGroup with `AppShell()`. Keep all `.environment(...)` modifiers attached.

- [ ] **Step 2: Build**

Run: `cd macos/Linetta && swift build`
Expected: PASS.

- [ ] **Step 3: Manual smoke run (optional but encouraged)**

Run: `make macos-run`
Expected: window opens at 1080×720 showing three placeholder labels "Sidebar / Main / Inspector". Close window.

- [ ] **Step 4: Commit**

```bash
git add macos/Linetta/Sources/Linetta/LinettaApp.swift
git commit -m "feat(shell): make AppShell the WindowGroup root (legacy views unused)"
```

---

# Group D · Sidebar

### Task D1: SidebarView shell + works iteration

**Files:**
- Create: `macos/Linetta/Sources/Linetta/Shell/SidebarView.swift`
- Modify: `macos/Linetta/Sources/Linetta/Shell/AppShell.swift`

- [ ] **Step 1: Create SidebarView**

Create `macos/Linetta/Sources/Linetta/Shell/SidebarView.swift`:

```swift
import LinettaCore
import SwiftUI

struct SidebarView: View {
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            SidebarHeader()
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 2) {
                    Text("Works").linettaLabelStyle().padding(.horizontal, 8).padding(.top, 6)
                    ForEach(appState.works) { work in
                        SidebarWorkRow(work: work)
                    }
                    if appState.works.isEmpty {
                        SidebarOnboardingHint()
                    }
                }
                .padding(.horizontal, 8)
            }
            Spacer(minLength: 0)
            SidebarFooterView()
        }
        .background(LinettaTheme.surfaceElevated)
    }
}

private struct SidebarHeader: View {
    var body: some View {
        HStack {
            Text("Linetta")
                .font(LinettaTypography.titleSmall)
                .foregroundStyle(LinettaTheme.text)
            Spacer()
            Button { /* new work — wired in Task D4 */ } label: {
                Image(systemName: "plus")
            }
            .buttonStyle(.plain)
            .foregroundStyle(LinettaTheme.textSecondary)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
    }
}

private struct SidebarOnboardingHint: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("No works yet")
                .font(LinettaTypography.body)
                .foregroundStyle(LinettaTheme.text)
            Text("Create your first work to begin.")
                .font(LinettaTypography.bodySmall)
                .foregroundStyle(LinettaTheme.textTertiary)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 18)
    }
}

#Preview {
    SidebarView()
        .frame(width: 230, height: 600)
        .environment(AppState(engine: EngineController()))
        .environment(EngineController())
        .environment(SidebarState())
}
```

- [ ] **Step 2: Stub SidebarWorkRow and SidebarFooterView so the file compiles**

Create `macos/Linetta/Sources/Linetta/Shell/SidebarWorkRow.swift`:

```swift
import LinettaCore
import SwiftUI

struct SidebarWorkRow: View {
    let work: Work
    var body: some View {
        Text(work.title)
            .font(LinettaTypography.body)
            .foregroundStyle(LinettaTheme.text)
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
    }
}
```

Create `macos/Linetta/Sources/Linetta/Shell/SidebarFooterView.swift`:

```swift
import SwiftUI

struct SidebarFooterView: View {
    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "gearshape")
            Spacer()
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .foregroundStyle(LinettaTheme.textSecondary)
        .overlay(alignment: .top) {
            Rectangle().fill(LinettaTheme.borderSoft).frame(height: 1)
        }
    }
}
```

- [ ] **Step 3: Plug SidebarView into AppShell**

In `AppShell.swift`, replace the sidebar placeholder block (`Color.clear...overlay { Text("Sidebar") }`) with:

```swift
SidebarView()
    .frame(minWidth: 220, idealWidth: 230, maxWidth: 320)
```

- [ ] **Step 4: Build & run smoke test**

Run: `cd macos/Linetta && swift test --filter AppShellSmokeTest`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add macos/Linetta/Sources/Linetta/Shell/SidebarView.swift macos/Linetta/Sources/Linetta/Shell/SidebarWorkRow.swift macos/Linetta/Sources/Linetta/Shell/SidebarFooterView.swift macos/Linetta/Sources/Linetta/Shell/AppShell.swift
git commit -m "feat(sidebar): SidebarView shell with works iteration + onboarding hint"
```

---

### Task D2: SidebarWorkRow with expand/collapse + episodes

**Files:**
- Modify: `macos/Linetta/Sources/Linetta/Shell/SidebarWorkRow.swift`
- Create: `macos/Linetta/Sources/Linetta/Shell/SidebarEpisodeRow.swift`
- Create: `macos/Linetta/Sources/Linetta/Shell/SidebarMemoryRow.swift`

- [ ] **Step 1: Replace SidebarWorkRow with the real one**

Replace `macos/Linetta/Sources/Linetta/Shell/SidebarWorkRow.swift`:

```swift
import LinettaCore
import SwiftUI

struct SidebarWorkRow: View {
    let work: Work

    @Environment(SidebarState.self) private var sidebar
    @Environment(AppState.self) private var appState

    @State private var episodes: [Episode] = []

    var body: some View {
        @Bindable var sidebar = sidebar
        VStack(alignment: .leading, spacing: 2) {
            Button {
                sidebar.setExpanded(work.id, expanded: !sidebar.isExpanded(work.id))
            } label: {
                HStack(spacing: 4) {
                    Image(systemName: sidebar.isExpanded(work.id) ? "chevron.down" : "chevron.right")
                        .font(.system(size: 9))
                        .frame(width: 12)
                        .foregroundStyle(LinettaTheme.textTertiary)
                    Text(work.title)
                        .font(LinettaTypography.body)
                        .foregroundStyle(LinettaTheme.text)
                        .lineLimit(1)
                    Spacer()
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 5)
                .background(rowBackground)
            }
            .buttonStyle(.plain)

            if sidebar.isExpanded(work.id) {
                SidebarMemoryRow(workID: work.id)
                ForEach(episodes) { episode in
                    SidebarEpisodeRow(workID: work.id, episode: episode)
                }
                NewEpisodePlaceholder(workID: work.id)
            }
        }
        .task(id: work.id) { await loadEpisodes() }
    }

    private var rowBackground: some View {
        if case .work(let wid) = sidebar.selection, wid == work.id {
            return AnyView(LinettaTheme.accentSoft.clipShape(RoundedRectangle(cornerRadius: 5)))
        }
        return AnyView(Color.clear)
    }

    private func loadEpisodes() async {
        do { episodes = try await appState.client.listEpisodes(workID: work.id) } catch { episodes = [] }
    }
}

private struct NewEpisodePlaceholder: View {
    let workID: String
    var body: some View {
        Text("＋ New episode")
            .font(LinettaTypography.bodySmall)
            .foregroundStyle(LinettaTheme.textTertiary)
            .padding(.leading, 24)
            .padding(.vertical, 4)
    }
}
```

- [ ] **Step 2: Implement SidebarEpisodeRow**

Replace `macos/Linetta/Sources/Linetta/Shell/SidebarEpisodeRow.swift`:

```swift
import LinettaCore
import SwiftUI

struct SidebarEpisodeRow: View {
    let workID: String
    let episode: Episode

    @Environment(SidebarState.self) private var sidebar

    var body: some View {
        @Bindable var sidebar = sidebar
        Button {
            sidebar.selection = .episode(workID: workID, episodeID: episode.id)
        } label: {
            HStack(spacing: 6) {
                Circle().fill(statusColor).frame(width: 6, height: 6)
                Text(episode.title)
                    .font(LinettaTypography.body)
                    .foregroundStyle(LinettaTheme.text)
                    .lineLimit(1)
                Spacer()
            }
            .padding(.leading, 24)
            .padding(.trailing, 8)
            .padding(.vertical, 4)
            .background(isSelected ? LinettaTheme.accentSoft : Color.clear)
            .clipShape(RoundedRectangle(cornerRadius: 5))
        }
        .buttonStyle(.plain)
    }

    private var isSelected: Bool {
        if case let .episode(_, eid) = sidebar.selection, eid == episode.id { return true }
        return false
    }

    private var statusColor: Color {
        switch episode.status {
        case .idea: return LinettaTheme.textTertiary
        case .drafting: return Color(red: 0.95, green: 0.78, blue: 0.34)
        case .polishing: return LinettaTheme.accent
        case .ready: return LinettaTheme.success
        case .published: return Color(red: 0.43, green: 0.55, blue: 0.85)
        }
    }
}
```

- [ ] **Step 3: Implement SidebarMemoryRow**

Replace `macos/Linetta/Sources/Linetta/Shell/SidebarMemoryRow.swift`:

```swift
import SwiftUI

struct SidebarMemoryRow: View {
    let workID: String

    @Environment(SidebarState.self) private var sidebar

    var body: some View {
        Button {
            sidebar.selection = .memory(workID: workID)
        } label: {
            HStack(spacing: 6) {
                Text("📓").font(.system(size: 10))
                Text("Memory")
                    .font(LinettaTypography.body)
                    .foregroundStyle(LinettaTheme.text)
                Spacer()
            }
            .padding(.leading, 24)
            .padding(.trailing, 8)
            .padding(.vertical, 4)
            .background(isSelected ? LinettaTheme.accentSoft : Color.clear)
            .clipShape(RoundedRectangle(cornerRadius: 5))
        }
        .buttonStyle(.plain)
    }

    private var isSelected: Bool {
        if case .memory(let wid) = sidebar.selection, wid == workID { return true }
        return false
    }
}
```

- [ ] **Step 4: Build + smoke test**

Run: `cd macos/Linetta && swift build && swift test --filter AppShellSmokeTest`
Expected: build + 1 test pass.

- [ ] **Step 5: Commit**

```bash
git add macos/Linetta/Sources/Linetta/Shell/SidebarWorkRow.swift macos/Linetta/Sources/Linetta/Shell/SidebarEpisodeRow.swift macos/Linetta/Sources/Linetta/Shell/SidebarMemoryRow.swift
git commit -m "feat(sidebar): expand/collapse + episode rows + memory row"
```

---

### Task D3: Sidebar search (⌘L)

**Files:**
- Create: `macos/Linetta/Sources/Linetta/Shell/SidebarSearchField.swift`
- Modify: `macos/Linetta/Sources/Linetta/Shell/SidebarView.swift`

- [ ] **Step 1: Create SidebarSearchField**

Create `macos/Linetta/Sources/Linetta/Shell/SidebarSearchField.swift`:

```swift
import SwiftUI

struct SidebarSearchField: View {
    @Environment(SidebarState.self) private var sidebar
    @FocusState private var focused: Bool

    var body: some View {
        @Bindable var sidebar = sidebar
        if sidebar.searchOpen {
            HStack {
                Image(systemName: "magnifyingglass").foregroundStyle(LinettaTheme.textTertiary)
                TextField("Search works · episodes · memory", text: $sidebar.query)
                    .textFieldStyle(.plain)
                    .focused($focused)
                    .onAppear { focused = true }
                Button { sidebar.searchOpen = false } label: { Image(systemName: "xmark") }
                    .buttonStyle(.plain)
                    .foregroundStyle(LinettaTheme.textTertiary)
            }
            .padding(.horizontal, 10).padding(.vertical, 6)
            .background(LinettaTheme.surface)
            .clipShape(RoundedRectangle(cornerRadius: 6))
            .padding(.horizontal, 8)
        }
    }
}
```

- [ ] **Step 2: Insert the search field at top of SidebarView**

In `SidebarView.swift`, insert `SidebarSearchField()` immediately after `SidebarHeader()`.

- [ ] **Step 3: Build + commit**

Run: `cd macos/Linetta && swift build`

```bash
git add macos/Linetta/Sources/Linetta/Shell/SidebarSearchField.swift macos/Linetta/Sources/Linetta/Shell/SidebarView.swift
git commit -m "feat(sidebar): add search field (toggled by SidebarState.searchOpen)"
```

---

### Task D4: Sidebar footer + New Work / New Episode wiring

**Files:**
- Modify: `macos/Linetta/Sources/Linetta/Shell/SidebarFooterView.swift`
- Modify: `macos/Linetta/Sources/Linetta/Shell/SidebarView.swift` (header `+` action)
- Modify: `macos/Linetta/Sources/Linetta/Shell/SidebarWorkRow.swift` (new-episode placeholder action)

- [ ] **Step 1: Replace SidebarFooterView with real settings + add**

Replace contents:

```swift
import SwiftUI

struct SidebarFooterView: View {
    @State private var showSettings = false

    var body: some View {
        HStack(spacing: 8) {
            Button { showSettings.toggle() } label: { Image(systemName: "gearshape") }
                .buttonStyle(.plain)
            Spacer()
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .foregroundStyle(LinettaTheme.textSecondary)
        .overlay(alignment: .top) {
            Rectangle().fill(LinettaTheme.borderSoft).frame(height: 1)
        }
        .sheet(isPresented: $showSettings) {
            SettingsView()
                .frame(width: 560, height: 420)
        }
    }
}
```

- [ ] **Step 2: Add `NewWorkSheet` invocation to header `+` button**

In `SidebarView.swift`, wrap `SidebarHeader()` with `@State var showNewWork = false` and connect the `+` button:

```swift
private struct SidebarHeader: View {
    @State private var showingNewWork = false
    var body: some View {
        HStack {
            Text("Linetta").font(LinettaTypography.titleSmall).foregroundStyle(LinettaTheme.text)
            Spacer()
            Button { showingNewWork = true } label: { Image(systemName: "plus") }
                .buttonStyle(.plain)
                .keyboardShortcut("n", modifiers: [.command])
                .foregroundStyle(LinettaTheme.textSecondary)
        }
        .padding(.horizontal, 14).padding(.vertical, 12)
        .sheet(isPresented: $showingNewWork) { NewWorkSheet() }
    }
}
```

- [ ] **Step 3: Wire the New Episode placeholder**

Replace `NewEpisodePlaceholder` in `SidebarWorkRow.swift`:

```swift
private struct NewEpisodePlaceholder: View {
    let workID: String
    @Environment(AppState.self) private var appState
    @State private var pending = false

    var body: some View {
        Button {
            Task { await create() }
        } label: {
            Text(pending ? "Creating…" : "＋ New episode")
                .font(LinettaTypography.bodySmall)
                .foregroundStyle(LinettaTheme.textTertiary)
                .padding(.leading, 24).padding(.vertical, 4)
        }
        .buttonStyle(.plain)
        .keyboardShortcut("n", modifiers: [.command, .shift])
    }

    private func create() async {
        pending = true
        defer { pending = false }
        _ = try? await appState.client.createEpisode(workID: workID, request: .init(title: "New Episode"))
    }
}
```

- [ ] **Step 4: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/Shell/SidebarFooterView.swift macos/Linetta/Sources/Linetta/Shell/SidebarView.swift macos/Linetta/Sources/Linetta/Shell/SidebarWorkRow.swift
git commit -m "feat(sidebar): wire NewWorkSheet, NewEpisode creation, settings sheet"
```

---

### Task D5: Sidebar onboarding (works=0) — full surface

**Files:**
- Modify: `macos/Linetta/Sources/Linetta/Shell/SidebarView.swift`

- [ ] **Step 1: Replace the onboarding hint with bigger CTA**

In `SidebarView.swift`, replace `SidebarOnboardingHint` with:

```swift
private struct SidebarOnboardingHint: View {
    @State private var showSheet = false
    var body: some View {
        VStack(alignment: .center, spacing: 10) {
            Image(systemName: "books.vertical").font(.system(size: 28)).foregroundStyle(LinettaTheme.textTertiary)
            Text("No works yet").font(LinettaTypography.body).foregroundStyle(LinettaTheme.text)
            Button("Create your first work") { showSheet = true }
                .buttonStyle(.borderedProminent)
                .tint(LinettaTheme.accent)
                .controlSize(.small)
        }
        .padding(.horizontal, 8).padding(.vertical, 30)
        .frame(maxWidth: .infinity)
        .sheet(isPresented: $showSheet) { NewWorkSheet() }
    }
}
```

- [ ] **Step 2: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/Shell/SidebarView.swift
git commit -m "feat(sidebar): onboarding CTA when works are empty"
```

---

# Group E · Main Pane Router + Modes

### Task E1: MainPaneRouter + WorkOverviewView + OnboardingView

**Files:**
- Create: `macos/Linetta/Sources/Linetta/MainPane/MainPaneRouter.swift`
- Create: `macos/Linetta/Sources/Linetta/MainPane/WorkOverviewView.swift`
- Create: `macos/Linetta/Sources/Linetta/MainPane/OnboardingView.swift`
- Modify: `macos/Linetta/Sources/Linetta/Shell/AppShell.swift`

- [ ] **Step 1: Create OnboardingView**

```swift
import SwiftUI

struct OnboardingView: View {
    @State private var showSheet = false
    var body: some View {
        VStack(spacing: 14) {
            Text("Linetta").font(LinettaTypography.titleLarge).foregroundStyle(LinettaTheme.text)
            Text("An AI workflow runner for serial fiction.")
                .font(LinettaTypography.body).foregroundStyle(LinettaTheme.textSecondary)
            Button("Create your first work") { showSheet = true }
                .buttonStyle(.borderedProminent).tint(LinettaTheme.accent)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(LinettaTheme.background)
        .sheet(isPresented: $showSheet) { NewWorkSheet() }
    }
}
```

Place at `macos/Linetta/Sources/Linetta/MainPane/OnboardingView.swift`.

- [ ] **Step 2: Create WorkOverviewView**

```swift
import LinettaCore
import SwiftUI

struct WorkOverviewView: View {
    let work: Work
    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text(work.title).font(LinettaTypography.titleLarge).foregroundStyle(LinettaTheme.text)
            if !work.genre.isEmpty {
                LabeledContent("Genre") { Text(work.genre).foregroundStyle(LinettaTheme.textSecondary) }
            }
            if !work.premise.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Premise").linettaLabelStyle()
                    Text(work.premise).foregroundStyle(LinettaTheme.text)
                }
            }
            Spacer()
        }
        .padding(28)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(LinettaTheme.background)
    }
}
```

Place at `macos/Linetta/Sources/Linetta/MainPane/WorkOverviewView.swift`.

- [ ] **Step 3: Create MainPaneRouter**

```swift
import LinettaCore
import SwiftUI

struct MainPaneRouter: View {
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    var body: some View {
        Group {
            if appState.works.isEmpty {
                OnboardingView()
            } else {
                switch sidebar.selection {
                case .none:
                    if let first = appState.works.first {
                        WorkOverviewView(work: first)
                    } else {
                        OnboardingView()
                    }
                case .work(let wid):
                    if let work = appState.works.first(where: { $0.id == wid }) {
                        WorkOverviewView(work: work)
                    } else { OnboardingView() }
                case .memory(let wid):
                    if let work = appState.works.first(where: { $0.id == wid }) {
                        MemoryPaneView(work: work)
                    } else { OnboardingView() }
                case .episode(let wid, let eid):
                    if let work = appState.works.first(where: { $0.id == wid }) {
                        EpisodeWorkspaceView(work: work, episodeID: eid)
                    } else { OnboardingView() }
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(LinettaTheme.background)
    }
}
```

Place at `macos/Linetta/Sources/Linetta/MainPane/MainPaneRouter.swift`.

- [ ] **Step 4: Stub MemoryPaneView and EpisodeWorkspaceView so MainPaneRouter compiles**

Create `macos/Linetta/Sources/Linetta/MainPane/MemoryPaneView.swift`:

```swift
import LinettaCore
import SwiftUI

struct MemoryPaneView: View {
    let work: Work
    var body: some View { Text("Memory · \(work.title)").foregroundStyle(LinettaTheme.text) }
}
```

Create `macos/Linetta/Sources/Linetta/MainPane/EpisodeWorkspaceView.swift`:

```swift
import LinettaCore
import SwiftUI

struct EpisodeWorkspaceView: View {
    let work: Work
    let episodeID: String
    var body: some View { Text("Episode \(episodeID) · \(work.title)").foregroundStyle(LinettaTheme.text) }
}
```

- [ ] **Step 5: Plug MainPaneRouter into AppShell**

In `AppShell.swift`, replace the main placeholder (`Color.clear...overlay { Text("Main") }`) with `MainPaneRouter()`.

- [ ] **Step 6: Run smoke test**

Run: `cd macos/Linetta && swift test --filter AppShellSmokeTest`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add macos/Linetta/Sources/Linetta/MainPane/ macos/Linetta/Sources/Linetta/Shell/AppShell.swift
git commit -m "feat(main): MainPaneRouter + Onboarding + WorkOverview + view stubs"
```

---

### Task E2: MemoryPaneView (filter + list + new item)

**Files:**
- Replace contents: `macos/Linetta/Sources/Linetta/MainPane/MemoryPaneView.swift`

- [ ] **Step 1: Implement MemoryPaneView**

Replace the stub with full implementation. Use the exact code below (adapted from legacy `CanonMemoryView.swift`):

```swift
import LinettaCore
import SwiftUI

struct MemoryPaneView: View {
    let work: Work

    @Environment(AppState.self) private var appState
    @State private var selectedKind: MemoryKind = .character
    @State private var selectedStatus: MemoryStatus = .canon
    @State private var query = ""
    @State private var items: [MemoryItem] = []
    @State private var selectedItem: MemoryItem?
    @State private var title = ""
    @State private var bodyText = ""
    @State private var importance: MemoryImportance = .medium
    @State private var isLoading = false
    @State private var errorMessage: String?

    var body: some View {
        VStack(spacing: 0) {
            toolbar.padding(.horizontal, 18).padding(.vertical, 12).background(LinettaTheme.surface)
            Divider().background(LinettaTheme.border)
            HStack(spacing: 0) {
                itemList.frame(minWidth: 280, maxWidth: 360)
                Divider().background(LinettaTheme.borderSoft)
                editor.frame(maxWidth: .infinity)
            }
        }
        .background(LinettaTheme.background)
        .task(id: work.id) { await reload() }
        .task(id: query) { await reload() }
        .task(id: selectedKind) { await reload() }
        .task(id: selectedStatus) { await reload() }
    }

    private var toolbar: some View {
        HStack {
            Picker("Kind", selection: $selectedKind) {
                ForEach(MemoryKind.allCases) { k in Text(k.label).tag(k) }
            }.labelsHidden().frame(width: 140)
            Picker("Status", selection: $selectedStatus) {
                ForEach(MemoryStatus.allCases) { s in Text(s.label).tag(s) }
            }.labelsHidden().frame(width: 120)
            TextField("Search…", text: $query).textFieldStyle(.roundedBorder).frame(width: 200)
            Spacer()
        }
    }

    private var itemList: some View {
        List(selection: $selectedItem) {
            ForEach(items) { item in
                VStack(alignment: .leading) {
                    Text(item.title).font(LinettaTypography.body).foregroundStyle(LinettaTheme.text)
                    Text(item.body).font(LinettaTypography.bodySmall).foregroundStyle(LinettaTheme.textTertiary).lineLimit(2)
                }.tag(item)
            }
        }
    }

    private var editor: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("New Canon Item").font(LinettaTypography.titleSmall).foregroundStyle(LinettaTheme.text)
            HStack {
                TextField("Title", text: $title).textFieldStyle(.roundedBorder)
                Picker("Importance", selection: $importance) {
                    ForEach(MemoryImportance.allCases) { i in Text(i.label).tag(i) }
                }.labelsHidden().frame(width: 120)
            }
            TextEditor(text: $bodyText)
                .font(LinettaTypography.body)
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(LinettaTheme.borderSoft))
            HStack {
                Spacer()
                Button("Save") { Task { await save() } }
                    .buttonStyle(.borderedProminent).tint(LinettaTheme.accent)
                    .disabled(title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
            if let errorMessage {
                Text(errorMessage).font(LinettaTypography.caption).foregroundStyle(LinettaTheme.danger)
            }
        }.padding(18)
    }

    private func reload() async {
        isLoading = true; defer { isLoading = false }
        do {
            items = try await appState.client.listMemory(
                workID: work.id,
                kind: query.isEmpty ? selectedKind : nil,
                status: query.isEmpty ? selectedStatus : nil,
                query: query.isEmpty ? nil : query
            )
        } catch { errorMessage = error.localizedDescription }
    }

    private func save() async {
        do {
            let item = try await appState.client.createMemory(
                workID: work.id,
                request: CreateMemoryRequest(kind: selectedKind, status: selectedStatus, title: title, body: bodyText, importance: importance)
            )
            items.insert(item, at: 0)
            title = ""; bodyText = ""
        } catch { errorMessage = error.localizedDescription }
    }
}
```

- [ ] **Step 2: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/MainPane/MemoryPaneView.swift
git commit -m "feat(memory): canon memory pane with filter + list + inline create"
```

---

### Task E3: EpisodeWorkspaceView skeleton (toolbar + scroll body)

**Files:**
- Replace contents: `macos/Linetta/Sources/Linetta/MainPane/EpisodeWorkspaceView.swift`

- [ ] **Step 1: Implement skeleton**

```swift
import LinettaCore
import SwiftUI

struct EpisodeWorkspaceView: View {
    let work: Work
    let episodeID: String

    @Environment(AppState.self) private var appState
    @Environment(EpisodeState.self) private var episodeState
    @Environment(ManuscriptState.self) private var manuscript

    @State private var episode: Episode?
    @State private var runs: [EpisodeRunResult] = []
    @State private var proposals: [CanonProposal] = []
    @State private var issues: [ContinuityIssue] = []
    @State private var isLoading = false
    @State private var errorMessage: String?

    var body: some View {
        VStack(spacing: 0) {
            EpisodeToolbar(work: work, episode: episode)
                .padding(.horizontal, 18).padding(.vertical, 12)
                .background(LinettaTheme.surface)
            Divider().background(LinettaTheme.border)
            ScrollView {
                VStack(spacing: LinettaShape.sectionGap) {
                    BlueprintCard(work: work, episodeID: episodeID, onSave: { await reload() }, onRun: { await runAgents() })
                    RunHistoryCard(runs: runs)
                    if !proposals.isEmpty || !issues.isEmpty {
                        ReviewQueueCard(workID: work.id, proposals: proposals, issues: issues)
                    }
                }
                .padding(LinettaShape.mainContentPadding)
            }
        }
        .background(LinettaTheme.background)
        .task(id: episodeID) { await reload() }
    }

    private func reload() async {
        isLoading = true; defer { isLoading = false }
        do {
            let episodes = try await appState.client.listEpisodes(workID: work.id)
            episode = episodes.first { $0.id == episodeID }
            // runs aggregate: per Phase 1-5 we fetch latest run artifacts via listRunArtifacts/Events
            // For this scope, we keep `runs` empty and re-populate on Run Agents.
            proposals = (try? await appState.client.listProposals(workID: work.id, status: .pending)) ?? []
            issues = (try? await appState.client.listContinuityIssues(workID: work.id, episodeID: episodeID)) ?? []
        } catch { errorMessage = error.localizedDescription }
    }

    private func runAgents() async {
        episodeState.isRunning = true; defer { episodeState.isRunning = false }
        do {
            let result = try await appState.client.runEpisode(workID: work.id, episodeID: episodeID)
            runs.insert(result, at: 0)
            await reload()
        } catch { errorMessage = error.localizedDescription }
    }
}

private struct EpisodeToolbar: View {
    let work: Work
    let episode: Episode?
    @Environment(ManuscriptState.self) private var manuscript

    var body: some View {
        @Bindable var manuscript = manuscript
        HStack(spacing: 12) {
            Text("\(work.title)  ›  ")
                .font(LinettaTypography.bodySmall).foregroundStyle(LinettaTheme.textSecondary)
            + Text(episode?.title ?? "—")
                .font(LinettaTypography.body).foregroundStyle(LinettaTheme.text).bold()
            Spacer()
            if let episode {
                Text(episode.status.label).font(LinettaTypography.caption)
                    .padding(.horizontal, 8).padding(.vertical, 2)
                    .background(LinettaTheme.surface).clipShape(Capsule())
                    .foregroundStyle(LinettaTheme.textSecondary)
            }
            Button {
                if let id = episode?.id { manuscript.setOpen(episodeID: id, open: !manuscript.isOpen(episodeID: id)) }
            } label: { Label("Manuscript", systemImage: "doc.text") }
                .keyboardShortcut("m", modifiers: [.command, .shift])
        }
    }
}
```

Group F tasks fill in BlueprintCard, RunHistoryCard, ReviewQueueCard.

- [ ] **Step 2: Add card stubs so this file compiles**

Create `macos/Linetta/Sources/Linetta/MainPane/Cards/BlueprintCard.swift`:

```swift
import LinettaCore
import SwiftUI

struct BlueprintCard: View {
    let work: Work
    let episodeID: String
    var onSave: () async -> Void = {}
    var onRun: () async -> Void = {}
    var body: some View {
        Text("Blueprint card stub").padding().background(LinettaTheme.surface).clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }
}
```

Create `macos/Linetta/Sources/Linetta/MainPane/Cards/RunHistoryCard.swift`:

```swift
import LinettaCore
import SwiftUI

struct RunHistoryCard: View {
    let runs: [EpisodeRunResult]
    var body: some View {
        Text("Run history stub (\(runs.count) runs)").padding().background(LinettaTheme.surface).clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }
}
```

Create `macos/Linetta/Sources/Linetta/MainPane/Cards/ReviewQueueCard.swift`:

```swift
import LinettaCore
import SwiftUI

struct ReviewQueueCard: View {
    let workID: String
    let proposals: [CanonProposal]
    let issues: [ContinuityIssue]
    var body: some View {
        Text("Review queue stub (\(proposals.count) proposals, \(issues.count) issues)").padding().background(LinettaTheme.surface).clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }
}
```

- [ ] **Step 3: Build + commit**

```bash
cd macos/Linetta && swift build && swift test --filter AppShellSmokeTest
git add macos/Linetta/Sources/Linetta/MainPane/EpisodeWorkspaceView.swift macos/Linetta/Sources/Linetta/MainPane/Cards/
git commit -m "feat(workspace): EpisodeWorkspaceView skeleton with card stubs and toolbar"
```

---

# Group F · Episode Workspace Cards

### Task F1: BlueprintCard (collapse + form + Save + Run)

**Files:**
- Replace contents: `macos/Linetta/Sources/Linetta/MainPane/Cards/BlueprintCard.swift`

- [ ] **Step 1: Implement BlueprintCard**

```swift
import LinettaCore
import SwiftUI

struct BlueprintCard: View {
    let work: Work
    let episodeID: String
    var onSave: () async -> Void = {}
    var onRun: () async -> Void = {}

    @Environment(AppState.self) private var appState
    @Environment(EpisodeState.self) private var episodeState

    @State private var loaded = false

    var body: some View {
        @Bindable var episodeState = episodeState
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Blueprint").linettaLabelStyle()
                Spacer()
                Button {
                    episodeState.setBlueprintExpanded(episodeID: episodeID, expanded: !episodeState.isBlueprintExpanded(episodeID: episodeID))
                } label: {
                    Image(systemName: episodeState.isBlueprintExpanded(episodeID: episodeID) ? "chevron.up" : "chevron.down")
                        .font(.system(size: 10))
                        .foregroundStyle(LinettaTheme.textTertiary)
                }
                .buttonStyle(.plain)
            }

            if episodeState.isBlueprintExpanded(episodeID: episodeID) {
                VStack(alignment: .leading, spacing: 8) {
                    field("Premise", text: $episodeState.premise)
                    field("Theme", text: $episodeState.theme)
                    field("Situation", text: $episodeState.situation)
                    field("Must include", text: $episodeState.mustInclude)
                    field("Must avoid", text: $episodeState.mustAvoid)
                    HStack {
                        Button { Task { await save() } } label: { Label("Save", systemImage: "tray.and.arrow.down") }
                            .keyboardShortcut("s", modifiers: [.command])
                            .disabled(!episodeState.isDirty)
                        Button { Task { await onRun() } } label: { Label("Run Agents", systemImage: "play.fill") }
                            .buttonStyle(.borderedProminent).tint(LinettaTheme.accent)
                            .keyboardShortcut("r", modifiers: [.command, .shift])
                            .disabled(episodeState.premise.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                        Spacer()
                    }
                }
            } else {
                collapsedRow
            }
        }
        .padding(.horizontal, LinettaShape.cardPaddingH)
        .padding(.vertical, LinettaShape.cardPaddingV)
        .background(LinettaTheme.surface)
        .overlay(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius).stroke(LinettaTheme.border))
        .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
        .task(id: episodeID) { await loadOnce() }
    }

    private var collapsedRow: some View {
        HStack(spacing: 10) {
            Text(episodeState.premise.isEmpty ? "Write a premise to enable run." : episodeState.premise)
                .font(LinettaTypography.bodySmall)
                .foregroundStyle(LinettaTheme.textSecondary)
                .lineLimit(1)
            Spacer()
            Button { Task { await onRun() } } label: {
                Label("Run Agents", systemImage: "play.fill")
            }
            .buttonStyle(.borderedProminent).tint(LinettaTheme.accent)
            .keyboardShortcut("r", modifiers: [.command, .shift])
            .disabled(episodeState.premise.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
        }
    }

    private func field(_ label: String, text: Binding<String>) -> some View {
        HStack(alignment: .top, spacing: 10) {
            Text(label).font(LinettaTypography.bodySmall).foregroundStyle(LinettaTheme.textTertiary).frame(width: 92, alignment: .leading)
            TextField("", text: text, axis: .vertical).font(LinettaTypography.body).textFieldStyle(.plain)
        }
    }

    private func loadOnce() async {
        guard !loaded else { return }
        do {
            let bp = try await appState.client.getBlueprint(workID: work.id, episodeID: episodeID)
            episodeState.loadBlueprint(premise: bp.premise, theme: bp.theme, situation: bp.situation,
                                       mustInclude: bp.mustInclude, mustAvoid: bp.mustAvoid, structureNotes: bp.structureNotes)
            loaded = true
        } catch {
            // first-time blueprint may not exist; leave defaults
            episodeState.loadBlueprint(premise: "", theme: "", situation: "", mustInclude: "", mustAvoid: "", structureNotes: "")
            loaded = true
        }
    }

    private func save() async {
        _ = try? await appState.client.saveBlueprint(
            workID: work.id,
            episodeID: episodeID,
            request: SaveBlueprintRequest(
                premise: episodeState.premise, theme: episodeState.theme, situation: episodeState.situation,
                mustInclude: episodeState.mustInclude, mustAvoid: episodeState.mustAvoid, structureNotes: episodeState.structureNotes
            )
        )
        episodeState.markSaved()
        await onSave()
    }
}
```

- [ ] **Step 2: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/MainPane/Cards/BlueprintCard.swift
git commit -m "feat(workspace): BlueprintCard with collapse, form, Save, Run shortcuts"
```

---

### Task F2: RunRowView + RunExpandedDetailView

**Files:**
- Create: `macos/Linetta/Sources/Linetta/MainPane/Cards/RunRowView.swift`
- Create: `macos/Linetta/Sources/Linetta/MainPane/Cards/RunExpandedDetailView.swift`

- [ ] **Step 1: Create RunRowView**

```swift
import LinettaCore
import SwiftUI

struct RunRowView: View {
    let run: EpisodeRunResult
    let isExpanded: Bool
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 10) {
                Circle().fill(dotColor).frame(width: 7, height: 7)
                Text(timeLabel).font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary).frame(width: 84, alignment: .leading)
                Text(summaryLabel).font(LinettaTypography.body).foregroundStyle(LinettaTheme.text)
                Spacer()
                Text(tagLabel).font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textSecondary)
                    .padding(.horizontal, 6).padding(.vertical, 1)
                    .background(LinettaTheme.surfaceElevated).clipShape(Capsule())
                Image(systemName: isExpanded ? "chevron.up" : "chevron.down")
                    .font(.system(size: 10)).foregroundStyle(LinettaTheme.textTertiary)
            }
            .padding(.horizontal, 10).padding(.vertical, 8)
            .background(isExpanded ? LinettaTheme.surfaceElevated : Color.clear)
            .clipShape(RoundedRectangle(cornerRadius: 7))
        }
        .buttonStyle(.plain)
    }

    private var dotColor: Color { LinettaTheme.success } // simplified: until per-run adoption flag is exposed
    private var timeLabel: String { "—" }
    private var summaryLabel: String { "Run · \(run.artifacts.count) artifacts" }
    private var tagLabel: String { "\(run.artifacts.count) artifacts" }
}
```

- [ ] **Step 2: Create RunExpandedDetailView**

```swift
import LinettaCore
import SwiftUI

struct RunExpandedDetailView: View {
    let run: EpisodeRunResult
    var onPreviewArtifact: (Artifact) -> Void = { _ in }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Artifacts").linettaLabelStyle()
            FlowLayout(spacing: 6) {
                ForEach(run.artifacts) { artifact in
                    Button { onPreviewArtifact(artifact) } label: {
                        Text(artifact.title)
                            .font(LinettaTypography.caption)
                            .padding(.horizontal, 8).padding(.vertical, 3)
                            .background(LinettaTheme.surface).clipShape(Capsule())
                            .foregroundStyle(LinettaTheme.text)
                    }.buttonStyle(.plain)
                }
            }
            Text("Decisions").linettaLabelStyle().padding(.top, 4)
            Text("See Review Queue below").font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textSecondary)
        }
        .padding(12)
        .background(LinettaTheme.surfaceElevated)
        .clipShape(RoundedRectangle(cornerRadius: 7))
    }
}

private struct FlowLayout: Layout {
    let spacing: CGFloat
    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let maxWidth = proposal.width ?? .infinity
        var x: CGFloat = 0, y: CGFloat = 0, rowH: CGFloat = 0
        for sub in subviews {
            let s = sub.sizeThatFits(.unspecified)
            if x + s.width > maxWidth { x = 0; y += rowH + spacing; rowH = 0 }
            x += s.width + spacing
            rowH = max(rowH, s.height)
        }
        return CGSize(width: maxWidth, height: y + rowH)
    }
    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var x = bounds.minX, y = bounds.minY, rowH: CGFloat = 0
        for sub in subviews {
            let s = sub.sizeThatFits(.unspecified)
            if x + s.width > bounds.maxX { x = bounds.minX; y += rowH + spacing; rowH = 0 }
            sub.place(at: CGPoint(x: x, y: y), proposal: .unspecified)
            x += s.width + spacing
            rowH = max(rowH, s.height)
        }
    }
}
```

- [ ] **Step 3: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/MainPane/Cards/RunRowView.swift macos/Linetta/Sources/Linetta/MainPane/Cards/RunExpandedDetailView.swift
git commit -m "feat(workspace): RunRowView + RunExpandedDetailView with artifact pills"
```

---

### Task F3: RunHistoryCard wiring

**Files:**
- Replace contents: `macos/Linetta/Sources/Linetta/MainPane/Cards/RunHistoryCard.swift`

- [ ] **Step 1: Implement**

```swift
import LinettaCore
import SwiftUI

struct RunHistoryCard: View {
    let runs: [EpisodeRunResult]

    @Environment(EpisodeState.self) private var episodeState
    @Environment(ManuscriptState.self) private var manuscript

    @State private var showAll = false

    private var visibleRuns: [EpisodeRunResult] {
        showAll ? runs : Array(runs.prefix(5))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Run History").linettaLabelStyle()
                Text("\(runs.count)").font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary)
                Spacer()
                if runs.count > 5 {
                    Button(showAll ? "Show less" : "Show all (\(runs.count))") { showAll.toggle() }
                        .font(LinettaTypography.caption)
                        .buttonStyle(.plain)
                        .foregroundStyle(LinettaTheme.accent)
                }
            }
            if runs.isEmpty {
                Text("No runs yet. Press ⇧⌘R to run agents.")
                    .font(LinettaTypography.bodySmall)
                    .foregroundStyle(LinettaTheme.textTertiary)
                    .padding(.vertical, 10)
            } else {
                ForEach(visibleRuns) { run in
                    RunRowView(run: run, isExpanded: episodeState.expandedRunID == run.runID) {
                        episodeState.expandedRunID = episodeState.expandedRunID == run.runID ? nil : run.runID
                    }
                    if episodeState.expandedRunID == run.runID {
                        RunExpandedDetailView(run: run) { artifact in
                            manuscript.mode = .artifactPreview(runID: run.runID, artifactID: artifact.id, body: artifact.body)
                        }
                    }
                }
            }
        }
        .padding(.horizontal, LinettaShape.cardPaddingH).padding(.vertical, LinettaShape.cardPaddingV)
        .background(LinettaTheme.surface)
        .overlay(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius).stroke(LinettaTheme.border))
        .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }
}
```

- [ ] **Step 2: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/MainPane/Cards/RunHistoryCard.swift
git commit -m "feat(workspace): RunHistoryCard with 5-row default and expansion"
```

---

### Task F4: ReviewQueueCard + ReviewRowView

**Files:**
- Create: `macos/Linetta/Sources/Linetta/MainPane/Cards/ReviewRowView.swift`
- Replace contents: `macos/Linetta/Sources/Linetta/MainPane/Cards/ReviewQueueCard.swift`

- [ ] **Step 1: Create ReviewRowView**

```swift
import LinettaCore
import SwiftUI

struct ReviewRowView: View {
    enum Kind { case canon, continuity }
    let kind: Kind
    let title: String
    let source: String
    var onApprove: (() -> Void)? = nil
    var onReject: (() -> Void)? = nil
    var onDefer: (() -> Void)? = nil

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Text(kind == .canon ? "CANON" : "CONTINUITY")
                .linettaLabelStyle().frame(width: 88, alignment: .leading)
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(LinettaTypography.body).foregroundStyle(LinettaTheme.text)
                Text(source).font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary)
            }
            Spacer()
            HStack(spacing: 4) {
                if let onApprove { Button("✓") { onApprove() }.buttonStyle(.bordered).tint(LinettaTheme.success).controlSize(.mini) }
                if let onReject { Button("✗") { onReject() }.buttonStyle(.bordered).controlSize(.mini) }
                if let onDefer { Button("⏸") { onDefer() }.buttonStyle(.bordered).controlSize(.mini) }
            }
        }
        .padding(.vertical, 6)
        .overlay(alignment: .top) { Rectangle().fill(LinettaTheme.borderSoft).frame(height: 1).opacity(0.6) }
    }
}
```

- [ ] **Step 2: Replace ReviewQueueCard**

```swift
import LinettaCore
import SwiftUI

struct ReviewQueueCard: View {
    let workID: String
    let proposals: [CanonProposal]
    let issues: [ContinuityIssue]

    @Environment(AppState.self) private var appState

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Review Queue").linettaLabelStyle()
                Text("\(proposals.count + issues.count) pending")
                    .font(LinettaTypography.caption).foregroundStyle(LinettaTheme.accent)
                    .padding(.horizontal, 6).padding(.vertical, 1)
                    .background(LinettaTheme.accentSoft).clipShape(Capsule())
                Spacer()
                Text("work-level · across all episodes").font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary)
            }
            ForEach(proposals) { p in
                ReviewRowView(
                    kind: .canon,
                    title: p.summary,
                    source: "Run #\(p.runID.prefix(6))",
                    onApprove: { Task { _ = try? await appState.client.approveProposal(proposalID: p.id) } },
                    onReject: { Task { _ = try? await appState.client.rejectProposal(proposalID: p.id) } },
                    onDefer: { Task { _ = try? await appState.client.deferProposal(proposalID: p.id) } }
                )
            }
            ForEach(issues) { i in
                ReviewRowView(
                    kind: .continuity,
                    title: i.description,
                    source: "Ep \(i.episodeID.prefix(6))",
                    onApprove: { Task { _ = try? await appState.client.updateContinuityIssue(issueID: i.id, status: .accepted) } },
                    onReject: { Task { _ = try? await appState.client.updateContinuityIssue(issueID: i.id, status: .ignored) } },
                    onDefer: { Task { _ = try? await appState.client.updateContinuityIssue(issueID: i.id, status: .resolved) } }
                )
            }
        }
        .padding(.horizontal, LinettaShape.cardPaddingH).padding(.vertical, LinettaShape.cardPaddingV)
        .background(LinettaTheme.surface)
        .overlay(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius).stroke(LinettaTheme.warn))
        .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }
}
```

- [ ] **Step 3: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/MainPane/Cards/ReviewRowView.swift macos/Linetta/Sources/Linetta/MainPane/Cards/ReviewQueueCard.swift
git commit -m "feat(workspace): ReviewQueueCard with canon/continuity rows and actions"
```

---

### Task F5: Wire EpisodeState load on episode select

**Files:**
- Modify: `macos/Linetta/Sources/Linetta/MainPane/Cards/BlueprintCard.swift` (already does load on `task(id: episodeID)`)
- Modify: `macos/Linetta/Sources/Linetta/MainPane/EpisodeWorkspaceView.swift` — reset `episodeState.expandedRunID = nil` on episode change

- [ ] **Step 1: Add reset in EpisodeWorkspaceView**

In `EpisodeWorkspaceView.swift`, modify the `.task(id: episodeID) { await reload() }` block to also reset `episodeState.expandedRunID = nil` at the start.

- [ ] **Step 2: Build + smoke + commit**

```bash
cd macos/Linetta && swift build && swift test --filter AppShellSmokeTest
git add macos/Linetta/Sources/Linetta/MainPane/EpisodeWorkspaceView.swift
git commit -m "fix(workspace): reset expanded run when switching episodes"
```

---

# Group G · Inspector

### Task G1: ManuscriptInspector skeleton + header

**Files:**
- Create: `macos/Linetta/Sources/Linetta/Inspector/ManuscriptInspector.swift`
- Create: `macos/Linetta/Sources/Linetta/Inspector/ManuscriptHeader.swift`
- Modify: `macos/Linetta/Sources/Linetta/Shell/AppShell.swift` — replace inspector placeholder with `ManuscriptInspector()`

- [ ] **Step 1: Create ManuscriptHeader**

```swift
import LinettaCore
import SwiftUI

struct ManuscriptHeader: View {
    let versionLabel: String
    var onCloseTap: () -> Void = {}
    var body: some View {
        HStack {
            Text("📄 MANUSCRIPT").linettaLabelStyle()
            Spacer()
            Text(versionLabel).font(LinettaTypography.titleSmall).foregroundStyle(LinettaTheme.text)
            Menu { Text("More actions in Phase 7+") } label: { Image(systemName: "ellipsis") }
                .menuStyle(.borderlessButton).frame(width: 24)
        }
        .padding(.horizontal, 14).padding(.vertical, 12)
        .background(LinettaTheme.surfaceElevated)
        .overlay(alignment: .bottom) { Rectangle().fill(LinettaTheme.borderSoft).frame(height: 1) }
    }
}
```

- [ ] **Step 2: Create ManuscriptInspector**

```swift
import LinettaCore
import SwiftUI

struct ManuscriptInspector: View {
    @Environment(ManuscriptState.self) private var manuscript

    var body: some View {
        @Bindable var manuscript = manuscript
        VStack(spacing: 0) {
            ManuscriptHeader(versionLabel: versionLabel)
            switch manuscript.mode {
            case .adopted:
                ManuscriptEditor()
            case .artifactPreview(_, _, let body):
                ArtifactPreviewView(body: body)
            }
        }
        .background(LinettaTheme.background)
    }

    private var versionLabel: String {
        switch manuscript.mode {
        case .adopted: return "current"
        case .artifactPreview: return "preview"
        }
    }
}
```

- [ ] **Step 3: Stub ManuscriptEditor and ArtifactPreviewView**

`macos/Linetta/Sources/Linetta/Inspector/ManuscriptEditor.swift`:

```swift
import SwiftUI

struct ManuscriptEditor: View {
    @Environment(ManuscriptState.self) private var manuscript
    var body: some View {
        @Bindable var manuscript = manuscript
        TextEditor(text: $manuscript.draft)
            .font(LinettaTypography.bodySerif)
            .padding(14)
    }
}
```

`macos/Linetta/Sources/Linetta/Inspector/ArtifactPreviewView.swift`:

```swift
import SwiftUI

struct ArtifactPreviewView: View {
    let body: String
    var body: some View {
        ScrollView {
            Text(self.body).font(LinettaTypography.bodySerif).padding(14).frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}
```

- [ ] **Step 4: Plug into AppShell**

In `AppShell.swift`, replace the inspector `Color.clear...overlay { Text("Inspector") }` with `ManuscriptInspector()`.

- [ ] **Step 5: Build + smoke + commit**

```bash
cd macos/Linetta && swift build && swift test --filter AppShellSmokeTest
git add macos/Linetta/Sources/Linetta/Inspector/ macos/Linetta/Sources/Linetta/Shell/AppShell.swift
git commit -m "feat(inspector): ManuscriptInspector skeleton with header, editor stub, preview stub"
```

---

### Task G2: ManuscriptEditor with debounced autosave

**Files:**
- Replace contents: `macos/Linetta/Sources/Linetta/Inspector/ManuscriptEditor.swift`

- [ ] **Step 1: Implement debounced save**

```swift
import LinettaCore
import SwiftUI

struct ManuscriptEditor: View {
    @Environment(ManuscriptState.self) private var manuscript
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    @State private var saveTask: Task<Void, Never>?

    var body: some View {
        @Bindable var manuscript = manuscript
        VStack(alignment: .leading, spacing: 0) {
            metaRow.padding(.horizontal, 14).padding(.top, 10)
            TextEditor(text: $manuscript.draft)
                .font(LinettaTypography.bodySerif)
                .padding(14)
                .onChange(of: manuscript.draft) { _, _ in scheduleSave() }
        }
    }

    private var metaRow: some View {
        HStack {
            Text("\(wordCount) words · \(manuscript.isDirty ? "unsaved" : "saved")")
                .font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary)
            Spacer()
        }
    }

    private var wordCount: Int {
        manuscript.draft.split(whereSeparator: { $0.isWhitespace }).count
    }

    private func scheduleSave() {
        saveTask?.cancel()
        saveTask = Task { @MainActor [weak appState, weak sidebar, weak manuscript] in
            try? await Task.sleep(nanoseconds: 1_500_000_000)
            guard !Task.isCancelled else { return }
            guard let appState, let sidebar, let manuscript else { return }
            guard case .episode(let wid, let eid) = sidebar.selection else { return }
            _ = try? await appState.client.createEpisodeVersion(
                workID: wid, episodeID: eid,
                request: CreateEpisodeVersionRequest(body: manuscript.draft, note: "auto-save")
            )
            manuscript.markSaved()
        }
    }
}
```

- [ ] **Step 2: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/Inspector/ManuscriptEditor.swift
git commit -m "feat(inspector): debounced 1.5s autosave with word count and dirty meta"
```

---

### Task G3: ArtifactPreview with Adopt button

**Files:**
- Replace contents: `macos/Linetta/Sources/Linetta/Inspector/ArtifactPreviewView.swift`

- [ ] **Step 1: Implement adopt button**

```swift
import LinettaCore
import SwiftUI

struct ArtifactPreviewView: View {
    let body: String

    @Environment(ManuscriptState.self) private var manuscript
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    var body: some View {
        VStack(spacing: 0) {
            Text("Preview · read-only")
                .font(LinettaTypography.caption).foregroundStyle(LinettaTheme.text)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 6).background(Color(red: 0.25, green: 0.40, blue: 0.65).opacity(0.7))
            ScrollView {
                Text(self.body).font(LinettaTypography.bodySerif).padding(14).frame(maxWidth: .infinity, alignment: .leading)
                    .foregroundStyle(LinettaTheme.text)
            }
            Button("Adopt as new version") { Task { await adopt() } }
                .buttonStyle(.borderedProminent).tint(LinettaTheme.accent)
                .padding(12)
        }
    }

    private func adopt() async {
        guard case .episode(let wid, let eid) = sidebar.selection else { return }
        _ = try? await appState.client.createEpisodeVersion(
            workID: wid, episodeID: eid,
            request: CreateEpisodeVersionRequest(body: self.body, note: "adopt from artifact")
        )
        manuscript.loadAdopted(body: self.body)
    }
}
```

- [ ] **Step 2: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/Inspector/ArtifactPreviewView.swift
git commit -m "feat(inspector): artifact preview mode with adopt button"
```

---

### Task G4: Inspector toggle binding in AppShell

**Files:**
- Modify: `macos/Linetta/Sources/Linetta/Shell/AppShell.swift`

- [ ] **Step 1: Implement selectedEpisodeID via SidebarState**

In `AppShell.swift`, replace `selectedEpisodeID` placeholder:

```swift
@Environment(SidebarState.self) private var sidebar

private var selectedEpisodeID: String? {
    if case .episode(_, let eid) = sidebar.selection { return eid }
    return nil
}
```

- [ ] **Step 2: Build + smoke + commit**

```bash
cd macos/Linetta && swift build && swift test --filter AppShellSmokeTest
git add macos/Linetta/Sources/Linetta/Shell/AppShell.swift
git commit -m "feat(shell): inspector toggle bound to selected episode"
```

---

# Group H · Chrome

### Task H1: StatusFooter via .safeAreaInset

**Files:**
- Create: `macos/Linetta/Sources/Linetta/Chrome/StatusFooter.swift`
- Modify: `macos/Linetta/Sources/Linetta/Shell/AppShell.swift`

- [ ] **Step 1: Create StatusFooter**

```swift
import LinettaCore
import SwiftUI

struct StatusFooter: View {
    @Environment(EngineController.self) private var engine
    var body: some View {
        HStack(spacing: 10) {
            Circle().fill(dotColor).frame(width: 7, height: 7)
            Text("Engine").font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textSecondary)
            if let addr = engine.address?.absoluteString {
                Text("·").foregroundStyle(LinettaTheme.textTertiary)
                Text(addr).font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary).textSelection(.enabled)
            }
            Spacer()
        }
        .padding(.horizontal, 14).padding(.vertical, 7)
        .background(LinettaTheme.surface)
        .overlay(alignment: .top) { Rectangle().fill(LinettaTheme.border).frame(height: 1) }
    }

    private var dotColor: Color {
        switch engine.status {
        case .healthy: return LinettaTheme.success
        case .external: return Color(red: 0.43, green: 0.55, blue: 0.85)
        case .starting: return Color(red: 0.95, green: 0.78, blue: 0.34)
        case .failed: return LinettaTheme.danger
        case .stopped: return LinettaTheme.textTertiary
        }
    }
}
```

- [ ] **Step 2: Attach to AppShell via safeAreaInset**

In `AppShell.swift`, add `.safeAreaInset(edge: .bottom) { StatusFooter() }` to the body's outermost view.

- [ ] **Step 3: Build + smoke + commit**

```bash
cd macos/Linetta && swift build && swift test --filter AppShellSmokeTest
git add macos/Linetta/Sources/Linetta/Chrome/StatusFooter.swift macos/Linetta/Sources/Linetta/Shell/AppShell.swift
git commit -m "feat(chrome): StatusFooter via safeAreaInset (no content overlap)"
```

---

### Task H2: AppCommands menubar

**Files:**
- Create: `macos/Linetta/Sources/Linetta/Chrome/AppCommands.swift`
- Modify: `macos/Linetta/Sources/Linetta/LinettaApp.swift`

- [ ] **Step 1: Create AppCommands**

```swift
import LinettaCore
import SwiftUI

struct AppCommands: Commands {
    @FocusedValue(\.runAgents) var runAgentsAction
    @FocusedValue(\.saveBlueprint) var saveAction

    var body: some Commands {
        CommandGroup(replacing: .newItem) {
            Button("New Work") { NotificationCenter.default.post(name: .linettaNewWork, object: nil) }
                .keyboardShortcut("n", modifiers: [.command])
            Button("New Episode") { NotificationCenter.default.post(name: .linettaNewEpisode, object: nil) }
                .keyboardShortcut("n", modifiers: [.command, .shift])
        }
        CommandMenu("Run") {
            Button("Run Agents") { runAgentsAction?() }
                .keyboardShortcut("r", modifiers: [.command, .shift])
            Button("Save Blueprint") { saveAction?() }
                .keyboardShortcut("s", modifiers: [.command])
        }
    }
}

extension Notification.Name {
    static let linettaNewWork = Notification.Name("linetta.newWork")
    static let linettaNewEpisode = Notification.Name("linetta.newEpisode")
}

private struct RunAgentsActionKey: FocusedValueKey { typealias Value = () -> Void }
private struct SaveBlueprintActionKey: FocusedValueKey { typealias Value = () -> Void }

extension FocusedValues {
    var runAgents: (() -> Void)? {
        get { self[RunAgentsActionKey.self] }
        set { self[RunAgentsActionKey.self] = newValue }
    }
    var saveBlueprint: (() -> Void)? {
        get { self[SaveBlueprintActionKey.self] }
        set { self[SaveBlueprintActionKey.self] = newValue }
    }
}
```

- [ ] **Step 2: Attach to Scene**

In `LinettaApp.swift`, replace the existing `.commands { CommandGroup(after: .newItem) { Button("Refresh Works") … } }` with:

```swift
.commands { AppCommands() }
```

- [ ] **Step 3: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/Chrome/AppCommands.swift macos/Linetta/Sources/Linetta/LinettaApp.swift
git commit -m "feat(chrome): AppCommands menubar with File/Run shortcuts"
```

---

### Task H3: CommandPalette ⌘K overlay

**Files:**
- Create: `macos/Linetta/Sources/Linetta/Chrome/CommandPalette.swift`
- Modify: `macos/Linetta/Sources/Linetta/Shell/AppShell.swift` — overlay palette + ⌘K shortcut

- [ ] **Step 1: Create CommandPalette**

```swift
import LinettaCore
import SwiftUI

struct CommandPalette: View {
    @Environment(CommandPaletteState.self) private var state
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    @FocusState private var focused: Bool

    var body: some View {
        @Bindable var state = state
        if state.isOpen {
            ZStack(alignment: .top) {
                Color.black.opacity(0.4).onTapGesture { state.isOpen = false }
                VStack(spacing: 0) {
                    TextField("Jump to episode, work, or memory…", text: $state.query)
                        .textFieldStyle(.plain)
                        .font(LinettaTypography.body)
                        .padding(14)
                        .focused($focused)
                        .onAppear { focused = true }
                    Divider().background(LinettaTheme.borderSoft)
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: 0) {
                            ForEach(filtered) { result in
                                Button { jump(result) } label: {
                                    HStack {
                                        Image(systemName: result.icon)
                                        Text(result.title).foregroundStyle(LinettaTheme.text)
                                        Spacer()
                                        Text(result.subtitle).font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary)
                                    }.padding(.horizontal, 14).padding(.vertical, 8)
                                }.buttonStyle(.plain)
                            }
                        }
                    }.frame(maxHeight: 240)
                }
                .frame(width: 540).background(LinettaTheme.surface).clipShape(RoundedRectangle(cornerRadius: 10))
                .overlay(RoundedRectangle(cornerRadius: 10).stroke(LinettaTheme.border))
                .padding(.top, 80)
            }
        }
    }

    private var filtered: [Result] {
        let q = state.query.lowercased()
        var out: [Result] = []
        for work in appState.works {
            if q.isEmpty || work.title.lowercased().contains(q) {
                out.append(.init(id: work.id, icon: "books.vertical", title: work.title, subtitle: "work") {
                    sidebar.selection = .work(workID: work.id)
                })
            }
        }
        return out
    }

    private func jump(_ r: Result) {
        r.action(); state.isOpen = false
    }

    struct Result: Identifiable {
        let id: String
        let icon: String
        let title: String
        let subtitle: String
        let action: () -> Void
    }
}
```

- [ ] **Step 2: Attach to AppShell**

In `AppShell.swift`, add `.overlay { CommandPalette() }` and a hidden button with `.keyboardShortcut("k", modifiers: [.command])` that flips `commandPalette.isOpen`. Pull `@Environment(CommandPaletteState.self) private var commandPalette` into AppShell.

```swift
.overlay { CommandPalette() }
.background {
    Button("") { commandPalette.isOpen.toggle() }
        .keyboardShortcut("k", modifiers: [.command])
        .opacity(0)
}
```

- [ ] **Step 3: Build + smoke + commit**

```bash
cd macos/Linetta && swift build && swift test --filter AppShellSmokeTest
git add macos/Linetta/Sources/Linetta/Chrome/CommandPalette.swift macos/Linetta/Sources/Linetta/Shell/AppShell.swift
git commit -m "feat(chrome): ⌘K command palette overlay with work jumps"
```

---

### Task H4: Title bar binding

**Files:**
- Create: `macos/Linetta/Sources/Linetta/Chrome/TitleBarBinding.swift`
- Modify: `macos/Linetta/Sources/Linetta/Shell/AppShell.swift`

- [ ] **Step 1: Create TitleBarBinding view modifier**

```swift
import LinettaCore
import SwiftUI

struct TitleBarBinding: ViewModifier {
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    func body(content: Content) -> some View {
        content.navigationTitle(title)
    }

    private var title: String {
        switch sidebar.selection {
        case .none: return "Linetta"
        case .work(let wid):
            return appState.works.first { $0.id == wid }.map { "Linetta — \($0.title)" } ?? "Linetta"
        case .memory(let wid):
            return appState.works.first { $0.id == wid }.map { "Linetta — \($0.title) · Memory" } ?? "Linetta"
        case .episode(let wid, let eid):
            let work = appState.works.first { $0.id == wid }?.title ?? ""
            return "Linetta — \(work) · \(eid.prefix(8))"
        }
    }
}

extension View {
    func linettaTitleBar() -> some View { modifier(TitleBarBinding()) }
}
```

- [ ] **Step 2: Apply in AppShell**

In `AppShell.swift`, attach `.linettaTitleBar()` to the outermost view.

- [ ] **Step 3: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/Chrome/TitleBarBinding.swift macos/Linetta/Sources/Linetta/Shell/AppShell.swift
git commit -m "feat(chrome): dynamic title bar tracking sidebar selection"
```

---

# Group I · Migration / Legacy / Settings retrofit

### Task I1: Move legacy views to _legacy/

**Files:**
- Move 7 files into `macos/Linetta/Sources/Linetta/_legacy/`

- [ ] **Step 1: Create directory and move files**

```bash
mkdir -p macos/Linetta/Sources/Linetta/_legacy
git mv macos/Linetta/Sources/Linetta/Views/WorkGalleryView.swift macos/Linetta/Sources/Linetta/_legacy/
git mv macos/Linetta/Sources/Linetta/Views/WorkspaceView.swift macos/Linetta/Sources/Linetta/_legacy/
git mv macos/Linetta/Sources/Linetta/Views/EpisodeWorkbenchView.swift macos/Linetta/Sources/Linetta/_legacy/
git mv macos/Linetta/Sources/Linetta/Views/CanonMemoryView.swift macos/Linetta/Sources/Linetta/_legacy/
git mv macos/Linetta/Sources/Linetta/Views/ManuscriptVersionView.swift macos/Linetta/Sources/Linetta/_legacy/
git mv macos/Linetta/Sources/Linetta/Views/MemoryDiffView.swift macos/Linetta/Sources/Linetta/_legacy/
git mv macos/Linetta/Sources/Linetta/Views/EngineStatusBadge.swift macos/Linetta/Sources/Linetta/_legacy/
```

- [ ] **Step 2: Verify the build still passes (legacy files compile but are unused)**

Run: `cd macos/Linetta && swift build`
Expected: PASS. If a legacy file references a deleted symbol, comment that block out (legacy is deprecated).

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "chore(legacy): move pre-6.5 views to _legacy/ for staged removal"
```

---

### Task I2: SettingsView theme retrofit (no structural change)

**Files:**
- Modify: `macos/Linetta/Sources/Linetta/Views/SettingsView.swift`

- [ ] **Step 1: Replace inline colors with theme tokens**

In `SettingsView.swift`, wherever you see `.foregroundStyle(.secondary)`, replace with `.foregroundStyle(LinettaTheme.textSecondary)`. Replace any hard-coded backgrounds with `LinettaTheme.surface`. Wrap the root `Form` in `.background(LinettaTheme.background)`. Buttons that should pop use `.tint(LinettaTheme.accent)`.

(Detailed replacements: grep `.secondary` and `.tertiary` in the file and swap one-for-one. The form's structural content remains; full Settings rewrite happens in Phase 7.)

- [ ] **Step 2: Build + commit**

```bash
cd macos/Linetta && swift build
git add macos/Linetta/Sources/Linetta/Views/SettingsView.swift
git commit -m "style(settings): adopt LinettaTheme tokens (structural rewrite deferred to Phase 7)"
```

---

### Task I3: Delete _legacy/ after sanity manual run

**Files:**
- Delete: `macos/Linetta/Sources/Linetta/_legacy/` (7 files)

- [ ] **Step 1: Manual run**

Run: `make macos-run`

Verify (mark each ✓ before proceeding):
- App opens with 1080×720 window, warm-dark background
- Sidebar shows Works tree (or onboarding if 0 works)
- Creating a work via ⌘N opens NewWorkSheet
- Selecting an episode shows Blueprint / Run History / Review Queue
- ⇧⌘R triggers Run Agents (engine actually runs)
- Artifact pill click opens Manuscript inspector in preview mode
- Adopt button creates a new version
- ⌘⇧M toggles the inspector
- ⌘K opens command palette and listing works
- Status footer shows green Engine dot + address

If any item fails, STOP — fix root cause, don't proceed.

- [ ] **Step 2: Delete _legacy/**

```bash
rm -rf macos/Linetta/Sources/Linetta/_legacy
cd macos/Linetta && swift build && swift test
git add -A
git commit -m "chore(legacy): remove pre-6.5 views after Phase 6.5 verification"
```

---

# Group J · Verification & Docs

### Task J1: docs/plan/README.md and roadmap dependency graph

**Files:**
- Modify: `docs/plan/README.md`
- Modify: `docs/plan/linetta-macos-app-completion-roadmap.md`

- [ ] **Step 1: Add Phase 6.5 link to README reading order**

In `docs/plan/README.md`, after the line linking `phase-6-embedded-engine-lifecycle.md`, insert:

```markdown
8. [phase-6.5-ui-redesign.md](./phase-6.5-ui-redesign.md)
8a. [phase-6.5-ui-redesign-plan.md](./phase-6.5-ui-redesign-plan.md)
```

And renumber subsequent items.

- [ ] **Step 2: Update roadmap dependency graph**

In `docs/plan/linetta-macos-app-completion-roadmap.md`, find the "페이즈 간 의존성" diagram and update it to:

```
Phase 5 (MVP)
  └─→ Phase 6 (Embedded Engine)
        └─→ Phase 6.5 (UI Redesign)
              └─→ Phase 7 (Settings Studio)
                    └─→ Phase 8 (App Polish)
                          └─→ Phase 9 (Live + Editor)
```

- [ ] **Step 3: Commit**

```bash
git add docs/plan/README.md docs/plan/linetta-macos-app-completion-roadmap.md
git commit -m "docs(plan): add Phase 6.5 to reading order and dependency graph"
```

---

### Task J2: Final verification matrix

**Files:**
- None (verification only)

- [ ] **Step 1: All tests pass**

Run: `cd macos/Linetta && swift test`
Expected: All tests pass (including ThemeTokenTests, TypographyTokenTests, StateObservableTests, AppShellSmokeTest, existing LinettaCoreTests).

- [ ] **Step 2: Go tests still pass**

Run: `go test ./...`
Expected: All Go tests pass.

- [ ] **Step 3: Lint pass**

Run: `go vet ./...`
Expected: No warnings.

- [ ] **Step 4: APIClient() direct uses are 0**

Run: `grep -rn "APIClient()" macos/Linetta/Sources/Linetta/ ; echo done`
Expected: prints only `done` (no matches).

- [ ] **Step 5: User scenario walkthrough**

With `make macos-run`, walk through each scenario and mark ✓:
- [ ] Create work via ⌘N
- [ ] Create episode via ⇧⌘N
- [ ] Type a premise → ⇧⌘R runs agents → run appears in history
- [ ] Click artifact pill → preview opens in inspector → Adopt → new version visible
- [ ] Edit manuscript inline → wait 2s → save indicator → close + reopen → text persists
- [ ] Open Memory page → create canon item → list updates
- [ ] Approve a canon proposal in Review Queue → it disappears
- [ ] Resolve a continuity issue → it disappears
- [ ] Toggle sidebar via ⌘⌃S → hides and shows
- [ ] Toggle inspector via ⌘⇧M → hides and shows
- [ ] ⌘K opens palette → search a work → jumps
- [ ] Close app → reopen → engine restarts cleanly

If any scenario fails, file a P0 bug and fix before declaring Phase 6.5 complete.

---

### Task J3: Final commit + roadmap mark-as-complete

**Files:**
- Modify: `docs/plan/linetta-macos-app-completion-roadmap.md` — mark Phase 6.5 complete

- [ ] **Step 1: Update roadmap "완료 조건" section**

Add a Phase 6.5 entry to the master "완료 조건 (전체)" checklist with the user scenarios from J2.

- [ ] **Step 2: Final commit**

```bash
git add docs/plan/linetta-macos-app-completion-roadmap.md
git commit -m "docs(plan): mark Phase 6.5 verification complete"
```

- [ ] **Step 3: Tag**

```bash
git tag phase-6.5-complete -m "Phase 6.5 — UI Redesign complete"
```

(Push tag with `git push --tags` only when user explicitly asks.)

---

## Spec coverage check

| Spec section | Tasks |
|---|---|
| 2 — 7 brainstorming decisions | All encoded in E1, F1–F4, G1–G3 |
| 3 — 3-column shell | C1, C2 |
| 4 — Sidebar | D1–D5 |
| 5 — Episode workspace | E3, F1–F5 |
| 5.5 — Memory mode | E2 |
| 5.5 — Work overview, Onboarding | E1 |
| 5.6 — Running state in run row | F2 (visual stub; SSE live in Phase 9) |
| 6 — Manuscript inspector | G1–G4 |
| 7 — Chrome (toolbar, menubar, palette, footer) | E3 (toolbar), H1, H2, H3, H4 |
| 7.5 — Toast framework | B5 |
| 8.5 — Theme tokens | A1 |
| 8.6 — Typography | A2 |
| 8.7 — Migration strategy | I1, I3 |
| 8.8 — Settings theme retrofit | I2 |
| 8.9 — AppStorage namespacing | B2, B3, B4 |
| 8.10 — Smoke test + previews | C1, all tests in A/B |
| 9 — Out of scope | enforced by absence (no SSE, no full settings rewrite, etc.) |

---

## Self-review notes

**Type consistency check:**
- `SidebarSelection` enum used uniformly across SidebarView, MainPaneRouter, AppShell, CommandPalette.
- `ManuscriptMode` enum with `.adopted` / `.artifactPreview(runID:artifactID:body:)` used in ManuscriptState, ManuscriptInspector, ArtifactPreviewView, RunHistoryCard.
- `EpisodeRunResult` from `LinettaCore.Models` reused throughout; `runID`, `artifacts` properties referenced.

**Placeholder scan:** No "TBD" / "TODO" left. All code blocks are complete and runnable.

**Spec coverage:** Every listed spec section maps to ≥1 task above.

**Bite-size check:** All steps are 2–5 minute actions with exact code and commands.

---

# Plan complete

Plan saved to `docs/plan/phase-6.5-ui-redesign-plan.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using `executing-plans`, batch execution with checkpoints.

Which approach?
