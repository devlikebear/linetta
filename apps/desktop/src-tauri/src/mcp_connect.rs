//! One-click MCP client connection (#69).
//!
//! The writer should not need a terminal or a JSON file to point their agent
//! at Linetta. Each supported client gets a detect + connect pair:
//!
//! - **Claude Code** — runs `claude mcp add -s user linetta -- <bridge>`.
//! - **Claude Desktop / Gemini CLI** — merges a `linetta` entry into the
//!   client's JSON config.
//! - **Codex CLI** — appends an `[mcp_servers.linetta]` table to config.toml.
//!
//! Everything registers the **stdio bridge**, not the HTTP endpoint: the
//! bridge finds the port and token in Linetta's discovery file at runtime, so
//! no token ever lands in another app's config, and the entry survives token
//! regeneration and port changes.
//!
//! Writing another app's config is a trust-sensitive act, so file writers
//! back up the original next to it and never touch anything outside the
//! `linetta` entry they add. The frontend shows exactly what will be written
//! before calling in here.

use serde::Serialize;
use std::path::PathBuf;

#[derive(Serialize, Clone)]
pub struct ClientStatus {
    pub id: String,
    pub installed: bool,
    pub connected: bool,
    pub config_path: Option<String>,
}

#[derive(Serialize)]
pub struct ConnectOutcome {
    pub ok: bool,
    /// "connected" | "already" — how it ended, for the pane's result line.
    pub outcome: String,
    pub config_path: Option<String>,
    pub backup_path: Option<String>,
    pub detail: Option<String>,
}

const CLIENT_IDS: [&str; 4] = ["claude-code", "claude-desktop", "codex", "gemini"];

fn home_dir() -> Option<PathBuf> {
    let var = if cfg!(target_os = "windows") { "USERPROFILE" } else { "HOME" };
    std::env::var(var).ok().filter(|v| !v.is_empty()).map(PathBuf::from)
}

/// The virtualized Claude config dir inside an MSIX package, when the
/// Microsoft Store build is installed. `packages_root` is
/// %LOCALAPPDATA%\Packages.
fn msix_claude_dir(packages_root: &std::path::Path) -> Option<PathBuf> {
    for entry in std::fs::read_dir(packages_root).ok()?.flatten() {
        let name = entry.file_name();
        if !name.to_string_lossy().starts_with("Claude_") {
            continue;
        }
        let dir = entry.path().join("LocalCache").join("Roaming").join("Claude");
        if dir.is_dir() {
            return Some(dir);
        }
    }
    None
}

/// Claude Desktop's config file, whether or not it exists yet. None when the
/// app itself is not installed (no Claude directory).
fn claude_desktop_config() -> Option<PathBuf> {
    let dir = if cfg!(target_os = "windows") {
        // The Store build of Claude runs in an MSIX container whose AppData is
        // virtualized: the app reads its config through
        // Packages\Claude_*\LocalCache\Roaming\Claude, never %APPDATA%\Claude.
        // Writing the plain path on such a machine would report success while
        // the app never sees the entry — the silent-no-op class this feature
        // exists to kill. Prefer the container's view when it exists.
        let msix = std::env::var("LOCALAPPDATA")
            .ok()
            .filter(|v| !v.is_empty())
            .map(|l| PathBuf::from(l).join("Packages"))
            .and_then(|p| msix_claude_dir(&p));
        match msix {
            Some(dir) => dir,
            None => PathBuf::from(std::env::var("APPDATA").ok()?).join("Claude"),
        }
    } else if cfg!(target_os = "macos") {
        home_dir()?.join("Library/Application Support/Claude")
    } else {
        match std::env::var("XDG_CONFIG_HOME") {
            Ok(x) if !x.is_empty() => PathBuf::from(x).join("Claude"),
            _ => home_dir()?.join(".config/Claude"),
        }
    };
    dir.is_dir().then(|| dir.join("claude_desktop_config.json"))
}

fn codex_config() -> Option<PathBuf> {
    let dir = home_dir()?.join(".codex");
    dir.is_dir().then(|| dir.join("config.toml"))
}

