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
