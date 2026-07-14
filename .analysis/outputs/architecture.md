# Linetta 아키텍처

```text
React UI → TypeScript rpc.ts → Tauri command → Rust FFI → Go C ABI
         → engineapp/rpc handlers → domain repositories → SQLite
```

주요 경계:

- UI/RPC: `apps/desktop/src/lib/rpc.ts`
- Tauri: `apps/desktop/src-tauri/src/lib.rs`
- FFI: `apps/desktop/src-tauri/src/ffi.rs`, `engine/cmd/linetta-ffi/ffi.go`
- composition root: `engine/internal/engineapp/engineapp.go`
- persistence: `engine/internal/store`, domain repositories
- recovery/export: `engine/internal/backup`, `export`, `importmd`, `foldersync`, `gitsync`

상세 그래프와 이슈는 `full-audit.md`를 참조한다.
