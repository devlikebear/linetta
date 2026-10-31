# Linetta development guide

This guide collects contributor-facing architecture, build, release, and troubleshooting details. For the product overview and downloads, see the [main README](../README.md).

## Architecture

- Tauri 2 Rust shell with React 18, Vite, and TypeScript
- Embedded Go engine linked through a C ABI
- JSON-RPC envelopes as the internal request contract
- SQLite under the local Linetta data directory
- [`github.com/devlikebear/tars`](https://github.com/devlikebear/tars) for URL fetching and the keyword memory store

Cargo links the embedded Go engine through `apps/desktop/src-tauri/build.rs`.

## Desktop development

Install dependencies once:

```sh
cd apps/desktop
pnpm install
```

Start the desktop app:

```sh
make dev
```

This wraps `scripts/dev.sh`, which launches `tauri dev`.

Build the standalone JSON-RPC engine for debugging:

```sh
make build-engine
```

The debug binary is written under `engine/bin/` and is not bundled into the Tauri app.

Build the desktop release binary for the current operating system:

```sh
make build-desktop
```

## Verification

Run the full local gate:

```sh
make test
```

Useful narrower checks:

```sh
make test-go
make test-desktop
make test-tauri
make test-mobile-engine
```

`make test` runs Go tests, frontend Vitest tests, the Vite production build, and the Tauri shell's `cargo check` and `cargo test`. `make test-mobile-engine` runs the Go engine suite with the `mobile` build tag so iOS- and Android-safe stubs stay covered.

## Mobile engine development

Build mobile engine artifacts directly when the platform toolchains are installed:

```sh
make build-mobile-engine-ios
make build-mobile-engine-android
```

The iOS target requires Xcode's iOS SDK. The Android target requires `ANDROID_NDK_HOME`.

Regenerate the ignored Tauri mobile native projects with:

```sh
make mobile-ios-init
make mobile-android-init
```

For local iOS simulator smoke testing, install Xcode's matching simulator runtime and run:

```sh
make build-mobile-ios-sim
make smoke-mobile-ios-sim
```

Open Xcode and complete its license and first-launch setup before running these
commands. A license error from `xcrun simctl list devices available` blocks both
native builds and simulator verification.

The app and embedded engine target iOS/iPadOS 15.0 or newer. Tauri preserves an
existing `apps/desktop/src-tauri/gen/apple/project.yml` during initialization.
If an older generated project still targets 14.0, back up that file outside the
generated directory, then run `make mobile-ios-init` to recreate it. Reapply any
local native customizations from the backup before building.

Use the Make targets for simulator builds and development: they prepend
`scripts/ios-toolchain` to PATH so swift-rs uses SwiftPM's native build system.
This avoids Xcode 27's default Swift build system mixing macOS and iOS SDK flags.
For direct `pnpm tauri ios build` commands (including device builds), prepend the
same absolute directory to PATH. This wrapper only changes `swift build`;
compiler information queries still pass through to the selected Xcode toolchain.

The smoke script defaults to an available iPhone. Verify iPad separately by
choosing its UDID from `xcrun simctl list devices available`:

```sh
LINETTA_IOS_SIM_UDID="<iPad simulator UDID>" make smoke-mobile-ios-sim
make dev-mobile-ios IOS_SIM="<installed iPad simulator name>"
```

The shared mobile CSS covers narrow desktop windows and touch screens up to
1366 CSS pixels, including landscape phones. The iPad rules override it at
701–1366 pixels wide and at least 600 pixels tall. Keep `MOBILE_WORKSPACE_QUERY`
in `useSizeClass.ts` aligned with the shared media query in `App.css`, so outline
state and rendered controls agree. Verify portrait and landscape, opening the
outline and inspectors, and software-keyboard transitions on both device types.
Browser layout checks do not replace the native engine smoke or device input QA.

For local Android smoke testing, install the Android SDK and NDK and run:

```sh
make build-mobile-android-debug
make build-mobile-android-release-smoke
```

The iOS simulator target creates a no-sign `.app`, links the embedded Go engine, installs and launches it, verifies engine symbols, and checks that `library.db` is created. The Android release smoke target builds APK and AAB artifacts, verifies packaged native libraries and signing, and avoids using real upload credentials.

Signed iOS export is handled by the manual mobile release workflow because it depends on Apple team and signing credentials.

## MCP tools

Linetta hosts an MCP server inside the running app, so an external client —
Claude Code, Claude Desktop — can work on the manuscript. It is off until the
writer consents and enables it under **Settings → Connect an external agent**,
binds `127.0.0.1` only, and requires a locally generated bearer token.

The default tool set has 45 tools (18 read, 27 write). Optional memory, skills,
and visual-write groups can be switched off in Settings → Tool budget. Write tools appear
only in `full` mode, so `read_only` omits them from `tools/list` entirely
rather than refusing them at call time.

Read:

- `linetta_list_works`, `linetta_get_outline`, `linetta_get_story_context`
- `linetta_read_scene`, `linetta_list_characters`, `linetta_get_fact_cards`
- `linetta_get_character_visuals`, `linetta_build_illustration_prompt`
- `linetta_analyze_style`: holds a scene, a chapter or the whole work against
  the writer's style rules (`internal/style`) and reports violations. It is
  deterministic and model-free; tone is left to the caller. The same call
  returns `stats` — counted measurements of the scenes read (`style.Measure`)
- `linetta_search_manuscript`, `linetta_where_does_appear`, `linetta_get_plot`
- `linetta_read_skill`
- `linetta_list_continuity`: recorded continuity observations and their ids,
  for `linetta_update_continuity`; open ones by default
- `linetta_list_proposals`: the suggestions inbox's undecided items and their
  ids, for `linetta_accept_proposals` and `linetta_reject_proposals`
- `linetta_publish_status`: each episode's state on Linetta Works (see
  [Publishing](#publishing-to-linetta-works) below). Answers locally, without
  contacting the site, when no account is connected

`linetta_read_scene` returns plain prose by default — the form every write
tool takes back. `format: "markdown"` returns the scene through
`export.DocToMarkdown` instead, for carrying it out of Linetta with its
formatting; it is not a form to write back.

Write (`full` mode only):

- `linetta_write_scene`: requires the `content_version` from a prior read, so a
  scene the writer edited meanwhile is never silently overwritten;
- `linetta_revise_scene`: exact-string replacement with a `dry_run` preview;
- `linetta_write_summary`, `linetta_apply_story_ops`;
- `linetta_create_checkpoint`, `linetta_undo_last_change`;
- `linetta_create_work`, `linetta_edit_memory`, `linetta_edit_skill`;
- `linetta_propose_style_profile`: saves an agent's description of the
  writer's style as a pending draft (`project_style_drafts`). Only the app's
  `style.approve_draft` RPC moves a draft into `projects.style_notes`, and
  so into the story brief; there is no MCP tool that approves;
- `linetta_set_character_visuals`, `linetta_set_art_style`;
- `linetta_add_reference_image`, `linetta_delete_reference_image`;
- `linetta_register_work`: registers the work on the site with a blurb, or
  changes its blurb or serial status, through `publish.Service.UpsertWork`.
  An empty field keeps the site's current value;
- `linetta_publish_episode`: publishes one episode, or updates its text on the
  site, through the same `publish.Service` the app's publish dialog uses.
  Refused until the work is registered;
- `linetta_publish_cover`: makes a gallery picture the work's cover and uploads
  it through `publish.Service.PublishCover`, the same call the gallery's
  `publish.cover_publish` RPC makes. Unlike the dialog's
  `publish.sheets_sync`, it sends no sheets. A refused picture or a failed
  upload leaves the site's cover and the writer's choice as they were. All
  three are refused until the writer has connected an account in the app —
  there is deliberately no tool for that, since pairing is approved in the
  writer's browser;
- `linetta_report_continuity`, `linetta_update_continuity`,
  `linetta_propose_story_ops`, `linetta_accept_proposals`, `linetta_reject_proposals`,
  `linetta_save_forecast`: the story-assist Review group — see below.

### World deletes and undo

`linetta_apply_story_ops` deletes entities, relationships, fact cards,
storylines and beats, and each repo's `Removal` reads what its delete will take
into a `store.RowSet` first, because a foreign-key cascade leaves nothing to
undo from. The batch's `undo_batch_id` restores them last-first, after the
outline when the batch also changed it, and a structural batch that rolls back
restores them on the spot. The RowSets sit in the same bounded in-memory window
as an outline undo. A dependent whose other parent has gone since stays gone; a
refusal on the record itself, such as its name having been taken, fails the
undo. An entity's name stays in the prose as a mention node with a dead id,
which `mention.Repo.ResyncForNode` already drops. `entities.delete`, the sheet's
button, has no undo: it asks first instead.

`linetta_propose_story_ops` refuses `remove_aliases` and `remove_attributes`,
because the inbox's card has no words for a removal.

### The Review group: check-and-record tools

These tools share one on-by-default switch, `review_tools_enabled` in
`settings.json` (**Settings → Tool budget → Check & propose**), and
`mcphost.ReviewToolNames`:

- `linetta_report_continuity` (Phase 2) records an agent's reading of one
  scene against Story World, fact cards, earlier scenes and the writer's
  direction — contradictions, gaps, questions. Written straight to
  `continuity_observations`: an observation already IS the review, so
  there is no separate approve step, only the writer's own dismiss/resolve.
- `linetta_list_continuity` and `linetta_update_continuity` are that same
  decision handed over, under the same rule as the inbox tools below: the
  description and the system prompt require the writer's word for it. Update
  dismisses, resolves, reopens or deletes by observation or by whole check. A
  check left with no observations is deleted too, since it would otherwise
  read as a clean check.
- `linetta_propose_story_ops` (Phase 3) and `linetta_save_forecast`
  (Phase 4) are drafts instead: nothing they write ever reaches Story
  World, the outline or the manuscript on its own. A proposal sits in
  `story_proposals`/`story_proposal_items` until it is accepted, which runs
  it through `storyops.ApplyOpsWithRefs`: by the writer's own
  `proposals.accept` click, or by an agent the writer told to, through
  `linetta_accept_proposals`. That tool and `linetta_reject_proposals` are
  the writer's decision handed over, not the agent's own: their descriptions
  and the system prompt require the writer's word for it in the conversation,
  and they reach nothing `linetta_apply_story_ops` does not already reach.
  `linetta_list_proposals` gives them their ids. All three are absent from
  the bounded background passes. A forecast's branch becomes a plan only
  through the writer's own `forecast.select` click, and reaching the
  outline from there is a SECOND draft: `linetta_propose_story_ops
  purpose=forecast`, through the same inbox.

All three verify citations mechanically rather than trusting the model:
`linetta_report_continuity`'s quotes and `linetta_propose_story_ops`'s
evidence are checked against the cited scene's actual numbered paragraphs
(`evidence.VerifyQuote`), and every `*_ref`/`*_id` a call names is checked
against this project's own rows. A ref that is not declared by something
else in the same call and does not resolve to a real id is refused, never
guessed at by name — the one exception by design is `storyops`'s own
apply path (`linetta_apply_story_ops`, and the inside of an already-
accepted batch), which still falls back to a name search.

A successful write fires `mcp.changed` like every other tool (so the
writer's screen refetches), plus its own inbox event when something a
writer clicks, not an agent, changes state: `continuity.changed` when a
status changes, `proposals.changed` when an item is accepted or rejected
(carrying which Story World `kinds` moved, `"outline"` included since
Phase 4's structural accepts). There is no `forecast.changed`: picking,
clearing or deleting a forecast is a direct RPC in the same window that
asked for it, and the brief that shows a chosen branch is rebuilt fresh
every turn anyway.

Visual editing: `linetta_set_character_visuals`, `linetta_set_art_style`,
`linetta_add_reference_image`, `linetta_delete_reference_image` (full mode only).
Each returns an `undo_batch_id` for `linetta_undo_last_change` or the agent undo button.
The optional `visual_write_tools_disabled` setting omits this four-tool group.
Undo retains up to eight changes in memory, including deleted image bytes and metadata;
restarting clears the window. Restoring an image still enforces the eight-image limit.

Scene writes are snapshotted, structural and visual changes return undo batch IDs.
Every tool call is recorded in the activity log and rate limited inside the registration decorator — so a new tool cannot be added
without inheriting the limit.

`web_fetch` (from TARS `pkg/tools`) is still used by the Fact Book to capture a
source URL. It needs no key, and it is not exposed as an MCP tool.

Since 1.2 the engine does link a language-model client, for the built-in
agent. `scripts/validate-story-core-deps.sh` bounds where:
`tars/pkg/agentloop` and `pkg/session` must not appear anywhere in the engine
— the agent's loop is ours, in `internal/agent` — and `tars/pkg/llm` may be
imported only by `internal/provider`, `internal/agent`, and the test-only
`internal/agenttest`. The story core (`storycontext`, `storyops`, `mcphost`,
`rpc/handlers`) must not reach it even transitively: those packages are shared
by every agent, built-in or connected over MCP, and a model client in their
dependency graph would mean one of them could call a model on its own.

## Versioning and builds

Keep all app version surfaces aligned with:

```sh
make bump-version VERSION=0.2.0
```

This updates the desktop `package.json`, Tauri configuration, Cargo metadata, the relevant lockfile entry, and embedded engine diagnostics version.

GitHub Actions workflows:

- `.github/workflows/ci.yml`: runs `make test` on pull requests and pushes to `main`;
- `.github/workflows/build.yml`: builds desktop artifacts on manual dispatch and `v*` tags, and publishes the release from a tag;
- `.github/workflows/release-tag.yml`: when a `chore: release vX.Y.Z` commit lands on `main`, tags it and starts `build.yml` on that tag, so merging the release PR is the whole release step;
- `.github/workflows/release-cache.yml`: keeps the Rust release build cached on `main`, the only ref every tag can read from, when third-party crates change and weekly;
- `.github/workflows/mobile-engine.yml`: verifies the mobile-tagged engine and uploads iOS and Android engine artifacts;
- `.github/workflows/mobile-release.yml`: builds signed Tauri mobile artifacts and can upload them to an existing GitHub release.

## Mac App Store release

The Mac App Store build is sandboxed and built, signed and uploaded from a
maintainer's Mac; no workflow does it. Credentials, certificates and the
provisioning profile live in `~/.linetta/apple/` and are never committed.

### One-time setup

1. `~/.linetta/apple/config.env` defines the App Store Connect API key:
   `APP_STORE_CONNECT_KEY_ID`, `APP_STORE_CONNECT_ISSUER_ID`, `APPLE_TEAM_ID`
   and `AUTH_KEY_PATH` (the `.p8` file beside it).
2. `bash scripts/setup-mas-apple.sh` registers the bundle id, creates the Mac
   App Distribution and Mac Installer Distribution certificates, and writes
   `linetta-mas.provisionprofile`. It creates real Apple resources and Apple
   caps distribution certificates per account, so run it once; the script's
   header describes the manual fallback when the API refuses a certificate.
3. Append the identities `security find-identity -v -p codesigning` reports,
   and the profile path, to `config.env`:
   ```sh
   MAS_APP_IDENTITY="Apple Distribution: <name> (<team id>)"
   MAS_INSTALLER_IDENTITY="Mac Installer Distribution: <name> (<team id>)"
   MAS_PROFILE_PATH="$HOME/.linetta/apple/linetta-mas.provisionprofile"
   ```
4. In App Store Connect, create the macOS app record for bundle id
   `com.devlikebear.linetta`. An upload is matched to it by bundle id.

### Each release

Build, sign and package the installer:

```sh
make release-mas-local
```

The package lands at
`apps/desktop/src-tauri/target/release/bundle/macos/Linetta.pkg`. Then upload it:

```sh
mkdir -p "$HOME/.appstoreconnect/private_keys"
cp "$HOME/.linetta/apple/AuthKey_<key id>.p8" "$HOME/.appstoreconnect/private_keys/"   # once: altool looks here
set -a; . "$HOME/.linetta/apple/config.env"; set +a
PKG="apps/desktop/src-tauri/target/release/bundle/macos/Linetta.pkg"
xcrun altool --validate-app -f "${PKG}" -t macos \
  --apiKey "${APP_STORE_CONNECT_KEY_ID}" --apiIssuer "${APP_STORE_CONNECT_ISSUER_ID}"
xcrun altool --upload-app -f "${PKG}" -t macos \
  --apiKey "${APP_STORE_CONNECT_KEY_ID}" --apiIssuer "${APP_STORE_CONNECT_ISSUER_ID}"
```

- Fix any asset or entitlement error validation reports and rebuild before
  uploading.
- If altool is unavailable, upload the same `.pkg` with Transporter.app using
  the same API key.
- The upload is done when the build appears in App Store Connect under
  TestFlight → Builds, first as "Processing".

## Troubleshooting

- **Engine startup failure:** use the desktop diagnostic screen to retry and copy diagnostics.
- **Embedded engine link failure:** run `cd apps/desktop/src-tauri && cargo build` and inspect the Go archive output from `build.rs`.
- **MCP client cannot connect:** confirm the server is on in Settings, that the port is not taken by something else, and that the token in your client config matches the current one.
- **Backup or Git sync failure:** open Settings and inspect the operation status card for the latest error and timestamp.

## Publishing to Linetta Works

The desktop app can publish episodes to [Linetta Works](https://works.marvin-42.com).
The wire contract is the site's: `docs/api.md` in the linetta-artworks
repository, `api_version` 1. Follow it rather than adjusting the client to
whatever the server happens to do; a contract change is made there first.

- `internal/episode` decides what an episode is (the same rules as
  `apps/desktop/src/lib/outlineEpisode.ts`, which still has its own copy), and
  `export.ExportEpisodeMarkdown` renders one for readers: scenes joined by
  `* * *`, mentions without `@`, note anchors dropped. The content hash is the
  SHA-256 of that Markdown, which the site recomputes.
- `internal/publish` holds the HTTP client, the device-pairing flow
  (`Connector`, shaped like `codexauth.Service`), and `Service`, which merges
  the local episode list with the site's and publishes. `publish_works` and
  `publish_episodes` (migration 0023) are a local copy of what the site said,
  for offline display.
- The token lives in the secret store as `publish.token`, or in
  `<home>/publish-token` (0600) where there is none (Linux); the account's
  non-secret details are `settings.json`'s `publish_account`.
- `publish.*` RPCs are registered by `engineapp/publish_enabled.go`; the
  `mobile` build registers refusing stubs (`publish_disabled.go`) and reports
  `publish_available: false`, which hides the UI.
- Nothing reaches the site before an account is connected:
  `TestPublishMethodsAreRegisteredAndSendNothingUntilConnected` holds that.

For development against a local server, point the engine elsewhere:

```sh
LINETTA_PUBLISH_BASE_URL=http://127.0.0.1:8090 make dev
```

The shell only opens `https://works.marvin-42.com` in the browser
(`EXTERNAL_URL_HOSTS`), so against a local server open the connect URL the
Settings pane shows by hand.

## Plan documents

`docs/plans/<feature>/` holds work orders for work that is still in progress:
a `roadmap.md` and one document per phase. When the work has shipped, delete
its folder in the same pull request that finishes it, or the next one. Git
history keeps every plan, and what is still worth knowing afterwards belongs in
this guide or the README, where it stays current. Logs, audit output and
one-off reports do not go in the repository.
