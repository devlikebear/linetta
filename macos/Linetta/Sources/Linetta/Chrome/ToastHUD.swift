import SwiftUI

/// Floating bottom-right HUD overlay that renders queued ToastCenter messages.
/// Attaches via `.overlay { ToastHUD() }` on AppShell.
struct ToastHUD: View {
    @Environment(ToastCenter.self) private var center

    var body: some View {
        VStack {
            Spacer()
            HStack {
                Spacer()
                VStack(alignment: .trailing, spacing: 8) {
                    ForEach(center.toasts) { toast in
                        ToastBubble(toast: toast)
                            .transition(.move(edge: .trailing).combined(with: .opacity))
                    }
                }
                .padding(.trailing, 18)
                .padding(.bottom, 18)
            }
        }
        .allowsHitTesting(!center.toasts.isEmpty)
        .animation(.easeInOut(duration: 0.18), value: center.toasts.count)
    }
}

private struct ToastBubble: View {
    let toast: ToastMessage
    @Environment(ToastCenter.self) private var center

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: icon)
                .foregroundStyle(iconColor)
            Text(toast.title)
                .font(LinettaTypography.bodySmall)
                .foregroundStyle(LinettaTheme.text)
                .lineLimit(2)
            Button {
                center.dismiss(toast.id)
            } label: {
                Image(systemName: "xmark")
                    .font(.system(size: 10))
                    .foregroundStyle(LinettaTheme.textTertiary)
            }
            .buttonStyle(.plain)
        }
        .padding(.horizontal, 14).padding(.vertical, 10)
        .background(LinettaTheme.surface)
        .overlay(RoundedRectangle(cornerRadius: 10).stroke(LinettaTheme.border))
        .clipShape(RoundedRectangle(cornerRadius: 10))
        .shadow(color: .black.opacity(0.35), radius: 12, x: 0, y: 4)
        .frame(minWidth: 260, maxWidth: 360)
    }

    private var icon: String {
        switch toast.kind {
        case .info: return "info.circle"
        case .success: return "checkmark.circle.fill"
        case .warn: return "exclamationmark.triangle.fill"
        case .error: return "xmark.octagon.fill"
        }
    }

    private var iconColor: Color {
        switch toast.kind {
        case .info: return LinettaTheme.textSecondary
        case .success: return LinettaTheme.success
        case .warn: return Color(red: 0.95, green: 0.78, blue: 0.34)
        case .error: return LinettaTheme.danger
        }
    }
}
