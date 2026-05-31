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
