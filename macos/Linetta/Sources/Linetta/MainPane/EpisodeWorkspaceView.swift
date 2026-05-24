import LinettaCore
import SwiftUI

struct EpisodeWorkspaceView: View {
    let work: Work
    let episodeID: String

    @Environment(AppState.self) private var appState
    @Environment(EpisodeState.self) private var episodeState
    @Environment(ManuscriptState.self) private var manuscript

    @State private var episode: Episode?
    @State private var runs: [EpisodeRunResult] = []
    @State private var proposals: [CanonProposal] = []
    @State private var issues: [ContinuityIssue] = []
    @State private var isLoading = false
    @State private var errorMessage: String?

    var body: some View {
        VStack(spacing: 0) {
            EpisodeToolbar(work: work, episode: episode)
                .padding(.horizontal, 18).padding(.vertical, 12)
                .background(LinettaTheme.surface)
            Divider().background(LinettaTheme.border)
            ScrollView {
                VStack(spacing: LinettaShape.sectionGap) {
                    BlueprintCard(work: work, episodeID: episodeID, onSave: { await reload() }, onRun: { await runAgents() })
                    RunHistoryCard(runs: runs)
                    if !proposals.isEmpty || !issues.isEmpty {
                        ReviewQueueCard(workID: work.id, proposals: proposals, issues: issues)
                    }
                }
                .padding(LinettaShape.mainContentPadding)
            }
        }
        .background(LinettaTheme.background)
        .task(id: episodeID) {
            episodeState.expandedRunID = nil
            await reload()
        }
    }

    private func reload() async {
        isLoading = true; defer { isLoading = false }
        do {
            let episodes = try await appState.client.listEpisodes(workID: work.id)
            episode = episodes.first { $0.id == episodeID }
            proposals = (try? await appState.client.listProposals(workID: work.id, status: .pending)) ?? []
            issues = (try? await appState.client.listContinuityIssues(workID: work.id, episodeID: episodeID)) ?? []
        } catch { errorMessage = error.localizedDescription }
    }

    private func runAgents() async {
        episodeState.isRunning = true; defer { episodeState.isRunning = false }
        do {
            let result = try await appState.client.runEpisode(workID: work.id, episodeID: episodeID)
            runs.insert(result, at: 0)
            await reload()
        } catch { errorMessage = error.localizedDescription }
    }
}

private struct EpisodeToolbar: View {
    let work: Work
    let episode: Episode?
    @Environment(ManuscriptState.self) private var manuscript

    var body: some View {
        HStack(spacing: 12) {
            (Text("\(work.title)  ›  ")
                .font(LinettaTypography.bodySmall).foregroundStyle(LinettaTheme.textSecondary)
            + Text(episode?.title ?? "—")
                .font(LinettaTypography.body).foregroundStyle(LinettaTheme.text).bold())
            Spacer()
            if let episode {
                Text(statusLabel(for: episode)).font(LinettaTypography.caption)
                    .padding(.horizontal, 8).padding(.vertical, 2)
                    .background(LinettaTheme.surface).clipShape(Capsule())
                    .foregroundStyle(LinettaTheme.textSecondary)
            }
            Button {
                if let id = episode?.id { manuscript.setOpen(episodeID: id, open: !manuscript.isOpen(episodeID: id)) }
            } label: { Label("Manuscript", systemImage: "doc.text") }
                .keyboardShortcut("m", modifiers: [.command, .shift])
        }
    }

    private func statusLabel(for episode: Episode) -> String {
        String(describing: episode.status).capitalized
    }
}
