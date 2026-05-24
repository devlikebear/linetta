import SwiftUI

struct SidebarFooterView: View {
    @State private var showSettings = false

    var body: some View {
        HStack(spacing: 8) {
            Button { showSettings.toggle() } label: { Image(systemName: "gearshape") }
                .buttonStyle(.plain)
            Spacer()
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .foregroundStyle(LinettaTheme.textSecondary)
        .overlay(alignment: .top) {
            Rectangle().fill(LinettaTheme.borderSoft).frame(height: 1)
        }
        .sheet(isPresented: $showSettings) {
            SettingsView()
                .frame(width: 560, height: 420)
        }
    }
}
