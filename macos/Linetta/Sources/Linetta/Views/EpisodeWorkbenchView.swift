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
    @State private var artifacts: [Artifact] = []
    @State private var events: [RunEvent] = []
    @State private var selectedArtifact: Artifact?
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
                            Text(episode.status.capitalized)
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
        VStack(alignment: .leading, spacing: 12) {
            Text("Agent Artifacts")
                .font(.headline)
            List(selection: $selectedArtifact) {
                ForEach(artifacts) { artifact in
                    Text(artifact.title)
                        .tag(artifact)
                }
            }
            if let selectedArtifact {
                ScrollView {
                    Text(selectedArtifact.body)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .textSelection(.enabled)
                }
                .frame(minHeight: 180)
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
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func loadBlueprint(for episode: Episode) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
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
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
