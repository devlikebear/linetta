//! Engine lifecycle: locate the bundled `linetta-engine` binary, spawn it with
//! `--stdio`, and surface a `Client` for the rest of the app to use.

use crate::jsonrpc::{Client, NotificationHandler};
use anyhow::{anyhow, Result};
use serde_json::Value;
use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::sync::Arc;
use tauri::{Emitter, Manager};
use tokio::process::Command;

pub struct EngineHandle {
    pub client: Arc<Client>,
    // Keep the child alive for the duration of the app; dropping it kills the
    // process (Tokio's Drop terminates child if not awaited).
    pub _child: tokio::process::Child,
}

pub async fn spawn(app: &tauri::AppHandle) -> Result<EngineHandle> {
    let binary = resolve_binary(app)?;
    let mut child = Command::new(&binary)
        .arg("--stdio")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::inherit())
        .spawn()
        .map_err(|e| anyhow!("spawn {}: {}", binary.display(), e))?;

    let stdin = child
        .stdin
        .take()
        .ok_or_else(|| anyhow!("child has no stdin"))?;
    let stdout = child
        .stdout
        .take()
        .ok_or_else(|| anyhow!("child has no stdout"))?;

    let handle_clone = app.clone();
    let on_notification: NotificationHandler =
        std::sync::Arc::new(move |method: String, params: Value| {
            // Route ai.* notifications to Tauri events with the same name (with `.` → `-`).
            let event = match method.as_str() {
                "ai.delta" => "ai-delta",
                "ai.reset" => "ai-reset",
                "ai.done" => "ai-done",
                "ai.error" => "ai-error",
                "ai.cancelled" => "ai-cancelled",
                "companion.delta" => "companion-delta",
                "companion.reset" => "companion-reset",
                "companion.done" => "companion-done",
                "companion.error" => "companion-error",
                "companion.cancelled" => "companion-cancelled",
                "companion.proposal" => "companion-proposal",
                "companion.applied" => "companion-applied",
                "companion.thinking" => "companion-thinking",
                "companion.reasoning" => "companion-reasoning",
                _ => return, // ignore unknown
            };
            let _ = handle_clone.emit(event, params);
        });
    let client = Client::new(stdin, stdout, on_notification);
    Ok(EngineHandle {
        client,
        _child: child,
    })
}

fn resolve_binary(app: &tauri::AppHandle) -> Result<PathBuf> {
    let triple = std::env::var("LINETTA_TARGET_TRIPLE")
        .or_else(|_| current_target_triple())
        .map_err(|e| anyhow!("resolve target triple: {}", e))?;
    // Production bundle: Tauri strips the target triple from externalBin entries
    // and places the sidecar next to the main executable (Contents/MacOS on
    // macOS, alongside the binary on Linux/Windows).
    let bundled_name = format!("linetta-engine{}", std::env::consts::EXE_SUFFIX);
    // Dev: scripts/build-engine.sh emits the triple-suffixed binary into
    // apps/desktop/src-tauri/binaries/.
    let dev_name = format!("linetta-engine-{}{}", triple, std::env::consts::EXE_SUFFIX);

    // Tauri resource dir, kept as a fallback for bundle layouts that differ.
    if let Ok(path) = app
        .path()
        .resolve(&bundled_name, tauri::path::BaseDirectory::Resource)
    {
        if path.exists() {
            return Ok(path);
        }
    }

    let exe = std::env::current_exe().ok();
    let exe_dir = exe.as_deref().and_then(Path::parent);
    let cwd = std::env::current_dir().ok();
    let candidates = engine_candidates(exe_dir, cwd.as_deref(), &bundled_name, &dev_name);
    for candidate in &candidates {
        if candidate.exists() {
            return Ok(candidate.clone());
        }
    }

    Err(anyhow!(
        "engine binary not found: {} (tried: {})",
        bundled_name,
        candidates
            .iter()
            .map(|p| p.display().to_string())
            .collect::<Vec<_>>()
            .join(", ")
    ))
}

/// Ordered list of locations to probe for the engine binary, given the running
/// executable's directory and the current working directory.
///
/// `bundled_name` is the triple-stripped sidecar name Tauri ships inside a
/// packaged app, sitting next to the main executable. `dev_name` is the
/// triple-suffixed name `scripts/build-engine.sh` writes into `binaries/` for
/// `tauri dev` and `cargo run`.
fn engine_candidates(
    exe_dir: Option<&Path>,
    cwd: Option<&Path>,
    bundled_name: &str,
    dev_name: &str,
) -> Vec<PathBuf> {
    let mut out = Vec::new();

    if let Some(dir) = exe_dir {
        // Production: the sidecar lives right next to the main executable.
        out.push(dir.join(bundled_name));

        // Dev: walk up from the exe (target/debug/linetta-desktop) looking for
        // a sibling `binaries/{dev_name}`.
        let mut cursor = Some(dir.to_path_buf());
        for _ in 0..4 {
            let Some(d) = cursor else { break };
            out.push(d.join("binaries").join(dev_name));
            cursor = d.parent().map(Path::to_path_buf);
        }
    }

    if let Some(cwd) = cwd {
        out.push(cwd.join("binaries").join(dev_name));
        out.push(cwd.join("src-tauri").join("binaries").join(dev_name));
        out.push(
            cwd.join("apps")
                .join("desktop")
                .join("src-tauri")
                .join("binaries")
                .join(dev_name),
        );
    }

    out
}

fn current_target_triple() -> std::result::Result<String, std::env::VarError> {
    // Best-effort detection without pulling in another crate.
    let arch = if cfg!(target_arch = "aarch64") {
        "aarch64"
    } else {
        "x86_64"
    };
    let os = if cfg!(target_os = "macos") {
        "apple-darwin"
    } else if cfg!(target_os = "linux") {
        "unknown-linux-gnu"
    } else if cfg!(target_os = "windows") {
        "pc-windows-msvc"
    } else {
        return Err(std::env::VarError::NotPresent);
    };
    Ok(format!("{}-{}", arch, os))
}

#[cfg(test)]
mod tests {
    use super::engine_candidates;
    use std::path::PathBuf;

    // Regression: a packaged macOS app puts the sidecar at
    // Contents/MacOS/linetta-engine (triple stripped), next to the main exe.
    // The resolver must probe that location, not only `binaries/` subdirs.
    #[test]
    fn probes_bundled_sidecar_next_to_exe() {
        let exe_dir = PathBuf::from("/Applications/Linetta.app/Contents/MacOS");
        let candidates = engine_candidates(
            Some(&exe_dir),
            None,
            "linetta-engine",
            "linetta-engine-aarch64-apple-darwin",
        );
        assert!(
            candidates.contains(&exe_dir.join("linetta-engine")),
            "bundled sidecar path missing from candidates: {candidates:?}"
        );
    }

    // Dev builds still resolve the triple-suffixed binary under `binaries/`.
    #[test]
    fn probes_dev_binaries_dir() {
        let exe_dir = PathBuf::from("/repo/apps/desktop/src-tauri/target/debug");
        let candidates = engine_candidates(
            Some(&exe_dir),
            None,
            "linetta-engine",
            "linetta-engine-aarch64-apple-darwin",
        );
        assert!(
            candidates.iter().any(|p| p
                .ends_with("binaries/linetta-engine-aarch64-apple-darwin")),
            "dev binaries path missing from candidates: {candidates:?}"
        );
    }
}
