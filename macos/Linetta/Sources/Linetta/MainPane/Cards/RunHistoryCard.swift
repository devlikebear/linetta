import LinettaCore
import SwiftUI

struct RunHistoryCard: View {
    let runs: [EpisodeRunResult]

    @Environment(EpisodeState.self) private var episodeState
    @Environment(ManuscriptState.self) private var manuscript

    @State private var showAll = false

    private var visibleRuns: [EpisodeRunResult] {
        showAll ? runs : Array(runs.prefix(5))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Run History").linettaLabelStyle()
                Text("\(runs.count)").font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary)
                Spacer()
                if runs.count > 5 {
                    Button(showAll ? "Show less" : "Show all (\(runs.count))") { showAll.toggle() }
                        .font(LinettaTypography.caption)
                        .buttonStyle(.plain)
                        .foregroundStyle(LinettaTheme.accent)
                }
            }
            if runs.isEmpty {
                Text("No runs yet. Press ⇧⌘R to run agents.")
                    .font(LinettaTypography.bodySmall)
                    .foregroundStyle(LinettaTheme.textTertiary)
                    .padding(.vertical, 10)
            } else {
                ForEach(visibleRuns, id: \.runID) { run in
                    RunRowView(run: run, runIdentifier: run.runID, isExpanded: episodeState.expandedRunID == run.runID) {
                        episodeState.expandedRunID = episodeState.expandedRunID == run.runID ? nil : run.runID
                    }
                    if episodeState.expandedRunID == run.runID {
                        RunExpandedDetailView(run: run) { artifact in
                            manuscript.mode = .artifactPreview(runID: run.runID, artifactID: artifact.id, body: artifact.body)
                        }
                    }
                }
            }
        }
        .padding(.horizontal, LinettaShape.cardPaddingH).padding(.vertical, LinettaShape.cardPaddingV)
        .background(LinettaTheme.surface)
        .overlay(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius).stroke(LinettaTheme.border))
        .clipShape(RoundedRectangle(cornerRadius: LinettaShape.cardCornerRadius))
    }
}
