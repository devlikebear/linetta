# Linetta development guide

This guide collects contributor-facing architecture, build, release, and troubleshooting details. For the product overview and downloads, see the [main README](../README.md).

## Architecture

- Tauri 2 Rust shell with React 18, Vite, and TypeScript
- Embedded Go engine linked through a C ABI
- JSON-RPC envelopes as the internal request contract
- SQLite under the local Linetta data directory
- [`github.com/devlikebear/tars`](https://github.com/devlikebear/tars) for LLM provider integration

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

For local Android smoke testing, install the Android SDK and NDK and run:

```sh
make build-mobile-android-debug
make build-mobile-android-release-smoke
```

The iOS simulator target creates a no-sign `.app`, links the embedded Go engine, installs and launches it, verifies engine symbols, and checks that `library.db` is created. The Android release smoke target builds APK and AAB artifacts, verifies packaged native libraries and signing, and avoids using real upload credentials.

Signed iOS export is handled by the manual mobile release workflow because it depends on Apple team and signing credentials.

## AI companion tools

The `Cmd/Ctrl+J` companion runs through the TARS agent loop. When the active provider supports tool calls, Linetta exposes these built-in tools:

- `web_search`: searches the web through Brave Search or Perplexity Sonar;
- `web_fetch`: fetches a URL and returns extracted text with SSRF protection;
- `linetta_apply_ops`: updates story state, including outlines, storylines, beats, characters, relationships, places, scenes, summaries, and memories.

Configure `web_search` in Settings under **LLM tools**. Provider credentials are stored locally; `web_fetch` does not require a key.

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
- **AI provider error:** confirm the provider configuration in Settings and verify that its credentials work in the same shell environment.
- **Backup or Git sync failure:** open Settings and inspect the operation status card for the latest error and timestamp.
