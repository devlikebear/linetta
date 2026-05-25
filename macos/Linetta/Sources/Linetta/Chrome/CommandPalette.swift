import LinettaCore
import SwiftUI

struct CommandPalette: View {
    @Environment(CommandPaletteState.self) private var state
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    @FocusState private var focused: Bool

    var body: some View {
        @Bindable var state = state
        if state.isOpen {
            ZStack(alignment: .top) {
                Color.black.opacity(0.4).onTapGesture { state.isOpen = false }
                VStack(spacing: 0) {
                    TextField("Jump to episode, work, or memory…", text: $state.query)
                        .textFieldStyle(.plain)
                        .font(LinettaTypography.body)
                        .padding(14)
                        .focused($focused)
                        .onAppear { focused = true }
                    Divider().background(LinettaTheme.borderSoft)
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: 0) {
                            ForEach(filtered) { result in
                                Button { jump(result) } label: {
                                    HStack {
                                        Image(systemName: result.icon)
                                        Text(result.title).foregroundStyle(LinettaTheme.text)
                                        Spacer()
                                        Text(result.subtitle).font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary)
                                    }.padding(.horizontal, 14).padding(.vertical, 8)
                                }.buttonStyle(.plain)
                            }
                        }
                    }.frame(maxHeight: 240)
                }
                .frame(width: 540).background(LinettaTheme.surface).clipShape(RoundedRectangle(cornerRadius: 10))
                .overlay(RoundedRectangle(cornerRadius: 10).stroke(LinettaTheme.border))
                .padding(.top, 80)
            }
        }
    }

    private var filtered: [Result] {
        let q = state.query.lowercased()
        var out: [Result] = []
        for work in appState.works {
            if q.isEmpty || work.title.lowercased().contains(q) {
                out.append(.init(id: work.id, icon: "books.vertical", title: work.title, subtitle: "work") {
                    sidebar.selection = .work(workID: work.id)
                })
            }
        }
        return out
    }

    private func jump(_ r: Result) {
        r.action(); state.isOpen = false
    }

    struct Result: Identifiable {
        let id: String
        let icon: String
        let title: String
        let subtitle: String
        let action: () -> Void
    }
}
