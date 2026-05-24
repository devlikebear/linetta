import LinettaCore
import SwiftUI

struct RunHistoryCard: View {
    let runs: [EpisodeRunResult]
    var body: some View {
        Text("Run history stub (\(runs.count) runs)")
            .padding()
            .background(LinettaTheme.surface)
            .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }
}
