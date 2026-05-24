import LinettaCore
import SwiftUI

struct ReviewRowView: View {
    enum Kind { case canon, continuity }
    let kind: Kind
    let title: String
    let source: String
    var onApprove: (() -> Void)? = nil
    var onReject: (() -> Void)? = nil
    var onDefer: (() -> Void)? = nil

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Text(kind == .canon ? "CANON" : "CONTINUITY")
                .linettaLabelStyle().frame(width: 88, alignment: .leading)
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(LinettaTypography.body).foregroundStyle(LinettaTheme.text)
                Text(source).font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary)
            }
            Spacer()
            HStack(spacing: 4) {
                if let onApprove {
                    Button("✓") { onApprove() }
                        .buttonStyle(.bordered).tint(LinettaTheme.success).controlSize(.mini)
                }
                if let onReject {
                    Button("✗") { onReject() }
                        .buttonStyle(.bordered).controlSize(.mini)
                }
                if let onDefer {
                    Button("⏸") { onDefer() }
                        .buttonStyle(.bordered).controlSize(.mini)
                }
            }
        }
        .padding(.vertical, 6)
        .overlay(alignment: .top) {
            Rectangle()
                .fill(LinettaTheme.borderSoft)
                .frame(height: 1)
                .opacity(0.6)
        }
    }
}
