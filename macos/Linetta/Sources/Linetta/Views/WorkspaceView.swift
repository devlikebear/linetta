import LinettaCore
import SwiftUI
import UniformTypeIdentifiers

struct WorkspaceView: View {
    var work: Work

    var body: some View {
        TabView {
            WorkOverview(work: work)
                .tabItem {
                    Label("Overview", systemImage: "text.book.closed")
                }
            CanonMemoryView(work: work)
                .tabItem {
                    Label("Memory", systemImage: "brain")
                }
            EpisodeWorkbenchView(work: work)
                .tabItem {
                    Label("Workbench", systemImage: "wand.and.stars")
                }
        }
        .navigationTitle(work.title)
    }
}

private struct WorkOverview: View {
    var work: Work

    @State private var exportDocument = TextExportDocument()
    @State private var isExporting = false
    @State private var errorMessage: String?

    private let client = APIClient()

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text(work.title)
                .font(.largeTitle)
                .fontWeight(.semibold)
            if !work.genre.isEmpty {
                LabeledContent("Genre", value: work.genre)
            }
            if !work.premise.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Premise")
                        .font(.headline)
                    Text(work.premise)
                        .foregroundStyle(.secondary)
                }
            }
            Button {
                Task { await exportMarkdown() }
            } label: {
                Label("Export Markdown", systemImage: "square.and.arrow.up")
            }
            .buttonStyle(.borderedProminent)
            if let errorMessage {
                Text(errorMessage)
                    .foregroundStyle(.red)
            }
            Spacer()
        }
        .padding(28)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .fileExporter(
            isPresented: $isExporting,
            document: exportDocument,
            contentType: .linettaMarkdown,
            defaultFilename: safeExportFilename(work.title, fallback: "linetta-work") + ".md"
        ) { result in
            if case .failure(let error) = result {
                errorMessage = error.localizedDescription
            }
        }
    }

    private func exportMarkdown() async {
        do {
            let markdown = try await client.exportWorkMarkdown(workID: work.id)
            exportDocument = TextExportDocument(text: markdown)
            isExporting = true
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
