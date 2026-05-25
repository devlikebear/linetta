import AppKit
import SwiftUI

/// NSTextView-based long-form editor with undo/redo, ⌘F find, and word count
/// support. Replaces the SwiftUI TextEditor in ManuscriptInspector so users get
/// proper macOS text editing (rich undo stack, find panel, system text actions)
/// for 5,000+ word manuscripts.
struct LongformEditor: NSViewRepresentable {
    @Binding var text: String
    var font: NSFont = NSFont(descriptor: NSFontDescriptor(name: "New York", size: 14), size: 14) ?? .systemFont(ofSize: 14)
    var onTextChange: () -> Void = {}

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeNSView(context: Context) -> NSScrollView {
        let scrollView = NSTextView.scrollableTextView()
        guard let textView = scrollView.documentView as? NSTextView else {
            return scrollView
        }
        textView.delegate = context.coordinator
        textView.allowsUndo = true
        textView.isAutomaticQuoteSubstitutionEnabled = false
        textView.isAutomaticDashSubstitutionEnabled = false
        textView.isAutomaticTextReplacementEnabled = false
        textView.isAutomaticSpellingCorrectionEnabled = false
        textView.isRichText = false
        textView.importsGraphics = false
        textView.usesFindBar = true
        textView.isIncrementalSearchingEnabled = true
        textView.usesAdaptiveColorMappingForDarkAppearance = true
        textView.backgroundColor = NSColor(red: 0.106, green: 0.102, blue: 0.090, alpha: 1)
        textView.textColor = NSColor(red: 0.839, green: 0.827, blue: 0.796, alpha: 1)
        textView.insertionPointColor = NSColor(red: 0.851, green: 0.467, blue: 0.341, alpha: 1)
        textView.font = font
        textView.textContainerInset = NSSize(width: 12, height: 12)
        textView.string = text
        scrollView.drawsBackground = true
        scrollView.backgroundColor = textView.backgroundColor
        scrollView.borderType = .noBorder
        scrollView.hasVerticalScroller = true
        scrollView.autohidesScrollers = true
        return scrollView
    }

    func updateNSView(_ scrollView: NSScrollView, context: Context) {
        guard let textView = scrollView.documentView as? NSTextView else { return }
        // Only push external changes to the text view when the bound text
        // differs — otherwise we'd clobber the user's cursor on every keystroke.
        if textView.string != text {
            // Preserve selection across programmatic updates.
            let selectedRanges = textView.selectedRanges
            textView.string = text
            textView.selectedRanges = selectedRanges
        }
        if textView.font != font {
            textView.font = font
        }
    }

    final class Coordinator: NSObject, NSTextViewDelegate {
        var parent: LongformEditor
        init(_ parent: LongformEditor) { self.parent = parent }

        func textDidChange(_ notification: Notification) {
            guard let textView = notification.object as? NSTextView else { return }
            parent.text = textView.string
            parent.onTextChange()
        }

        func textView(_ textView: NSTextView, doCommandBy commandSelector: Selector) -> Bool {
            // Let NSTextView handle ⌘F by showing the find bar (default behavior).
            // Return false to allow defaults.
            return false
        }
    }
}
