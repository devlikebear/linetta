import SwiftUI

struct ManuscriptEditor: View {
    @Environment(ManuscriptState.self) private var manuscript
    var body: some View {
        @Bindable var manuscript = manuscript
        TextEditor(text: $manuscript.draft)
            .font(LinettaTypography.bodySerif)
            .padding(14)
    }
}
