import LinettaCore
import SwiftUI

struct CanonMemoryView: View {
    var work: Work

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

    private let client = APIClient()

    var body: some View {
        NavigationSplitView {
            VStack(spacing: 12) {
                Picker("Kind", selection: $selectedKind) {
                    ForEach(MemoryKind.allCases) { kind in
                        Text(kind.label).tag(kind)
                    }
                }
                .pickerStyle(.segmented)

                Picker("Status", selection: $selectedStatus) {
                    ForEach(MemoryStatus.allCases) { status in
                        Text(status.rawValue.capitalized).tag(status)
                    }
                }
                .pickerStyle(.segmented)

                TextField("Search memory", text: $query)
                    .textFieldStyle(.roundedBorder)

                List(selection: $selectedItem) {
                    ForEach(items) { item in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(item.title)
                                .font(.headline)
                                .lineLimit(1)
                            Text(item.body)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                                .lineLimit(2)
                        }
                        .padding(.vertical, 4)
                        .tag(item)
                    }
                }

                Button {
                    startNewItem()
                } label: {
                    Label("New Memory", systemImage: "plus")
                }
                .buttonStyle(.borderedProminent)
                .keyboardShortcut("m", modifiers: [.command, .shift])
            }
            .padding(16)
            .navigationTitle("Canon Memory")
            .task {
                await loadItems()
            }
            .onChange(of: selectedKind) { _, _ in
                Task { await loadItems() }
            }
            .onChange(of: selectedStatus) { _, _ in
                Task { await loadItems() }
            }
            .onSubmit {
                Task { await loadItems() }
            }
        } detail: {
            VStack(alignment: .leading, spacing: 14) {
                HStack {
                    Text(selectedItem == nil ? "New Canon Item" : "Edit Canon Item")
                        .font(.title2)
                        .fontWeight(.semibold)
                    Spacer()
                    Picker("Importance", selection: $importance) {
                        ForEach(MemoryImportance.allCases) { value in
                            Text(value.rawValue.capitalized).tag(value)
                        }
                    }
                    .frame(width: 160)
                }

                TextField("Title", text: $title)
                TextEditor(text: $bodyText)
                    .frame(minHeight: 260)
                    .overlay {
                        RoundedRectangle(cornerRadius: 6)
                            .stroke(Color.secondary.opacity(0.25))
                    }

                HStack {
                    if isLoading {
                        ProgressView()
                    }
                    if let errorMessage {
                        Text(errorMessage)
                            .foregroundStyle(.red)
                    }
                    Spacer()
                    if let selectedItem {
                        Button("Archive") {
                            Task { await archive(item: selectedItem) }
                        }
                    }
                    Button("Save") {
                        Task { await save() }
                    }
                    .keyboardShortcut(.defaultAction)
                    .keyboardShortcut("s", modifiers: [.command])
                    .disabled(title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
            }
            .padding(22)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
        .onChange(of: selectedItem) { _, item in
            edit(item: item)
        }
    }

    private func loadItems() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            items = try await client.listMemory(
                workID: work.id,
                kind: query.isEmpty ? selectedKind : nil,
                status: query.isEmpty ? selectedStatus : nil,
                query: query
            )
            if let selectedItem, !items.contains(where: { $0.id == selectedItem.id }) {
                self.selectedItem = nil
            }
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func startNewItem() {
        selectedItem = nil
        title = ""
        bodyText = ""
        importance = .medium
    }

    private func edit(item: MemoryItem?) {
        guard let item else {
            return
        }
        title = item.title
        bodyText = item.body
        importance = item.importance
    }

    private func save() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            if let selectedItem {
                let updated = try await client.updateMemory(
                    workID: work.id,
                    itemID: selectedItem.id,
                    request: UpdateMemoryRequest(
                        title: title,
                        body: bodyText,
                        status: selectedStatus,
                        importance: importance,
                        reason: "Updated in Canon Memory"
                    )
                )
                self.selectedItem = updated
            } else {
                let created = try await client.createMemory(
                    workID: work.id,
                    request: CreateMemoryRequest(
                        kind: selectedKind,
                        title: title,
                        body: bodyText,
                        status: selectedStatus,
                        importance: importance,
                        reason: "Created in Canon Memory"
                    )
                )
                selectedItem = created
            }
            await loadItems()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func archive(item: MemoryItem) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            _ = try await client.archiveMemory(
                workID: work.id,
                itemID: item.id,
                request: ArchiveMemoryRequest(reason: "Archived in Canon Memory")
            )
            selectedItem = nil
            await loadItems()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
