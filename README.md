# Linetta

Linetta is a local-first desktop writing app for long-form fiction. The app keeps the writer in a focused Tauri workspace while a bundled Go engine handles SQLite persistence, snapshots, markdown import/export, AI generation, companion chat, background summaries, daily backups, and optional Git sync.

## Stack

- Tauri 2 Rust shell + React 18 + Vite + TypeScript
- Embedded Go engine through a C ABI, with JSONRPC envelopes kept as the internal request contract
- SQLite under the local Linetta data directory
- `github.com/devlikebear/tars` for LLM provider integration

## Install

On macOS (Apple Silicon) install the prebuilt app from the Homebrew tap:

```sh
brew tap devlikebear/tap
brew install --cask linetta
```

The fully qualified `devlikebear/tap/linetta` cask name also works in a single `brew install` without a separate `brew tap` step.

Linetta is not notarized yet. The Homebrew cask clears the quarantine attribute after install so macOS can launch the app while the project is still a hobby release. If macOS reports that an older app is damaged, reinstall the latest cask first:

```sh
brew update
brew reinstall --cask linetta
```

If you are intentionally running an older cask before the post-install fix, clear the quarantine attribute once:

```sh
xattr -dr com.apple.quarantine "/Applications/Linetta.app"
```

Other platforms (Linux, Windows) and Intel Macs are not yet covered by a prebuilt download — build from source with `make build-desktop` (see below).

## Development

Install dependencies in the desktop app once:

```sh
cd apps/desktop
pnpm install
```

Start the desktop app:

```sh
make dev
```

This wraps `scripts/dev.sh`, which launches `tauri dev`. Cargo links the embedded Go engine through `apps/desktop/src-tauri/build.rs`.

Build the standalone JSONRPC engine for debugging:

```sh
make build-engine
```

The debug binary is written under `engine/bin/` and is not bundled into the Tauri app.

Build the desktop release binary for the current operating system:

```sh
make build-desktop
```

Search is available from the Library search button, the Library menu, the Workspace command palette, and `Cmd+F` / `Ctrl+F` in the Workspace. Results search visible projects by project title, scene label/title, and scene body text.

## AI Companion Tools

The `cmd+j` companion chat runs through the TARS agent loop. When the active provider supports tool calls, Linetta exposes these built-in tools:

- `web_search`: searches the web through Brave Search or Perplexity Sonar.
- `web_fetch`: fetches a URL and returns extracted text with SSRF protection.
- `linetta_apply_ops`: updates Linetta story state directly, including outline, storylines, beats, characters, relationships, places, scenes, summaries, and memories.

Configure `web_search` in Settings under **LLM 도구**. The provider and API key are stored locally in `settings.json`; `web_fetch` does not require a key.

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

`make test` runs Go tests, frontend Vitest tests, the Vite production build, and Rust `cargo check`.
`make test-mobile-engine` runs the Go engine suite with the `mobile` build tag so the iOS/Android-safe stubs stay covered.

Mobile engine artifacts can be built directly when the platform toolchains are installed:

```sh
make build-mobile-engine-ios
make build-mobile-engine-android
```

The iOS target requires Xcode's iOS SDK. The Android target requires `ANDROID_NDK_HOME`.

The ignored Tauri mobile native projects can be regenerated with:

```sh
make mobile-ios-init
make mobile-android-init
```

For local iOS simulator smoke testing, install Xcode's matching iOS simulator
runtime and run:

```sh
make build-mobile-ios-sim
make smoke-mobile-ios-sim
```

For local Android smoke testing, install the Android SDK/NDK and run:

```sh
make build-mobile-android-debug
make build-mobile-android-release-smoke
```

The iOS simulator build target creates a no-sign `.app` bundle and links the embedded Go engine into it. The iOS simulator smoke target also installs and launches the app in an available iPhone simulator, verifies the embedded engine symbols, and checks that `library.db` is created in the simulator app container. The Android debug target creates a Tauri APK and links the embedded Go engine into the Android app. The Android release smoke target creates a temporary local keystore, patches the generated Gradle signing hook, builds both release APK and Play-style AAB artifacts, verifies that both native libraries are packaged, checks APK Signature Scheme v2, and verifies the signed AAB without using real upload credentials. iOS signed app export is handled by the manual mobile release workflow because it depends on Apple team/signing credentials.

## Versioning And Builds

Keep all app version surfaces aligned with:

```sh
make bump-version VERSION=0.2.0
```

This updates the desktop `package.json`, Tauri config, Cargo metadata, lockfile package entry, and embedded engine diagnostics version.

GitHub Actions:

- `.github/workflows/ci.yml`: runs `make test` on PRs and pushes to `main`.
- `.github/workflows/build.yml`: builds OS-specific Tauri artifacts on `workflow_dispatch` and `v*` tags for macOS, Linux, and Windows.
- `.github/workflows/mobile-engine.yml`: verifies the mobile-tagged embedded engine, checks Android debug APK packaging, and uploads iOS/Android engine artifacts.
- `.github/workflows/mobile-release.yml`: manual iOS/Android release path for signed Tauri artifacts; it initializes the ignored mobile projects, applies Android signing wiring, builds `.ipa` / `.aab` / `.apk` artifacts, and can upload them to an existing GitHub Release tag.

## Data And Safety

Linetta stores all writing data locally. Set `LINETTA_HOME` to override the data directory; otherwise the per-OS defaults are:

```text
macOS    ~/Library/Application Support/com.devlikebear.linetta
Linux    ${XDG_DATA_HOME:-~/.local/share}/com.devlikebear.linetta
Windows  %APPDATA%\com.devlikebear.linetta
```

Important files and folders:

- `library.db`: main SQLite database (projects, scenes, entities, and version snapshots)
- `settings.json`: app preferences
- `backups/YYYY-MM-DD/library-HHMMSS.db`: daily full-database backups, kept for 14 days
- `companion/`: companion transcript and memory files

Linetta keeps two layers of history. Daily backups (above) snapshot the whole database; scene-level **version snapshots** live inside `library.db`. Manual and AI-replace snapshots are kept indefinitely, while autosave snapshots are thinned over time (all kept for the first 24 hours, then one per hour up to 30 days, then one per day).

Git sync is optional. When configured in Settings, Linetta exports active projects as markdown into the selected Git repository, then runs `git add`, `git commit`, and `git push` using the system Git credentials.

## Troubleshooting

- Engine startup failure: the desktop shell shows an engine diagnostic screen with retry and copy-diagnostics actions.
- Embedded engine link failure: run `cd apps/desktop/src-tauri && cargo build` and inspect the Go archive build output from `build.rs`.
- AI provider errors: check Settings for the selected provider and confirm the corresponding CLI credentials work in the same shell environment.
- Backup or Git sync failures: open Settings and check the operation status cards for the latest error and timestamp.

## License

Linetta is licensed under the GNU Affero General Public License version 3 only
(`AGPL-3.0-only`). See [LICENSE](LICENSE) and [LICENSE-NOTICE.md](LICENSE-NOTICE.md)
for details, including commercial licensing options.
