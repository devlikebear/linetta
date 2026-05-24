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

    func loadBlueprint(premise: String, theme: String, situation: String, mustInclude: String, mustAvoid: String, structureNotes: String) {
        self.premise = premise
        self.theme = theme
        self.situation = situation
        self.mustInclude = mustInclude
        self.mustAvoid = mustAvoid
        self.structureNotes = structureNotes
        self.loadedSnapshot = currentSnapshot
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
