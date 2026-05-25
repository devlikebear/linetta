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
