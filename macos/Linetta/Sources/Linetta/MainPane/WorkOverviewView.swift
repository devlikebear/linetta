import LinettaCore
import SwiftUI

struct WorkOverviewView: View {
    let work: Work

    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    @State private var stats: WorkStats?
    @State private var episodes: [Episode] = []
    @State private var errorMessage: String?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                header
                if !work.premise.isEmpty {
                    premiseSection
                }
                statsSection
                actionsRow
                episodesSection
                Spacer(minLength: 0)
            }
            .padding(28)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
        .background(LinettaTheme.background)
        .task(id: work.id) { await reload() }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(work.title)
                .font(LinettaTypography.titleLarge)
                .foregroundStyle(LinettaTheme.text)
            if !work.genre.isEmpty {
                Text(work.genre)
                    .font(LinettaTypography.body)
                    .foregroundStyle(LinettaTheme.textSecondary)
            }
        }
    }

    private var premiseSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Premise").linettaLabelStyle()
            Text(work.premise)
                .font(LinettaTypography.body)
                .foregroundStyle(LinettaTheme.text)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private var statsSection: some View {
        HStack(spacing: 14) {
            statCell(icon: "list.number", label: "Episodes", value: "\(stats?.episodeCount ?? episodes.count)")
            statCell(icon: "checkmark.circle", label: "Ready", value: "\(stats?.readyCount ?? 0)")
            statCell(icon: "text.word.spacing", label: "Words", value: "\(stats?.wordCount ?? 0)")
            if let s = stats, s.openContinuityIssueCount > 0 {
                statCell(icon: "exclamationmark.triangle", label: "Issues", value: "\(s.openContinuityIssueCount)", warn: true)
            }
            if let s = stats, s.pendingCanonProposalCount > 0 {
                statCell(icon: "sparkles", label: "Proposals", value: "\(s.pendingCanonProposalCount)", warn: true)
            }
            Spacer()
        }
    }

    private func statCell(icon: String, label: String, value: String, warn: Bool = false) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 4) {
                Image(systemName: icon)
                    .font(.system(size: 11))
                    .foregroundStyle(warn ? LinettaTheme.accent : LinettaTheme.textTertiary)
                Text(label)
                    .font(LinettaTypography.caption)
                    .foregroundStyle(LinettaTheme.textTertiary)
            }
            Text(value)
                .font(LinettaTypography.titleSmall)
                .foregroundStyle(LinettaTheme.text)
        }
        .padding(.horizontal, 14).padding(.vertical, 10)
        .background(LinettaTheme.surface)
        .overlay(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius).stroke(LinettaTheme.border))
        .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }

    private var actionsRow: some View {
        HStack(spacing: 10) {
            Button {
                sidebar.selection = .memory(workID: work.id)
            } label: {
                Label("Open Memory", systemImage: "books.vertical")
            }
            .buttonStyle(.bordered)

            if let lastEpisode {
                Button {
                    sidebar.selection = .episode(workID: work.id, episodeID: lastEpisode.id)
                } label: {
                    Label("Open Latest Episode", systemImage: "play.fill")
                }
                .buttonStyle(.borderedProminent)
                .tint(LinettaTheme.accent)
            } else {
                Button {
                    Task { await createFirstEpisode() }
                } label: {
                    Label("Create First Episode", systemImage: "plus")
                }
                .buttonStyle(.borderedProminent)
                .tint(LinettaTheme.accent)
            }
            Spacer()
        }
    }

    private var episodesSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Episodes").linettaLabelStyle()
            if episodes.isEmpty {
                Text("No episodes yet. Click \"Create First Episode\" above.")
                    .font(LinettaTypography.bodySmall)
                    .foregroundStyle(LinettaTheme.textTertiary)
                    .padding(.vertical, 8)
            } else {
                VStack(spacing: 4) {
                    ForEach(episodes) { episode in
                        episodeRow(episode)
                    }
                }
            }
        }
    }

    private func episodeRow(_ episode: Episode) -> some View {
        Button {
            sidebar.selection = .episode(workID: work.id, episodeID: episode.id)
        } label: {
            HStack(spacing: 10) {
                Circle().fill(statusColor(episode)).frame(width: 7, height: 7)
                Text(episode.title)
                    .font(LinettaTypography.body)
                    .foregroundStyle(LinettaTheme.text)
                Spacer()
                Text(String(describing: episode.status).capitalized)
                    .font(LinettaTypography.caption)
                    .foregroundStyle(LinettaTheme.textTertiary)
                Image(systemName: "chevron.right")
                    .font(.system(size: 10))
                    .foregroundStyle(LinettaTheme.textTertiary)
            }
            .padding(.horizontal, 12).padding(.vertical, 8)
            .background(LinettaTheme.surface)
            .clipShape(RoundedRectangle(cornerRadius: 7))
        }
        .buttonStyle(.plain)
    }

    private var lastEpisode: Episode? { episodes.last }

    private func statusColor(_ episode: Episode) -> Color {
        switch episode.status {
        case .idea: return LinettaTheme.textTertiary
        case .drafting: return Color(red: 0.95, green: 0.78, blue: 0.34)
        case .reviewing: return LinettaTheme.accent
        case .outlined: return Color(red: 0.55, green: 0.66, blue: 0.78)
        case .ready: return LinettaTheme.success
        case .published: return Color(red: 0.43, green: 0.55, blue: 0.85)
        }
    }

    private func reload() async {
        do {
            async let s = try? await appState.client.workStats(workID: work.id)
            async let e = (try? await appState.client.listEpisodes(workID: work.id)) ?? []
            stats = await s
            episodes = await e
        }
    }

    private func createFirstEpisode() async {
        do {
            let episode = try await appState.client.createEpisode(workID: work.id, request: .init(title: "Episode 1"))
            episodes.append(episode)
            sidebar.selection = .episode(workID: work.id, episodeID: episode.id)
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
