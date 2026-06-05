# Linetta

Linetta is a local-first desktop writing app for long-form fiction. The app keeps the writer in a focused Tauri workspace while a bundled Go engine handles SQLite persistence, snapshots, markdown import/export, AI generation, companion chat, background summaries, daily backups, and optional Git sync.

## Stack

- Tauri 2 Rust shell + React 18 + Vite + TypeScript
- Go engine over JSONRPC stdio
- SQLite under the local Linetta data directory
- `github.com/devlikebear/tars` for LLM provider integration

## Install

On macOS (Apple Silicon) install the prebuilt app from the Homebrew tap:

```sh
brew tap devlikebear/tap
brew install --cask linetta
```

The fully qualified `devlikebear/tap/linetta` cask name also works in a single `brew install` without a separate `brew tap` step.

Release casks are built from Developer ID signed and notarized macOS archives. Older casks published before the signing pipeline was enabled, including v0.4.3 and earlier, were ad-hoc signed and can be blocked by macOS Gatekeeper. If macOS reports that an older app is damaged, reinstall the latest cask first:

```sh
brew update
brew reinstall --cask linetta
```

If you are intentionally running one of those older ad-hoc builds, clear the quarantine attribute once:

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

Build the sidecar engine and start the desktop app:

```sh
make dev
```

This wraps `scripts/dev.sh`, which builds the Go engine once and then launches `tauri dev`.

Build only the sidecar engine:

```sh
make build-engine
```

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
```

`make test` runs Go tests, frontend Vitest tests, the Vite production build, and Rust `cargo check`.

## Versioning And Builds

Keep all app version surfaces aligned with:

```sh
make bump-version VERSION=0.2.0
```

This updates the desktop `package.json`, Tauri config, Cargo metadata, lockfile package entry, and Go engine diagnostics version.

GitHub Actions:

- `.github/workflows/ci.yml`: runs `make test` on PRs and pushes to `main`.
- `.github/workflows/build.yml`: builds OS-specific Tauri artifacts on `workflow_dispatch` and `v*` tags for macOS, Linux, and Windows.

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
- Missing sidecar binary: run `make build-engine`, then restart the app.
- AI provider errors: check Settings for the selected provider and confirm the corresponding CLI credentials work in the same shell environment.
- Backup or Git sync failures: open Settings and check the operation status cards for the latest error and timestamp.
