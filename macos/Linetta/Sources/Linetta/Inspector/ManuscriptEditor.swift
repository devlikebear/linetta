import LinettaCore
import SwiftUI

struct ManuscriptEditor: View {
    @Environment(ManuscriptState.self) private var manuscript
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    @State private var saveTask: Task<Void, Never>?

    var body: some View {
        @Bindable var manuscript = manuscript
        VStack(alignment: .leading, spacing: 0) {
            metaRow.padding(.horizontal, 14).padding(.top, 10)
            TextEditor(text: $manuscript.draft)
                .font(LinettaTypography.bodySerif)
                .padding(14)
                .onChange(of: manuscript.draft) { _, _ in scheduleSave() }
        }
    }

    private var metaRow: some View {
        HStack {
            Text("\(wordCount) words · \(manuscript.isDirty ? "unsaved" : "saved")")
                .font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary)
            Spacer()
        }
    }

    private var wordCount: Int {
        manuscript.draft.split(whereSeparator: { $0.isWhitespace }).count
    }

    private func scheduleSave() {
        saveTask?.cancel()
        let currentDraft = manuscript.draft
        let currentSelection = sidebar.selection
        let client = appState.client
        saveTask = Task { @MainActor [weak manuscript] in
            try? await Task.sleep(nanoseconds: 1_500_000_000)
            guard !Task.isCancelled, let manuscript else { return }
            guard case .episode(let wid, let eid) = currentSelection else { return }
            _ = try? await client.createEpisodeVersion(
                workID: wid, episodeID: eid,
                request: CreateEpisodeVersionRequest(body: currentDraft, note: "auto-save")
            )
            manuscript.markSaved()
        }
    }
}
