import LinettaCore
import SwiftUI

struct AppCommands: Commands {
    @FocusedValue(\.runAgents) var runAgentsAction
    @FocusedValue(\.saveBlueprint) var saveAction

    var body: some Commands {
        CommandGroup(replacing: .newItem) {
            Button("New Work") {
                NotificationCenter.default.post(name: .linettaNewWork, object: nil)
            }
            .keyboardShortcut("n", modifiers: [.command])

            Button("New Episode") {
                NotificationCenter.default.post(name: .linettaNewEpisode, object: nil)
            }
            .keyboardShortcut("n", modifiers: [.command, .shift])

            Divider()

            Button("Export Episode (TXT)…") {
                NotificationCenter.default.post(name: .linettaExportEpisode, object: nil)
            }
            .keyboardShortcut("e", modifiers: [.command])

            Button("Import Backup…") {
                NotificationCenter.default.post(name: .linettaImportBackup, object: nil)
            }
            .keyboardShortcut("i", modifiers: [.command, .shift])
        }

        CommandMenu("Run") {
            Button("Run Agents") { runAgentsAction?() }
                .keyboardShortcut("r", modifiers: [.command, .shift])
            Button("Save Blueprint") { saveAction?() }
                .keyboardShortcut("s", modifiers: [.command])
        }

        CommandGroup(after: .toolbar) {
            Button("Toggle Manuscript Inspector") {
                NotificationCenter.default.post(name: .linettaToggleInspector, object: nil)
            }
            .keyboardShortcut("m", modifiers: [.command, .shift])

            Button("Toggle Command Palette") {
                NotificationCenter.default.post(name: .linettaToggleCommandPalette, object: nil)
            }
            .keyboardShortcut("k", modifiers: [.command])
        }

        CommandGroup(replacing: .help) {
            Link("Linetta on GitHub", destination: URL(string: "https://github.com/devlikebear/linetta")!)
            Link("Provider Setup Guide", destination: URL(string: "https://github.com/devlikebear/linetta#provider-secrets")!)
        }
    }
}

extension Notification.Name {
    static let linettaNewWork = Notification.Name("linetta.newWork")
    static let linettaNewEpisode = Notification.Name("linetta.newEpisode")
    static let linettaImportBackup = Notification.Name("linetta.importBackup")
    static let linettaExportEpisode = Notification.Name("linetta.exportEpisode")
    static let linettaToggleInspector = Notification.Name("linetta.toggleInspector")
    static let linettaToggleCommandPalette = Notification.Name("linetta.toggleCommandPalette")
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
