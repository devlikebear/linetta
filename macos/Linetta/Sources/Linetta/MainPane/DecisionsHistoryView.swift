import LinettaCore
import SwiftUI

struct DecisionsHistoryView: View {
    let work: Work

    @Environment(AppState.self) private var appState

    @State private var decisions: [MemoryDecision] = []
    @State private var isLoading = false
    @State private var errorMessage: String?
    @State private var canonItems: [String: MemoryItem] = [:]

    var body: some View {
        VStack(spacing: 0) {
            header
                .padding(.horizontal, 18).padding(.vertical, 12)
                .background(LinettaTheme.surface)
            Divider().background(LinettaTheme.border)
            content
        }
        .background(LinettaTheme.background)
        .task(id: work.id) { await reload() }
    }

    private var header: some View {
        HStack {
            Text("Canon Decisions").font(LinettaTypography.titleSmall).foregroundStyle(LinettaTheme.text)
            Text(work.title).font(LinettaTypography.bodySmall).foregroundStyle(LinettaTheme.textSecondary)
            Spacer()
            Text("\(decisions.count) entries")
                .font(LinettaTypography.caption)
                .foregroundStyle(LinettaTheme.textTertiary)
            Button { Task { await reload() } } label: {
                Image(systemName: "arrow.clockwise")
            }
            .buttonStyle(.plain)
            .foregroundStyle(LinettaTheme.textSecondary)
        }
    }

    @ViewBuilder
    private var content: some View {
        if decisions.isEmpty {
            VStack(spacing: 8) {
                Image(systemName: "clock.arrow.circlepath")
                    .font(.system(size: 28))
                    .foregroundStyle(LinettaTheme.textTertiary)
                Text(isLoading ? "Loading…" : "No canon decisions yet.")
                    .font(LinettaTypography.body)
                    .foregroundStyle(LinettaTheme.textSecondary)
                Text("Decisions appear here after you approve, reject, or defer canon proposals in the Review queue.")
                    .font(LinettaTypography.caption)
                    .foregroundStyle(LinettaTheme.textTertiary)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: 460)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 0) {
                    ForEach(decisions) { decision in
                        decisionRow(decision)
                        Divider().background(LinettaTheme.borderSoft)
                    }
                }
                .padding(.horizontal, 18)
            }
        }
    }

    private func decisionRow(_ decision: MemoryDecision) -> some View {
        HStack(alignment: .top, spacing: 12) {
            kindBadge(for: decision.decisionType)
                .frame(width: 86, alignment: .leading)
            VStack(alignment: .leading, spacing: 4) {
                Text(canonItemTitle(for: decision.canonItemID))
                    .font(LinettaTypography.body)
                    .foregroundStyle(LinettaTheme.text)
                if !decision.reason.isEmpty {
                    Text(decision.reason)
                        .font(LinettaTypography.bodySmall)
                        .foregroundStyle(LinettaTheme.textSecondary)
                }
            }
            Spacer()
            VStack(alignment: .trailing, spacing: 4) {
                Text(decision.actor)
                    .font(LinettaTypography.caption)
                    .foregroundStyle(LinettaTheme.textSecondary)
                Text(decision.createdAt)
                    .font(LinettaTypography.caption)
                    .foregroundStyle(LinettaTheme.textTertiary)
                    .lineLimit(1)
            }
        }
        .padding(.vertical, 10)
    }

    private func canonItemTitle(for itemID: String) -> String {
        canonItems[itemID]?.title ?? "Canon item · \(String(itemID.prefix(8)))"
    }

    private func kindBadge(for type: String) -> some View {
        Text(type.uppercased())
            .font(LinettaTypography.label)
            .tracking(0.5)
            .padding(.horizontal, 7).padding(.vertical, 2)
            .background(color(for: type).opacity(0.18))
            .foregroundStyle(color(for: type))
            .clipShape(Capsule())
    }

    private func color(for type: String) -> Color {
        switch type.lowercased() {
        case "approve", "approved": return LinettaTheme.success
        case "reject", "rejected": return LinettaTheme.danger
        case "defer", "deferred": return Color(red: 0.95, green: 0.78, blue: 0.34)
        default: return LinettaTheme.textSecondary
        }
    }

    private func reload() async {
        isLoading = true; defer { isLoading = false }
        do {
            decisions = try await appState.client.listMemoryDecisions(workID: work.id)
            await preloadCanonItems()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func preloadCanonItems() async {
        // Bulk-load canon items so we can show titles instead of opaque IDs.
        guard !decisions.isEmpty else { return }
        let items = (try? await appState.client.listMemory(workID: work.id, kind: nil, status: nil, query: nil)) ?? []
        var map: [String: MemoryItem] = [:]
        for item in items { map[item.id] = item }
        canonItems = map
    }
}
