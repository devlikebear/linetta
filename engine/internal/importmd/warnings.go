package importmd

// Import warnings are codes, not sentences.
//
// They used to be Korean strings written in the engine and rendered verbatim,
// which meant an English or Japanese writer importing a file got a Korean
// paragraph in the middle of their own UI (#45). The engine has no idea what
// language the reader uses — the desktop app carries the ko/en/ja catalogue —
// so it names the situation and lets the app say it.
//
// Keep in step with apps/desktop/src/lib/importWarnings.ts.
const (
	// WarnNoHeadings: the markdown had no #/##/###/#### headings, so the
	// import produces an empty work.
	WarnNoHeadings = "import.no_headings"
	// WarnFrontmatterUnreadable: Linetta frontmatter was present but could not
	// be parsed, so stored metadata was not restored.
	WarnFrontmatterUnreadable = "import.frontmatter_unreadable"
	// WarnRelationshipsSkipped: some relationships named an entity the import
	// did not create.
	WarnRelationshipsSkipped = "import.relationships_skipped"
	// WarnUnknownOutlinePreset: the requested outline preset does not exist and
	// was ignored. Carries the offending value after a colon.
	WarnUnknownOutlinePreset = "import.unknown_outline_preset"
)
