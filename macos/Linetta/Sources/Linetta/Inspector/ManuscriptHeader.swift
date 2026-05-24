import LinettaCore
import SwiftUI

struct ManuscriptHeader: View {
    let versionLabel: String
    var onCloseTap: () -> Void = {}
    var body: some View {
        HStack {
            Text("📄 MANUSCRIPT").linettaLabelStyle()
            Spacer()
            Text(versionLabel).font(LinettaTypography.titleSmall).foregroundStyle(LinettaTheme.text)
            Menu { Text("More actions in Phase 7+") } label: { Image(systemName: "ellipsis") }
                .menuStyle(.borderlessButton).frame(width: 24)
        }
        .padding(.horizontal, 14).padding(.vertical, 12)
        .background(LinettaTheme.surfaceElevated)
        .overlay(alignment: .bottom) { Rectangle().fill(LinettaTheme.borderSoft).frame(height: 1) }
    }
}
