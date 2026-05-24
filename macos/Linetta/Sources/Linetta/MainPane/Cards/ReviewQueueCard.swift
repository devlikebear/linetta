import LinettaCore
import SwiftUI

struct ReviewQueueCard: View {
    let workID: String
    let proposals: [CanonProposal]
    let issues: [ContinuityIssue]

    @Environment(AppState.self) private var appState

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Review Queue").linettaLabelStyle()
                Text("\(proposals.count + issues.count) pending")
                    .font(LinettaTypography.caption).foregroundStyle(LinettaTheme.accent)
                    .padding(.horizontal, 6).padding(.vertical, 1)
                    .background(LinettaTheme.accentSoft).clipShape(Capsule())
                Spacer()
                Text("work-level · across all episodes")
                    .font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary)
            }
            ForEach(proposals) { p in
                ReviewRowView(
                    kind: .canon,
                    title: p.title,
                    source: "Run #\(String(p.runID.prefix(6)))",
                    onApprove: { Task { _ = try? await appState.client.approveProposal(proposalID: p.id) } },
                    onReject: { Task { _ = try? await appState.client.rejectProposal(proposalID: p.id) } },
                    onDefer: { Task { _ = try? await appState.client.deferProposal(proposalID: p.id) } }
                )
            }
            ForEach(issues) { i in
                ReviewRowView(
                    kind: .continuity,
                    title: i.title,
                    source: "Ep \(String(i.episodeID.prefix(6)))",
                    onApprove: { Task { _ = try? await appState.client.updateContinuityIssue(issueID: i.id, status: .accepted) } },
                    onReject: { Task { _ = try? await appState.client.updateContinuityIssue(issueID: i.id, status: .ignored) } },
                    onDefer: { Task { _ = try? await appState.client.updateContinuityIssue(issueID: i.id, status: .resolved) } }
                )
            }
        }
        .padding(.horizontal, LinettaShape.cardPaddingH)
        .padding(.vertical, LinettaShape.cardPaddingV)
        .background(LinettaTheme.surface)
        .overlay(
            RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius)
                .stroke(LinettaTheme.warn)
        )
        .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }
}
