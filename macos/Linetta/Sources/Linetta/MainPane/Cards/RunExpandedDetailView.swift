import LinettaCore
import SwiftUI

struct RunExpandedDetailView: View {
    let run: EpisodeRunResult
    var onPreviewArtifact: (Artifact) -> Void = { _ in }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Artifacts").linettaLabelStyle()
            FlowLayout(spacing: 6) {
                ForEach(run.artifacts) { artifact in
                    Button { onPreviewArtifact(artifact) } label: {
                        Text(artifact.title)
                            .font(LinettaTypography.caption)
                            .padding(.horizontal, 8).padding(.vertical, 3)
                            .background(LinettaTheme.surface).clipShape(Capsule())
                            .foregroundStyle(LinettaTheme.text)
                    }.buttonStyle(.plain)
                }
            }
            Text("Decisions").linettaLabelStyle().padding(.top, 4)
            Text("See Review Queue below").font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textSecondary)
        }
        .padding(12)
        .background(LinettaTheme.surfaceElevated)
        .clipShape(RoundedRectangle(cornerRadius: 7))
    }
}

private struct FlowLayout: Layout {
    let spacing: CGFloat
    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let maxWidth = proposal.width ?? .infinity
        var x: CGFloat = 0, y: CGFloat = 0, rowH: CGFloat = 0
        for sub in subviews {
            let s = sub.sizeThatFits(.unspecified)
            if x + s.width > maxWidth { x = 0; y += rowH + spacing; rowH = 0 }
            x += s.width + spacing
            rowH = max(rowH, s.height)
        }
        return CGSize(width: maxWidth, height: y + rowH)
    }
    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var x = bounds.minX, y = bounds.minY, rowH: CGFloat = 0
        for sub in subviews {
            let s = sub.sizeThatFits(.unspecified)
            if x + s.width > bounds.maxX { x = bounds.minX; y += rowH + spacing; rowH = 0 }
            sub.place(at: CGPoint(x: x, y: y), proposal: .unspecified)
            x += s.width + spacing
            rowH = max(rowH, s.height)
        }
    }
}
