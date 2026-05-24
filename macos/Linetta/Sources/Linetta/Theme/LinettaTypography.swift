import SwiftUI

public enum LinettaTypography {
    public static let titleLarge = Font.system(size: 28, weight: .semibold, design: .default)
    public static let titleSmall = Font.system(size: 13, weight: .semibold, design: .default)
    public static let body = Font.system(size: 13, weight: .regular, design: .default)
    public static let bodySerif = Font.system(size: 14, weight: .regular, design: .serif)
    public static let bodySmall = Font.system(size: 12, weight: .regular, design: .default)
    public static let caption = Font.system(size: 11, weight: .regular, design: .default)
    public static let label = Font.system(size: 10, weight: .semibold, design: .default)

    /// Reusable view modifier for label-style uppercase text.
    public struct LabelStyle: ViewModifier {
        public func body(content: Content) -> some View {
            content
                .font(LinettaTypography.label)
                .textCase(.uppercase)
                .tracking(0.7)
                .foregroundStyle(LinettaTheme.textTertiary)
        }
    }
}

public extension View {
    func linettaLabelStyle() -> some View {
        modifier(LinettaTypography.LabelStyle())
    }
}
