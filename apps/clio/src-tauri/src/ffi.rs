use serde::Serialize;
use serde_json::Value;
use std::ffi::{c_char, c_int, CStr, CString};
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::OnceLock;
use tauri::AppHandle;

type NotifyCallback = extern "C" fn(*const c_char, *const c_char);

mod engine_abi {
    use super::{c_char, c_int, NotifyCallback};

    unsafe extern "C" {
        fn ClioEngineStart(home: *const c_char) -> c_int;
        fn ClioEngineCall(request: *const c_char) -> *mut c_char;
        fn ClioEngineFreeCString(s: *mut c_char);
        fn ClioEngineStop() -> c_int;
        fn ClioEngineSetNotifyCallback(cb: NotifyCallback);
    }

    pub fn set_notify_callback(cb: NotifyCallback) -> Result<(), String> {
        unsafe { ClioEngineSetNotifyCallback(cb) };
        Ok(())
    }

    pub fn start(home: *const c_char) -> Result<c_int, String> {
        Ok(unsafe { ClioEngineStart(home) })
    }

    pub fn call(request: *const c_char) -> Result<*mut c_char, String> {
        Ok(unsafe { ClioEngineCall(request) })
    }

    pub fn free_cstring(s: *mut c_char) -> Result<(), String> {
        unsafe { ClioEngineFreeCString(s) };
        Ok(())
    }

    pub fn stop() -> Result<c_int, String> {
        Ok(unsafe { ClioEngineStop() })
    }
}

static NOTIFY_APP: OnceLock<AppHandle> = OnceLock::new();

extern "C" fn notify_trampoline(method: *const c_char, params: *const c_char) {
    let _ = std::panic::catch_unwind(|| {
        if method.is_null() {
            return;
        }
        let method = unsafe { CStr::from_ptr(method) }
            .to_string_lossy()
            .into_owned();
        let params_str = if params.is_null() {
            "null".to_string()
        } else {
            unsafe { CStr::from_ptr(params) }
                .to_string_lossy()
                .into_owned()
        };
        let payload: Value = match serde_json::from_str(&params_str) {
            Ok(payload) => payload,
            Err(error) => {
                eprintln!("[clio] ignored invalid {method} notification payload: {error}");
                return;
            }
        };
        let Some(event) = notification_event(&method) else {
            eprintln!("[clio] ignored unmapped notification {method}");
            return;
        };
        if let Some(app) = NOTIFY_APP.get() {
            if let Err(error) = tauri::Emitter::emit(app, event, payload) {
                eprintln!("[clio] failed to emit {method} notification: {error}");
            }
        }
    });
}

/// Engine notification name to renderer event name. Phase 1 has no
/// notifications; later phases add their arms here.
fn notification_event(_method: &str) -> Option<&'static str> {
    None
}

#[derive(Serialize)]
struct Request<'a> {
    jsonrpc: &'static str,
    id: i64,
    method: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    params: Option<&'a Value>,
}

pub struct Engine {
    next_id: AtomicI64,
}

#[derive(Debug, Clone, Serialize)]
pub struct EngineCallError {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub code: Option<i64>,
    pub message: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub data: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub request_id: Option<i64>,
}

impl EngineCallError {
    fn transport(message: impl Into<String>, request_id: Option<i64>) -> Self {
        Self {
            code: None,
            message: message.into(),
            data: None,
            request_id,
        }
    }
}

impl std::fmt::Display for EngineCallError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(formatter, "{}", self.message)
    }
}

impl std::error::Error for EngineCallError {}

impl Engine {
    /// Starts the engine with its default home. An empty home makes the Go
    /// side use `paths.HomeFor("com.devlikebear.clio", "CLIO_HOME")`, so
    /// `CLIO_HOME` overrides it and Linetta's home is never touched.
    pub fn start(app: &AppHandle) -> Result<Engine, String> {
        let _ = NOTIFY_APP.set(app.clone());
        engine_abi::set_notify_callback(notify_trampoline)?;
        Self::start_raw("")
    }

