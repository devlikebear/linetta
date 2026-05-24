import LinettaCore
import SwiftUI

struct RunRowView: View {
    let run: EpisodeRunResult
    let runIdentifier: String
    let isExpanded: Bool
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 10) {
                Circle().fill(LinettaTheme.success).frame(width: 7, height: 7)
                Text("—").font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary).frame(width: 84, alignment: .leading)
                Text("Run · \(run.artifacts.count) artifacts").font(LinettaTypography.body).foregroundStyle(LinettaTheme.text)
                Spacer()
                Text("\(run.artifacts.count) artifacts").font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textSecondary)
                    .padding(.horizontal, 6).padding(.vertical, 1)
                    .background(LinettaTheme.surfaceElevated).clipShape(Capsule())
                Image(systemName: isExpanded ? "chevron.up" : "chevron.down")
                    .font(.system(size: 10)).foregroundStyle(LinettaTheme.textTertiary)
            }
            .padding(.horizontal, 10).padding(.vertical, 8)
            .background(isExpanded ? LinettaTheme.surfaceElevated : Color.clear)
            .clipShape(RoundedRectangle(cornerRadius: 7))
        }
        .buttonStyle(.plain)
    }
}
