//! Engine lifecycle: locate the bundled `linetta-engine` binary, spawn it with
//! `--stdio`, and surface a `Client` for the rest of the app to use.

use crate::jsonrpc::Client;
use anyhow::{anyhow, Result};
use std::process::Stdio;
use std::sync::Arc;
use tauri::Manager;
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

    let stdin = child.stdin.take().ok_or_else(|| anyhow!("child has no stdin"))?;
    let stdout = child.stdout.take().ok_or_else(|| anyhow!("child has no stdout"))?;

    let client = Client::new(stdin, stdout);
    Ok(EngineHandle { client, _child: child })
}

fn resolve_binary(app: &tauri::AppHandle) -> Result<std::path::PathBuf> {
    // In production: Tauri places externalBin entries in the resource dir,
    // postfixed with the target triple. In dev: scripts/dev.sh symlinks the
    // dev-built engine to apps/desktop/src-tauri/binaries/linetta-engine-{triple}.
    let triple = std::env::var("LINETTA_TARGET_TRIPLE")
        .or_else(|_| current_target_triple())
        .map_err(|e| anyhow!("resolve target triple: {}", e))?;
    let resource_name = format!("linetta-engine-{}{}", triple, std::env::consts::EXE_SUFFIX);

    if let Ok(path) = app.path().resolve(&resource_name, tauri::path::BaseDirectory::Resource) {
        if path.exists() {
            return Ok(path);
        }
    }
    // Dev fallback: alongside the running binary or in src-tauri/binaries/.
    let exe_dir = std::env::current_exe()
        .ok()
        .and_then(|p| p.parent().map(|p| p.to_path_buf()));
    if let Some(dir) = exe_dir {
        let dev_path = dir.join(&resource_name);
        if dev_path.exists() {
            return Ok(dev_path);
        }
    }
    let cwd_path = std::env::current_dir()?
        .join("src-tauri")
        .join("binaries")
        .join(&resource_name);
    if cwd_path.exists() {
        return Ok(cwd_path);
    }
    Err(anyhow!("engine binary not found: {}", resource_name))
}

fn current_target_triple() -> std::result::Result<String, std::env::VarError> {
    // Best-effort detection without pulling in another crate.
    let arch = if cfg!(target_arch = "aarch64") { "aarch64" } else { "x86_64" };
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