    pub fn start_raw(home: &str) -> Result<Engine, String> {
        let c_home = CString::new(home).map_err(|e| e.to_string())?;
        let rc = engine_abi::start(c_home.as_ptr())?;
        if rc != 0 {
            return Err(format!("ClioEngineStart failed (code {rc})"));
        }
        Ok(Engine {
            next_id: AtomicI64::new(1),
        })
    }

    pub fn call(&self, method: &str, params: Option<Value>) -> Result<Value, EngineCallError> {
        let id = self.next_id.fetch_add(1, Ordering::Relaxed);
        let request = Request {
            jsonrpc: "2.0",
            id,
            method,
            params: params.as_ref(),
        };
        let body = serde_json::to_string(&request)
            .map_err(|e| EngineCallError::transport(e.to_string(), Some(id)))?;
        let c_req =
            CString::new(body).map_err(|e| EngineCallError::transport(e.to_string(), Some(id)))?;
        let ptr = engine_abi::call(c_req.as_ptr())
            .map_err(|e| EngineCallError::transport(e, Some(id)))?;
        if ptr.is_null() {
            return Err(EngineCallError::transport("engine returned null", Some(id)));
        }
        let resp_str = unsafe { CStr::from_ptr(ptr) }
            .to_string_lossy()
            .into_owned();
        engine_abi::free_cstring(ptr).map_err(|e| EngineCallError::transport(e, Some(id)))?;
        let resp: Value = serde_json::from_str(&resp_str)
            .map_err(|e| EngineCallError::transport(e.to_string(), Some(id)))?;
        if let Some(err) = resp.get("error") {
            let msg = err
                .get("message")
                .and_then(|m| m.as_str())
                .unwrap_or("engine error");
            return Err(EngineCallError {
                code: err.get("code").and_then(Value::as_i64),
                message: msg.to_string(),
                data: err.get("data").cloned(),
                request_id: resp.get("id").and_then(Value::as_i64).or(Some(id)),
            });
        }
        Ok(resp.get("result").cloned().unwrap_or(Value::Null))
    }
}

impl Drop for Engine {
    fn drop(&mut self) {
        let _ = engine_abi::stop();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{Mutex, OnceLock};

    fn test_lock() -> std::sync::MutexGuard<'static, ()> {
        static LOCK: OnceLock<Mutex<()>> = OnceLock::new();
        LOCK.get_or_init(|| Mutex::new(())).lock().unwrap()
    }

    fn start_in_tmp(tag: &str) -> Engine {
        let tmp = std::env::temp_dir().join(format!("clio-ffi-{tag}-{}", std::process::id()));
        std::fs::create_dir_all(&tmp).unwrap();
        Engine::start_raw(tmp.to_str().unwrap()).expect("start")
    }

    #[test]
    fn ping_round_trips() {
        let _guard = test_lock();
        let eng = start_in_tmp("ping");
        assert_eq!(
            eng.call("ping", None).expect("ping"),
            serde_json::json!("pong")
        );
    }

    #[test]
    fn jsonrpc_error_preserves_code_and_request_id() {
        let _guard = test_lock();
        let eng = start_in_tmp("error");
        let error = eng.call("method.that.does.not.exist", None).unwrap_err();

        assert_eq!(error.code, Some(-32601));
        assert!(error.message.contains("method not found"));
        assert!(error.request_id.is_some());
    }

    #[test]
    fn counts_are_served_from_an_empty_clio_database() {
        let _guard = test_lock();
        let eng = start_in_tmp("counts");
        let counts = eng.call("posts.counts", None).expect("posts.counts");
        assert!(counts.is_array() || counts.is_null(), "unexpected: {counts}");
    }

    #[test]
    fn no_notifications_are_mapped_yet() {
        assert_eq!(notification_event("anything"), None);
    }
}
