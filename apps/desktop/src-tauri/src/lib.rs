mod engine;
mod jsonrpc;

use std::sync::Arc;
use serde_json::Value;
use tauri::Manager;

pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .setup(|app| {
            let handle = app.handle().clone();
            tauri::async_runtime::block_on(async move {
                match engine::spawn(&handle).await {
                    Ok(engine_handle) => {
                        handle.manage(EngineState {
                            client: engine_handle.client.clone(),
                            _engine: Arc::new(engine_handle),
                        });
                    }
                    Err(e) => {
                        eprintln!("[linetta] failed to spawn engine: {e:#}");
                    }
                }
            });
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![engine_ping, engine_call])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

struct EngineState {
    client: Arc<jsonrpc::Client>,
    _engine: Arc<engine::EngineHandle>,
}

#[tauri::command]
async fn engine_ping(state: tauri::State<'_, EngineState>) -> Result<String, String> {
    let result = state
        .client
        .call("ping", None)
        .await
        .map_err(|e| e.to_string())?;
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
    state
        .client
        .call(&method, params)
        .await
        .map_err(|e| e.to_string())
}
