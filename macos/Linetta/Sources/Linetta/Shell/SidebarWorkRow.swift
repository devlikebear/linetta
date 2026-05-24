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
            Button {
                sidebar.setExpanded(work.id, expanded: !sidebar.isExpanded(work.id))
            } label: {
                HStack(spacing: 4) {
                    Image(systemName: sidebar.isExpanded(work.id) ? "chevron.down" : "chevron.right")
                        .font(.system(size: 9))
                        .frame(width: 12)
                        .foregroundStyle(LinettaTheme.textTertiary)
                    Text(work.title)
                        .font(LinettaTypography.body)
                        .foregroundStyle(LinettaTheme.text)
                        .lineLimit(1)
                    Spacer()
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 5)
                .background(rowBackground)
            }
            .buttonStyle(.plain)

            if sidebar.isExpanded(work.id) {
                SidebarMemoryRow(workID: work.id)
                ForEach(episodes) { episode in
                    SidebarEpisodeRow(workID: work.id, episode: episode)
                }
                NewEpisodePlaceholder(workID: work.id)
            }
        }
        .task(id: work.id) { await loadEpisodes() }
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
    var body: some View {
        Text("＋ New episode")
            .font(LinettaTypography.bodySmall)
            .foregroundStyle(LinettaTheme.textTertiary)
            .padding(.leading, 24)
            .padding(.vertical, 4)
    }
}
