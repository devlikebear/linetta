import SwiftUI

struct NewWorkSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar

    @State private var title = ""
    @State private var genre = ""
    @State private var premise = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("New Work")
                .font(LinettaTypography.titleSmall)
                .foregroundStyle(LinettaTheme.text)

            TextField("Title", text: $title)
            TextField("Genre", text: $genre)
            TextEditor(text: $premise)
                .frame(minHeight: 120)
                .scrollContentBackground(.hidden)
                .background(LinettaTheme.surface)
                .overlay {
                    RoundedRectangle(cornerRadius: 6)
                        .stroke(LinettaTheme.borderSoft)
                }

            HStack {
                Spacer()
                Button("Cancel") {
                    dismiss()
                }
                Button("Create") {
                    Task {
                        await appState.createWork(title: title, genre: genre, premise: premise)
                        if let created = appState.selectedWork {
                            sidebar.selection = .work(workID: created.id)
                            sidebar.setExpanded(created.id, expanded: true)
                        }
                        dismiss()
                    }
                }
                .buttonStyle(.borderedProminent)
                .tint(LinettaTheme.accent)
                .keyboardShortcut(.defaultAction)
                .disabled(title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }
        .padding(20)
        .frame(width: 460)
        .background(LinettaTheme.background)
        .preferredColorScheme(.dark)
    }
}
