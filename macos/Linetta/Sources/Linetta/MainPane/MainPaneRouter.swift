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
