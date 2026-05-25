import LinettaCore
import SwiftUI

struct EngineStatusFooter: View {
    @EnvironmentObject private var engine: EngineController
    var body: some View {
        HStack(spacing: 10) {
            Circle().fill(dotColor).frame(width: 7, height: 7)
            Text("Engine").font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textSecondary)
            if let addr = engine.address?.absoluteString {
                Text("·").foregroundStyle(LinettaTheme.textTertiary)
                Text(addr).font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary).textSelection(.enabled)
            }
            Spacer()
        }
        .padding(.horizontal, 14).padding(.vertical, 7)
        .background(LinettaTheme.surface)
        .overlay(alignment: .top) { Rectangle().fill(LinettaTheme.border).frame(height: 1) }
    }

    private var dotColor: Color {
        switch engine.status {
        case .healthy: return LinettaTheme.success
        case .external: return Color(red: 0.43, green: 0.55, blue: 0.85)
        case .starting: return Color(red: 0.95, green: 0.78, blue: 0.34)
        case .failed: return LinettaTheme.danger
        case .stopped: return LinettaTheme.textTertiary
        }
    }
}
