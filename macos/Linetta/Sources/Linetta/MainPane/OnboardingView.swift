import AppKit
import LinettaCore
import SwiftUI
import UniformTypeIdentifiers

struct OnboardingView: View {
    @Environment(AppState.self) private var appState
    @Environment(ToastCenter.self) private var toast
    @State private var showSheet = false
    @State private var importing = false

    var body: some View {
        VStack(spacing: 16) {
            Image(systemName: "books.vertical")
                .font(.system(size: 48))
                .foregroundStyle(LinettaTheme.textTertiary)
            Text("Linetta")
                .font(LinettaTypography.titleLarge)
                .foregroundStyle(LinettaTheme.text)
            Text("An AI workflow runner for serial fiction.")
                .font(LinettaTypography.body)
                .foregroundStyle(LinettaTheme.textSecondary)
            HStack(spacing: 10) {
                Button("Create your first work") { showSheet = true }
                    .buttonStyle(.borderedProminent)
                    .tint(LinettaTheme.accent)
                Button {
                    Task { await importBackup() }
                } label: {
                    Label(importing ? "Importing…" : "Import backup…", systemImage: "tray.and.arrow.down")
                }
                .buttonStyle(.bordered)
                .disabled(importing)
            }
            Text("Importing a backup writes a restored database to a new file. Open Settings → Storage → Default DB path to switch to it, then Restart Engine.")
                .font(LinettaTypography.caption)
                .foregroundStyle(LinettaTheme.textTertiary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 460)
                .padding(.top, 4)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(LinettaTheme.background)
        .sheet(isPresented: $showSheet) { NewWorkSheet() }
    }

    private func importBackup() async {
        importing = true; defer { importing = false }
        let openPanel = NSOpenPanel()
        openPanel.allowedContentTypes = [UTType.zip]
        openPanel.allowsMultipleSelection = false
        guard openPanel.runModal() == .OK, let inURL = openPanel.url else { return }

        let savePanel = NSSavePanel()
        savePanel.message = "Restore the library to a NEW database file."
        savePanel.allowedContentTypes = [UTType.data]
        savePanel.nameFieldStringValue = "linetta-imported.db"
        savePanel.canCreateDirectories = true
        guard savePanel.runModal() == .OK, let outURL = savePanel.url else { return }

        do {
            let result = try await appState.client.libraryRestore(
                inPath: inURL.path,
                dbOut: outURL.path,
                configOut: nil,
                force: true
            )
            UserDefaults.standard.set(result.dbPath, forKey: "linetta.defaultDBPath")
            toast.enqueue(.init(
                title: "Imported to \(result.dbPath). Restart engine from Settings to load it.",
                kind: .success
            ))
        } catch {
            toast.enqueue(.init(title: "Import failed: \(error.localizedDescription)", kind: .error))
        }
    }
}
