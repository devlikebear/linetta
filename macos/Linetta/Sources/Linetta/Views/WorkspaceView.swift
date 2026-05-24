import LinettaCore
import SwiftUI

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
            PlaceholderPane(
                title: "Episode Workbench",
                systemImage: "wand.and.stars",
                detail: "Human blueprints and Tessera agent runs will be coordinated here."
            )
            .tabItem {
                Label("Workbench", systemImage: "wand.and.stars")
            }
        }
        .navigationTitle(work.title)
    }
}

private struct WorkOverview: View {
    var work: Work

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
            Spacer()
        }
        .padding(28)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }
}

private struct PlaceholderPane: View {
    var title: String
    var systemImage: String
    var detail: String

    var body: some View {
        VStack(spacing: 12) {
            Image(systemName: systemImage)
                .font(.system(size: 38))
                .foregroundStyle(.secondary)
            Text(title)
                .font(.title2)
                .fontWeight(.semibold)
            Text(detail)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 420)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding(28)
    }
}
