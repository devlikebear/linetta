import SwiftUI

struct SidebarSearchField: View {
    @Environment(SidebarState.self) private var sidebar
    @FocusState private var focused: Bool

    var body: some View {
        @Bindable var sidebar = sidebar
        if sidebar.searchOpen {
            HStack {
                Image(systemName: "magnifyingglass").foregroundStyle(LinettaTheme.textTertiary)
                TextField("Search works · episodes · memory", text: $sidebar.query)
                    .textFieldStyle(.plain)
                    .focused($focused)
                    .onAppear { focused = true }
                Button { sidebar.searchOpen = false } label: { Image(systemName: "xmark") }
                    .buttonStyle(.plain)
                    .foregroundStyle(LinettaTheme.textTertiary)
            }
            .padding(.horizontal, 10).padding(.vertical, 6)
            .background(LinettaTheme.surface)
            .clipShape(RoundedRectangle(cornerRadius: 6))
            .padding(.horizontal, 8)
        }
    }
}
