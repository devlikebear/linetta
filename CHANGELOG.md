# Changelog

## v0.4.5 - 2026-06-05

- Added a Fact Book persistence layer with sourced fact cards for project-wide and scene-linked claims.
- Added companion support for creating sourced fact cards and reusing saved Fact Book context in later responses.
- Added a Workspace Fact Book panel with current-scene review prompts, source links, deletion, and localized UI copy.

## v0.4.4 - 2026-06-05

- Added a Homebrew cask post-install step that clears the macOS quarantine attribute for hobby releases that are not notarized yet.
- Kept optional Developer ID signing and notarization support in the release workflow for a future public distribution path.

## v0.4.3 - 2026-06-04

- Added written draft excerpts to companion context so character, relationship, and scene analysis can use the actual manuscript text.
- Saved the current editor document immediately before companion sends to avoid stale or missing draft context.
- Added an outline-building companion prompt example and made the companion composer grow for multi-line prompts.

## v0.4.2 - 2026-06-04

- Added companion empty-state prompt examples and a built-in tool help toggle.
- Added companion help copy for `web_search`, `web_fetch`, and `linetta_apply_ops` in Korean, English, and Japanese.
- Fixed OpenAI Codex ChatGPT-account connections by defaulting to `gpt-5.3-codex-spark` and replacing the unsupported legacy `gpt-5.3-codex` setting.

## v0.4.1 - 2026-06-04

- Added Windows and Linux release packaging for NSIS, MSI, AppImage, deb, and rpm bundles.
- Added winget manifest rendering and Flathub submission starter metadata.
- Added distribution metadata validation for release automation.

## v0.4.0 - 2026-06-03

- Added a guided onboarding tour for first-run users across the library and workspace.
- Added Settings controls to enable or disable automatic tours and replay the tour manually.
- Added persisted tour state so completed or skipped tours stay dismissed until the tour version changes.
- Localized onboarding tour copy in Korean, English, and Japanese.

## v0.3.0 - 2026-06-03

- Added Korean, English, and Japanese app UI language support with Korean as the default.
- Localized workspace, settings, search, AI generation, companion, entity, relationship, thread, version, import, and engine-gate surfaces.
- Added language-aware display for default scene and chapter labels without mutating stored manuscript data.

## v0.2.16 - 2026-06-03

- Added a beginner AI setup wizard with official setup links for OpenAI Codex, OpenAI API, Claude API, and Gemini API.
- Added a Settings connection test for the active AI provider.
- Moved provider and web-search API keys out of `settings.json` into macOS Keychain with redacted Settings responses.
