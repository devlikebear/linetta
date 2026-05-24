import SwiftUI

struct OnboardingView: View {
    @State private var showSheet = false
    var body: some View {
        VStack(spacing: 14) {
            Text("Linetta").font(LinettaTypography.titleLarge).foregroundStyle(LinettaTheme.text)
            Text("An AI workflow runner for serial fiction.")
                .font(LinettaTypography.body).foregroundStyle(LinettaTheme.textSecondary)
            Button("Create your first work") { showSheet = true }
                .buttonStyle(.borderedProminent).tint(LinettaTheme.accent)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(LinettaTheme.background)
        .sheet(isPresented: $showSheet) { NewWorkSheet() }
    }
}
