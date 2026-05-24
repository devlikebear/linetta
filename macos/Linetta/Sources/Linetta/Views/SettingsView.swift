import LinettaCore
import SwiftUI

struct SettingsView: View {
    @AppStorage(APIClient.engineAddressDefaultsKey) private var engineAddress = "http://127.0.0.1:43190"
    @AppStorage("linetta.defaultDBPath") private var defaultDBPath = ".linetta/dev.db"
    @AppStorage("linetta.tesseraConfigPath") private var tesseraConfigPath = ".linetta/tessera.yaml"
    @State private var message: String?

    var body: some View {
        Form {
            Section("Engine") {
                TextField("Address", text: $engineAddress)
                TextField("Default DB path", text: $defaultDBPath)
            }
            Section("Tessera") {
                TextField("Config path", text: $tesseraConfigPath)
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
                        .foregroundStyle(.secondary)
                }
            }
        }
        .padding(20)
        .frame(width: 520)
    }

    private func createDefaultConfig() {
        do {
            let url = try configURL()
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

    private func configURL() throws -> URL {
        let expanded = (tesseraConfigPath as NSString).expandingTildeInPath
        if expanded.hasPrefix("/") {
            return URL(fileURLWithPath: expanded)
        }
        return URL(fileURLWithPath: FileManager.default.currentDirectoryPath).appending(path: expanded)
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
