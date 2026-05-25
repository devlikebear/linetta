import LinettaCore
import SwiftUI

struct ManuscriptEditor: View {
    @Environment(ManuscriptState.self) private var manuscript
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar
    @Environment(ToastCenter.self) private var toast

    @State private var saveTask: Task<Void, Never>?
    @State private var lastSaveStatus: SaveStatus = .clean

    private enum SaveStatus: Equatable {
        case clean
        case dirty
        case saving
        case saved(at: Date)
        case failed(String)
    }

    var body: some View {
        @Bindable var manuscript = manuscript
        VStack(alignment: .leading, spacing: 0) {
            metaRow.padding(.horizontal, 14).padding(.top, 10).padding(.bottom, 6)
            LongformEditor(text: $manuscript.draft, onTextChange: scheduleSave)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private var metaRow: some View {
        HStack(spacing: 8) {
            Text("\(wordCount) words")
                .font(LinettaTypography.caption)
                .foregroundStyle(LinettaTheme.textTertiary)
            Text("·").foregroundStyle(LinettaTheme.textTertiary)
            statusBadge
            Spacer()
            Text("⌘F to find")
                .font(LinettaTypography.caption)
                .foregroundStyle(LinettaTheme.textTertiary)
        }
    }

    @ViewBuilder
    private var statusBadge: some View {
        switch lastSaveStatus {
        case .clean:
            Text("up to date")
                .font(LinettaTypography.caption)
                .foregroundStyle(LinettaTheme.textTertiary)
        case .dirty:
            Text("unsaved")
                .font(LinettaTypography.caption)
                .foregroundStyle(LinettaTheme.accent)
        case .saving:
            HStack(spacing: 4) {
                ProgressView().controlSize(.mini)
                Text("saving…").font(LinettaTypography.caption).foregroundStyle(LinettaTheme.textTertiary)
            }
        case .saved(let when):
            Text("saved \(relativeTime(when))")
                .font(LinettaTypography.caption)
                .foregroundStyle(LinettaTheme.success)
        case .failed(let reason):
            Text("save failed — \(reason)")
                .font(LinettaTypography.caption)
                .foregroundStyle(LinettaTheme.danger)
                .lineLimit(1)
                .truncationMode(.tail)
        }
    }

    private var wordCount: Int {
        manuscript.draft.split(whereSeparator: { $0.isWhitespace }).count
    }

    private func relativeTime(_ date: Date) -> String {
        let seconds = Int(Date().timeIntervalSince(date))
        if seconds < 5 { return "just now" }
        if seconds < 60 { return "\(seconds)s ago" }
        return "\(seconds / 60)m ago"
    }

    private func scheduleSave() {
        if manuscript.isDirty { lastSaveStatus = .dirty }
        saveTask?.cancel()
        let currentSelection = sidebar.selection
        let client = appState.client
        saveTask = Task { @MainActor [weak manuscript] in
            try? await Task.sleep(nanoseconds: 1_500_000_000)
            guard !Task.isCancelled, let manuscript else { return }
            guard manuscript.isDirty else { return }
            guard case .episode(let wid, let eid) = currentSelection else { return }
            let bodyToSave = manuscript.draft
            lastSaveStatus = .saving
            do {
                _ = try await client.createEpisodeVersion(
                    workID: wid, episodeID: eid,
                    request: CreateEpisodeVersionRequest(body: bodyToSave, note: "auto-save")
                )
                manuscript.markSaved(as: bodyToSave)
                lastSaveStatus = .saved(at: Date())
            } catch {
                lastSaveStatus = .failed(error.localizedDescription)
                toast.enqueue(.init(title: "Autosave failed: \(error.localizedDescription)", kind: .error))
            }
        }
    }
}
