import SwiftUI

struct SettingsView: View {
    var body: some View {
        Form {
            Section("Engine") {
                LabeledContent("Address", value: "http://127.0.0.1:43190")
                Text("Start the local engine with `linetta serve` before opening the gallery.")
                    .foregroundStyle(.secondary)
            }
        }
        .padding(20)
        .frame(width: 460)
    }
}
