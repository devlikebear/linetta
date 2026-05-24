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
