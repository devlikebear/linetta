import AppKit
import LinettaCore
import SwiftUI
import UniformTypeIdentifiers

struct SettingsView: View {
    @EnvironmentObject private var engine: EngineController

    var body: some View {
        Form {
            SettingsEngineSection()
            SettingsStorageSection()
            SettingsTesseraSection()
            SettingsAboutSection()
        }
        .formStyle(.grouped)
        .scrollContentBackground(.hidden)
        .background(LinettaTheme.background)
        .frame(minWidth: 620, idealWidth: 680, minHeight: 560)
        .preferredColorScheme(.dark)
    }
}

// MARK: - Engine

struct SettingsEngineSection: View {
    @EnvironmentObject private var engine: EngineController

    @AppStorage(APIClient.engineAddressDefaultsKey) private var engineAddress = "http://127.0.0.1:43190"
    @AppStorage("linetta.useExternalEngine") private var useExternalEngine = false

    @State private var message: String?
    @State private var showLogPopover = false

    var body: some View {
        Section("Engine") {
            Toggle("Use external engine", isOn: $useExternalEngine)
            Text("When enabled, Linetta will not spawn its own engine — run `make serve` separately. Takes effect on next app launch.")
                .font(.caption)
                .foregroundStyle(LinettaTheme.textSecondary)

            LabeledContent("Status") {
                HStack(spacing: 6) {
                    Circle().fill(statusColor).frame(width: 8, height: 8)
                    Text(statusText).foregroundStyle(LinettaTheme.textSecondary)
                }
            }
            LabeledContent("Current address") {
                if let addr = engine.address?.absoluteString {
                    Text(addr).textSelection(.enabled).foregroundStyle(LinettaTheme.textSecondary)
                } else {
                    Text("—").foregroundStyle(LinettaTheme.textTertiary)
                }
            }
            LabeledContent("PID") {
                if let pid = engine.pid {
                    Text("\(pid)").foregroundStyle(LinettaTheme.textSecondary)
                } else {
                    Text("—").foregroundStyle(LinettaTheme.textTertiary)
                }
            }

            HStack {
                Button {
                    Task { await restart() }
                } label: {
                    Label("Restart Engine", systemImage: "arrow.clockwise")
                }
                .disabled(useExternalEngine || engine.status == .starting)

                Button {
                    Task { await stop() }
                } label: {
                    Label("Stop Engine", systemImage: "stop.fill")
                }
                .disabled(useExternalEngine || engine.status == .stopped)

                Button {
                    showLogPopover.toggle()
                } label: {
                    Label("Show Log", systemImage: "doc.text.below.ecg")
                }
                .popover(isPresented: $showLogPopover, arrowEdge: .trailing) {
                    EngineLogView().frame(width: 520, height: 320)
                }

                if engine.status == .starting {
                    ProgressView().controlSize(.small)
                }
                Spacer()
            }

            TextField("External address override", text: $engineAddress)
                .disabled(!useExternalEngine)
                .foregroundStyle(useExternalEngine ? .primary : LinettaTheme.textSecondary)

            if let message {
                Text(message)
                    .font(.caption)
                    .foregroundStyle(LinettaTheme.textSecondary)
            }
        }
    }

    private var statusColor: Color {
        switch engine.status {
        case .healthy: return LinettaTheme.success
        case .external: return Color(red: 0.43, green: 0.55, blue: 0.85)
        case .starting: return Color(red: 0.95, green: 0.78, blue: 0.34)
        case .failed: return LinettaTheme.danger
        case .stopped: return LinettaTheme.textTertiary
        }
    }

    private var statusText: String {
        switch engine.status {
        case .stopped: return "Stopped"
        case .starting: return "Starting…"
        case .healthy: return "Healthy (embedded)"
        case .external: return "Healthy (external)"
        case .failed(let reason): return "Failed — \(reason)"
        }
    }

    private func restart() async {
        message = "Restarting engine…"
        do {
            try await engine.restart()
            message = "Engine restarted on \(engine.address?.absoluteString ?? "—")"
        } catch {
            message = "Restart failed: \(error.localizedDescription)"
        }
    }

    private func stop() async {
        message = "Stopping engine…"
        await engine.stop()
        message = "Engine stopped."
    }
}

