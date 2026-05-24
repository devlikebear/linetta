import LinettaCore
import SwiftUI

struct EpisodeWorkbenchView: View {
    var work: Work

    @State private var episodes: [Episode] = []
    @State private var selectedEpisode: Episode?
    @State private var premise = ""
    @State private var theme = ""
    @State private var situation = ""
    @State private var mustInclude = ""
    @State private var mustAvoid = ""
    @State private var structureNotes = ""
    @State private var episodeStatus: EpisodeStatus = .idea
    @State private var artifacts: [Artifact] = []
    @State private var events: [RunEvent] = []
    @State private var selectedArtifact: Artifact?
    @State private var versions: [EpisodeVersion] = []
    @State private var selectedVersion: EpisodeVersion?
    @State private var manuscriptBody = ""
    @State private var proposals: [CanonProposal] = []
    @State private var selectedProposal: CanonProposal?
    @State private var continuityIssues: [ContinuityIssue] = []
    @State private var selectedIssue: ContinuityIssue?
    @State private var isLoading = false
    @State private var errorMessage: String?

    private let client = APIClient()

    var body: some View {
        NavigationSplitView {
            VStack(spacing: 12) {
                List(selection: $selectedEpisode) {
                    ForEach(episodes) { episode in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(episode.title)
                                .font(.headline)
                            Text(episode.status.label)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        .tag(episode)
                    }
                }
                Button {
                    Task { await createEpisode() }
                } label: {
                    Label("New Episode", systemImage: "plus")
                }
                .buttonStyle(.borderedProminent)
            }
            .padding(16)
            .navigationTitle("Episodes")
            .task {
                await loadEpisodes()
            }
        } detail: {
            if let selectedEpisode {
                VStack(spacing: 0) {
                    HStack {
                        Text(selectedEpisode.title)
                            .font(.title2)
                            .fontWeight(.semibold)
                        Spacer()
                        if isLoading {
                            ProgressView()
                        }
                        Picker("Status", selection: $episodeStatus) {
                            ForEach(EpisodeStatus.allCases) { status in
                                Text(status.label).tag(status)
                            }
                        }
                        .frame(width: 150)
                        Button {
                            Task { await updateSelectedEpisodeStatus() }
                        } label: {
                            Label("Update Status", systemImage: "checkmark.circle")
                        }
                        .disabled(episodeStatus == selectedEpisode.status)
                        Button("Save Blueprint") {
                            Task { await saveBlueprint() }
                        }
                        Button("Run Agents") {
                            Task { await runAgents() }
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(premise.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    }
                    .padding([.horizontal, .top], 18)

                    HSplitView {
                        blueprintEditor
                            .frame(minWidth: 360)
                        runPanel
                            .frame(minWidth: 360)
                    }
                    .padding(18)

                    if let errorMessage {
                        Text(errorMessage)
                            .foregroundStyle(.red)
                            .padding(.horizontal, 18)
                            .padding(.bottom, 12)
                    }
                }
                .task(id: selectedEpisode.id) {
                    await loadBlueprint(for: selectedEpisode)
                    await loadReview(for: selectedEpisode)
                    await loadVersions(for: selectedEpisode)
                }
            } else {
                VStack(spacing: 12) {
                    Image(systemName: "rectangle.and.pencil.and.ellipsis")
                        .font(.system(size: 42))
                        .foregroundStyle(.secondary)
                    Text("Create an episode to start the workbench")
                        .font(.title2)
                        .fontWeight(.semibold)
                    Button("New Episode") {
                        Task { await createEpisode() }
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
    }

    private var blueprintEditor: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                Text("Human Blueprint")
                    .font(.headline)
                TextField("Premise", text: $premise)
                TextField("Theme", text: $theme)
                TextField("Situation", text: $situation)
                TextField("Must include", text: $mustInclude)
                TextField("Must avoid", text: $mustAvoid)
                TextEditor(text: $structureNotes)
                    .frame(minHeight: 180)
                    .overlay {
                        RoundedRectangle(cornerRadius: 6)
                            .stroke(Color.secondary.opacity(0.25))
                    }
            }
            .padding(.trailing, 10)
        }
    }

    private var runPanel: some View {
        TabView {
            artifactTimelinePanel
                .tabItem {
                    Label("Artifacts", systemImage: "doc.text")
                }
            ManuscriptVersionView(
                bodyText: $manuscriptBody,
                versions: versions,
                selectedVersion: $selectedVersion,
                selectedArtifact: selectedArtifact,
                saveVersion: {
                    Task { await saveManuscriptVersion() }
                },
                adoptArtifact: { artifact in
                    Task { await adopt(artifact: artifact) }
                },
                restoreVersion: { version in
                    Task { await restore(version: version) }
                }
            )
            .tabItem {
                Label("Manuscript", systemImage: "doc.plaintext")
            }
            MemoryDiffView(
                proposals: proposals,
                selectedProposal: $selectedProposal,
                issues: continuityIssues,
                selectedIssue: $selectedIssue,
                approveProposal: { proposal in
                    Task { await approve(proposal: proposal) }
                },
                rejectProposal: { proposal in
                    Task { await reject(proposal: proposal) }
                },
                deferProposal: { proposal in
                    Task { await deferDecision(proposal: proposal) }
                },
                acceptIssue: { issue in
                    Task { await update(issue: issue, status: .accepted) }
                },
                resolveIssue: { issue in
                    Task { await update(issue: issue, status: .resolved) }
                },
                ignoreIssue: { issue in
                    Task { await update(issue: issue, status: .ignored) }
                }
            )
            .tabItem {
                Label("Review", systemImage: "checkmark.seal")
            }
        }
    }

    private var artifactTimelinePanel: some View {
        VStack(alignment: .leading, spacing: 12) {
            HSplitView {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Agent Artifacts")
                        .font(.headline)
                    List(selection: $selectedArtifact) {
                        ForEach(artifacts) { artifact in
                            Text(artifact.title)
                                .tag(artifact)
                        }
                    }
                }
                .frame(minWidth: 160)

                if let selectedArtifact {
                    ScrollView {
                        Text(selectedArtifact.body)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .textSelection(.enabled)
                    }
                    .frame(minWidth: 260)
                } else {
                    ContentUnavailableView("No Artifact", systemImage: "doc.text")
                        .frame(minWidth: 260)
                }
            }
            Text("Run Timeline")
                .font(.headline)
            List(events) { event in
                HStack {
                    Text("#\(event.seq)")
                        .foregroundStyle(.secondary)
                    Text(event.type)
                    Spacer()
                    if let role = event.role {
                        Text(role)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            .frame(minHeight: 160)
        }
    }

    private func loadEpisodes() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            episodes = try await client.listEpisodes(workID: work.id)
            selectedEpisode = selectedEpisode ?? episodes.first
            episodeStatus = selectedEpisode?.status ?? .idea
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func createEpisode() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let episode = try await client.createEpisode(
                workID: work.id,
                request: CreateEpisodeRequest(title: "Episode \(episodes.count + 1)")
            )
            episodes.append(episode)
            selectedEpisode = episode
            episodeStatus = episode.status
            proposals = []
            selectedProposal = nil
            continuityIssues = []
            selectedIssue = nil
            versions = []
            selectedVersion = nil
            manuscriptBody = ""
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func loadBlueprint(for episode: Episode) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            episodeStatus = episode.status
            let blueprint = try await client.getBlueprint(workID: work.id, episodeID: episode.id)
            premise = blueprint.premise
            theme = blueprint.theme
            situation = blueprint.situation
            mustInclude = blueprint.mustInclude
            mustAvoid = blueprint.mustAvoid
            structureNotes = blueprint.structureNotes
        } catch {
            premise = ""
            theme = ""
            situation = ""
            mustInclude = ""
            mustAvoid = ""
            structureNotes = ""
        }
    }

    private func saveBlueprint() async {
        guard let selectedEpisode else {
            return
        }
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            _ = try await client.saveBlueprint(
                workID: work.id,
                episodeID: selectedEpisode.id,
                request: SaveBlueprintRequest(
                    premise: premise,
                    theme: theme,
                    situation: situation,
                    mustInclude: mustInclude,
                    mustAvoid: mustAvoid,
                    structureNotes: structureNotes
                )
            )
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func runAgents() async {
        guard let selectedEpisode else {
            return
        }
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            _ = try await client.saveBlueprint(
                workID: work.id,
                episodeID: selectedEpisode.id,
                request: SaveBlueprintRequest(
                    premise: premise,
                    theme: theme,
                    situation: situation,
                    mustInclude: mustInclude,
                    mustAvoid: mustAvoid,
                    structureNotes: structureNotes
                )
            )
            let result = try await client.runEpisode(workID: work.id, episodeID: selectedEpisode.id)
            artifacts = result.artifacts
            events = result.events
            selectedArtifact = artifacts.first(where: { $0.kind == .draft }) ?? artifacts.first
            await loadReview(for: selectedEpisode)
            await loadVersions(for: selectedEpisode)
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func loadVersions(for episode: Episode) async {
        do {
            versions = try await client.listEpisodeVersions(workID: work.id, episodeID: episode.id)
            selectedVersion = versions.first
            manuscriptBody = versions.first?.body ?? ""
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func saveManuscriptVersion() async {
        guard let selectedEpisode else {
            return
        }
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let saved = try await client.createEpisodeVersion(
                workID: work.id,
                episodeID: selectedEpisode.id,
                request: CreateEpisodeVersionRequest(body: manuscriptBody, note: "manual save")
            )
            manuscriptBody = saved.body
            await loadVersions(for: selectedEpisode)
            selectedVersion = saved
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func adopt(artifact: Artifact) async {
        guard let selectedEpisode else {
            return
        }
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let saved = try await client.createEpisodeVersion(
                workID: work.id,
                episodeID: selectedEpisode.id,
                request: CreateEpisodeVersionRequest(
                    sourceArtifactID: artifact.id,
                    body: artifact.body,
                    note: "adopt \(artifact.title)"
                )
            )
            manuscriptBody = saved.body
            await loadVersions(for: selectedEpisode)
            selectedVersion = saved
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func restore(version: EpisodeVersion) async {
        guard let selectedEpisode else {
            return
        }
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let saved = try await client.createEpisodeVersion(
                workID: work.id,
                episodeID: selectedEpisode.id,
                request: CreateEpisodeVersionRequest(
                    sourceArtifactID: version.sourceArtifactID,
                    body: version.body,
                    note: "restore \(version.id)"
                )
            )
            manuscriptBody = saved.body
            await loadVersions(for: selectedEpisode)
            selectedVersion = saved
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func loadReview(for episode: Episode) async {
        do {
            async let proposalResult = client.listProposals(workID: work.id, status: .pending)
            async let issueResult = client.listContinuityIssues(workID: work.id, episodeID: episode.id)
            let loadedProposals = try await proposalResult
            let loadedIssues = try await issueResult
            proposals = loadedProposals.filter { $0.episodeID == episode.id }
            continuityIssues = loadedIssues
            selectedProposal = proposals.first
            selectedIssue = continuityIssues.first
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func approve(proposal: CanonProposal) async {
        await decide(proposal: proposal) {
            try await client.approveProposal(proposalID: proposal.id)
        }
    }

    private func reject(proposal: CanonProposal) async {
        await decide(proposal: proposal) {
            try await client.rejectProposal(proposalID: proposal.id)
        }
    }

    private func deferDecision(proposal: CanonProposal) async {
        await decide(proposal: proposal) {
            try await client.deferProposal(proposalID: proposal.id)
        }
    }

    private func decide(proposal: CanonProposal, action: () async throws -> CanonProposal) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            _ = try await action()
            if let selectedEpisode {
                await loadReview(for: selectedEpisode)
            }
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func update(issue: ContinuityIssue, status: ContinuityIssueStatus) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            _ = try await client.updateContinuityIssue(issueID: issue.id, status: status)
            if let selectedEpisode {
                await loadReview(for: selectedEpisode)
            }
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func updateSelectedEpisodeStatus() async {
        guard let selectedEpisode else {
            return
        }
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let updated = try await client.updateEpisodeStatus(
                workID: work.id,
                episodeID: selectedEpisode.id,
                status: episodeStatus
            )
            self.selectedEpisode = updated
            episodeStatus = updated.status
            if let index = episodes.firstIndex(where: { $0.id == updated.id }) {
                episodes[index] = updated
            }
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