fn gemini_config() -> Option<PathBuf> {
    let dir = home_dir()?.join(".gemini");
    dir.is_dir().then(|| dir.join("settings.json"))
}

/// Full path of the `claude` CLI, or None when it is not on PATH. Resolved
/// rather than trusted so Windows can route .cmd shims through cmd.exe.
fn claude_cli_path() -> Option<PathBuf> {
    let path = std::env::var_os("PATH")?;
    let exts: &[&str] = if cfg!(target_os = "windows") {
        &["claude.exe", "claude.cmd", "claude.bat"]
    } else {
        &["claude"]
    };
    for dir in std::env::split_paths(&path) {
        for name in exts {
            let candidate = dir.join(name);
            if candidate.is_file() {
                return Some(candidate);
            }
        }
    }
    None
}

fn file_mentions_linetta(path: &PathBuf, needle: &str) -> bool {
    std::fs::read_to_string(path).map(|s| s.contains(needle)).unwrap_or(false)
}

fn status_of(id: &str) -> ClientStatus {
    let (installed, connected, config_path) = match id {
        "claude-code" => {
            // Whether linetta is already registered would need `claude mcp
            // list` (seconds of node startup); the add command reports
            // "already exists" cheaply, so connected stays unknown-false.
            (claude_cli_path().is_some(), false, None)
        }
        "claude-desktop" => match claude_desktop_config() {
            Some(p) => (true, file_mentions_linetta(&p, "\"linetta\""), Some(p)),
            None => (false, false, None),
        },
        "codex" => match codex_config() {
            Some(p) => (true, file_mentions_linetta(&p, "[mcp_servers.linetta]"), Some(p)),
            None => (false, false, None),
        },
        "gemini" => match gemini_config() {
            Some(p) => (true, file_mentions_linetta(&p, "\"linetta\""), Some(p)),
            None => (false, false, None),
        },
        _ => (false, false, None),
    };
    ClientStatus {
        id: id.to_string(),
        installed,
        connected,
        config_path: config_path.map(|p| p.to_string_lossy().into_owned()),
    }
}

#[tauri::command]
pub fn mcp_client_status() -> Vec<ClientStatus> {
    CLIENT_IDS.iter().map(|id| status_of(id)).collect()
}

// ---------- pure config transforms (unit-tested below) ----------

/// Merge a `linetta` stdio entry into a JSON config's `mcpServers` map,
/// preserving everything else byte-for-byte semantically. Empty or missing
/// input starts a fresh object. Errors on a file that parses but is not an
/// object — overwriting someone's broken config would destroy their repair
/// material.
fn merge_json_mcp_servers(content: &str, bridge: &str) -> Result<String, String> {
    let mut root: serde_json::Value = if content.trim().is_empty() {
        serde_json::json!({})
    } else {
        serde_json::from_str(content).map_err(|e| format!("config is not valid JSON: {e}"))?
    };
    let obj = root.as_object_mut().ok_or("config root is not a JSON object")?;
    let servers = obj
        .entry("mcpServers")
        .or_insert_with(|| serde_json::json!({}));
    let servers = servers
        .as_object_mut()
        .ok_or("mcpServers is not a JSON object")?;
    servers.insert(
        "linetta".into(),
        serde_json::json!({ "command": bridge, "args": [] }),
    );
    serde_json::to_string_pretty(&root).map_err(|e| e.to_string())
}

/// Append an `[mcp_servers.linetta]` table to a Codex config. Returns None
/// when the table is already there. Appending (rather than parsing TOML)
/// cannot disturb what the writer already has; the path uses a TOML literal
/// string so Windows backslashes need no escaping.
fn append_codex_server(content: &str, bridge: &str) -> Option<String> {
    if content.contains("[mcp_servers.linetta]") {
        return None;
    }
    let mut out = content.to_string();
    if !out.is_empty() && !out.ends_with('\n') {
        out.push('\n');
    }
    if !out.is_empty() {
        out.push('\n');
    }
    out.push_str(&format!("[mcp_servers.linetta]\ncommand = '{bridge}'\nargs = []\n"));
    Some(out)
}

// ---------- effectful connect ----------

