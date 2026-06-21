use serde::Serialize;
use serde_json::Value;
use std::ffi::{c_char, c_int, CStr, CString};
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::OnceLock;
#[cfg(target_os = "windows")]
use tauri::Manager;
use tauri::{AppHandle, Emitter};

type NotifyCallback = extern "C" fn(*const c_char, *const c_char);

#[cfg(not(target_os = "windows"))]
mod engine_abi {
    use super::{c_char, c_int, NotifyCallback};

    unsafe extern "C" {
        fn LinettaEngineStart(home: *const c_char) -> c_int;
        fn LinettaEngineCall(request: *const c_char) -> *mut c_char;
        fn LinettaEngineFreeCString(s: *mut c_char);
        fn LinettaEngineStop() -> c_int;
        fn LinettaEngineSetNotifyCallback(cb: NotifyCallback);
    }

    pub fn set_notify_callback(cb: NotifyCallback) -> Result<(), String> {
        unsafe { LinettaEngineSetNotifyCallback(cb) };
        Ok(())
    }

    pub fn start(home: *const c_char) -> Result<c_int, String> {
        Ok(unsafe { LinettaEngineStart(home) })
    }

    pub fn call(request: *const c_char) -> Result<*mut c_char, String> {
        Ok(unsafe { LinettaEngineCall(request) })
    }

    pub fn free_cstring(s: *mut c_char) -> Result<(), String> {
        unsafe { LinettaEngineFreeCString(s) };
        Ok(())
    }

    pub fn stop() -> Result<c_int, String> {
        Ok(unsafe { LinettaEngineStop() })
    }
}

#[cfg(target_os = "windows")]
mod engine_abi {
    use super::{c_char, c_int, NotifyCallback};
    use libloading::Library;
    use std::path::PathBuf;
    use std::sync::OnceLock;

    const DLL_NAME: &str = "linetta_engine_ffi.dll";

    type StartFn = unsafe extern "C" fn(*const c_char) -> c_int;
    type CallFn = unsafe extern "C" fn(*const c_char) -> *mut c_char;
    type FreeCStringFn = unsafe extern "C" fn(*mut c_char);
    type StopFn = unsafe extern "C" fn() -> c_int;
    type SetNotifyCallbackFn = unsafe extern "C" fn(NotifyCallback);

    struct EngineApi {
        start: StartFn,
        call: CallFn,
        free_cstring: FreeCStringFn,
        stop: StopFn,
        set_notify_callback: SetNotifyCallbackFn,
    }

    static RESOURCE_DIR: OnceLock<PathBuf> = OnceLock::new();
    static API: OnceLock<Result<EngineApi, String>> = OnceLock::new();

    pub fn set_resource_dir(path: PathBuf) {
        let _ = RESOURCE_DIR.set(path);
    }

    pub fn set_notify_callback(cb: NotifyCallback) -> Result<(), String> {
        let api = api()?;
        unsafe { (api.set_notify_callback)(cb) };
        Ok(())
    }

    pub fn start(home: *const c_char) -> Result<c_int, String> {
        let api = api()?;
        Ok(unsafe { (api.start)(home) })
    }

    pub fn call(request: *const c_char) -> Result<*mut c_char, String> {
        let api = api()?;
        Ok(unsafe { (api.call)(request) })
    }

    pub fn free_cstring(s: *mut c_char) -> Result<(), String> {
        let api = api()?;
        unsafe { (api.free_cstring)(s) };
        Ok(())
    }

    pub fn stop() -> Result<c_int, String> {
        let api = api()?;
        Ok(unsafe { (api.stop)() })
    }

    pub(crate) fn windows_engine_library_path() -> Result<PathBuf, String> {
        let candidates = windows_engine_library_candidates();
        for candidate in &candidates {
            if candidate.is_file() {
                return Ok(candidate.clone());
            }
        }
        let checked = candidates
            .iter()
            .map(|path| path.display().to_string())
            .collect::<Vec<_>>()
            .join(", ");
        Err(format!("{DLL_NAME} not found; checked: {checked}"))
    }

