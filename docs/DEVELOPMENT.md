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

The default tool set has 27 tools (13 read, 14 write). Optional memory, skills,
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
- `linetta_add_reference_image`, `linetta_delete_reference_image`.

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
- `.github/workflows/build.yml`: builds desktop artifacts on manual dispatch and `v*` tags;
- `.github/workflows/mobile-engine.yml`: verifies the mobile-tagged engine and uploads iOS and Android engine artifacts;
- `.github/workflows/mobile-release.yml`: builds signed Tauri mobile artifacts and can upload them to an existing GitHub release.

## Troubleshooting

- **Engine startup failure:** use the desktop diagnostic screen to retry and copy diagnostics.
- **Embedded engine link failure:** run `cd apps/desktop/src-tauri && cargo build` and inspect the Go archive output from `build.rs`.
- **MCP client cannot connect:** confirm the server is on in Settings, that the port is not taken by something else, and that the token in your client config matches the current one.
- **Backup or Git sync failure:** open Settings and inspect the operation status card for the latest error and timestamp.
