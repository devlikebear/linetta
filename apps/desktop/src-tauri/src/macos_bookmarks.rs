#![cfg(all(target_os = "macos", feature = "mas"))]

use std::ffi::{CStr, CString};
use std::os::raw::{c_char, c_void};
use std::path::Path;

extern "C" {
    fn linetta_bookmark_create(path: *const c_char, out_len: *mut usize) -> *const c_void;
    fn linetta_bookmark_start(
        data: *const c_void,
        len: usize,
        out_path: *mut *mut c_char,
    ) -> *mut c_void;
    fn linetta_bookmark_stop(handle: *mut c_void);
    fn linetta_free(ptr: *mut c_void);
}

/// Create a persistable security-scoped bookmark for a folder the app currently
/// has access to (e.g. just selected via the open panel).
pub fn create_bookmark(path: &Path) -> Result<Vec<u8>, String> {
    let c = CString::new(path.to_string_lossy().as_bytes()).map_err(|e| e.to_string())?;
    let mut len: usize = 0;
    let ptr = unsafe { linetta_bookmark_create(c.as_ptr(), &mut len as *mut usize) };
    if ptr.is_null() {
        return Err("failed to create security-scoped bookmark".into());
    }
    let bytes = unsafe { std::slice::from_raw_parts(ptr as *const u8, len) }.to_vec();
    unsafe { linetta_free(ptr as *mut c_void) };
    Ok(bytes)
}

/// Resolve `bookmark`, start security-scoped access, run `f` with the resolved
/// path, then always stop access. Outer Err = bookmark/access failure; inner
/// Result is whatever `f` returns.
pub fn with_scoped_access<T, F>(bookmark: &[u8], f: F) -> Result<Result<T, String>, String>
where
    F: FnOnce(&Path) -> Result<T, String>,
{
    let mut out_path: *mut c_char = std::ptr::null_mut();
    let handle = unsafe {
        linetta_bookmark_start(
            bookmark.as_ptr() as *const c_void,
            bookmark.len(),
            &mut out_path as *mut *mut c_char,
        )
    };
    if handle.is_null() {
        return Err("폴더 접근 권한을 잃었습니다. 다시 선택하세요".into());
    }
    let path = unsafe { CStr::from_ptr(out_path) }.to_string_lossy().into_owned();
    unsafe { linetta_free(out_path as *mut c_void) };
    let result = f(Path::new(&path));
    unsafe { linetta_bookmark_stop(handle) };
    Ok(result)
}
