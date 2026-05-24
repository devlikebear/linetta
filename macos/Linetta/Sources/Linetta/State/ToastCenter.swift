import Foundation
import Observation

struct ToastMessage: Identifiable, Equatable {
    enum Kind { case info, success, warn, error }
    let id = UUID()
    let title: String
    let kind: Kind
}

@MainActor
@Observable
final class ToastCenter {
    private(set) var toasts: [ToastMessage] = []

    func enqueue(_ message: ToastMessage) {
        toasts.append(message)
        Task { @MainActor [weak self] in
            try? await Task.sleep(nanoseconds: 4_000_000_000)
            self?.dismiss(message.id)
        }
    }

    func dismiss(_ id: UUID) {
        toasts.removeAll { $0.id == id }
    }
}
