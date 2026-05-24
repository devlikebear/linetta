import SwiftUI

struct SidebarMemoryRow: View {
    let workID: String

    @Environment(SidebarState.self) private var sidebar

    var body: some View {
        Button {
            sidebar.selection = .memory(workID: workID)
        } label: {
            HStack(spacing: 6) {
                Text("📓").font(.system(size: 10))
                Text("Memory")
                    .font(LinettaTypography.body)
                    .foregroundStyle(LinettaTheme.text)
                Spacer()
            }
            .padding(.leading, 24)
            .padding(.trailing, 8)
            .padding(.vertical, 4)
            .background(isSelected ? LinettaTheme.accentSoft : Color.clear)
            .clipShape(RoundedRectangle(cornerRadius: 5))
        }
        .buttonStyle(.plain)
    }

    private var isSelected: Bool {
        if case .memory(let wid) = sidebar.selection, wid == workID { return true }
        return false
    }
}
