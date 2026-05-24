import LinettaCore
import SwiftUI

struct SidebarView: View {
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            SidebarHeader()
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 2) {
                    Text("Works").linettaLabelStyle().padding(.horizontal, 8).padding(.top, 6)
                    ForEach(appState.works) { work in
                        SidebarWorkRow(work: work)
                    }
                    if appState.works.isEmpty {
                        SidebarOnboardingHint()
                    }
                }
                .padding(.horizontal, 8)
            }
            Spacer(minLength: 0)
            SidebarFooterView()
        }
        .background(LinettaTheme.surfaceElevated)
    }
}

private struct SidebarHeader: View {
    var body: some View {
        HStack {
            Text("Linetta")
                .font(LinettaTypography.titleSmall)
                .foregroundStyle(LinettaTheme.text)
            Spacer()
            Button { /* new work — wired in Task D4 */ } label: {
                Image(systemName: "plus")
            }
            .buttonStyle(.plain)
            .foregroundStyle(LinettaTheme.textSecondary)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
    }
}

private struct SidebarOnboardingHint: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("No works yet")
                .font(LinettaTypography.body)
                .foregroundStyle(LinettaTheme.text)
            Text("Create your first work to begin.")
                .font(LinettaTypography.bodySmall)
                .foregroundStyle(LinettaTheme.textTertiary)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 18)
    }
}
