import LinettaCore
import SwiftUI

struct ArtifactPreviewView: View {
    let text: String

    @Environment(ManuscriptState.self) private var manuscript
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    var body: some View {
        VStack(spacing: 0) {
            Text("Preview · read-only")
                .font(LinettaTypography.caption).foregroundStyle(LinettaTheme.text)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 6).background(Color(red: 0.25, green: 0.40, blue: 0.65).opacity(0.7))
            ScrollView {
                Text(text).font(LinettaTypography.bodySerif).padding(14).frame(maxWidth: .infinity, alignment: .leading)
                    .foregroundStyle(LinettaTheme.text)
            }
            Button("Adopt as new version") { Task { await adopt() } }
                .buttonStyle(.borderedProminent).tint(LinettaTheme.accent)
                .padding(12)
        }
    }

    private func adopt() async {
        guard case .episode(let wid, let eid) = sidebar.selection else { return }
        _ = try? await appState.client.createEpisodeVersion(
            workID: wid, episodeID: eid,
            request: CreateEpisodeVersionRequest(body: text, note: "adopt from artifact")
        )
        manuscript.loadAdopted(body: text)
    }
}
