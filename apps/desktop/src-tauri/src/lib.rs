mod engine;
mod jsonrpc;

use serde_json::Value;
use std::sync::Arc;
use tauri::Manager;

pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .setup(|app| {
            let handle = app.handle().clone();
            tauri::async_runtime::block_on(async move {
                let state = match engine::spawn(&handle).await {
                    Ok(engine_handle) => EngineState {
                        client: Some(engine_handle.client.clone()),
                        startup_error: None,
                        _engine: Some(Arc::new(engine_handle)),
                    },
                    Err(e) => {
                        let msg = format!("{e:#}");
                        eprintln!("[linetta] failed to spawn engine: {msg}");
                        EngineState {
                            client: None,
                            startup_error: Some(msg),
                            _engine: None,
                        }
                    }
                };
                handle.manage(state);
            });
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            engine_ping,
            engine_call,
            engine_status
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

struct EngineState {
    client: Option<Arc<jsonrpc::Client>>,
    startup_error: Option<String>,
    _engine: Option<Arc<engine::EngineHandle>>,
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
    let client = engine_client(&state)?;
    let result = client.call("ping", None).await.map_err(|e| e.to_string())?;
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
    let client = engine_client(&state)?;
    client
        .call(&method, params)
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
async fn engine_status(state: tauri::State<'_, EngineState>) -> Result<EngineStatus, String> {
    let Some(client) = state.client.clone() else {
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
    match client.call("diagnostics.version", None).await {
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
            ok: false,
            error: Some(e.to_string()),
            version: None,
            home: None,
            db_path: None,
            migration_version: None,
            migration_count: None,
        }),
    }
}

fn engine_client(state: &EngineState) -> Result<Arc<jsonrpc::Client>, String> {
    state.client.clone().ok_or_else(|| {
        state
            .startup_error
            .clone()
            .unwrap_or_else(|| "engine unavailable".to_string())
    })
}

fn opt_string(v: &Value, key: &str) -> Option<String> {
    v.get(key).and_then(|x| x.as_str()).map(|s| s.to_string())
}

fn opt_i64(v: &Value, key: &str) -> Option<i64> {
    v.get(key).and_then(|x| x.as_i64())
}