/// Back up `path` beside itself before the first write. A missing original
/// needs no backup. Returns the backup path when one was made.
fn back_up(path: &PathBuf) -> Result<Option<String>, String> {
    if !path.is_file() {
        return Ok(None);
    }
    let backup = path.with_extension(format!(
        "{}bak-linetta",
        path.extension().map(|e| format!("{}.", e.to_string_lossy())).unwrap_or_default()
    ));
    std::fs::copy(path, &backup).map_err(|e| format!("could not back up config: {e}"))?;
    Ok(Some(backup.to_string_lossy().into_owned()))
}

fn write_config(path: &PathBuf, content: &str) -> Result<(), String> {
    std::fs::write(path, content).map_err(|e| format!("could not write config: {e}"))
}

fn connect_json_client(path: PathBuf, bridge: &str) -> Result<ConnectOutcome, String> {
    let existing = std::fs::read_to_string(&path).unwrap_or_default();
    if existing.contains("\"linetta\"") {
        return Ok(ConnectOutcome {
            ok: true,
            outcome: "already".into(),
            config_path: Some(path.to_string_lossy().into_owned()),
            backup_path: None,
            detail: None,
        });
    }
    let merged = merge_json_mcp_servers(&existing, bridge)?;
    let backup_path = back_up(&path)?;
    write_config(&path, &merged)?;
    Ok(ConnectOutcome {
        ok: true,
        outcome: "connected".into(),
        config_path: Some(path.to_string_lossy().into_owned()),
        backup_path,
        detail: None,
    })
}

fn connect_codex(path: PathBuf, bridge: &str) -> Result<ConnectOutcome, String> {
    let existing = std::fs::read_to_string(&path).unwrap_or_default();
    match append_codex_server(&existing, bridge) {
        None => Ok(ConnectOutcome {
            ok: true,
            outcome: "already".into(),
            config_path: Some(path.to_string_lossy().into_owned()),
            backup_path: None,
            detail: None,
        }),
        Some(next) => {
            let backup_path = back_up(&path)?;
            write_config(&path, &next)?;
            Ok(ConnectOutcome {
                ok: true,
                outcome: "connected".into(),
                config_path: Some(path.to_string_lossy().into_owned()),
                backup_path,
                detail: None,
            })
        }
    }
}

fn connect_claude_code(bridge: &str) -> Result<ConnectOutcome, String> {
    let cli = claude_cli_path().ok_or("claude CLI is not on PATH")?;
    let cli_str = cli.to_string_lossy().into_owned();
    // User scope: the registration must work from any directory, not just
    // wherever this GUI process happens to have its cwd.
    let args = ["mcp", "add", "-s", "user", "linetta", "--", bridge];

    let mut cmd = if cfg!(target_os = "windows")
        && (cli_str.to_lowercase().ends_with(".cmd") || cli_str.to_lowercase().ends_with(".bat"))
    {
        // cmd shims cannot be spawned directly.
        let mut c = std::process::Command::new("cmd");
        c.arg("/C").arg(&cli_str).args(args);
        c
    } else {
        let mut c = std::process::Command::new(&cli);
        c.args(args);
        c
    };

    #[cfg(target_os = "windows")]
    {
        use std::os::windows::process::CommandExt;
        // CREATE_NO_WINDOW: no console flash over the writer's workspace.
        cmd.creation_flags(0x0800_0000);
    }

    let output = cmd
        .output()
        .map_err(|e| format!("could not run claude: {e}"))?;
    let stderr = String::from_utf8_lossy(&output.stderr);
    let stdout = String::from_utf8_lossy(&output.stdout);
    if output.status.success() {
        return Ok(ConnectOutcome {
            ok: true,
            outcome: "connected".into(),
            config_path: None,
            backup_path: None,
            detail: None,
        });
    }
    let combined = format!("{stdout}{stderr}");
    if combined.contains("already exists") {
        return Ok(ConnectOutcome {
            ok: true,
            outcome: "already".into(),
            config_path: None,
            backup_path: None,
            detail: None,
        });
    }
    Err(combined.trim().to_string())
}