private struct EngineLogView: View {
    @EnvironmentObject private var engine: EngineController
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack {
                Text("Engine Log (last \(engine.recentLog.count))").font(LinettaTypography.caption)
                Spacer()
            }
            .padding(8)
            .background(LinettaTheme.surface)
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 2) {
                    ForEach(Array(engine.recentLog.enumerated()), id: \.offset) { _, line in
                        Text(line)
                            .font(.system(.caption2, design: .monospaced))
                            .textSelection(.enabled)
                            .foregroundStyle(LinettaTheme.text)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    if engine.recentLog.isEmpty {
                        Text("(no output yet)")
                            .font(LinettaTypography.caption)
                            .foregroundStyle(LinettaTheme.textTertiary)
                    }
                }
                .padding(10)
            }
            .background(LinettaTheme.background)
        }
    }
}

// MARK: - Storage

struct SettingsStorageSection: View {
    @Environment(AppState.self) private var appState

    @AppStorage("linetta.defaultDBPath") private var defaultDBPath = StoragePaths.defaultDB.path

    @State private var liveDBPath: String = ""
    @State private var message: String?
    @State private var inFlight = false

    var body: some View {
        Section("Storage") {
            LabeledContent("Live database") {
                Text(liveDBPath.isEmpty ? "—" : liveDBPath)
                    .textSelection(.enabled)
                    .foregroundStyle(LinettaTheme.textSecondary)
                    .frame(maxWidth: .infinity, alignment: .trailing)
                    .lineLimit(1).truncationMode(.middle)
            }

            HStack {
                TextField("Default DB path", text: $defaultDBPath)
                Button("Choose…") { pickFile(into: $defaultDBPath, kinds: [.database]) }
                Button("Reveal") { revealInFinder(path: defaultDBPath) }
                    .disabled(!FileManager.default.fileExists(atPath: defaultDBPath))
            }

            HStack(spacing: 10) {
                Button {
                    Task { await runBackup() }
                } label: {
                    Label("Backup Library…", systemImage: "tray.and.arrow.up")
                }
                .disabled(inFlight)

                Button {
                    Task { await runRestore() }
                } label: {
                    Label("Restore Library…", systemImage: "tray.and.arrow.down")
                }
                .disabled(inFlight)

                if inFlight {
                    ProgressView().controlSize(.small)
                }
                Spacer()
            }

            if let message {
                Text(message)
                    .font(.caption)
                    .foregroundStyle(LinettaTheme.textSecondary)
            }
        }
        .task { await refreshLiveDB() }
    }

    private func refreshLiveDB() async {
        if let info = try? await appState.client.libraryInfo() {
            liveDBPath = info.dbPath
        }
    }

    private func runBackup() async {
        inFlight = true; defer { inFlight = false }
        let panel = NSSavePanel()
        panel.allowedContentTypes = [UTType.zip]
        panel.nameFieldStringValue = "linetta-backup-\(timestamp()).zip"
        panel.canCreateDirectories = true
        guard panel.runModal() == .OK, let url = panel.url else { return }
        do {
            let result = try await appState.client.libraryBackup(outPath: url.path)
            message = "Backed up \(byteString(result.sizeBytes)) → \(result.outPath)"
        } catch {
            message = "Backup failed: \(error.localizedDescription)"
        }
    }

    private func runRestore() async {
        inFlight = true; defer { inFlight = false }
        let openPanel = NSOpenPanel()
        openPanel.allowedContentTypes = [UTType.zip]
        openPanel.allowsMultipleSelection = false
        guard openPanel.runModal() == .OK, let inURL = openPanel.url else { return }

        let outPanel = NSSavePanel()
        outPanel.message = "Restore the library to a NEW database file (not the live one). You will switch to it via Default DB path + Restart Engine."
        outPanel.nameFieldStringValue = "linetta-restored-\(timestamp()).db"
        outPanel.canCreateDirectories = true
        guard outPanel.runModal() == .OK, let outURL = outPanel.url else { return }

        do {
            let result = try await appState.client.libraryRestore(
                inPath: inURL.path,
                dbOut: outURL.path,
                configOut: nil,
                force: true
            )
            defaultDBPath = result.dbPath
            message = "Restored to \(result.dbPath). Default DB path updated — click Restart Engine to switch."
        } catch {
            message = "Restore failed: \(error.localizedDescription)"
        }
    }

    private func timestamp() -> String {
        let f = DateFormatter()
        f.dateFormat = "yyyyMMdd-HHmmss"
        return f.string(from: Date())
    }

    private func byteString(_ n: Int64) -> String {
        let kb = Double(n) / 1024
        if kb < 1024 { return String(format: "%.1f KB", kb) }
        return String(format: "%.2f MB", kb / 1024)
    }
}

