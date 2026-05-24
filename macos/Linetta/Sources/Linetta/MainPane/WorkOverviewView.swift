import LinettaCore
import SwiftUI

struct WorkOverviewView: View {
    let work: Work
    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text(work.title).font(LinettaTypography.titleLarge).foregroundStyle(LinettaTheme.text)
            if !work.genre.isEmpty {
                LabeledContent("Genre") { Text(work.genre).foregroundStyle(LinettaTheme.textSecondary) }
            }
            if !work.premise.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Premise").linettaLabelStyle()
                    Text(work.premise).foregroundStyle(LinettaTheme.text)
                }
            }
            Spacer()
        }
        .padding(28)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(LinettaTheme.background)
    }
}
