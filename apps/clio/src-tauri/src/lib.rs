mod ffi;

use serde_json::Value;
use std::sync::Arc;
use std::time::Duration;

const ENGINE_STATUS_TIMEOUT: Duration = Duration::from_secs(2);

// Keep renderer access explicit. New engine methods must be reviewed before the
// Tauri webview can call them. Keep this list sorted: it is binary-searched.
const RENDERER_ENGINE_METHODS: &[&str] = &[
    "ping",
    "posts.counts",
    "posts.import",
    "posts.import_preview",
    "posts.list",
    "voice.profile",
];

pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .setup(|app| {
            let handle = app.handle().clone();
            let state = match ffi::Engine::start(&handle) {
                Ok(engine) => EngineState {
                    engine: Some(Arc::new(engine)),
                    startup_error: None,
                },
                Err(e) => {
                    eprintln!("[clio] failed to start engine: {e}");
                    EngineState {
                        engine: None,
                        startup_error: Some(e),
                    }
                }
            };
            tauri::Manager::manage(&handle, state);
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            engine_ping,
            engine_call,
            engine_status,
            open_external_url
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
        return Ok(EngineStatus {
            ok: false,
            error: Some(
                state
                    .startup_error
                    .clone()
                    .unwrap_or_else(|| "engine unavailable".to_string()),
            ),
        });
    };
    match call_engine_with_timeout(engine, "ping".to_string(), None, ENGINE_STATUS_TIMEOUT).await {
        Ok(_) => Ok(EngineStatus {
            ok: true,
            error: None,
        }),
        Err(e) => Ok(EngineStatus {
            ok: false,
            error: Some(e),
        }),
    }
}

// Hosts the renderer may open in the OS browser. Each is an exact match unless
// the flag is true; every subdomain admitted is another name a mistake could
// hide behind.
const EXTERNAL_URL_HOSTS: [(&str, bool); 2] = [("x.com", false), ("www.threads.com", false)];

fn validate_external_url(raw: &str) -> Result<String, String> {
    let parsed = url::Url::parse(raw.trim()).map_err(|e| format!("invalid url: {e}"))?;
    if parsed.scheme() != "https" {
        return Err("only https urls can be opened".to_string());
    }
    // Reject credentials, which can make a hostile host look like an allowed
    // one in the address bar (https://x.com@example.com/).
    if !parsed.username().is_empty() || parsed.password().is_some() {
        return Err("urls with credentials cannot be opened".to_string());
    }
    let host = parsed.host_str().unwrap_or_default().to_ascii_lowercase();
    let allowed = EXTERNAL_URL_HOSTS.iter().any(|(candidate, with_subdomains)| {
        host == *candidate || (*with_subdomains && host.ends_with(&format!(".{candidate}")))
    });
    if !allowed {
        return Err(format!("host is not allowed: {host}"));
    }
    Ok(parsed.to_string())
}

/// Open an external URL in the OS browser. `window.open` does not reach the
/// system browser from the webview, so it goes through the opener plugin.
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

pub(crate) fn engine_handle(state: &EngineState) -> Result<Arc<ffi::Engine>, String> {
    state.engine.clone().ok_or_else(|| {
        state
            .startup_error
            .clone()
            .unwrap_or_else(|| "engine unavailable".to_string())
    })
}

async fn call_engine(
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn renderer_rpc_allowlist_is_sorted() {
        // is_renderer_engine_method binary-searches this list. An entry out
        // of order is unreachable, and binary_search on unsorted data does
        // not fail loudly: it just sometimes returns Err for a method that
        // is actually present, which is a silent refusal the UI cannot
        // tell apart from a missing feature.
        assert!(
            RENDERER_ENGINE_METHODS.windows(2).all(|pair| pair[0] <= pair[1]),
            "RENDERER_ENGINE_METHODS is not sorted"
        );
    }

    #[test]
    fn clio_methods_are_reachable_from_the_renderer() {
        for method in [
            "ping",
            "posts.counts",
            "posts.import",
            "posts.import_preview",
            "posts.list",
            "voice.profile",
        ] {
            assert!(is_renderer_engine_method(method), "{method}");
        }
    }

    #[test]
    fn other_methods_are_refused() {
        assert!(!is_renderer_engine_method("projects.list"));
        assert!(!is_renderer_engine_method("debug.execute"));
        assert!(!is_renderer_engine_method("posts"));
        assert!(!is_renderer_engine_method(""));
    }

    #[test]
    fn external_urls_are_limited_to_the_allowlist() {
        assert!(validate_external_url("https://x.com/intent/post?text=hi").is_ok());
        assert!(validate_external_url("https://www.threads.com/intent/post").is_ok());
        assert!(validate_external_url("http://x.com/").is_err());
        assert!(validate_external_url("https://evil.example/").is_err());
        assert!(validate_external_url("https://x.com@evil.example/").is_err());
        assert!(validate_external_url("https://sub.x.com/").is_err());
    }
}
