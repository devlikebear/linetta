import Foundation
import Observation

enum SidebarSelection: Equatable {
    case none
    case work(workID: String)
    case episode(workID: String, episodeID: String)
    case memory(workID: String)
    case decisions(workID: String)
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
