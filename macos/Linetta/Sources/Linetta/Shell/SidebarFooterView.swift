import SwiftUI

struct SidebarFooterView: View {
    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "gearshape")
            Spacer()
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .foregroundStyle(LinettaTheme.textSecondary)
        .overlay(alignment: .top) {
            Rectangle().fill(LinettaTheme.borderSoft).frame(height: 1)
        }
    }
}
