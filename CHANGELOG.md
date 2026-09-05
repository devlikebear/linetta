# Changelog

## v1.2.0 - 2026-09-06

- Added a built-in AI writing agent: connect a provider — sign in with a
  ChatGPT account, or add an API key for Anthropic, Google Gemini, or an
  OpenAI-compatible endpoint — and open the panel with `Cmd/Ctrl+J`. Consent to
  send your writing to a provider is given separately for each one.
- Added ChatGPT (Codex) sign-in inside the app, so connecting it no longer
  needs the Codex CLI installed. Tokens are kept at
  `<app data>/codex/auth.json` with owner-only permissions (0600), in the same
  file format the Codex CLI uses for its own login, so the two can share one
  sign-in — except in the Mac App Store build, which is sandboxed and cannot
  read `~/.codex`, and so needs its own sign-in.
- The MCP activity log now records whether the built-in agent or an external
  client made each tool call. Activity entries and the `mcp.changed`
  notification both carry a new `source` field for it.
- The tool surface is unchanged: the built-in agent is a client of Linetta's
  own MCP server, reaching the same tools external clients use, rather than a
  second set built for it.
- The consent text you tick before connecting a provider now spells out what
  the agent's tools can actually read — any scene in any work, not only the one
  you have open, plus summaries, the outline, manuscript-wide search, the
  character, plot and fact cards, remembered facts, and the list of your works.
  The behaviour did not change; the sentence you agree to now matches it.

## v1.1.0 - 2026-08-31

- Export now carries the whole work: synopsis, plot threads and beats, fact
  cards, margin notes, style notes, and each scene's status and summary ride in
  the markdown frontmatter, and import restores all of it. Git and folder sync
  write the same complete files.
- Restore no longer means replacing the library. Settings → Backup lists every
  backup, shows the works inside one, and brings a chosen work back as a new
  project — nothing you are writing now is touched, and a safety snapshot is
  taken first.
- Linetta can stay running in the system tray after the window closes, so a
  connected agent never loses its MCP connection, and can start hidden at
  login. Both are opt-in toggles under Settings → Run in background.
- Settings got a sidebar: App / Writing / Connections / Data categories replace
  the single long page.

## v1.0.0 - 2026-08-29

Linetta is a writing tool. It no longer talks to a language model.

- Removed the built-in AI companion, the provider settings, and every path that
  sent a manuscript to a third party. Linetta stores no API keys and calls no
  models.
- Added an MCP server hosted inside the app, so the agent you already run —
  Claude Code, Claude Desktop — can read and write the work directly. It is off
  until you consent, binds `127.0.0.1` only, and needs a token generated on your
  machine. `read_only` mode omits the writing tools entirely.
- Every change an agent makes is snapshotted before it lands, recorded in an
  activity log, and undoable. A scene you are part way through editing is never
  replaced behind your back.
- Scene summaries no longer need a model: a short scene is its own summary, a
  longer one keeps its opening, and an agent can replace either with a real one.
- The Fact Book is now a place to record what you checked and where, rather than
  to ask. Cards are readable by a connected agent.
- Existing companion conversations are preserved. Settings → Backup can export
  every project's transcript and remembered facts as one markdown file.
- Provider API keys already in your OS keychain are left untouched; remove them
  there if you want them gone.
- `Cmd/Ctrl+J` and `Cmd/Ctrl+I` are unbound.
- Added a Story World panel: the characters, places, items and concepts your
  work has registered, with per-kind lists, search across names and aliases, and
  a click through to the record. When an agent creates three characters, there
  is now somewhere to go and look.
- Registered names appearing in your prose are noticed as you write, instead of
  waiting for you to press a scan button. They are counted, not linked — a
  homonym would be linked to the wrong record, so applying stays your call.
- Engine failure and import messages now speak the reader's language. They were
  Korean sentences written in the engine, which an English or Japanese writer
  saw verbatim. So do the section headings in an exported manuscript, and the
  companion archive. Import reads all three, so a file exported in one language
  still opens after you switch.
- Fixed a character marked Protagonist counting for nothing unless you write in
  Korean. The engine matched core story roles against Korean labels only, so a
  connected agent asking for story context got a work with no cast.
- Markdown export says where the file landed, and names the sync folder when it
  landed in one.

