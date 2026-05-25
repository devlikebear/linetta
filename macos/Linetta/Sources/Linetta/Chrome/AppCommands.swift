import LinettaCore
import SwiftUI

struct AppCommands: Commands {
    @FocusedValue(\.runAgents) var runAgentsAction
    @FocusedValue(\.saveBlueprint) var saveAction

    var body: some Commands {
        CommandGroup(replacing: .newItem) {
            Button("New Work") { NotificationCenter.default.post(name: .linettaNewWork, object: nil) }
                .keyboardShortcut("n", modifiers: [.command])
            Button("New Episode") { NotificationCenter.default.post(name: .linettaNewEpisode, object: nil) }
                .keyboardShortcut("n", modifiers: [.command, .shift])
        }
        CommandMenu("Run") {
            Button("Run Agents") { runAgentsAction?() }
                .keyboardShortcut("r", modifiers: [.command, .shift])
            Button("Save Blueprint") { saveAction?() }
                .keyboardShortcut("s", modifiers: [.command])
        }
    }
}

extension Notification.Name {
    static let linettaNewWork = Notification.Name("linetta.newWork")
    static let linettaNewEpisode = Notification.Name("linetta.newEpisode")
}

private struct RunAgentsActionKey: FocusedValueKey { typealias Value = () -> Void }
private struct SaveBlueprintActionKey: FocusedValueKey { typealias Value = () -> Void }

extension FocusedValues {
    var runAgents: (() -> Void)? {
        get { self[RunAgentsActionKey.self] }
        set { self[RunAgentsActionKey.self] = newValue }
    }
    var saveBlueprint: (() -> Void)? {
        get { self[SaveBlueprintActionKey.self] }
        set { self[SaveBlueprintActionKey.self] = newValue }
    }
}
