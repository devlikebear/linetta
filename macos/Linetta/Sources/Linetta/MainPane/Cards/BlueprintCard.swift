import LinettaCore
import SwiftUI

struct BlueprintCard: View {
    let work: Work
    let episodeID: String
    var onSave: () async -> Void = {}
    var onRun: () async -> Void = {}
    var body: some View {
        Text("Blueprint card stub")
            .padding()
            .background(LinettaTheme.surface)
            .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }
}
