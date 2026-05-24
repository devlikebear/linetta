import LinettaCore
import SwiftUI

struct ReviewQueueCard: View {
    let workID: String
    let proposals: [CanonProposal]
    let issues: [ContinuityIssue]
    var body: some View {
        Text("Review queue stub (\(proposals.count) proposals, \(issues.count) issues)")
            .padding()
            .background(LinettaTheme.surface)
            .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }
}
