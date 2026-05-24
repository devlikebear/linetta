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
