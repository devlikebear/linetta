import LinettaCore
import SwiftUI

struct SidebarView: View {
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            SidebarHeader()
            SidebarSearchField()
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
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            SidebarFooterView()
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(LinettaTheme.surfaceElevated)
    }
}

private struct SidebarHeader: View {
    @State private var showingNewWork = false
    var body: some View {
        HStack {
            Text("Linetta").font(LinettaTypography.titleSmall).foregroundStyle(LinettaTheme.text)
            Spacer()
            Button { showingNewWork = true } label: { Image(systemName: "plus") }
                .buttonStyle(.plain)
                .keyboardShortcut("n", modifiers: [.command])
                .foregroundStyle(LinettaTheme.textSecondary)
        }
        .padding(.horizontal, 14).padding(.vertical, 12)
        .sheet(isPresented: $showingNewWork) { NewWorkSheet() }
    }
}

private struct SidebarOnboardingHint: View {
    @State private var showSheet = false
    var body: some View {
        VStack(alignment: .center, spacing: 10) {
            Image(systemName: "books.vertical").font(.system(size: 28)).foregroundStyle(LinettaTheme.textTertiary)
            Text("No works yet").font(LinettaTypography.body).foregroundStyle(LinettaTheme.text)
            Button("Create your first work") { showSheet = true }
                .buttonStyle(.borderedProminent)
                .tint(LinettaTheme.accent)
                .controlSize(.small)
        }
        .padding(.horizontal, 8).padding(.vertical, 30)
        .frame(maxWidth: .infinity)
        .sheet(isPresented: $showSheet) { NewWorkSheet() }
    }
}
