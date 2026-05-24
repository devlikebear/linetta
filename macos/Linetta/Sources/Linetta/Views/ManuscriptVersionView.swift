import LinettaCore
import SwiftUI

struct ManuscriptVersionView: View {
    @Binding var bodyText: String
    var versions: [EpisodeVersion]
    @Binding var selectedVersion: EpisodeVersion?
    var selectedArtifact: Artifact?
    var saveVersion: () -> Void
    var adoptArtifact: (Artifact) -> Void
    var restoreVersion: (EpisodeVersion) -> Void

    var body: some View {
        HSplitView {
            VStack(alignment: .leading, spacing: 10) {
                HStack {
                    Text("Manuscript")
                        .font(.headline)
                    Spacer()
                    Text("\(bodyText.count) chars")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                TextEditor(text: $bodyText)
                    .font(.body)
                    .frame(minWidth: 340, minHeight: 420)
                    .overlay {
                        RoundedRectangle(cornerRadius: 6)
                            .stroke(Color.secondary.opacity(0.25))
                    }
                HStack {
                    Button {
                        saveVersion()
                    } label: {
                        Label("Save Version", systemImage: "square.and.pencil")
                    }
                    .buttonStyle(.borderedProminent)
                    .keyboardShortcut("s", modifiers: [.command, .option])
                    .disabled(bodyText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)

                    Button {
                        if let selectedArtifact {
                            adoptArtifact(selectedArtifact)
                        }
                    } label: {
                        Label("Adopt Artifact", systemImage: "arrow.down.doc")
                    }
                    .disabled(selectedArtifact == nil)
                }
            }

            VStack(alignment: .leading, spacing: 10) {
                Text("Versions")
                    .font(.headline)
                if versions.isEmpty {
                    ContentUnavailableView("No Version", systemImage: "clock.arrow.circlepath")
                } else {
                    List(selection: $selectedVersion) {
                        ForEach(versions) { version in
                            VersionRow(version: version)
                                .tag(version)
                        }
                    }
                    .frame(minWidth: 220)

                    if let selectedVersion {
                        ScrollView {
                            Text(selectedVersion.body)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .textSelection(.enabled)
                        }
                        .frame(minHeight: 160)
                        Button {
                            restoreVersion(selectedVersion)
                        } label: {
                            Label("Restore", systemImage: "arrow.uturn.backward")
                        }
                    }
                }
            }
            .frame(minWidth: 240)
        }
        .padding(.top, 8)
        .onChange(of: selectedVersion) { _, version in
            if let version {
                bodyText = version.body
            }
        }
    }
}

private struct VersionRow: View {
    var version: EpisodeVersion

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Image(systemName: version.sourceArtifactID.isEmpty ? "square.and.pencil" : "sparkles")
                    .foregroundStyle(.secondary)
                Text(version.note.isEmpty ? "Saved manuscript" : version.note)
                    .font(.headline)
                    .lineLimit(1)
            }
            Text(version.createdAt)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
        }
        .padding(.vertical, 4)
    }
}
