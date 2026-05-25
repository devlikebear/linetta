import LinettaCore
import SwiftUI

struct CommandPalette: View {
    @Environment(CommandPaletteState.self) private var state
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    @FocusState private var focused: Bool
    @State private var episodeCache: [String: [Episode]] = [:]

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
                                            .foregroundStyle(LinettaTheme.textSecondary)
                                            .frame(width: 16)
                                        Text(result.title).foregroundStyle(LinettaTheme.text)
                                        Spacer()
                                        Text(result.subtitle)
                                            .font(LinettaTypography.caption)
                                            .foregroundStyle(LinettaTheme.textTertiary)
                                    }
                                    .padding(.horizontal, 14)
                                    .padding(.vertical, 8)
                                    .contentShape(Rectangle())
                                }
                                .buttonStyle(.plain)
                            }
                            if filtered.isEmpty {
                                Text("No matches")
                                    .font(LinettaTypography.bodySmall)
                                    .foregroundStyle(LinettaTheme.textTertiary)
                                    .padding(14)
                            }
                        }
                    }
                    .frame(maxHeight: 320)
                }
                .frame(width: 540)
                .background(LinettaTheme.surface)
                .clipShape(RoundedRectangle(cornerRadius: 10))
                .overlay(RoundedRectangle(cornerRadius: 10).stroke(LinettaTheme.border))
                .padding(.top, 80)
            }
            .task(id: appState.works.map(\.id).joined()) { await preloadEpisodes() }
        }
    }

    private var filtered: [Result] {
        let q = state.query.lowercased()
        var out: [Result] = []

        // Works
        for work in appState.works {
            if q.isEmpty || work.title.lowercased().contains(q) {
                out.append(.init(
                    id: "work-\(work.id)",
                    icon: "books.vertical",
                    title: work.title,
                    subtitle: "work"
                ) {
                    sidebar.selection = .work(workID: work.id)
                    sidebar.setExpanded(work.id, expanded: true)
                })
            }
            // Memory (every work has one)
            if q.isEmpty || "memory".contains(q) || work.title.lowercased().contains(q) {
                out.append(.init(
                    id: "memory-\(work.id)",
                    icon: "books.vertical.fill",
                    title: "\(work.title) — Memory",
                    subtitle: "canon memory"
                ) {
                    sidebar.selection = .memory(workID: work.id)
                    sidebar.setExpanded(work.id, expanded: true)
                })
            }
            // Episodes
            for episode in episodeCache[work.id] ?? [] {
                if q.isEmpty || episode.title.lowercased().contains(q) || work.title.lowercased().contains(q) {
                    out.append(.init(
                        id: "episode-\(episode.id)",
                        icon: "doc.text",
                        title: episode.title,
                        subtitle: "\(work.title) · episode"
                    ) {
                        sidebar.selection = .episode(workID: work.id, episodeID: episode.id)
                        sidebar.setExpanded(work.id, expanded: true)
                    })
                }
            }
        }

        // Static commands
        let staticCommands: [Result] = [
            .init(id: "cmd-new-work", icon: "plus", title: "New Work", subtitle: "command · ⌘N") {
                NotificationCenter.default.post(name: .linettaNewWork, object: nil)
            },
        ]
        for cmd in staticCommands where q.isEmpty || cmd.title.lowercased().contains(q) {
            out.append(cmd)
        }

        return out
    }

    private func jump(_ r: Result) {
        r.action()
        state.isOpen = false
    }

    private func preloadEpisodes() async {
        // Fetch episodes for every work so palette can search them. Cached per work.
        for work in appState.works where episodeCache[work.id] == nil {
            episodeCache[work.id] = (try? await appState.client.listEpisodes(workID: work.id)) ?? []
        }
    }

    struct Result: Identifiable {
        let id: String
        let icon: String
        let title: String
        let subtitle: String
        let action: () -> Void
    }
}
