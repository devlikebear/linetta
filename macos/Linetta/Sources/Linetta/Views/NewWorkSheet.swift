import SwiftUI

struct NewWorkSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(AppState.self) private var appState

    @State private var title = ""
    @State private var genre = ""
    @State private var premise = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("New Work")
                .font(.title2)
                .fontWeight(.semibold)

            TextField("Title", text: $title)
            TextField("Genre", text: $genre)
            TextEditor(text: $premise)
                .frame(minHeight: 120)
                .overlay {
                    RoundedRectangle(cornerRadius: 6)
                        .stroke(Color.secondary.opacity(0.25))
                }

            HStack {
                Spacer()
                Button("Cancel") {
                    dismiss()
                }
                Button("Create") {
                    Task {
                        await appState.createWork(title: title, genre: genre, premise: premise)
                        dismiss()
                    }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }
        .padding(20)
        .frame(width: 460)
    }
}
