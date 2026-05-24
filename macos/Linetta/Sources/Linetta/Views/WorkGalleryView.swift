import LinettaCore
import SwiftUI

struct WorkGalleryView: View {
    @EnvironmentObject private var appState: AppState
    @State private var showingNewWork = false

    var body: some View {
        NavigationSplitView {
            List(selection: $appState.selectedWork) {
                ForEach(appState.works) { work in
                    WorkRow(work: work)
                        .tag(work)
                }
            }
            .navigationTitle("Works")
            .toolbar {
                ToolbarItemGroup {
                    Button {
                        Task { await appState.refreshWorks() }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                    }
                    Button {
                        showingNewWork = true
                    } label: {
                        Image(systemName: "plus")
                    }
                }
            }
        } detail: {
            if let work = appState.selectedWork {
                WorkspaceView(work: work)
            } else {
                GalleryEmptyState(showingNewWork: $showingNewWork)
            }
        }
        .frame(minWidth: 920, minHeight: 620)
        .sheet(isPresented: $showingNewWork) {
            NewWorkSheet()
        }
        .overlay(alignment: .bottom) {
            if let errorMessage = appState.errorMessage {
                Text(errorMessage)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 8))
                    .padding()
            }
        }
    }
}

private struct WorkRow: View {
    var work: Work

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(work.title)
                .font(.headline)
                .lineLimit(1)
            if !work.genre.isEmpty {
                Text(work.genre)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            if !work.premise.isEmpty {
                Text(work.premise)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
        }
        .padding(.vertical, 6)
    }
}

private struct GalleryEmptyState: View {
    @Binding var showingNewWork: Bool

    var body: some View {
        VStack(spacing: 14) {
            Image(systemName: "books.vertical")
                .font(.system(size: 44))
                .foregroundStyle(.secondary)
            Text("Create or select a work")
                .font(.title2)
                .fontWeight(.semibold)
            Button {
                showingNewWork = true
            } label: {
                Label("New Work", systemImage: "plus")
            }
            .buttonStyle(.borderedProminent)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}
