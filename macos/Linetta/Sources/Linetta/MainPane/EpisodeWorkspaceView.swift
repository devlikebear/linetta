import LinettaCore
import SwiftUI

struct EpisodeWorkspaceView: View {
    let work: Work
    let episodeID: String
    var body: some View { Text("Episode \(episodeID) · \(work.title)").foregroundStyle(LinettaTheme.text) }
}