#[tauri::command]
pub async fn mcp_connect_client(
    app: tauri::AppHandle,
    client: String,
) -> Result<ConnectOutcome, String> {
    let bridge = crate::resolve_bridge_path(&app)
        .ok_or("this build ships no bridge; use manual setup instead")?;
    tauri::async_runtime::spawn_blocking(move || match client.as_str() {
        "claude-code" => connect_claude_code(&bridge),
        "claude-desktop" => {
            let path = claude_desktop_config().ok_or("Claude Desktop is not installed")?;
            connect_json_client(path, &bridge)
        }
        "codex" => {
            let path = codex_config().ok_or("Codex CLI is not installed")?;
            connect_codex(path, &bridge)
        }
        "gemini" => {
            let path = gemini_config().ok_or("Gemini CLI is not installed")?;
            connect_json_client(path, &bridge)
        }
        other => Err(format!("unknown client: {other}")),
    })
    .await
    .map_err(|e| e.to_string())?
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn merging_into_an_empty_config_creates_the_whole_shape() {
        let out = merge_json_mcp_servers("", "/opt/linetta-mcp").unwrap();
        let v: serde_json::Value = serde_json::from_str(&out).unwrap();
        assert_eq!(v["mcpServers"]["linetta"]["command"], "/opt/linetta-mcp");
        assert!(v["mcpServers"]["linetta"]["args"].as_array().unwrap().is_empty());
    }

    #[test]
    fn merging_preserves_every_existing_key_and_sibling_server() {
        let existing = r#"{
            "theme": "dark",
            "mcpServers": { "other": { "command": "keepme", "args": ["-x"] } }
        }"#;
        let out = merge_json_mcp_servers(existing, "C:\\Linetta\\linetta-mcp.exe").unwrap();
        let v: serde_json::Value = serde_json::from_str(&out).unwrap();
        assert_eq!(v["theme"], "dark");
        assert_eq!(v["mcpServers"]["other"]["command"], "keepme");
        assert_eq!(v["mcpServers"]["linetta"]["command"], "C:\\Linetta\\linetta-mcp.exe");
    }

    #[test]
    fn a_broken_config_is_refused_rather_than_replaced() {
        // Half a config is the writer's repair material; never clobber it.
        assert!(merge_json_mcp_servers("{ not json", "/b").is_err());
        assert!(merge_json_mcp_servers("[1,2]", "/b").is_err());
    }

    #[test]
    fn codex_append_writes_a_literal_string_windows_path() {
        let out = append_codex_server("model = \"o5\"\n", "C:\\Linetta\\linetta-mcp.exe").unwrap();
        assert!(out.starts_with("model = \"o5\"\n"));
        // Literal (single-quoted) TOML string: backslashes stay as-is.
        assert!(out.contains("command = 'C:\\Linetta\\linetta-mcp.exe'"));
        assert!(out.contains("[mcp_servers.linetta]"));
    }

    #[test]
    fn codex_append_is_idempotent() {
        let once = append_codex_server("", "/opt/bridge").unwrap();
        assert!(append_codex_server(&once, "/opt/bridge").is_none());
    }

    #[test]
    fn msix_claude_dir_finds_the_containers_view_and_ignores_other_packages() {
        let root = std::env::temp_dir().join(format!("linetta-msix-test-{}", std::process::id()));
        let claude = root.join("Claude_abc123").join("LocalCache").join("Roaming").join("Claude");
        let other = root.join("Other_pkg").join("LocalCache").join("Roaming").join("Claude");
        std::fs::create_dir_all(&claude).unwrap();
        std::fs::create_dir_all(&other).unwrap();

        assert_eq!(msix_claude_dir(&root), Some(claude));
        // A packages root with no Claude container yields nothing, so the
        // caller falls back to the plain %APPDATA% path.
        assert_eq!(msix_claude_dir(&root.join("Other_pkg")), None);

        std::fs::remove_dir_all(&root).ok();
    }

    #[test]
    fn codex_append_separates_itself_from_a_file_without_trailing_newline() {
        let out = append_codex_server("model = \"o5\"", "/opt/bridge").unwrap();
        // The new table must not glue onto the previous line.
        assert!(out.contains("model = \"o5\"\n\n[mcp_servers.linetta]"));
    }
}
