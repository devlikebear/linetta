import AppKit
import LinettaCore
import SwiftUI
import UniformTypeIdentifiers

struct EpisodeWorkspaceView: View {
    let work: Work
    let episodeID: String

    @Environment(AppState.self) private var appState
    @Environment(EpisodeState.self) private var episodeState
    @Environment(ManuscriptState.self) private var manuscript
    @Environment(ToastCenter.self) private var toast

    @State private var episode: Episode?
    @State private var runs: [EpisodeRunResult] = []
    @State private var proposals: [CanonProposal] = []
    @State private var issues: [ContinuityIssue] = []
    @State private var isLoading = false
    @State private var lastRunError: String?
    @State private var showRunErrorDetail = false
    @State private var liveRunID: String?
    @State private var liveEvents: [RunEvent] = []
    @State private var liveLastEvent: RunEvent?
    @State private var streamTask: Task<Void, Never>?

    var body: some View {
        VStack(spacing: 0) {
            EpisodeToolbar(work: work, episode: episode)
                .padding(.horizontal, 18).padding(.vertical, 12)
                .background(LinettaTheme.surface)
            Divider().background(LinettaTheme.border)
            ScrollView {
                VStack(spacing: LinettaShape.sectionGap) {
                    BlueprintCard(work: work, episodeID: episodeID, onSave: { await reload() }, onRun: { await runAgents() })
                    if liveRunID != nil {
                        LiveRunCard(runID: liveRunID ?? "", events: liveEvents, lastEvent: liveLastEvent, onCancel: cancelStream)
                    }
                    if let lastRunError {
                        RunErrorBanner(error: lastRunError, isExpanded: $showRunErrorDetail, onRetry: { Task { await runAgents() } }, onDismiss: { self.lastRunError = nil })
                    }
                    RunHistoryCard(runs: runs)
                    if !proposals.isEmpty || !issues.isEmpty {
                        ReviewQueueCard(workID: work.id, proposals: proposals, issues: issues, onDecision: { await reload() })
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
        .onReceive(NotificationCenter.default.publisher(for: .linettaExportEpisode)) { _ in
            Task { await exportEpisode() }
        }
    }

    private func exportEpisode() async {
        do {
            let text = try await appState.client.exportEpisodeText(workID: work.id, episodeID: episodeID)
            let panel = NSSavePanel()
            panel.allowedContentTypes = [UTType.plainText]
            panel.nameFieldStringValue = "\(episode?.title ?? "episode").txt"
            panel.canCreateDirectories = true
            guard panel.runModal() == .OK, let url = panel.url else { return }
            try text.write(to: url, atomically: true, encoding: .utf8)
            toast.enqueue(.init(title: "Exported to \(url.lastPathComponent)", kind: .success))
        } catch {
            toast.enqueue(.init(title: "Export failed: \(error.localizedDescription)", kind: .error))
        }
    }

    private func reload() async {
        isLoading = true; defer { isLoading = false }
        do {
            let episodes = try await appState.client.listEpisodes(workID: work.id)
            episode = episodes.first { $0.id == episodeID }
            proposals = (try? await appState.client.listProposals(workID: work.id, status: .pending)) ?? []
            issues = (try? await appState.client.listContinuityIssues(workID: work.id, episodeID: episodeID)) ?? []
            await loadLatestManuscript()
        } catch {
            toast.enqueue(.init(title: "Failed to reload episode: \(error.localizedDescription)", kind: .error))
        }
    }

    private func loadLatestManuscript() async {
        let versions = (try? await appState.client.listEpisodeVersions(workID: work.id, episodeID: episodeID)) ?? []
        let latestBody = versions.first?.body ?? ""
        manuscript.loadAdopted(body: latestBody)
    }

    private func runAgents() async {
        cancelStream() // any prior subscription is invalid now
        episodeState.isRunning = true
        lastRunError = nil
        liveEvents = []
        liveLastEvent = nil
        // Server reads the persisted blueprint when running. If the user just
        // applied a suggestion (or typed new values) and hit Run without
        // saving, the server would see an empty/missing blueprint and bail.
        // Always push the current form state before kicking off the run.
        await persistCurrentBlueprint()
        do {
            let start = try await appState.client.runEpisodeAsync(workID: work.id, episodeID: episodeID)
            liveRunID = start.runID
            subscribeToStream(runID: start.runID)
        } catch {
            episodeState.isRunning = false
            lastRunError = error.localizedDescription
            toast.enqueue(.init(title: "Run failed to start — see banner for retry", kind: .error))
        }
    }

    private func persistCurrentBlueprint() async {
        do {
            _ = try await appState.client.saveBlueprint(
                workID: work.id,
                episodeID: episodeID,
                request: SaveBlueprintRequest(
                    premise: episodeState.premise,
                    theme: episodeState.theme,
                    situation: episodeState.situation,
                    mustInclude: episodeState.mustInclude,
                    mustAvoid: episodeState.mustAvoid,
                    structureNotes: episodeState.structureNotes
                )
            )
            episodeState.markSaved()
        } catch {
            // Best-effort: don't block the run on save failure; surface as toast.
            toast.enqueue(.init(title: "Blueprint pre-save failed: \(error.localizedDescription)", kind: .warn))
        }
    }

    private func subscribeToStream(runID: String) {
        streamTask?.cancel()
        let client = appState.client
        streamTask = Task { @MainActor in
            do {
                for try await event in client.eventStream(runID: runID) {
                    if Task.isCancelled { break }
                    liveEvents.append(event)
                    liveLastEvent = event
                }
            } catch {
                lastRunError = error.localizedDescription
            }
            // Stream ended (run completed or stream closed). Refresh + clear live state.
            await onStreamFinished()
        }
    }

    private func onStreamFinished() async {
        let runID = liveRunID
        episodeState.isRunning = false
        // Fetch run artifacts so they appear in Run History card.
        if let runID, let artifacts = try? await appState.client.listRunArtifacts(runID: runID) {
            let events = (try? await appState.client.listRunEvents(runID: runID)) ?? []
            let synthesized = EpisodeRunResult(
                runID: runID,
                tesseraRunID: runID,
                status: "closed",
                closure: "normal",
                artifacts: artifacts,
                events: events
            )
            runs.insert(synthesized, at: 0)
            toast.enqueue(.init(title: "Run completed · \(artifacts.count) artifacts", kind: .success))
        } else if liveLastEvent != nil {
            toast.enqueue(.init(title: "Run finished", kind: .info))
        }
        liveRunID = nil
        liveEvents = []
        liveLastEvent = nil
        await reload()
    }

    private func cancelStream() {
        streamTask?.cancel()
        streamTask = nil
        liveRunID = nil
        liveEvents = []
        liveLastEvent = nil
        episodeState.isRunning = false
    }
}

private struct LiveRunCard: View {
    let runID: String
    let events: [RunEvent]
    let lastEvent: RunEvent?
    let onCancel: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                ProgressView().controlSize(.small)
                Text("Run in progress")
                    .font(LinettaTypography.titleSmall)
                    .foregroundStyle(LinettaTheme.text)
                Text("\(events.count) events")
                    .font(LinettaTypography.caption)
                    .foregroundStyle(LinettaTheme.textTertiary)
                    .padding(.horizontal, 6).padding(.vertical, 1)
                    .background(LinettaTheme.surfaceElevated).clipShape(Capsule())
                Spacer()
                Button("Stop watching", action: onCancel)
                    .buttonStyle(.bordered)
                    .controlSize(.small)
            }
            if let last = lastEvent {
                HStack(spacing: 8) {
                    Text(last.type)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(LinettaTheme.accent)
                    if let role = last.role, !role.isEmpty {
                        Text(role)
                            .font(LinettaTypography.caption)
                            .foregroundStyle(LinettaTheme.textSecondary)
                    }
                    if let stage = last.stage, !stage.isEmpty {
                        Text("· \(stage)")
                            .font(LinettaTypography.caption)
                            .foregroundStyle(LinettaTheme.textTertiary)
                    }
                    Spacer()
                }
            }
            if events.count > 1 {
                ScrollViewReader { proxy in
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: 2) {
                            ForEach(events.suffix(20)) { event in
                                HStack(spacing: 6) {
                                    Text("#\(event.seq)")
                                        .frame(width: 32, alignment: .trailing)
                                        .foregroundStyle(LinettaTheme.textTertiary)
                                    Text(event.type)
                                        .foregroundStyle(LinettaTheme.text)
                                    Spacer()
                                    if let role = event.role { Text(role).foregroundStyle(LinettaTheme.textSecondary) }
                                }
                                .font(.system(.caption2, design: .monospaced))
                                .id(event.id)
                            }
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 6)
                    }
                    .frame(maxHeight: 120)
                    .background(LinettaTheme.surfaceElevated)
                    .clipShape(RoundedRectangle(cornerRadius: 6))
                    .onChange(of: events.count) { _, _ in
                        if let last = events.last { proxy.scrollTo(last.id, anchor: .bottom) }
                    }
                }
            }
        }
        .padding(.horizontal, LinettaShape.cardPaddingH)
        .padding(.vertical, LinettaShape.cardPaddingV)
        .background(LinettaTheme.surface)
        .overlay(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius).stroke(LinettaTheme.accent.opacity(0.5)))
        .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }
}

