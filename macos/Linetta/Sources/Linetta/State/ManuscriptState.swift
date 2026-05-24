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

    func markSaved() {
        loadedSnapshot = draft
    }

    func isOpen(episodeID: String) -> Bool {
        UserDefaults.standard.bool(forKey: "linetta.ui.inspector.open.\(episodeID)")
    }

    func setOpen(episodeID: String, open: Bool) {
        UserDefaults.standard.set(open, forKey: "linetta.ui.inspector.open.\(episodeID)")
    }
}
