import SwiftUI
import AppKit

@main
struct LinettaApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
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

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)
        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }
}
