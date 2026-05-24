import LinettaCore
import SwiftUI

struct MemoryDiffView: View {
    var proposals: [CanonProposal]
    @Binding var selectedProposal: CanonProposal?
    var issues: [ContinuityIssue]
    @Binding var selectedIssue: ContinuityIssue?
    var approveProposal: (CanonProposal) -> Void
    var rejectProposal: (CanonProposal) -> Void
    var deferProposal: (CanonProposal) -> Void
    var acceptIssue: (ContinuityIssue) -> Void
    var resolveIssue: (ContinuityIssue) -> Void
    var ignoreIssue: (ContinuityIssue) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 18) {
                Text("Memory Review")
                    .font(.headline)
                Label("\(proposals.count)", systemImage: "sparkles")
                    .foregroundStyle(.secondary)
                Label("\(issues.count)", systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.secondary)
                Spacer()
            }

            HSplitView {
                reviewList
                    .frame(minWidth: 220)
                detailPanel
                    .frame(minWidth: 320)
            }
        }
        .padding(.top, 8)
        .onChange(of: selectedProposal) { _, value in
            if value != nil {
                selectedIssue = nil
            }
        }
        .onChange(of: selectedIssue) { _, value in
            if value != nil {
                selectedProposal = nil
            }
        }
    }

    private var reviewList: some View {
        VStack(alignment: .leading, spacing: 12) {
            VStack(alignment: .leading, spacing: 8) {
                Text("Proposals")
                    .font(.subheadline)
                    .fontWeight(.semibold)
                if proposals.isEmpty {
                    ContentUnavailableView("No Pending Proposal", systemImage: "checkmark.circle")
                } else {
                    List(selection: $selectedProposal) {
                        ForEach(proposals) { proposal in
                            ProposalRow(proposal: proposal)
                                .tag(proposal)
                        }
                    }
                    .frame(minHeight: 160)
                }
            }

            VStack(alignment: .leading, spacing: 8) {
                Text("Continuity")
                    .font(.subheadline)
                    .fontWeight(.semibold)
                if issues.isEmpty {
                    ContentUnavailableView("No Issue", systemImage: "checkmark.seal")
                } else {
                    List(selection: $selectedIssue) {
                        ForEach(issues) { issue in
                            IssueRow(issue: issue)
                                .tag(issue)
                        }
                    }
                    .frame(minHeight: 150)
                }
            }
        }
    }

    @ViewBuilder
    private var detailPanel: some View {
        if let selectedProposal {
            proposalDetail(selectedProposal)
        } else if let selectedIssue {
            issueDetail(selectedIssue)
        } else {
            ContentUnavailableView("Select Review Item", systemImage: "sidebar.left")
        }
    }

    private func proposalDetail(_ proposal: CanonProposal) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .firstTextBaseline) {
                Text(proposal.title)
                    .font(.headline)
                Spacer()
                Text(proposal.changeType.label)
                    .foregroundStyle(.secondary)
                Text(proposal.confidence.formatted(.percent.precision(.fractionLength(0))))
                    .foregroundStyle(.secondary)
            }

            HSplitView {
                bodyColumn(title: "Before", text: proposal.beforeBody)
                bodyColumn(title: "After", text: proposal.afterBody)
            }

            if !proposal.reason.isEmpty {
                Text(proposal.reason)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }

            HStack {
                Spacer()
                Button {
                    deferProposal(proposal)
                } label: {
                    Label("Defer", systemImage: "clock")
                }
                Button(role: .destructive) {
                    rejectProposal(proposal)
                } label: {
                    Label("Reject", systemImage: "xmark")
                }
                Button {
                    approveProposal(proposal)
                } label: {
                    Label("Approve", systemImage: "checkmark")
                }
                .buttonStyle(.borderedProminent)
            }
        }
    }

    private func issueDetail(_ issue: ContinuityIssue) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .firstTextBaseline) {
                Text(issue.title)
                    .font(.headline)
                Spacer()
                Label(issue.severity.label, systemImage: issue.severity.symbolName)
                    .foregroundStyle(issue.severity.tint)
                Text(issue.status.label)
                    .foregroundStyle(.secondary)
            }

            ScrollView {
                VStack(alignment: .leading, spacing: 10) {
                    Text(issue.body)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .textSelection(.enabled)
                    if !issue.relatedItemIDs.isEmpty {
                        Label(issue.relatedItemIDs, systemImage: "link")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .textSelection(.enabled)
                    }
                }
            }

            HStack {
                Spacer()
                Button {
                    ignoreIssue(issue)
                } label: {
                    Label("Ignore", systemImage: "eye.slash")
                }
                Button {
                    acceptIssue(issue)
                } label: {
                    Label("Accept", systemImage: "hand.thumbsup")
                }
                Button {
                    resolveIssue(issue)
                } label: {
                    Label("Resolve", systemImage: "checkmark")
                }
                .buttonStyle(.borderedProminent)
            }
        }
    }

    private func bodyColumn(title: String, text: String) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title)
                .font(.subheadline)
                .fontWeight(.semibold)
            ScrollView {
                Text(text.isEmpty ? "Empty" : text)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .foregroundStyle(text.isEmpty ? .secondary : .primary)
                    .textSelection(.enabled)
            }
        }
    }
}

private struct ProposalRow: View {
    var proposal: CanonProposal

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Image(systemName: "sparkles")
                    .foregroundStyle(.secondary)
                Text(proposal.title)
                    .font(.headline)
                    .lineLimit(1)
            }
            Text(proposal.kind.label)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(.vertical, 4)
    }
}

private struct IssueRow: View {
    var issue: ContinuityIssue

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Image(systemName: issue.severity.symbolName)
                    .foregroundStyle(issue.severity.tint)
                Text(issue.title)
                    .font(.headline)
                    .lineLimit(1)
            }
            Text(issue.status.label)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(.vertical, 4)
    }
}

private extension ProposalChangeType {
    var label: String {
        switch self {
        case .create: "Create"
        case .update: "Update"
        case .archive: "Archive"
        case .link: "Link"
        }
    }
}

private extension ContinuityIssueSeverity {
    var label: String {
        switch self {
        case .info: "Info"
        case .warning: "Warning"
        case .blocker: "Blocker"
        }
    }

    var symbolName: String {
        switch self {
        case .info: "info.circle"
        case .warning: "exclamationmark.triangle"
        case .blocker: "octagon"
        }
    }

    var tint: Color {
        switch self {
        case .info: .secondary
        case .warning: .orange
        case .blocker: .red
        }
    }
}

private extension ContinuityIssueStatus {
    var label: String {
        switch self {
        case .open: "Open"
        case .accepted: "Accepted"
        case .resolved: "Resolved"
        case .ignored: "Ignored"
        }
    }
}
