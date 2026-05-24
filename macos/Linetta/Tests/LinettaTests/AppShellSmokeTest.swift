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
            .environmentObject(engine)
            .environment(SidebarState())
            .environment(EpisodeState())
            .environment(ManuscriptState())
            .environment(CommandPaletteState())
            .environment(ToastCenter())
        let host = NSHostingController(rootView: view)
        XCTAssertNotNil(host.view)
    }
}