## v0.9.6 - 2026-08-22

- Fixed folder sync and backup restore on Windows, which failed on every write because the atomic-write flush ran on a read-only file handle.
- Fixed the OpenRouter sign-in flow, which never opened a browser on Windows and left the writer to follow the fallback link by hand.
- Ran the verification gate on Windows in CI, and included the Tauri shell's Rust unit tests in it, so platform regressions are caught before a release.

## v0.9.5 - 2026-08-22

- Made Windows a working development and runtime target: LF checkouts, a debug-build fix for the embedded engine DLL, and Claude Code CLI detection that no longer relies on Unix permission bits.
- Added a Windows Credential Manager backend so API keys can be stored outside macOS; key-storage copy now names the platform's own store.
- Fixed the OpenRouter model list, which returned nothing once the API began reporting structured pricing values.

## v0.9.4 - 2026-08-03

- Updated React Router to address current client-routing security advisories and added a guard that rejects the RSC-only audit exception if RSC APIs are introduced.

## v0.9.3 - 2026-08-03

- Fixed AI provider connection tests to honor explicit data-sharing consent for the active provider without reusing consent across providers.

## v0.9.2 - 2026-07-14

- Prevented stale autosaves from overwriting another scene by serializing per-scene writes and enforcing optimistic content-version checks.
- Added verified daily and pre-migration backups, full-library `.linetta` recovery snapshots, startup restore controls, and atomic sync/import writes with partial-failure reporting.
- Required explicit, revocable consent before manuscript content is sent to a selected AI provider, and aligned the in-app disclosure and Korean/English/Japanese privacy policy.
- Narrowed renderer IPC and file-opening access, preserved structured RPC errors, aligned localization catalogs, and added lint and dependency-audit gates to CI.

## v0.9.1 - 2026-07-05

- Localized every AI surface for the selected app language (Korean/English/Japanese): the companion, inline editor AI, contextual edit, and Fact Book now build prompts and reply in the app language, including tool status text, apply results, and query outputs.
- Translated canonical outline node labels (부/장/씬, 권/화) in AI prompts and the workspace UI, so an English or Japanese UI shows Arc 1/Episode 1 or 第1巻/第1話 instead of raw Korean labels.
- Added English and Japanese keywords to the companion's intent detection so requests like "rewrite this scene" or 「書き直して」 apply changes directly, matching the Korean behavior.
- Added an English (U.S.) App Store listing.

## v0.9.0 - 2026-06-24

- Promoted desktop distribution to a public release: the macOS app is Developer ID signed and notarized through the Homebrew tap, with Windows (NSIS/MSI) and Linux (AppImage/deb/rpm) prebuilt installers published on every GitHub Release.
- Updated the README install guide to document the notarized macOS path and removed the obsolete quarantine workaround.

## v0.8.5 - 2026-06-23

- Improved the companion action picker with clearer current-scene and whole-work action scopes, plus persistent curated actions after a choice is used.
- Refined the OpenRouter beginner path with smarter model presets, friendlier credit/key-limit errors, and connection tests that avoid leaking raw provider JSON.
- Fixed Tauri dev/build invalidation so embedded Go engine changes are rebuilt when internal engine files change.

## v0.8.4 - 2026-06-22

- Added inline OpenRouter API key entry, model selection, model refresh, and connection testing to the beginner AI setup wizard.
- Loaded available OpenRouter models from the OpenRouter API while keeping `openrouter/auto` as the safe default.
- Compacted the companion AI setup dialog with tabbed setup choices, collapsed steps, and bounded modal height so it no longer gets cut off.

## v0.8.3 - 2026-06-21

- Replaced raw companion AI setup failures with a guided recovery card that preserves the blocked prompt for retry.
- Added a reusable AI setup surface with a beginner OpenRouter path, subscription guidance, and advanced direct-key options.
- Added OpenRouter provider support with OAuth PKCE connection, Keychain-backed key storage, model defaults, and key limit/credit status checks.

## v0.8.1 - 2026-06-21

- Restored Windows desktop CI packaging with NSIS and MSI installers.
- Build the embedded Go engine as `linetta_engine_ffi.dll` on Windows and load it at runtime to avoid MSVC c-archive link failures.
- Documented the Windows embedded-engine packaging path and added distribution validation coverage for the DLL bundle.

