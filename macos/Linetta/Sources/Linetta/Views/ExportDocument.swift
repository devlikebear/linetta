import Foundation
import SwiftUI
import UniformTypeIdentifiers

struct TextExportDocument: FileDocument {
    static var readableContentTypes: [UTType] {
        [.plainText, .linettaMarkdown]
    }

    var text: String

    init(text: String = "") {
        self.text = text
    }

    init(configuration: ReadConfiguration) throws {
        if let data = configuration.file.regularFileContents {
            text = String(decoding: data, as: UTF8.self)
        } else {
            text = ""
        }
    }

    func fileWrapper(configuration: WriteConfiguration) throws -> FileWrapper {
        FileWrapper(regularFileWithContents: Data(text.utf8))
    }
}

extension UTType {
    static var linettaMarkdown: UTType {
        UTType(filenameExtension: "md") ?? .plainText
    }
}

func safeExportFilename(_ value: String, fallback: String) -> String {
    let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
    let base = trimmed.isEmpty ? fallback : trimmed
    let invalid = CharacterSet(charactersIn: "/\\:?%*|\"<>")
    return base.components(separatedBy: invalid).joined(separator: "-")
}