// MARK: - Tessera

struct SettingsTesseraSection: View {
    @Environment(AppState.self) private var appState

    @AppStorage("linetta.tesseraConfigPath") private var tesseraConfigPath = StoragePaths.dataDir.appendingPathComponent("tessera.yaml").path

    @State private var configBuffer = ""
    @State private var loaded = false
    @State private var dirty = false
    @State private var message: String?

    var body: some View {
        Section("Tessera") {
            HStack {
                TextField("Config path", text: $tesseraConfigPath)
                    .onChange(of: tesseraConfigPath) { _, _ in
                        loaded = false
                        Task { await loadConfig() }
                    }
                Button("Choose…") { pickFile(into: $tesseraConfigPath, kinds: [.yaml]) }
                Button("Reveal") { revealInFinder(path: tesseraConfigPath) }
                    .disabled(!FileManager.default.fileExists(atPath: tesseraConfigPath))
            }

            Text("Inline YAML editor — edits autosave on demand. Provider secrets stay in Environment / Keychain (this file does not store them).")
                .font(.caption)
                .foregroundStyle(LinettaTheme.textSecondary)

            HStack {
                Button { Task { await loadConfig() } } label: { Label("Reload", systemImage: "arrow.clockwise") }
                    .disabled(!FileManager.default.fileExists(atPath: tesseraConfigPath))
                Button { Task { await saveConfig() } } label: { Label("Save", systemImage: "tray.and.arrow.down") }
                    .disabled(!dirty)
                Button { createDefault() } label: { Label("Create Default", systemImage: "doc.badge.plus") }
                Spacer()
                if dirty {
                    Text("Unsaved").font(.caption).foregroundStyle(LinettaTheme.accent)
                }
            }

            TextEditor(text: $configBuffer)
                .font(.system(.body, design: .monospaced))
                .scrollContentBackground(.hidden)
                .background(LinettaTheme.surfaceElevated)
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(LinettaTheme.borderSoft))
                .frame(minHeight: 220)
                .onChange(of: configBuffer) { _, _ in
                    if loaded { dirty = true }
                }

            LabeledContent("Provider secrets", value: "Environment or Keychain")

            if let message {
                Text(message)
                    .font(.caption)
                    .foregroundStyle(LinettaTheme.textSecondary)
            }
        }
        .task { await loadConfig() }
    }

    private func loadConfig() async {
        let url = configURL()
        if let data = try? Data(contentsOf: url), let text = String(data: data, encoding: .utf8) {
            configBuffer = text
            message = nil
        } else {
            configBuffer = ""
            message = "No config at \(url.path) yet."
        }
        dirty = false
        loaded = true
    }

    private func saveConfig() async {
        let url = configURL()
        do {
            try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
            try configBuffer.write(to: url, atomically: true, encoding: .utf8)
            dirty = false
            message = "Saved \(url.path) at \(timeOnly()). Engine picks up the new config on next run."
        } catch {
            message = "Save failed: \(error.localizedDescription)"
        }
    }

    private func createDefault() {
        do {
            let url = configURL()
            try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
            try defaultTesseraConfig.write(to: url, atomically: true, encoding: .utf8)
            configBuffer = defaultTesseraConfig
            dirty = false
            loaded = true
            message = "Default config written to \(url.path)."
        } catch {
            message = error.localizedDescription
        }
    }

    private func configURL() -> URL {
        let expanded = (tesseraConfigPath as NSString).expandingTildeInPath
        if expanded.hasPrefix("/") { return URL(fileURLWithPath: expanded) }
        return FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(expanded)
    }

    private func timeOnly() -> String {
        let f = DateFormatter()
        f.dateFormat = "HH:mm:ss"
        return f.string(from: Date())
    }
}

// MARK: - About

struct SettingsAboutSection: View {
    @Environment(AppState.self) private var appState
    @State private var versionInfo: VersionInfo?