    fn api() -> Result<&'static EngineApi, String> {
        match API.get_or_init(load_engine_api) {
            Ok(api) => Ok(api),
            Err(e) => Err(e.clone()),
        }
    }

    fn load_engine_api() -> Result<EngineApi, String> {
        let path = windows_engine_library_path()?;
        let library = unsafe { Library::new(&path) }
            .map_err(|e| format!("load {} failed: {e}", path.display()))?;
        let start = unsafe { load_symbol(&library, b"LinettaEngineStart\0")? };
        let call = unsafe { load_symbol(&library, b"LinettaEngineCall\0")? };
        let free_cstring = unsafe { load_symbol(&library, b"LinettaEngineFreeCString\0")? };
        let stop = unsafe { load_symbol(&library, b"LinettaEngineStop\0")? };
        let set_notify_callback =
            unsafe { load_symbol(&library, b"LinettaEngineSetNotifyCallback\0")? };

        // Keep the DLL loaded for the process lifetime so copied function
        // pointers remain valid after `load_engine_api` returns.
        let _ = Box::leak(Box::new(library));

        Ok(EngineApi {
            start,
            call,
            free_cstring,
            stop,
            set_notify_callback,
        })
    }

    unsafe fn load_symbol<T: Copy>(library: &Library, name: &'static [u8]) -> Result<T, String> {
        let symbol = unsafe { library.get::<T>(name) }
            .map_err(|e| format!("load symbol {} failed: {e}", symbol_name(name)))?;
        Ok(*symbol)
    }

    fn symbol_name(name: &[u8]) -> String {
        String::from_utf8_lossy(name.strip_suffix(&[0]).unwrap_or(name)).into_owned()
    }

    fn windows_engine_library_candidates() -> Vec<PathBuf> {
        let mut candidates = Vec::new();
        if let Some(resource_dir) = RESOURCE_DIR.get() {
            candidates.push(resource_dir.join(DLL_NAME));
        }
        if let Ok(path) = std::env::var("LINETTA_ENGINE_DLL_PATH") {
            candidates.push(PathBuf::from(path));
        }
        if let Ok(exe) = std::env::current_exe() {
            if let Some(dir) = exe.parent() {
                candidates.push(dir.join(DLL_NAME));
            }
        }
        if let Some(path) = option_env!("LINETTA_ENGINE_DLL_BUILD_PATH") {
            candidates.push(PathBuf::from(path));
        }
        candidates
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
        #[cfg(target_os = "windows")]
        if let Ok(resource_dir) = app.path().resource_dir() {
            engine_abi::set_resource_dir(resource_dir);
        }
        engine_abi::set_notify_callback(notify_trampoline)?;
        if let Some(home) = home.filter(|home| !home.is_empty()) {
            std::env::set_var("LINETTA_HOME", home);
        }
        Self::start_raw(home.unwrap_or(""))
    }

    pub fn start_raw(home: &str) -> Result<Engine, String> {
        let c_home = CString::new(home).map_err(|e| e.to_string())?;
        let rc = engine_abi::start(c_home.as_ptr())?;
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
        let ptr = engine_abi::call(c_req.as_ptr())?;
        if ptr.is_null() {
            return Err("engine returned null".to_string());
        }
        let resp_str = unsafe { CStr::from_ptr(ptr) }
            .to_string_lossy()
            .into_owned();
        engine_abi::free_cstring(ptr)?;
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

    #[cfg(target_os = "windows")]
    #[test]
    fn windows_engine_dll_is_discoverable() {
        let path = engine_abi::windows_engine_library_path().expect("engine dll path");
        assert_eq!(
            path.file_name().and_then(|name| name.to_str()),
            Some("linetta_engine_ffi.dll")
        );
    }
}
