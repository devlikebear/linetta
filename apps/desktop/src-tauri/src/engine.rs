//! Engine lifecycle: locate the bundled `linetta-engine` binary, spawn it with
//! `--stdio`, and surface a `Client` for the rest of the app to use.

use crate::jsonrpc::{Client, NotificationHandler};
use anyhow::{anyhow, Result};
use serde_json::Value;
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

fn resolve_binary(app: &tauri::AppHandle) -> Result<std::path::PathBuf> {
    // In production: Tauri places externalBin entries in the resource dir,
    // postfixed with the target triple. In dev: scripts/dev.sh symlinks the
    // dev-built engine to apps/desktop/src-tauri/binaries/linetta-engine-{triple}.
    let triple = std::env::var("LINETTA_TARGET_TRIPLE")
        .or_else(|_| current_target_triple())
        .map_err(|e| anyhow!("resolve target triple: {}", e))?;
    let resource_name = format!("linetta-engine-{}{}", triple, std::env::consts::EXE_SUFFIX);

    // 1. Production: Tauri resource dir.
    if let Ok(path) = app
        .path()
        .resolve(&resource_name, tauri::path::BaseDirectory::Resource)
    {
        if path.exists() {
            return Ok(path);
        }
    }

    let mut tried: Vec<std::path::PathBuf> = Vec::new();

    // 2. Dev: walk up from the running exe and look for `binaries/{resource_name}`.
    // In `cargo run` / `pnpm tauri dev`, the exe lives at
    // apps/desktop/src-tauri/target/debug/linetta-desktop, so the engine binary
    // sits two directories above the exe's parent.
    if let Ok(exe) = std::env::current_exe() {
        let mut cursor = exe.parent().map(|p| p.to_path_buf());
        for _ in 0..4 {
            let Some(dir) = cursor else { break };
            let candidate = dir.join("binaries").join(&resource_name);
            if candidate.exists() {
                return Ok(candidate);
            }
            tried.push(candidate);
            cursor = dir.parent().map(|p| p.to_path_buf());
        }
    }

    // 3. Dev: try common cwd locations relative to where the dev script may run.
    if let Ok(cwd) = std::env::current_dir() {
        let candidates = [
            cwd.join("binaries").join(&resource_name),
            cwd.join("src-tauri").join("binaries").join(&resource_name),
            cwd.join("apps")
                .join("desktop")
                .join("src-tauri")
                .join("binaries")
                .join(&resource_name),
        ];
        for candidate in candidates {
            if candidate.exists() {
                return Ok(candidate);
            }
            tried.push(candidate);
        }
    }

    Err(anyhow!(
        "engine binary not found: {} (tried: {})",
        resource_name,
        tried
            .iter()
            .map(|p| p.display().to_string())
            .collect::<Vec<_>>()
            .join(", ")
    ))
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