private struct RunErrorBanner: View {
    let error: String
    @Binding var isExpanded: Bool
    let onRetry: () -> Void
    let onDismiss: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundStyle(LinettaTheme.danger)
                Text("Run Agents failed").font(LinettaTypography.titleSmall).foregroundStyle(LinettaTheme.text)
                Spacer()
                Button(isExpanded ? "Hide details" : "Show details") { isExpanded.toggle() }
                    .buttonStyle(.plain)
                    .font(LinettaTypography.caption)
                    .foregroundStyle(LinettaTheme.accent)
                Button("Retry", action: onRetry)
                    .buttonStyle(.borderedProminent)
                    .tint(LinettaTheme.accent)
                    .controlSize(.small)
                Button {
                    onDismiss()
                } label: {
                    Image(systemName: "xmark")
                }
                .buttonStyle(.plain)
                .foregroundStyle(LinettaTheme.textTertiary)
            }
            if isExpanded {
                ScrollView {
                    Text(error)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(LinettaTheme.textSecondary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .textSelection(.enabled)
                        .padding(8)
                }
                .frame(maxHeight: 120)
                .background(LinettaTheme.surfaceElevated)
                .clipShape(RoundedRectangle(cornerRadius: 6))
            }
        }
        .padding(.horizontal, LinettaShape.cardPaddingH)
        .padding(.vertical, LinettaShape.cardPaddingV)
        .background(LinettaTheme.surface)
        .overlay(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius).stroke(LinettaTheme.danger.opacity(0.4)))
        .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
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
