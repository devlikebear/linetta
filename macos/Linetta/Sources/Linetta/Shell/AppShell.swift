import LinettaCore
import SwiftUI

struct AppShell: View {
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar
    @Environment(ManuscriptState.self) private var manuscript
    @Environment(CommandPaletteState.self) private var commandPalette

    var body: some View {
        NavigationSplitView {
            SidebarView()
                .frame(minWidth: 220, idealWidth: 230, maxWidth: 320)
        } detail: {
            MainPaneRouter()
                .inspector(isPresented: inspectorBinding) {
                    ManuscriptInspector()
                        .frame(minWidth: 280, idealWidth: manuscript.width, maxWidth: 480)
                }
        }
        .frame(minWidth: 1080, minHeight: 720)
        .background(LinettaTheme.background)
        .preferredColorScheme(.dark)
        .safeAreaInset(edge: .bottom) { EngineStatusFooter() }
        .overlay { CommandPalette() }
        .background {
            Button("") {
                commandPalette.isOpen.toggle()
            }
            .keyboardShortcut("k", modifiers: [.command])
            .opacity(0)
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
