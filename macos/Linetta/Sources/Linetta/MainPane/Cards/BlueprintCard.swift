import LinettaCore
import SwiftUI

struct BlueprintCard: View {
    let work: Work
    let episodeID: String
    var onSave: () async -> Void = {}
    var onRun: () async -> Void = {}

    @Environment(AppState.self) private var appState
    @Environment(EpisodeState.self) private var episodeState
    @Environment(ToastCenter.self) private var toast

    @State private var loaded = false
    @State private var suggesting = false

    var body: some View {
        @Bindable var episodeState = episodeState
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Blueprint").linettaLabelStyle()
                Spacer()
                Button {
                    episodeState.setBlueprintExpanded(episodeID: episodeID, expanded: !episodeState.isBlueprintExpanded(episodeID: episodeID))
                } label: {
                    Image(systemName: episodeState.isBlueprintExpanded(episodeID: episodeID) ? "chevron.up" : "chevron.down")
                        .font(.system(size: 10))
                        .foregroundStyle(LinettaTheme.textTertiary)
                }
                .buttonStyle(.plain)
            }

            if episodeState.isBlueprintExpanded(episodeID: episodeID) {
                VStack(alignment: .leading, spacing: 8) {
                    field("Premise", text: $episodeState.premise)
                    field("Theme", text: $episodeState.theme)
                    field("Situation", text: $episodeState.situation)
                    field("Must include", text: $episodeState.mustInclude)
                    field("Must avoid", text: $episodeState.mustAvoid)
                    HStack {
                        Button { Task { await suggest() } } label: {
                            Label(suggesting ? "Suggesting…" : "Suggest", systemImage: "sparkles")
                        }
                        .buttonStyle(.bordered)
                        .help("Auto-fill empty fields using Canon memory + LLM (preserves whatever you've already typed). ⌥⌘S")
                        .keyboardShortcut("s", modifiers: [.command, .option])
                        .disabled(suggesting)

                        Button { Task { await save() } } label: { Label("Save", systemImage: "tray.and.arrow.down") }
                            .keyboardShortcut("s", modifiers: [.command])
                            .disabled(!episodeState.isDirty)
                        Button { Task { await onRun() } } label: { Label("Run Agents", systemImage: "play.fill") }
                            .buttonStyle(.borderedProminent).tint(LinettaTheme.accent)
                            .keyboardShortcut("r", modifiers: [.command, .shift])
                            .disabled(episodeState.premise.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                        Spacer()
                    }
                }
            } else {
                collapsedRow
            }
        }
        .padding(.horizontal, LinettaShape.cardPaddingH)
        .padding(.vertical, LinettaShape.cardPaddingV)
        .background(LinettaTheme.surface)
        .overlay(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius).stroke(LinettaTheme.border))
        .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
        .task(id: episodeID) { await loadOnce() }
    }

    private var collapsedRow: some View {
        HStack(spacing: 10) {
            Text(episodeState.premise.isEmpty ? "Write a premise to enable run." : episodeState.premise)
                .font(LinettaTypography.bodySmall)
                .foregroundStyle(LinettaTheme.textSecondary)
                .lineLimit(1)
            Spacer()
            Button { Task { await onRun() } } label: {
                Label("Run Agents", systemImage: "play.fill")
            }
            .buttonStyle(.borderedProminent).tint(LinettaTheme.accent)
            .keyboardShortcut("r", modifiers: [.command, .shift])
            .disabled(episodeState.premise.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
        }
    }

    private func field(_ label: String, text: Binding<String>) -> some View {
        HStack(alignment: .top, spacing: 10) {
            Text(label).font(LinettaTypography.bodySmall).foregroundStyle(LinettaTheme.textTertiary).frame(width: 92, alignment: .leading)
            TextField("", text: text, axis: .vertical).font(LinettaTypography.body).textFieldStyle(.plain)
        }
    }

    private func loadOnce() async {
        guard !loaded else { return }
        do {
            let bp = try await appState.client.getBlueprint(workID: work.id, episodeID: episodeID)
            episodeState.loadBlueprint(premise: bp.premise, theme: bp.theme, situation: bp.situation,
                                       mustInclude: bp.mustInclude, mustAvoid: bp.mustAvoid, structureNotes: bp.structureNotes)
            loaded = true
        } catch {
            episodeState.loadBlueprint(premise: "", theme: "", situation: "", mustInclude: "", mustAvoid: "", structureNotes: "")
            loaded = true
        }
    }

    private func suggest() async {
        suggesting = true
        defer { suggesting = false }
        let partial = BlueprintSuggestRequest(
            premise: episodeState.premise,
            theme: episodeState.theme,
            situation: episodeState.situation,
            mustInclude: episodeState.mustInclude,
            mustAvoid: episodeState.mustAvoid,
            structureNotes: episodeState.structureNotes
        )
        do {
            let s = try await appState.client.suggestBlueprint(workID: work.id, episodeID: episodeID, partial: partial)
            episodeState.loadBlueprint(
                premise: s.premise,
                theme: s.theme,
                situation: s.situation,
                mustInclude: s.mustInclude,
                mustAvoid: s.mustAvoid,
                structureNotes: s.structureNotes
            )
            // loadBlueprint resets isDirty — but the user almost certainly
            // wants to save the suggestion, so make it dirty by toggling
            // premise to itself. Cheap workaround.
            let p = episodeState.premise
            episodeState.premise = p + " "
            episodeState.premise = p
            let label = s.source == "llm" ? "Suggested via LLM" : "Suggested (no API key — fallback)"
            toast.enqueue(.init(title: label, kind: .success))
        } catch {
            toast.enqueue(.init(title: "Suggest failed: \(error.localizedDescription)", kind: .error))
        }
    }

    private func save() async {
        _ = try? await appState.client.saveBlueprint(
            workID: work.id,
            episodeID: episodeID,
            request: SaveBlueprintRequest(
                premise: episodeState.premise, theme: episodeState.theme, situation: episodeState.situation,
                mustInclude: episodeState.mustInclude, mustAvoid: episodeState.mustAvoid, structureNotes: episodeState.structureNotes
            )
        )
        episodeState.markSaved()
        await onSave()
    }
}
