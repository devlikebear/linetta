mod ffi;
mod folder_sync;
#[cfg(all(target_os = "macos", feature = "mas"))]
mod macos_bookmarks;

use serde_json::Value;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;
use tauri::Manager;

const ENGINE_STATUS_TIMEOUT: Duration = Duration::from_secs(2);

// Keep renderer access explicit. New engine methods must be reviewed before the
// Tauri webview can call them.
const RENDERER_ENGINE_METHODS: &[&str] = &[
    "backup.create_recovery",
    "beats.create",
    "beats.delete",
    "beats.list_by_node",
    "beats.list_by_thread",
    "beats.reorder",
    "beats.update",
    "contextual.apply_change",
    "contextual.check_consistency",
    "contextual.plan_change",
    "contextual.resolve_target",
    "diagnostics.get",
    "entities.create",
    "entities.get",
    "entities.list",
    "entities.scenes",
    "entities.search",
    "entities.update",
    "export.companion_history",
    "export.node",
    "export.nodeText",
    "export.project",
    "facts.create",
    "facts.create_from_url",
    "facts.delete",
    "facts.list",
    "facts.update",
    "git_sync.init",
    "git_sync.run",
    "imports.markdown",
    "imports.preview",
    "manuscript.replace_apply",
    "manuscript.replace_preview",
    "manuscript.search",
    "mcp.activity",
    "mcp.disable",
    "mcp.enable",
    "mcp.regenerate_token",
    "mcp.status",
    "mentions.list_for_node",
    "nodes.convert_to_container",
    "nodes.create_child",
    "nodes.create_sibling",
    "nodes.delete",
    "nodes.get",
    "nodes.list_tree",
    "nodes.move_down",
    "nodes.move_to",
    "nodes.move_to_parent",
    "nodes.move_to_root",
    "nodes.move_up",
    "nodes.rename",
    "nodes.restore_outline",
    "nodes.set_last_opened",
    "nodes.set_status",
    "nodes.update_content",
    "notes.create",
    "notes.delete",
    "notes.get",
    "notes.list_for_node",
    "notes.update",
    "ops_status.clear_error",
    "ops_status.get",
    "plot.spine_panel",
    "projects.archive",
    "projects.clear_synopsis",
    "projects.create",
    "projects.delete",
    "projects.get",
    "projects.list",
    "projects.restore",
    "projects.update",
    "relationships.create_one",
    "relationships.create_pair",
    "relationships.delete",
    "relationships.list_by_entity",
    "relationships.update",
    "search.query",
    "settings.get",
    "settings.set",
    "snapshots.compare",
    "snapshots.create_auto",
    "snapshots.create_manual",
    "snapshots.list_for_node",
    "snapshots.restore",
    "stats.range",
    "stats.summary",
    "stats.today",
    "threads.close",
    "threads.create",
    "threads.get",
    "threads.list",
    "threads.reopen",
    "threads.update",
];

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .plugin(tauri_plugin_opener::init())
        .setup(|app| {
            let handle = app.handle().clone();
            let recovery_home = resolve_recovery_home(&handle).ok();
            let state = match mobile_engine_home(&handle)
                .and_then(|home| ffi::Engine::start(&handle, home.as_deref()))
            {
                Ok(engine) => EngineState {
                    engine: Some(Arc::new(engine)),
                    startup_error: None,
                    recovery_home,
                },
                Err(e) => {
                    eprintln!("[linetta] failed to start engine: {e}");
                    EngineState {
                        engine: None,
                        startup_error: Some(e),
                        recovery_home,
                    }
                }
            };
            handle.manage(state);
            #[cfg(all(target_os = "macos", feature = "mas"))]
            {
                let timer_handle = app.handle().clone();
                tauri::async_runtime::spawn(async move {
                    use tauri::Manager;
                    // Let the engine settle after launch, then run daily.
                    tokio::time::sleep(std::time::Duration::from_secs(30)).await;
                    loop {
                        let state = timer_handle.state::<EngineState>();
                        let _ =
                            folder_sync::run_folder_sync_mas(&timer_handle, state.inner()).await;
                        tokio::time::sleep(std::time::Duration::from_secs(86_400)).await;
                    }
                });
            }
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            engine_ping,
            engine_call,
            mcp_bridge_path,
            engine_status,
            open_recovery_folder,
            restore_latest_backup,
            open_path,
            open_external_url,
            folder_sync::set_folder_sync_dir,
            folder_sync::folder_sync_now
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

pub(crate) struct EngineState {
    pub(crate) engine: Option<Arc<ffi::Engine>>,
    pub(crate) startup_error: Option<String>,
    recovery_home: Option<PathBuf>,
}

#[derive(serde::Serialize)]
struct EngineStatus {
    ok: bool,
    error: Option<String>,
    version: Option<String>,
    home: Option<String>,
    db_path: Option<String>,
    migration_version: Option<i64>,
    migration_count: Option<i64>,
}

/// Absolute path to the bundled MCP bridge, or None when it is not present
/// (the Mac App Store build ships without it, and a dev build may not have run
/// the build script yet). The settings pane prints this into the writer's
/// client config, so it must be the real installed path, not a guess.
#[tauri::command]
fn mcp_bridge_path(app: tauri::AppHandle) -> Option<String> {
    let name = if cfg!(target_os = "windows") {
        "linetta-mcp.exe"
    } else {
        "linetta-mcp"
    };
    let candidate = app.path().resource_dir().ok()?.join("resources").join(name);
    if candidate.is_file() {
        return Some(candidate.to_string_lossy().into_owned());
    }
    // Dev builds run from the crate directory rather than a bundle.
    let dev = std::env::current_dir().ok()?.join("src-tauri/resources").join(name);
    dev.is_file().then(|| dev.to_string_lossy().into_owned())
}

#[tauri::command]
async fn engine_ping(state: tauri::State<'_, EngineState>) -> Result<String, String> {
    let engine = engine_handle(state.inner())?;
    let result = call_engine(engine, "ping".to_string(), None).await?;
    result
        .as_str()
        .map(|s| s.to_string())
        .ok_or_else(|| format!("ping result is not a string: {result}"))
}

#[tauri::command]
async fn engine_call(
    state: tauri::State<'_, EngineState>,
    method: String,
    params: Option<Value>,
) -> Result<Value, ffi::EngineCallError> {
    if !is_renderer_engine_method(&method) {
        return Err(ffi::EngineCallError {
            code: Some(-32601),
            message: format!("renderer RPC method is not allowed: {method}"),
            data: None,
            request_id: None,
        });
    }
    let engine = engine_handle(state.inner()).map_err(|message| ffi::EngineCallError {
        code: None,
        message,
        data: None,
        request_id: None,
    })?;
    call_engine_typed(engine, method, params).await
}

#[tauri::command]
async fn engine_status(state: tauri::State<'_, EngineState>) -> Result<EngineStatus, String> {
    let Some(engine) = state.engine.clone() else {
        let home = state
            .recovery_home
            .as_ref()
            .map(|path| path.to_string_lossy().into_owned());
        let db_path = state
            .recovery_home
            .as_ref()
            .map(|path| path.join("library.db").to_string_lossy().into_owned());
        return Ok(EngineStatus {
            ok: false,
            error: Some(
                state
                    .startup_error
                    .clone()
                    .unwrap_or_else(|| "engine unavailable".to_string()),
            ),
            version: None,
            home,
            db_path,
            migration_version: None,
            migration_count: None,
        });
    };
    if let Err(e) = call_engine_with_timeout(
        engine.clone(),
        "ping".to_string(),
        None,
        ENGINE_STATUS_TIMEOUT,
    )
    .await
    {
        return Ok(EngineStatus {
            ok: false,
            error: Some(e.to_string()),
            version: None,
            home: None,
            db_path: None,
            migration_version: None,
            migration_count: None,
        });
    }

    match call_engine_with_timeout(
        engine,
        "diagnostics.version".to_string(),
        None,
        ENGINE_STATUS_TIMEOUT,
    )
    .await
    {
        Ok(v) => Ok(EngineStatus {
            ok: true,
            error: None,
            version: opt_string(&v, "version"),
            home: opt_string(&v, "home"),
            db_path: opt_string(&v, "db_path"),
            migration_version: opt_i64(&v, "migration_version"),
            migration_count: opt_i64(&v, "migration_count"),
        }),
        Err(e) => Ok(EngineStatus {
            ok: true,
            error: Some(format!("diagnostics unavailable: {e}")),
            version: None,
            home: None,
            db_path: None,
            migration_version: None,
            migration_count: None,
        }),
    }
}

#[tauri::command]
async fn open_recovery_folder(
    app: tauri::AppHandle,
    state: tauri::State<'_, EngineState>,
) -> Result<(), String> {
    use tauri_plugin_opener::OpenerExt;
    let home = state
        .recovery_home
        .as_ref()
        .ok_or_else(|| "recovery folder is unavailable".to_string())?;
    let backups = home.join("backups");
    let target = if backups.is_dir() { &backups } else { home };
    app.opener()
        .open_path(target.to_string_lossy(), None::<&str>)
        .map_err(|e| e.to_string())
}

#[derive(serde::Serialize)]
struct RestoreBackupResult {
    backup_path: String,
    quarantined_path: Option<String>,
}

#[tauri::command]
async fn restore_latest_backup(
    state: tauri::State<'_, EngineState>,
) -> Result<RestoreBackupResult, String> {
    if state.engine.is_some() {
        return Err("recovery is only available when the engine failed to start".to_string());
    }
    let home = state
        .recovery_home
        .as_ref()
        .ok_or_else(|| "recovery folder is unavailable".to_string())?;
    let backup = latest_completed_backup(home)?;
    let quarantined = restore_backup_database(home, &backup)?;
    Ok(RestoreBackupResult {
        backup_path: backup.to_string_lossy().into_owned(),
        quarantined_path: quarantined.map(|path| path.to_string_lossy().into_owned()),
    })
}

#[tauri::command]
async fn open_path(
    app: tauri::AppHandle,
    state: tauri::State<'_, EngineState>,
    path: String,
) -> Result<(), String> {
    use tauri_plugin_opener::OpenerExt;
    let home = state
        .recovery_home
        .as_ref()
        .ok_or_else(|| "app data folder is unavailable".to_string())?;
    let path = validate_open_path(Path::new(path.trim()), home)?;
    app.opener()
        .open_path(path.to_string_lossy(), None::<&str>)
        .map_err(|e| e.to_string())
}

/// Hosts the renderer may ask the OS browser to open.
///
/// The renderer hands back a URL the engine produced, so this is a narrow
/// allowlist rather than a general "open anything" primitive, matching the way
/// `open_path` is confined to the app data directory.
const EXTERNAL_URL_HOSTS: [&str; 1] = ["openrouter.ai"];

fn validate_external_url(raw: &str) -> Result<String, String> {
    let parsed = url::Url::parse(raw.trim()).map_err(|e| format!("invalid url: {e}"))?;
    if parsed.scheme() != "https" {
        return Err("only https urls can be opened".to_string());
    }
    // Reject credentials, which can make a hostile host look like an allowed
    // one in the address bar (https://openrouter.ai@example.com/).
    if !parsed.username().is_empty() || parsed.password().is_some() {
        return Err("urls with credentials cannot be opened".to_string());
    }
    let host = parsed.host_str().unwrap_or_default().to_ascii_lowercase();
    let allowed = EXTERNAL_URL_HOSTS
        .iter()
        .any(|candidate| host == *candidate || host.ends_with(&format!(".{candidate}")));
    if !allowed {
        return Err(format!("host is not allowed: {host}"));
    }
    Ok(parsed.to_string())
}

/// Open an external URL in the OS browser.
///
/// `window.open` does not reach the system browser from the webview, so the
/// OAuth flow has to go through the opener plugin on the Rust side.
#[tauri::command]
async fn open_external_url(app: tauri::AppHandle, url: String) -> Result<(), String> {
    use tauri_plugin_opener::OpenerExt;
    let url = validate_external_url(&url)?;
    app.opener()
        .open_url(url, None::<&str>)
        .map_err(|e| e.to_string())
}

fn is_renderer_engine_method(method: &str) -> bool {
    RENDERER_ENGINE_METHODS.binary_search(&method).is_ok()
}

fn validate_open_path(requested: &Path, home: &Path) -> Result<PathBuf, String> {
    if requested.as_os_str().is_empty() {
        return Err("path required".to_string());
    }
    let requested = requested
        .canonicalize()
        .map_err(|e| format!("resolve requested path: {e}"))?;
    let home = home
        .canonicalize()
        .map_err(|e| format!("resolve app data folder: {e}"))?;
    let backups = home.join("backups");
    let backups = backups.canonicalize().ok();
    if requested == home || backups.as_ref() == Some(&requested) {
        return Ok(requested);
    }
    Err("opening this path is not allowed".to_string())
}

pub(crate) fn engine_handle(state: &EngineState) -> Result<Arc<ffi::Engine>, String> {
    state.engine.clone().ok_or_else(|| {
        state
            .startup_error
            .clone()
            .unwrap_or_else(|| "engine unavailable".to_string())
    })
}

pub(crate) async fn call_engine(
    engine: Arc<ffi::Engine>,
    method: String,
    params: Option<Value>,
) -> Result<Value, String> {
    call_engine_typed(engine, method, params)
        .await
        .map_err(|e| e.to_string())
}

async fn call_engine_typed(
    engine: Arc<ffi::Engine>,
    method: String,
    params: Option<Value>,
) -> Result<Value, ffi::EngineCallError> {
    tauri::async_runtime::spawn_blocking(move || engine.call(&method, params))
        .await
        .map_err(|e| ffi::EngineCallError {
            code: None,
            message: e.to_string(),
            data: None,
            request_id: None,
        })?
}

async fn call_engine_with_timeout(
    engine: Arc<ffi::Engine>,
    method: String,
    params: Option<Value>,
    timeout: Duration,
) -> Result<Value, String> {
    match tokio::time::timeout(timeout, call_engine(engine, method, params)).await {
        Ok(result) => result,
        Err(_) => Err(format!("engine timeout after {}ms", timeout.as_millis())),
    }
}

fn opt_string(v: &Value, key: &str) -> Option<String> {
    v.get(key).and_then(|x| x.as_str()).map(|s| s.to_string())
}

fn opt_i64(v: &Value, key: &str) -> Option<i64> {
    v.get(key).and_then(|x| x.as_i64())
}

fn mobile_engine_home(app: &tauri::AppHandle) -> Result<Option<String>, String> {
    #[cfg(any(target_os = "android", target_os = "ios"))]
    {
        let dir = app.path().app_data_dir().map_err(|e| e.to_string())?;
        std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
        Ok(Some(dir.to_string_lossy().into_owned()))
    }
    #[cfg(not(any(target_os = "android", target_os = "ios")))]
    {
        let _ = app;
        Ok(None)
    }
}

fn resolve_recovery_home(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    if let Ok(value) = std::env::var("LINETTA_HOME") {
        if !value.trim().is_empty() {
            return Ok(PathBuf::from(value));
        }
    }
    app.path().app_data_dir().map_err(|e| e.to_string())
}

fn latest_completed_backup(home: &Path) -> Result<PathBuf, String> {
    let root = home.join("backups");
    let directories = std::fs::read_dir(&root)
        .map_err(|e| format!("read backup folder: {e}"))?
        .filter_map(Result::ok)
        .filter(|entry| entry.path().is_dir())
        .collect::<Vec<_>>();
    let mut latest: Option<(u128, PathBuf)> = None;
    for directory in directories {
        let dir = directory.path();
        let marker = match std::fs::read_to_string(dir.join(".complete")) {
            Ok(value) => value,
            Err(_) => continue,
        };
        let (filename, manifest_created_at) = match parse_completed_marker(&marker) {
            Some(value) => value,
            None => continue,
        };
        let candidate_name = Path::new(&filename);
        if candidate_name.file_name().and_then(|name| name.to_str()) != Some(filename.as_str())
            || !matches!(
                candidate_name.extension().and_then(|ext| ext.to_str()),
                Some("db" | "linetta")
            )
        {
            continue;
        }
        let candidate = dir.join(&filename);
        let Ok(metadata) = candidate.metadata() else {
            continue;
        };
        if !metadata.is_file() || metadata.len() == 0 {
            continue;
        }
        let modified = metadata
            .modified()
            .ok()
            .and_then(|time| time.duration_since(std::time::UNIX_EPOCH).ok())
            .map(|duration| duration.as_millis())
            .unwrap_or(0);
        let score = manifest_created_at
            .map(|value| value as u128)
            .unwrap_or(modified);
        if latest
            .as_ref()
            .map(|(best, _)| score > *best)
            .unwrap_or(true)
        {
            latest = Some((score, candidate));
        }
    }
    latest
        .map(|(_, path)| path)
        .ok_or_else(|| "no completed database backup was found".to_string())
}

fn parse_completed_marker(marker: &str) -> Option<(String, Option<u64>)> {
    let trimmed = marker.trim();
    if trimmed.starts_with('{') {
        let value: Value = serde_json::from_str(trimmed).ok()?;
        if value.get("format").and_then(Value::as_str) != Some("linetta-library-backup")
            || value.get("format_version").and_then(Value::as_u64) != Some(1)
        {
            return None;
        }
        return Some((
            value.get("database")?.as_str()?.to_string(),
            value.get("created_at").and_then(Value::as_u64),
        ));
    }
    Some((trimmed.to_string(), None))
}

fn restore_backup_database(home: &Path, backup: &Path) -> Result<Option<PathBuf>, String> {
    let db = home.join("library.db");
    let tmp = home.join("library.db.restore.tmp");
    let _ = std::fs::remove_file(&tmp);
    std::fs::copy(backup, &tmp).map_err(|e| format!("copy recovery database: {e}"))?;
    folder_sync::sync_file(&tmp).map_err(|e| format!("sync recovery database: {e}"))?;

    let timestamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|duration| duration.as_millis())
        .unwrap_or(0);
    let quarantined = if db.exists() {
        let path = home.join(format!("library.db.corrupt-{timestamp}"));
        std::fs::rename(&db, &path).map_err(|e| format!("quarantine current database: {e}"))?;
        Some(path)
    } else {
        None
    };

    for suffix in ["-wal", "-shm"] {
        let sidecar = PathBuf::from(format!("{}{}", db.to_string_lossy(), suffix));
        if sidecar.exists() {
            let quarantine = home.join(format!("library.db{suffix}.corrupt-{timestamp}"));
            if let Err(e) = std::fs::rename(&sidecar, quarantine) {
                if let Some(previous) = &quarantined {
                    let _ = std::fs::rename(previous, &db);
                }
                let _ = std::fs::remove_file(&tmp);
                return Err(format!("quarantine database sidecar: {e}"));
            }
        }
    }

    if let Err(e) = std::fs::rename(&tmp, &db) {
        if let Some(previous) = &quarantined {
            let _ = std::fs::rename(previous, &db);
        }
        let _ = std::fs::remove_file(&tmp);
        return Err(format!("publish recovered database: {e}"));
    }
    Ok(quarantined)
}

#[cfg(test)]
mod recovery_tests {
    use super::{latest_completed_backup, restore_backup_database};

    #[test]
    fn finds_completed_backup_and_quarantines_current_database() {
        let home = tempdir();
        let day = home.join("backups/2026-07-12");
        std::fs::create_dir_all(&day).unwrap();
        let backup = day.join("library-090000.db");
        std::fs::write(&backup, b"recovered database").unwrap();
        std::fs::write(day.join(".complete"), b"library-090000.db\n").unwrap();
        std::fs::write(home.join("library.db"), b"broken database").unwrap();

        let manual = home.join("backups/recovery-20260712-100000.000");
        std::fs::create_dir_all(&manual).unwrap();
        let archive = manual.join("library.linetta");
        std::fs::write(&archive, b"newest recovered database").unwrap();
        std::fs::write(
            manual.join(".complete"),
            br#"{"format":"linetta-library-backup","format_version":1,"database":"library.linetta","created_at":9000000000000}"#,
        )
        .unwrap();

        assert_eq!(latest_completed_backup(&home).unwrap(), archive);
        let quarantined = restore_backup_database(&home, &archive).unwrap().unwrap();
        assert_eq!(
            std::fs::read(home.join("library.db")).unwrap(),
            b"newest recovered database"
        );
        assert_eq!(std::fs::read(quarantined).unwrap(), b"broken database");
    }

    fn tempdir() -> std::path::PathBuf {
        let mut path = std::env::temp_dir();
        path.push(format!(
            "linetta-recovery-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        std::fs::create_dir_all(&path).unwrap();
        path
    }
}

#[cfg(test)]
mod security_boundary_tests {
    use super::{is_renderer_engine_method, validate_external_url, validate_open_path};

    #[test]
    fn external_url_allows_only_https_openrouter() {
        assert!(validate_external_url("https://openrouter.ai/auth?callback=x").is_ok());
        assert!(validate_external_url("https://api.openrouter.ai/v1").is_ok());
        assert!(validate_external_url("  https://openrouter.ai/auth  ").is_ok());

        assert!(validate_external_url("http://openrouter.ai/auth").is_err());
        assert!(validate_external_url("https://example.com/").is_err());
        assert!(validate_external_url("file:///C:/Windows/System32").is_err());
        assert!(validate_external_url("javascript:alert(1)").is_err());
        assert!(validate_external_url("not a url").is_err());
        assert!(validate_external_url("").is_err());
    }

    #[test]
    fn external_url_rejects_a_host_disguised_by_credentials() {
        // https://openrouter.ai@example.com/ resolves to example.com.
        assert!(validate_external_url("https://openrouter.ai@example.com/").is_err());
        assert!(validate_external_url("https://user:pass@openrouter.ai/").is_err());
    }

    #[test]
    fn external_url_rejects_a_lookalike_suffix() {
        assert!(validate_external_url("https://notopenrouter.ai/").is_err());
        assert!(validate_external_url("https://openrouter.ai.evil.test/").is_err());
    }

    #[test]
    fn renderer_rpc_allowlist_rejects_internal_and_unknown_methods() {
        assert!(is_renderer_engine_method("projects.list"));
        assert!(is_renderer_engine_method("export.companion_history"));
        // The companion's own methods are gone; the allowlist must not still
        // be handing them to the renderer.
        assert!(!is_renderer_engine_method("companion.send"));
        assert!(!is_renderer_engine_method("providers.test"));
        assert!(!is_renderer_engine_method("diagnostics.version"));
        assert!(!is_renderer_engine_method("debug.execute"));
    }

    #[test]
    fn open_path_only_allows_the_app_home_and_backup_directory() {
        let root = tempdir();
        let home = root.join("app-data");
        let backups = home.join("backups");
        let sibling = root.join("private");
        std::fs::create_dir_all(&backups).unwrap();
        std::fs::create_dir_all(&sibling).unwrap();

        assert_eq!(
            validate_open_path(&home, &home).unwrap(),
            home.canonicalize().unwrap()
        );
        assert_eq!(
            validate_open_path(&backups, &home).unwrap(),
            backups.canonicalize().unwrap()
        );
        assert!(validate_open_path(&sibling, &home).is_err());
        assert!(validate_open_path(&backups.join("2026-07-12"), &home).is_err());
    }

    fn tempdir() -> std::path::PathBuf {
        let mut path = std::env::temp_dir();
        path.push(format!(
            "linetta-security-boundary-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        std::fs::create_dir_all(&path).unwrap();
        path
    }
}
