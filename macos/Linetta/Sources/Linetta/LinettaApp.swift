import SwiftUI

@main
struct LinettaApp: App {
    @StateObject private var appState = AppState()

    var body: some Scene {
        WindowGroup {
            WorkGalleryView()
                .environmentObject(appState)
                .task {
                    await appState.refreshWorks()
                }
        }
        .commands {
            CommandGroup(after: .newItem) {
                Button("Refresh Works") {
                    Task { await appState.refreshWorks() }
                }
                .keyboardShortcut("r", modifiers: [.command])
            }
        }
        Settings {
            SettingsView()
        }
    }
}
