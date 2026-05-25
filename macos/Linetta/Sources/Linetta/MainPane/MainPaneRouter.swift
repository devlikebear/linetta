import LinettaCore
import SwiftUI

struct MainPaneRouter: View {
    @Environment(AppState.self) private var appState
    @Environment(SidebarState.self) private var sidebar
    @EnvironmentObject private var engine: EngineController

    var body: some View {
        Group {
            if shouldShowEngineRecovery {
                EngineRecoveryView()
            } else if appState.works.isEmpty {
                OnboardingView()
            } else {
                switch sidebar.selection {
                case .none:
                    if let first = appState.works.first {
                        WorkOverviewView(work: first)
                    } else {
                        OnboardingView()
                    }
                case .work(let wid):
                    if let work = appState.works.first(where: { $0.id == wid }) {
                        WorkOverviewView(work: work)
                    } else { OnboardingView() }
                case .memory(let wid):
                    if let work = appState.works.first(where: { $0.id == wid }) {
                        MemoryPaneView(work: work)
                    } else { OnboardingView() }
                case .decisions(let wid):
                    if let work = appState.works.first(where: { $0.id == wid }) {
                        DecisionsHistoryView(work: work)
                    } else { OnboardingView() }
                case .episode(let wid, let eid):
                    if let work = appState.works.first(where: { $0.id == wid }) {
                        EpisodeWorkspaceView(work: work, episodeID: eid)
                    } else { OnboardingView() }
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(LinettaTheme.background)
    }

    /// Show the recovery screen when the engine is unavailable AND we have no
    /// cached works to keep the user busy with. If works are loaded, we keep
    /// the normal UI and rely on the StatusFooter to surface engine trouble.
    private var shouldShowEngineRecovery: Bool {
        guard appState.works.isEmpty else { return false }
        switch engine.status {
        case .failed, .stopped: return true
        default: return false
        }
    }
}

private struct EngineRecoveryView: View {
    @EnvironmentObject private var engine: EngineController
    @Environment(ToastCenter.self) private var toast
    @State private var inFlight = false

    var body: some View {
        VStack(spacing: 16) {
            Image(systemName: "antenna.radiowaves.left.and.right.slash")
                .font(.system(size: 42))
                .foregroundStyle(LinettaTheme.danger)
            Text("Engine offline")
                .font(LinettaTypography.titleLarge)
                .foregroundStyle(LinettaTheme.text)
            Text(detailMessage)
                .font(LinettaTypography.body)
                .foregroundStyle(LinettaTheme.textSecondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 480)
            HStack(spacing: 10) {
                Button {
                    Task { await start() }
                } label: {
                    Label(inFlight ? "Starting…" : "Start engine", systemImage: "play.fill")
                }
                .buttonStyle(.borderedProminent)
                .tint(LinettaTheme.accent)
                .disabled(inFlight)

                SettingsLink {
                    Label("Open Settings", systemImage: "gearshape")
                }
                .buttonStyle(.bordered)
            }
            .padding(.top, 8)
            let recent = engine.recentLog.suffix(3).joined(separator: "\n")
            if !recent.isEmpty {
                ScrollView {
                    Text(recent)
                        .font(.system(.caption2, design: .monospaced))
                        .foregroundStyle(LinettaTheme.textTertiary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(10)
                }
                .frame(maxWidth: 520, maxHeight: 100)
                .background(LinettaTheme.surfaceElevated)
                .clipShape(RoundedRectangle(cornerRadius: 7))
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(LinettaTheme.background)
    }

    private var detailMessage: String {
        if case .failed(let reason) = engine.status {
            return "The Linetta engine could not start.\n\(reason)"
        }
        return "The Linetta engine is not running. Start it to load your library."
    }

    private func start() async {
        inFlight = true; defer { inFlight = false }
        do {
            try await engine.restart()
            toast.enqueue(.init(title: "Engine started", kind: .success))
        } catch {
            toast.enqueue(.init(title: "Engine start failed: \(error.localizedDescription)", kind: .error))
        }
    }
}
