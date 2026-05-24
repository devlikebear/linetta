import LinettaCore
import SwiftUI

struct ManuscriptInspector: View {
    @Environment(ManuscriptState.self) private var manuscript

    var body: some View {
        VStack(spacing: 0) {
            ManuscriptHeader(versionLabel: versionLabel)
            switch manuscript.mode {
            case .adopted:
                ManuscriptEditor()
            case .artifactPreview(_, _, let body):
                ArtifactPreviewView(text: body)
            }
        }
        .background(LinettaTheme.background)
    }

    private var versionLabel: String {
        switch manuscript.mode {
        case .adopted: return "current"
        case .artifactPreview: return "preview"
        }
    }
}
