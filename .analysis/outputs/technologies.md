# 기술 구성

- Desktop shell: Tauri 2 / Rust 2021
- Frontend: React 18, TypeScript, Vite, Tiptap
- Engine: Go 1.26.2 at audited commit
- Persistence: SQLite via `modernc.org/sqlite`, WAL, single connection
- IPC: Tauri invoke + JSON-RPC envelope over in-process Go C ABI
- Distribution: Mac App Store sandbox, Developer ID/notarized direct macOS, Windows/Linux packaging
- Localization: ko baseline, en, ja; single TypeScript catalog
