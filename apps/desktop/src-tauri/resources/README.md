# Bundled resources

Build outputs that ship inside the app bundle land here. They are produced by
the build, not committed:

- `linetta-mcp` (`linetta-mcp.exe` on Windows) — the stdio MCP bridge that
  Claude Desktop launches. Built by `make build-mcp-bridge`.

This file is committed on purpose. `bundle.resources` globs this directory, and
Tauri treats a glob that matches nothing as a build error — so an empty
directory would break `cargo build` for anyone who has not built the bridge
yet.
