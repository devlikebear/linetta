use serde::Serialize;
use serde_json::Value;
use std::ffi::{c_char, c_int, CStr, CString};
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::OnceLock;
use tauri::{AppHandle, Emitter};

type NotifyCallback = extern "C" fn(*const c_char, *const c_char);

unsafe extern "C" {
    fn LinettaEngineStart(home: *const c_char) -> c_int;
    fn LinettaEngineCall(request: *const c_char) -> *mut c_char;
    fn LinettaEngineFreeCString(s: *mut c_char);
    fn LinettaEngineStop() -> c_int;
    fn LinettaEngineSetNotifyCallback(cb: NotifyCallback);
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
        let payload: Value = serde_json::from_str(&params_str).unwrap_or(Value::Null);
        let Some(event) = notification_event(&method) else {
            return;
        };
        if let Some(app) = NOTIFY_APP.get() {
            let _ = app.emit(event, payload);
        }
    });
}

fn notification_event(method: &str) -> Option<&'static str> {
    match method {
        "ai.delta" => Some("ai-delta"),
        "ai.reset" => Some("ai-reset"),
        "ai.done" => Some("ai-done"),
        "ai.error" => Some("ai-error"),
        "ai.cancelled" => Some("ai-cancelled"),
        "companion.delta" => Some("companion-delta"),
        "companion.reset" => Some("companion-reset"),
        "companion.done" => Some("companion-done"),
        "companion.error" => Some("companion-error"),
        "companion.cancelled" => Some("companion-cancelled"),
        "companion.proposal" => Some("companion-proposal"),
        "companion.choices" => Some("companion-choices"),
        "companion.applied" => Some("companion-applied"),
        "companion.thinking" => Some("companion-thinking"),
        "companion.reasoning" => Some("companion-reasoning"),
        _ => None,
    }
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

impl Engine {
    pub fn start(app: &AppHandle, home: Option<&str>) -> Result<Engine, String> {
        let _ = NOTIFY_APP.set(app.clone());
        unsafe { LinettaEngineSetNotifyCallback(notify_trampoline) };
        if let Some(home) = home.filter(|home| !home.is_empty()) {
            std::env::set_var("LINETTA_HOME", home);
        }
        Self::start_raw(home.unwrap_or(""))
    }

    pub fn start_raw(home: &str) -> Result<Engine, String> {
        let c_home = CString::new(home).map_err(|e| e.to_string())?;
        let rc = unsafe { LinettaEngineStart(c_home.as_ptr()) };
        if rc != 0 {
            return Err(format!("LinettaEngineStart failed (code {rc})"));
        }
        Ok(Engine {
            next_id: AtomicI64::new(1),
        })
    }

    pub fn call(&self, method: &str, params: Option<Value>) -> Result<Value, String> {
        let id = self.next_id.fetch_add(1, Ordering::Relaxed);
        let request = Request {
            jsonrpc: "2.0",
            id,
            method,
            params: params.as_ref(),
        };
        let body = serde_json::to_string(&request).map_err(|e| e.to_string())?;
        let c_req = CString::new(body).map_err(|e| e.to_string())?;
        let ptr = unsafe { LinettaEngineCall(c_req.as_ptr()) };
        if ptr.is_null() {
            return Err("engine returned null".to_string());
        }
        let resp_str = unsafe { CStr::from_ptr(ptr) }
            .to_string_lossy()
            .into_owned();
        unsafe { LinettaEngineFreeCString(ptr) };
        let resp: Value = serde_json::from_str(&resp_str).map_err(|e| e.to_string())?;
        if let Some(err) = resp.get("error") {
            let msg = err
                .get("message")
                .and_then(|m| m.as_str())
                .unwrap_or("engine error");
            return Err(msg.to_string());
        }
        Ok(resp.get("result").cloned().unwrap_or(Value::Null))
    }
}

impl Drop for Engine {
    fn drop(&mut self) {
        unsafe { LinettaEngineStop() };
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

    #[test]
    fn ping_round_trips() {
        let _guard = test_lock();
        let tmp = std::env::temp_dir().join(format!("linetta-ffi-test-{}", std::process::id()));
        std::fs::create_dir_all(&tmp).unwrap();
        let eng = Engine::start_raw(tmp.to_str().unwrap()).expect("start");
        assert_eq!(
            eng.call("ping", None).expect("ping"),
            serde_json::json!("pong")
        );
    }

    #[test]
    fn diagnostics_version_present() {
        let _guard = test_lock();
        let tmp = std::env::temp_dir().join(format!("linetta-ffi-ver-{}", std::process::id()));
        std::fs::create_dir_all(&tmp).unwrap();
        let eng = Engine::start_raw(tmp.to_str().unwrap()).unwrap();
        let v = eng.call("diagnostics.version", None).unwrap();
        assert!(
            v.get("version").and_then(|x| x.as_str()).is_some(),
            "version missing: {v}"
        );
    }
}
