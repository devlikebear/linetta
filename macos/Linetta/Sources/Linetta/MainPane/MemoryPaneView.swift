import LinettaCore
import SwiftUI

struct MemoryPaneView: View {
    let work: Work

    @Environment(AppState.self) private var appState
    @State private var selectedKind: MemoryKind = .character
    @State private var selectedStatus: MemoryStatus = .canon
    @State private var query = ""
    @State private var items: [MemoryItem] = []
    @State private var selectedItem: MemoryItem?
    @State private var title = ""
    @State private var bodyText = ""
    @State private var importance: MemoryImportance = .medium
    @State private var isLoading = false
    @State private var errorMessage: String?

    var body: some View {
        VStack(spacing: 0) {
            toolbar.padding(.horizontal, 18).padding(.vertical, 12).background(LinettaTheme.surface)
            Divider().background(LinettaTheme.border)
            HStack(spacing: 0) {
                itemList.frame(minWidth: 280, maxWidth: 360)
                Divider().background(LinettaTheme.borderSoft)
                editor.frame(maxWidth: .infinity)
            }
        }
        .background(LinettaTheme.background)
        .task(id: work.id) { await reload() }
        .task(id: query) { await reload() }
        .task(id: selectedKind) { await reload() }
        .task(id: selectedStatus) { await reload() }
    }

    private var toolbar: some View {
        HStack {
            Picker("Kind", selection: $selectedKind) {
                ForEach(MemoryKind.allCases) { k in Text(k.label).tag(k) }
            }.labelsHidden().frame(width: 140)
            Picker("Status", selection: $selectedStatus) {
                ForEach(MemoryStatus.allCases) { s in Text(s.rawValue.capitalized).tag(s) }
            }.labelsHidden().frame(width: 120)
            TextField("Search…", text: $query).textFieldStyle(.roundedBorder).frame(width: 200)
            Spacer()
        }
    }

    private var itemList: some View {
        List(selection: $selectedItem) {
            ForEach(items) { item in
                VStack(alignment: .leading) {
                    Text(item.title).font(LinettaTypography.body).foregroundStyle(LinettaTheme.text)
                    Text(item.body).font(LinettaTypography.bodySmall).foregroundStyle(LinettaTheme.textTertiary).lineLimit(2)
                }.tag(item)
            }
        }
    }

    private var editor: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("New Canon Item").font(LinettaTypography.titleSmall).foregroundStyle(LinettaTheme.text)
            HStack {
                TextField("Title", text: $title).textFieldStyle(.roundedBorder)
                Picker("Importance", selection: $importance) {
                    ForEach(MemoryImportance.allCases) { i in Text(i.rawValue.capitalized).tag(i) }
                }.labelsHidden().frame(width: 120)
            }
            TextEditor(text: $bodyText)
                .font(LinettaTypography.body)
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(LinettaTheme.borderSoft))
            HStack {
                Spacer()
                Button("Save") { Task { await save() } }
                    .buttonStyle(.borderedProminent).tint(LinettaTheme.accent)
                    .disabled(title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
            if let errorMessage {
                Text(errorMessage).font(LinettaTypography.caption).foregroundStyle(LinettaTheme.danger)
            }
        }.padding(18)
    }

    private func reload() async {
        isLoading = true; defer { isLoading = false }
        do {
            items = try await appState.client.listMemory(
                workID: work.id,
                kind: query.isEmpty ? selectedKind : nil,
                status: query.isEmpty ? selectedStatus : nil,
                query: query.isEmpty ? nil : query
            )
        } catch { errorMessage = error.localizedDescription }
    }

    private func save() async {
        do {
            let item = try await appState.client.createMemory(
                workID: work.id,
                request: CreateMemoryRequest(
                    kind: selectedKind,
                    title: title,
                    body: bodyText,
                    status: selectedStatus,
                    importance: importance,
                    reason: "Created from Memory pane"
                )
            )
            items.insert(item, at: 0)
            title = ""; bodyText = ""
        } catch { errorMessage = error.localizedDescription }
    }
}
