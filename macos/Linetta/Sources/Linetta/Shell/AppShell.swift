import LinettaCore
import SwiftUI

struct AppShell: View {
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar
    @Environment(ManuscriptState.self) private var manuscript
    @Environment(CommandPaletteState.self) private var commandPalette
    @Environment(ToastCenter.self) private var toast

    @State private var showingNewWork = false

    var body: some View {
        NavigationSplitView {
            SidebarView()
                .navigationSplitViewColumnWidth(min: 220, ideal: 240, max: 360)
        } detail: {
            MainPaneRouter()
                .safeAreaInset(edge: .bottom, spacing: 0) { EngineStatusFooter() }
                .inspector(isPresented: inspectorBinding) {
                    ManuscriptInspector()
                        .inspectorColumnWidth(min: 280, ideal: 320, max: 480)
                }
        }
        .frame(minWidth: 1080, minHeight: 720)
        .background(LinettaTheme.background)
        .preferredColorScheme(.dark)
        .linettaTitleBar()
        .overlay { CommandPalette() }
        .overlay { ToastHUD() }
        .background {
            Button("") {
                commandPalette.isOpen.toggle()
            }
            .keyboardShortcut("k", modifiers: [.command])
            .opacity(0)
        }
        .sheet(isPresented: $showingNewWork) { NewWorkSheet() }
        .onReceive(NotificationCenter.default.publisher(for: .linettaNewWork)) { _ in
            showingNewWork = true
        }
        .onReceive(NotificationCenter.default.publisher(for: .linettaNewEpisode)) { _ in
            createNewEpisode()
        }
        .onReceive(NotificationCenter.default.publisher(for: .linettaToggleInspector)) { _ in
            toggleInspector()
        }
        .onReceive(NotificationCenter.default.publisher(for: .linettaToggleCommandPalette)) { _ in
            commandPalette.isOpen.toggle()
        }
    }

    private func toggleInspector() {
        guard let eid = selectedEpisodeID else {
            toast.enqueue(.init(title: "Select an episode first to toggle Manuscript.", kind: .info))
            return
        }
        manuscript.setOpen(episodeID: eid, open: !manuscript.isOpen(episodeID: eid))
    }

    private func createNewEpisode() {
        guard let workID = currentWorkID else {
            toast.enqueue(.init(title: "Select a work first to create an episode.", kind: .info))
            return
        }
        Task {
            let count = (try? await appState.client.listEpisodes(workID: workID).count) ?? 0
            guard let episode = try? await appState.client.createEpisode(
                workID: workID,
                request: .init(title: "Episode \(count + 1)")
            ) else {
                toast.enqueue(.init(title: "Failed to create episode.", kind: .error))
                return
            }
            sidebar.selection = .episode(workID: workID, episodeID: episode.id)
            toast.enqueue(.init(title: "Created \(episode.title)", kind: .success))
        }
    }

    private var currentWorkID: String? {
        switch sidebar.selection {
        case .work(let wid), .memory(let wid), .decisions(let wid), .episode(let wid, _):
            return wid
        case .none:
            return appState.works.first?.id
        }
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
        if case .episode(_, let eid) = sidebar.selection { return eid }
        return nil
    }
}

#Preview {
    AppShell()
        .environment(AppState(engine: EngineController()))
        .environmentObject(EngineController())
        .environment(SidebarState())
        .environment(EpisodeState())
        .environment(ManuscriptState())
        .environment(CommandPaletteState())
        .environment(ToastCenter())
}
