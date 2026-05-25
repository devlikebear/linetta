import LinettaCore
import SwiftUI

struct SidebarWorkRow: View {
    let work: Work

    @Environment(SidebarState.self) private var sidebar
    @Environment(AppState.self) private var appState

    @State private var episodes: [Episode] = []

    var body: some View {
        @Bindable var sidebar = sidebar
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 4) {
                Button {
                    sidebar.setExpanded(work.id, expanded: !sidebar.isExpanded(work.id))
                } label: {
                    Image(systemName: sidebar.isExpanded(work.id) ? "chevron.down" : "chevron.right")
                        .font(.system(size: 9))
                        .frame(width: 14, height: 18)
                        .contentShape(Rectangle())
                        .foregroundStyle(LinettaTheme.textTertiary)
                }
                .buttonStyle(.plain)

                Button {
                    selectWork()
                } label: {
                    HStack {
                        Text(work.title)
                            .font(LinettaTypography.body)
                            .foregroundStyle(LinettaTheme.text)
                            .lineLimit(1)
                        Spacer()
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 5)
            .background(rowBackground)

            if sidebar.isExpanded(work.id) {
                SidebarMemoryRow(workID: work.id)
                SidebarDecisionsRow(workID: work.id)
                ForEach(episodes) { episode in
                    SidebarEpisodeRow(workID: work.id, episode: episode)
                }
                NewEpisodePlaceholder(workID: work.id)
            }
        }
        .task(id: work.id) { await loadEpisodes() }
    }

    private func selectWork() {
        sidebar.selection = .work(workID: work.id)
        // Auto-expand on selection so the user immediately sees Memory + Episodes.
        if !sidebar.isExpanded(work.id) {
            sidebar.setExpanded(work.id, expanded: true)
        }
    }

    private var rowBackground: some View {
        if case .work(let wid) = sidebar.selection, wid == work.id {
            return AnyView(LinettaTheme.accentSoft.clipShape(RoundedRectangle(cornerRadius: 5)))
        }
        return AnyView(Color.clear)
    }

    private func loadEpisodes() async {
        do { episodes = try await appState.client.listEpisodes(workID: work.id) } catch { episodes = [] }
    }
}

private struct NewEpisodePlaceholder: View {
    let workID: String
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar
    @State private var pending = false

    var body: some View {
        Button {
            Task { await create() }
        } label: {
            Text(pending ? "Creating…" : "＋ New episode")
                .font(LinettaTypography.bodySmall)
                .foregroundStyle(LinettaTheme.textTertiary)
                .padding(.leading, 24).padding(.vertical, 4)
        }
        .buttonStyle(.plain)
        .keyboardShortcut("n", modifiers: [.command, .shift])
    }

    private func create() async {
        pending = true
        defer { pending = false }
        let count = (try? await appState.client.listEpisodes(workID: workID).count) ?? 0
        guard let created = try? await appState.client.createEpisode(
            workID: workID,
            request: .init(title: "Episode \(count + 1)")
        ) else { return }
        sidebar.selection = .episode(workID: workID, episodeID: created.id)
    }
}
