import SwiftUI

public enum LinettaTheme {
    public static let background = Color(red: 0.106, green: 0.102, blue: 0.090)
    public static let surface = Color(red: 0.129, green: 0.118, blue: 0.094)
    public static let surfaceElevated = Color(red: 0.086, green: 0.078, blue: 0.059)
    public static let border = Color(red: 0.165, green: 0.153, blue: 0.133)
    public static let borderSoft = Color(red: 0.145, green: 0.133, blue: 0.110)

    public static let text = Color(red: 0.839, green: 0.827, blue: 0.796)
    public static let textSecondary = Color(red: 0.612, green: 0.584, blue: 0.541)
    public static let textTertiary = Color(red: 0.431, green: 0.416, blue: 0.376)

    public static let accent = Color(red: 0.851, green: 0.467, blue: 0.341)
    public static let accentSoft = Color(red: 0.851, green: 0.467, blue: 0.341).opacity(0.16)

    public static let success = Color(red: 0.435, green: 0.631, blue: 0.463)
    public static let warn = Color(red: 0.851, green: 0.667, blue: 0.341).opacity(0.25)
    public static let danger = Color.red.opacity(0.85)
}
