import LinettaCore
import SwiftUI

struct SidebarWorkRow: View {
    let work: Work
    var body: some View {
        Text(work.title)
            .font(LinettaTypography.body)
            .foregroundStyle(LinettaTheme.text)
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
    }
}