    var body: some View {
        Section("About") {
            LabeledContent("Linetta") {
                Text(appVersion).foregroundStyle(LinettaTheme.textSecondary)
            }
            LabeledContent("Build") {
                Text(buildNumber).foregroundStyle(LinettaTheme.textSecondary)
            }
            LabeledContent("Engine (Go)") {
                Text(versionInfo?.linetta ?? "—").foregroundStyle(LinettaTheme.textSecondary)
            }
            LabeledContent("Tessera") {
                Text(versionInfo?.tessera.isEmpty == false ? versionInfo!.tessera : "—")
                    .foregroundStyle(LinettaTheme.textSecondary)
            }
            LabeledContent("Go runtime") {
                Text(versionInfo?.go ?? "—").foregroundStyle(LinettaTheme.textSecondary)
            }
            LabeledContent("LLM agents") {
                if let v = versionInfo {
                    HStack(spacing: 6) {
                        Circle()
                            .fill(v.llmEnabled ? LinettaTheme.success : LinettaTheme.textTertiary)
                            .frame(width: 7, height: 7)
                        VStack(alignment: .trailing, spacing: 2) {
                            if v.llmEnabled {
                                Text("\(v.llmProvider) · \(v.llmModel.isEmpty ? "default" : v.llmModel)")
                                    .foregroundStyle(LinettaTheme.textSecondary)
                                    .textSelection(.enabled)
                            } else {
                                Text("fallback")
                                    .foregroundStyle(LinettaTheme.textSecondary)
                            }
                            if !v.llmReason.isEmpty {
                                Text(v.llmReason)
                                    .font(LinettaTypography.caption)
                                    .foregroundStyle(LinettaTheme.textTertiary)
                            }
                        }
                    }
                } else {
                    Text("—").foregroundStyle(LinettaTheme.textTertiary)
                }
            }
            LabeledContent("Data directory") {
                HStack {
                    Text(StoragePaths.dataDir.path)
                        .textSelection(.enabled)
                        .foregroundStyle(LinettaTheme.textSecondary)
                        .lineLimit(1).truncationMode(.middle)
                    Button("Open") {
                        NSWorkspace.shared.activateFileViewerSelecting([StoragePaths.dataDir])
                    }
                }
            }
            LabeledContent("Provider secrets") {
                HStack(spacing: 8) {
                    Text("Environment or Keychain")
                        .foregroundStyle(LinettaTheme.textSecondary)
                    Link("(guide)", destination: URL(string: "https://github.com/devlikebear/linetta#provider-secrets")!)
                        .foregroundStyle(LinettaTheme.accent)
                        .font(LinettaTypography.caption)
                }
            }
            LabeledContent("GitHub") {
                Link("devlikebear/linetta", destination: URL(string: "https://github.com/devlikebear/linetta")!)
                    .foregroundStyle(LinettaTheme.accent)
            }
        }
        .task { await loadVersion() }
    }

    private var appVersion: String {
        Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev"
    }

    private var buildNumber: String {
        Bundle.main.infoDictionary?["CFBundleVersion"] as? String ?? "—"
    }

    private func loadVersion() async {
        versionInfo = try? await appState.client.version()
    }
}

// MARK: - Shared helpers

enum PickerFileKind {
    case database
    case yaml

    var contentTypes: [UTType] {
        switch self {
        case .database: return [.data]
        case .yaml: return [UTType(filenameExtension: "yaml") ?? .text,
                            UTType(filenameExtension: "yml") ?? .text]
        }
    }
}

func pickFile(into binding: Binding<String>, kinds: [PickerFileKind]) {
    let panel = NSOpenPanel()
    panel.canChooseFiles = true
    panel.canChooseDirectories = false
    panel.allowsMultipleSelection = false
    panel.allowedContentTypes = kinds.flatMap(\.contentTypes)
    if panel.runModal() == .OK, let url = panel.url {
        binding.wrappedValue = url.path
    }
}

func revealInFinder(path: String) {
    let url = URL(fileURLWithPath: (path as NSString).expandingTildeInPath)
    NSWorkspace.shared.activateFileViewerSelecting([url])
}

let defaultTesseraConfig = """
run:
  id: linetta-run
  workers: 4
  max_attempts: 2
  role_limits:
    researcher: 1
    leader: 1
    writer: 1
    editor: 1

queue:
  type: inmemory
  lease_timeout: 30s

llm:
  default_provider: codex
  providers:
    codex:
      provider: openai-codex
      model: gpt-5-codex
      auth_mode: oauth
      work_dir: .

roles:
  researcher:
    llm_provider: codex
    max_iterations: 2
  leader:
    llm_provider: codex
    max_iterations: 2
  writer:
    llm_provider: codex
    max_iterations: 4
  editor:
    llm_provider: codex
    max_iterations: 2

observe:
  events_jsonl: .tessera/runs/linetta/events.jsonl
  report_json: .tessera/runs/linetta/report.json
  html_report: .tessera/runs/linetta/report.html
"""
