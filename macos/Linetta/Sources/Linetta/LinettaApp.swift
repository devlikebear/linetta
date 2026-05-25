import AppKit
import LinettaCore
import SwiftUI

@MainActor
private let sharedEngine = EngineController()

@MainActor
private let sharedAppState = AppState(engine: sharedEngine)

@MainActor private let sharedSidebarState = SidebarState()
@MainActor private let sharedEpisodeState = EpisodeState()
@MainActor private let sharedManuscriptState = ManuscriptState()
@MainActor private let sharedCommandPalette = CommandPaletteState()
@MainActor private let sharedToastCenter = ToastCenter()

@main
struct LinettaApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var engine = sharedEngine
    @State private var appState = sharedAppState

    var body: some Scene {
        WindowGroup {
            AppShell()
                .environment(appState)
                .environmentObject(engine)
                .environment(sharedSidebarState)
                .environment(sharedEpisodeState)
                .environment(sharedManuscriptState)
                .environment(sharedCommandPalette)
                .environment(sharedToastCenter)
        }
        .commands { AppCommands() }
        Settings {
            SettingsView()
                .environment(appState)
                .environmentObject(engine)
                .environment(sharedSidebarState)
                .environment(sharedEpisodeState)
                .environment(sharedManuscriptState)
                .environment(sharedCommandPalette)
                .environment(sharedToastCenter)
        }
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)
        NSApp.applicationIconImage = Self.makeAppIcon()
        NSApp.activate(ignoringOtherApps: true)
        Task { @MainActor in
            await Self.bootEngine()
        }
    }

    private static func makeAppIcon() -> NSImage {
        let size = NSSize(width: 512, height: 512)
        let image = NSImage(size: size)
        image.lockFocus()
        defer { image.unlockFocus() }

        let rect = NSRect(origin: .zero, size: size).insetBy(dx: 36, dy: 36)
        let gradient = NSGradient(starting: NSColor(calibratedRed: 0.22, green: 0.28, blue: 0.50, alpha: 1),
                                  ending: NSColor(calibratedRed: 0.10, green: 0.13, blue: 0.28, alpha: 1))
        let bg = NSBezierPath(roundedRect: rect, xRadius: 96, yRadius: 96)
        bg.addClip()
        gradient?.draw(in: rect, angle: 270)

        // Letter "L" centered
        let glyph = "L"
        let attrs: [NSAttributedString.Key: Any] = [
            .font: NSFont.systemFont(ofSize: 300, weight: .heavy),
            .foregroundColor: NSColor.white,
        ]
        let glyphSize = (glyph as NSString).size(withAttributes: attrs)
        let origin = NSPoint(
            x: rect.midX - glyphSize.width / 2,
            y: rect.midY - glyphSize.height / 2 - 12
        )
        (glyph as NSString).draw(at: origin, withAttributes: attrs)

        // Underline tick to evoke a writer's mark
        let tick = NSBezierPath()
        let tickY = rect.midY - glyphSize.height / 2 - 24
        tick.move(to: NSPoint(x: rect.midX - 56, y: tickY))
        tick.line(to: NSPoint(x: rect.midX + 80, y: tickY))
        tick.lineWidth = 14
        NSColor(calibratedRed: 0.95, green: 0.80, blue: 0.55, alpha: 1).setStroke()
        tick.stroke()

        return image
    }

    func applicationWillTerminate(_ notification: Notification) {
        MainActor.assumeIsolated {
            sharedEngine.stopSync()
        }
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        let isDirty = MainActor.assumeIsolated { sharedManuscriptState.isDirty }
        guard isDirty else { return .terminateNow }

        let alert = NSAlert()
        alert.messageText = "Manuscript has unsaved changes"
        alert.informativeText = "Your manuscript draft has not been autosaved yet. Quit anyway, save first, or cancel?"
        alert.alertStyle = .warning
        alert.addButton(withTitle: "Save and Quit")
        alert.addButton(withTitle: "Discard and Quit")
        alert.addButton(withTitle: "Cancel")

        let response = alert.runModal()
        switch response {
        case .alertFirstButtonReturn:
            // Save and quit — fire the save synchronously by triggering a save
            // task and waiting briefly. We can't fully await on the main thread
            // without deadlocking, so we run a short blocking pump.
            Task { @MainActor in
                await saveBeforeQuit()
                sender.reply(toApplicationShouldTerminate: true)
            }
            return .terminateLater
        case .alertSecondButtonReturn:
            return .terminateNow
        default:
            return .terminateCancel
        }
    }

    @MainActor
    private func saveBeforeQuit() async {
        // Best-effort: try to push the latest manuscript draft to the server.
        guard case .episode(let wid, let eid) = sharedSidebarState.selection else { return }
        let body = sharedManuscriptState.draft
        _ = try? await sharedAppState.client.createEpisodeVersion(
            workID: wid, episodeID: eid,
            request: CreateEpisodeVersionRequest(body: body, note: "save-before-quit")
        )
        sharedManuscriptState.markSaved(as: body)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    @MainActor
    private static func bootEngine() async {
        let engine = sharedEngine
        let useExternal = UserDefaults.standard.bool(forKey: "linetta.useExternalEngine")
        if useExternal {
            engine.attachExternal(address: APIClient.defaultBaseURL)
            await sharedAppState.refreshWorks()
            return
        }
        guard let binary = EngineController.resolveBinaryPath(override: nil) else {
            engine.attachExternal(address: APIClient.defaultBaseURL)
            return
        }
        do {
            try StoragePaths.ensureDataDirectory()
            try await engine.startEmbedded(binaryPath: binary, dbPath: StoragePaths.defaultDB)
            await sharedAppState.refreshWorks()
        } catch {
            NSLog("Linetta engine failed to start: %@", error.localizedDescription)
        }
    }
}
