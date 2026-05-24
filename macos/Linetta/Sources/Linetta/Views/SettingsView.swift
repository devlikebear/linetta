import AppKit
import LinettaCore
import SwiftUI
import UniformTypeIdentifiers

struct SettingsView: View {
    @EnvironmentObject private var engine: EngineController

    @AppStorage(APIClient.engineAddressDefaultsKey) private var engineAddress = "http://127.0.0.1:43190"
    @AppStorage("linetta.defaultDBPath") private var defaultDBPath = StoragePaths.defaultDB.path
    @AppStorage("linetta.tesseraConfigPath") private var tesseraConfigPath = StoragePaths.dataDir.appendingPathComponent("tessera.yaml").path
    @AppStorage("linetta.useExternalEngine") private var useExternalEngine = false
    @State private var message: String?

    var body: some View {
        Form {
            Section("Engine") {
                Toggle("Use external engine", isOn: $useExternalEngine)
                Text("When enabled, Linetta will not spawn its own engine. Run `make serve` separately. Toggle takes effect on next app launch.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                LabeledContent("Status") {
                    Text(currentStatusText)
                        .foregroundStyle(.secondary)
                }
                LabeledContent("Current address") {
                    if let addr = engine.address?.absoluteString {
                        Text(addr).textSelection(.enabled).foregroundStyle(.secondary)
                    } else {
                        Text("—").foregroundStyle(.tertiary)
                    }
                }
                HStack {
                    Button {
                        Task { await restartEngine() }
                    } label: {
                        Label("Restart Engine", systemImage: "arrow.clockwise")
                    }
                    .disabled(useExternalEngine || engine.status == .starting)
                    if engine.status == .starting {
                        ProgressView().controlSize(.small)
                    }
                    Spacer()
                }
                TextField("External address override", text: $engineAddress)
                    .disabled(!useExternalEngine)
                    .foregroundStyle(useExternalEngine ? .primary : .secondary)
                HStack {
                    TextField("Default DB path", text: $defaultDBPath)
                    Button("Choose…") { pickFile(into: $defaultDBPath, kinds: [.database]) }
                    Button("Reveal") { revealInFinder(path: defaultDBPath) }
                        .disabled(!FileManager.default.fileExists(atPath: defaultDBPath))
                }
            }
            Section("Tessera") {
                HStack {
                    TextField("Config path", text: $tesseraConfigPath)
                    Button("Choose…") { pickFile(into: $tesseraConfigPath, kinds: [.yaml]) }
                    Button("Reveal") { revealInFinder(path: tesseraConfigPath) }
                        .disabled(!FileManager.default.fileExists(atPath: tesseraConfigPath))
                }
                Button {
                    createDefaultConfig()
                } label: {
                    Label("Create Default Config", systemImage: "doc.badge.plus")
                }
                LabeledContent("Provider secrets", value: "Environment or Keychain")
            }
            if let message {
                Section {
                    Text(message)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .padding(20)
        .frame(width: 560)
    }

    private var currentStatusText: String {
        switch engine.status {
        case .stopped: return "Stopped"
        case .starting: return "Starting…"
        case .healthy: return "Healthy (embedded)"
        case .external: return "Healthy (external)"
        case .failed(let reason): return "Failed — \(reason)"
        }
    }

    private func restartEngine() async {
        message = "Restarting engine…"
        do {
            try await engine.restart()
            message = "Engine restarted on \(engine.address?.absoluteString ?? "—")"
        } catch {
            message = "Restart failed: \(error.localizedDescription)"
        }
    }

    private func createDefaultConfig() {
        do {
            let url = configURL()
            try FileManager.default.createDirectory(
                at: url.deletingLastPathComponent(),
                withIntermediateDirectories: true
            )
            try defaultTesseraConfig.write(to: url, atomically: true, encoding: .utf8)
            message = "Created \(url.path)"
        } catch {
            message = error.localizedDescription
        }
    }

    private func configURL() -> URL {
        let expanded = (tesseraConfigPath as NSString).expandingTildeInPath
        if expanded.hasPrefix("/") {
            return URL(fileURLWithPath: expanded)
        }
        // Relative paths anchor under the user's home directory so legacy
        // values like ".linetta/tessera.yaml" resolve to ~/.linetta/tessera.yaml
        // regardless of the app's current working directory.
        return FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(expanded)
    }

    private func pickFile(into binding: Binding<String>, kinds: [PickerFileKind]) {
        let panel = NSOpenPanel()
        panel.canChooseFiles = true
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = false
        panel.allowedContentTypes = kinds.flatMap(\.contentTypes)
        if panel.runModal() == .OK, let url = panel.url {
            binding.wrappedValue = url.path
        }
    }

    private func revealInFinder(path: String) {
        let url = URL(fileURLWithPath: (path as NSString).expandingTildeInPath)
        NSWorkspace.shared.activateFileViewerSelecting([url])
    }
}

private enum PickerFileKind {
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

private let defaultTesseraConfig = """
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
