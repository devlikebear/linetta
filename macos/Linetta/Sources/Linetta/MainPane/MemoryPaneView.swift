import LinettaCore
import SwiftUI

struct MemoryPaneView: View {
    let work: Work
    var body: some View { Text("Memory · \(work.title)").foregroundStyle(LinettaTheme.text) }
}
