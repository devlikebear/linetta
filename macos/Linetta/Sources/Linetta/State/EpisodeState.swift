import Foundation
import Observation

@MainActor
@Observable
final class EpisodeState {
    var premise = ""
    var theme = ""
    var situation = ""
    var mustInclude = ""
    var mustAvoid = ""
    var structureNotes = ""

    var expandedRunID: String?
    var isRunning = false

    @ObservationIgnored private var loadedSnapshot: String = ""

    var isDirty: Bool { currentSnapshot != loadedSnapshot }

    private var currentSnapshot: String {
        [premise, theme, situation, mustInclude, mustAvoid, structureNotes].joined(separator: "|")
    }

    /// Loads a blueprint that came from the server (or initial empty state).
    /// Resets the dirty flag because the fields now match what's persisted.
    func loadBlueprint(premise: String, theme: String, situation: String, mustInclude: String, mustAvoid: String, structureNotes: String) {
        self.premise = premise
        self.theme = theme
        self.situation = situation
        self.mustInclude = mustInclude
        self.mustAvoid = mustAvoid
        self.structureNotes = structureNotes
        self.loadedSnapshot = currentSnapshot
    }

    /// Applies a *suggested* blueprint without resetting the dirty flag — the
    /// suggestion is in memory only, so the user still needs to Save to push
    /// it to the server. Without this, loadBlueprint would mark the form
    /// clean immediately and the Save button would stay disabled.
    func applySuggestion(premise: String, theme: String, situation: String, mustInclude: String, mustAvoid: String, structureNotes: String) {
        self.premise = premise
        self.theme = theme
        self.situation = situation
        self.mustInclude = mustInclude
        self.mustAvoid = mustAvoid
        self.structureNotes = structureNotes
        // Deliberately do NOT touch loadedSnapshot so isDirty stays true
        // until the user explicitly saves.
    }

    func markSaved() {
        loadedSnapshot = currentSnapshot
    }

    func isBlueprintExpanded(episodeID: String) -> Bool {
        let key = "linetta.ui.blueprint.expanded.\(episodeID)"
        if UserDefaults.standard.object(forKey: key) == nil { return true }
        return UserDefaults.standard.bool(forKey: key)
    }

    func setBlueprintExpanded(episodeID: String, expanded: Bool) {
        UserDefaults.standard.set(expanded, forKey: "linetta.ui.blueprint.expanded.\(episodeID)")
    }
}
