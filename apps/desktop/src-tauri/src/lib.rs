mod ffi;
mod folder_sync;
#[cfg(all(target_os = "macos", feature = "mas"))]
mod macos_bookmarks;

use serde_json::Value;
use std::sync::Arc;
use std::time::Duration;
use tauri::Manager;

const ENGINE_STATUS_TIMEOUT: Duration = Duration::from_secs(2);

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .plugin(tauri_plugin_opener::init())
        .setup(|app| {
            let handle = app.handle().clone();
            let state = match mobile_engine_home(&handle)
                .and_then(|home| ffi::Engine::start(&handle, home.as_deref()))
            {
                Ok(engine) => EngineState {
                    engine: Some(Arc::new(engine)),
                    startup_error: None,
                },
                Err(e) => {
                    eprintln!("[linetta] failed to start engine: {e}");
                    EngineState {
                        engine: None,
                        startup_error: Some(e),
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
            engine_status,
            open_path,
            folder_sync::set_folder_sync_dir,
            folder_sync::folder_sync_now
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

pub(crate) struct EngineState {
    pub(crate) engine: Option<Arc<ffi::Engine>>,
    pub(crate) startup_error: Option<String>,
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
) -> Result<Value, String> {
    let engine = engine_handle(state.inner())?;
    call_engine(engine, method, params).await
}

#[tauri::command]
async fn engine_status(state: tauri::State<'_, EngineState>) -> Result<EngineStatus, String> {
    let Some(engine) = state.engine.clone() else {
        return Ok(EngineStatus {
            ok: false,
            error: Some(
                state
                    .startup_error
                    .clone()
                    .unwrap_or_else(|| "engine unavailable".to_string()),
            ),
            version: None,
            home: None,
            db_path: None,
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
async fn open_path(app: tauri::AppHandle, path: String) -> Result<(), String> {
    use tauri_plugin_opener::OpenerExt;
    let path = path.trim();
    if path.is_empty() {
        return Err("path required".to_string());
    }
    app.opener()
        .open_path(path, None::<&str>)
        .map_err(|e| e.to_string())
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
    tauri::async_runtime::spawn_blocking(move || engine.call(&method, params))
        .await
        .map_err(|e| e.to_string())?
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
