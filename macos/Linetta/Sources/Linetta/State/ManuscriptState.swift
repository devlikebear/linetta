import Foundation
import Observation

enum ManuscriptMode: Equatable {
    case adopted
    case artifactPreview(runID: String, artifactID: String, body: String)
}

@MainActor
@Observable
final class ManuscriptState {
    var mode: ManuscriptMode = .adopted
    var draft: String = ""
    private(set) var loadedSnapshot: String = ""
    var width: Double {
        get { _width }
        set {
            _width = max(280, min(480, newValue))
            UserDefaults.standard.set(_width, forKey: "linetta.ui.inspector.width")
        }
    }
    private var _width: Double = 320

    init() {
        let stored = UserDefaults.standard.double(forKey: "linetta.ui.inspector.width")
        _width = max(280, min(480, stored == 0 ? 320 : stored))
    }

    var isDirty: Bool { draft != loadedSnapshot }

    func loadAdopted(body: String) {
        mode = .adopted
        draft = body
        loadedSnapshot = body
    }

    /// Sets `loadedSnapshot` to the body the autosave just wrote to the server.
    /// Pass the exact body that was sent — using `draft` here is wrong because
    /// the user may have typed more during the network call, and the next
    /// keystroke would then be considered "clean".
    func markSaved(as savedBody: String) {
        loadedSnapshot = savedBody
    }

    func isOpen(episodeID: String) -> Bool {
        UserDefaults.standard.bool(forKey: "linetta.ui.inspector.open.\(episodeID)")
    }

    func setOpen(episodeID: String, open: Bool) {
        UserDefaults.standard.set(open, forKey: "linetta.ui.inspector.open.\(episodeID)")
    }
}
