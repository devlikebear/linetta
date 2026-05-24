import SwiftUI

struct ArtifactPreviewView: View {
    let text: String
    var body: some View {
        ScrollView {
            Text(text).font(LinettaTypography.bodySerif).padding(14).frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}
