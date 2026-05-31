# Linetta

Linetta is a local-first desktop writing app for long-form fiction. The app keeps the writer in a focused Tauri workspace while a bundled Go engine handles SQLite persistence, snapshots, markdown import/export, AI generation, companion chat, background summaries, daily backups, and optional Git sync.

## Stack

- Tauri 2 Rust shell + React 18 + Vite + TypeScript
- Go engine over JSONRPC stdio
- SQLite under the local Linetta data directory
- `github.com/devlikebear/tars` for LLM provider integration

## Development

Install dependencies in the desktop app once:

```sh
cd apps/desktop
pnpm install
```

Build the sidecar engine and start the desktop app:

```sh
./scripts/dev.sh
```

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
make bump-version VERSION=0.1.0
```

This updates the desktop `package.json`, Tauri config, Cargo metadata, lockfile package entry, and Go engine diagnostics version.

GitHub Actions:

- `.github/workflows/ci.yml`: runs `make test` on PRs and pushes to `main`.
- `.github/workflows/build.yml`: builds OS-specific Tauri artifacts on `workflow_dispatch` and `v*` tags for macOS, Linux, and Windows.

## Data And Safety

Linetta stores all writing data locally. Set `LINETTA_HOME` to override the data directory; otherwise the default macOS path is:

```text
~/Library/Application Support/com.devlikebear.linetta
```

Important files and folders:

- `library.db`: main SQLite database
- `settings.json`: app preferences
- `backups/YYYY-MM-DD/library-HHMMSS.db`: daily SQLite backups, kept for 14 days
- `companion/`: companion transcript and memory files

Git sync is optional. When configured in Settings, Linetta exports active projects as markdown into the selected Git repository, then runs `git add`, `git commit`, and `git push` using the system Git credentials.

## Troubleshooting

- Engine startup failure: the desktop shell shows an engine diagnostic screen with retry and copy-diagnostics actions.
- Missing sidecar binary: run `make build-engine`, then restart the app.
- AI provider errors: check Settings for the selected provider and confirm the corresponding CLI credentials work in the same shell environment.
- Backup or Git sync failures: open Settings and check the operation status cards for the latest error and timestamp.