## v0.8.0 - 2026-06-21

- Embedded the Go engine in-process through a C ABI for desktop and mobile builds, removing the packaged desktop sidecar path.
- Added iOS xcframework and Android shared-library build scripts, mobile engine CI, and manual mobile release workflows.
- Added responsive Workspace coverage for mobile-sized screens and release smoke tooling for iOS simulator and Android APK/AAB packaging.

## v0.4.17 - 2026-06-14

- Made companion scene-writing and scene-edit requests transactional, so current-scene text changes require verified `set_scene_text` application instead of accepting model-only success claims.
- Added explicit companion scene intents, apply readback metadata, and current-editor refresh from verified changed-node events.
- Added retry handling for failed companion scene edits and kept failed proposal applications from refreshing as successful.

## v0.4.16 - 2026-06-14

- Kept long companion responses attached to the project session when the companion panel is closed or remounted, so completed replies and apply proposals are still visible when the writer returns.
- Added project-aware companion stream events so background companion runs can be routed safely across panel and screen changes.
- Serialized SQLite access through one connection and closed Fact Book cursors before nested source queries to avoid `SQLITE_BUSY` and single-connection deadlocks.

## v0.4.15 - 2026-06-14

- Turned the companion empty state into a writer action palette for continuing a scene, smoothing dialogue, raising tension, checking continuity, shaping the next-episode hook, and finishing an episode.
- Fixed companion proposal handling so `set_scene_text` scene-replacement ops from proofread/rewrite flows are preserved and labeled as current-scene text replacement instead of exposing raw op names.
- Replaced raw companion tool capability chips with writer-facing labels while keeping search, URL reading, apply-ops, context, and image attachment controls available.

## v0.4.14 - 2026-06-12

- Fixed the "이번 화" character count in the editor footer and ZEN mode summing the whole 권 when writing in a leaf episode created directly under an arc.
- Showed the episode character gauge on leaf episodes in the outline rail, matching container episodes.
- Seeded new webnovel projects with a `1권 > 1화` outline instead of a root `씬 1` so the first keystroke lands in an episode.

## v0.4.12 - 2026-06-07

- Fixed the Windows desktop build that broke in v0.4.11: bumped the bundled `tars` engine to v0.34.0, whose `claude-code-cli` process-teardown used unix-only syscalls. Updated to tars v0.34.1, which builds for Windows again. macOS and Linux were unaffected.

## v0.4.11 - 2026-06-07

- Upgraded the bundled `tars` LLM engine to v0.34.0: `claude-code-cli` runs are now bounded by a timeout (`CLAUDE_CODE_CLI_TIMEOUT`, default 5m) and a hung run no longer blocks until cancelled, descendant processes are killed as a group, and per-call startup is ~1s faster.

## v0.4.10 - 2026-06-07

- Recovered leaked `linetta_apply_ops` JSON into applyable Fact Book proposal cards instead of showing raw tool payloads.
- Wrapped long Fact Book assistant text, source URLs, and proposal cards so feedback stays inside the side panel.
- Added a Fact Book retry action for failed source URL saves so writers can search for an alternative source without restarting the review.

## v0.4.9 - 2026-06-07

- Added story-element presets for items, skills, magic, abilities, and relationship labels in the Entity Sheet.
- Extended companion story-state tools so worldbuilding elements can store structured attributes such as effects, costs, triggers, limits, and weaknesses.
- Renamed entity and AI context surfaces toward story/worldbuilding elements so items and concepts appear alongside characters and places.

## v0.4.7 - 2026-06-06

- Added companion context inspection, per-message copy controls, and image attachments from file upload or clipboard paste.
- Added outline inspection and repair tools that preserve node content while moving, restoring, renumbering, and undoing structure changes.
- Improved outline editing controls for parts, chapters, and scenes, including scrollable repair issue lists for long diagnostics.

## v0.4.6 - 2026-06-06

- Improved Fact Book source saving with direct URL fallback, clearer in-panel failure feedback, and web search API key connection testing.
- Added editor selection actions for fact-checking selected blocks and replacing selected text with AI.
- Added Linetta frontmatter metadata to markdown exports so imported markdown restores characters, places, and relationships instead of treating metadata appendices as scenes.

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
