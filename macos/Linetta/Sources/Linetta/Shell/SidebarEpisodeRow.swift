import LinettaCore
import SwiftUI

struct SidebarEpisodeRow: View {
    let workID: String
    let episode: Episode

    @Environment(SidebarState.self) private var sidebar

    var body: some View {
        Button {
            sidebar.selection = .episode(workID: workID, episodeID: episode.id)
        } label: {
            HStack(spacing: 6) {
                Circle().fill(statusColor).frame(width: 6, height: 6)
                Text(episode.title)
                    .font(LinettaTypography.body)
                    .foregroundStyle(LinettaTheme.text)
                    .lineLimit(1)
                Spacer()
            }
            .padding(.leading, 24)
            .padding(.trailing, 8)
            .padding(.vertical, 4)
            .background(isSelected ? LinettaTheme.accentSoft : Color.clear)
            .clipShape(RoundedRectangle(cornerRadius: 5))
        }
        .buttonStyle(.plain)
    }

    private var isSelected: Bool {
        if case let .episode(_, eid) = sidebar.selection, eid == episode.id { return true }
        return false
    }

    private var statusColor: Color {
        switch episode.status {
        case .idea: return LinettaTheme.textTertiary
        case .outlined: return Color(red: 0.70, green: 0.75, blue: 0.90)
        case .drafting: return Color(red: 0.95, green: 0.78, blue: 0.34)
        case .reviewing: return LinettaTheme.accent
        case .ready: return LinettaTheme.success
        case .published: return Color(red: 0.43, green: 0.55, blue: 0.85)
        }
    }
}
