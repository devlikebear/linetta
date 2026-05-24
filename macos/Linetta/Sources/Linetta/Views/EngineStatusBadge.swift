import LinettaCore
import SwiftUI

struct EngineStatusBadge: View {
    @EnvironmentObject private var engine: EngineController
    @State private var showingDetail = false

    var body: some View {
        Button {
            showingDetail.toggle()
        } label: {
            HStack(spacing: 6) {
                Circle()
                    .fill(color)
                    .frame(width: 8, height: 8)
                    .opacity(engine.status == .starting ? 0.55 : 1.0)
                    .scaleEffect(engine.status == .starting ? 1.15 : 1.0)
                    .animation(
                        engine.status == .starting
                            ? .easeInOut(duration: 0.7).repeatForever(autoreverses: true)
                            : .default,
                        value: engine.status
                    )
                Text(label)
                    .font(.caption.weight(.medium))
                    .foregroundStyle(.primary)
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(.regularMaterial, in: Capsule())
        }
        .buttonStyle(.plain)
        .help(tooltip)
        .popover(isPresented: $showingDetail, arrowEdge: .bottom) {
            detailPopover
                .padding(14)
                .frame(width: 340)
        }
    }

    private var color: Color {
        switch engine.status {
        case .stopped: return .gray
        case .starting: return .yellow
        case .healthy: return .green
        case .failed: return .red
        case .external: return .blue
        }
    }

    private var label: String {
        switch engine.status {
        case .stopped: return "Engine off"
        case .starting: return "Starting…"
        case .healthy: return "Engine"
        case .failed: return "Engine down"
        case .external: return "External"
        }
    }

    private var tooltip: String {
        if let addr = engine.address {
            return "\(label) — \(addr.absoluteString)"
        }
        return label
    }

    private var detailPopover: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Circle().fill(color).frame(width: 10, height: 10)
                Text(label).font(.headline)
                Spacer()
            }
            if let addr = engine.address {
                Text("Address: \(addr.absoluteString)")
                    .font(.caption)
                    .textSelection(.enabled)
            }
            if let pid = engine.pid {
                Text("PID: \(pid)").font(.caption)
            }
            if case let .failed(reason) = engine.status {
                Text("Error: \(reason)")
                    .font(.caption)
                    .foregroundStyle(.red)
            }
            Divider()
            Text("Recent log")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
            ScrollView {
                VStack(alignment: .leading, spacing: 2) {
                    ForEach(Array(engine.recentLog.suffix(5).enumerated()), id: \.offset) { _, line in
                        Text(line)
                            .font(.system(.caption2, design: .monospaced))
                            .lineLimit(2)
                            .foregroundStyle(.secondary)
                    }
                    if engine.recentLog.isEmpty {
                        Text("(no output yet)")
                            .font(.caption2)
                            .foregroundStyle(.tertiary)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .frame(maxHeight: 100)
        }
    }
}
