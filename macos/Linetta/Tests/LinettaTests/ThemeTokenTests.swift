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
