use std::path::Path;

use crate::EngineState;

/// Copy each named file from `staging` into `target`. Returns the count copied.
/// Used by the MAS orchestration path and by tests; unused in plain non-MAS builds.
#[allow(dead_code)]
pub(crate) fn copy_files(staging: &Path, target: &Path, files: &[String]) -> Result<usize, String> {
    let mut n = 0usize;
    for f in files {
        let src = staging.join(f);
        let dst = target.join(f);
        std::fs::copy(&src, &dst).map_err(|e| format!("copy {f}: {e}"))?;
        n += 1;
    }
    Ok(n)
}

#[cfg(all(target_os = "macos", feature = "mas"))]
fn now_millis() -> i64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

/// Persist the chosen folder path in engine settings, and (MAS) create + store a
/// security-scoped bookmark for later unattended access.
#[tauri::command]
pub(crate) async fn set_folder_sync_dir(
    state: tauri::State<'_, EngineState>,
    app: tauri::AppHandle,
    path: String,
) -> Result<(), String> {
    let client = crate::engine_client(&state)?;
    client
        .call("settings.set", Some(serde_json::json!({ "folder_sync_dir": path })))
        .await
        .map_err(|e| e.to_string())?;

    #[cfg(all(target_os = "macos", feature = "mas"))]
    {
        let bookmark = crate::macos_bookmarks::create_bookmark(std::path::Path::new(&path))?;
        let store = bookmark_store_path(&app)?;
        if let Some(parent) = store.parent() {
            std::fs::create_dir_all(parent).map_err(|e| e.to_string())?;
        }
        std::fs::write(&store, &bookmark).map_err(|e| e.to_string())?;
    }
    #[cfg(not(all(target_os = "macos", feature = "mas")))]
    {
        let _ = &app;
    }
    Ok(())
}

/// Run a folder sync now. Non-MAS forwards to the engine; MAS orchestrates the
/// staged copy via the security-scoped bookmark.
#[tauri::command]
pub(crate) async fn folder_sync_now(
    state: tauri::State<'_, EngineState>,
    app: tauri::AppHandle,
) -> Result<serde_json::Value, String> {
    #[cfg(all(target_os = "macos", feature = "mas"))]
    {
        return run_folder_sync_mas(&app, state.inner()).await;
    }
    #[cfg(not(all(target_os = "macos", feature = "mas")))]
    {
        let _ = &app;
        let client = crate::engine_client(&state)?;
        client
            .call("folder_sync.run", None)
            .await
            .map_err(|e| e.to_string())
    }
}

#[cfg(all(target_os = "macos", feature = "mas"))]
fn bookmark_store_path(app: &tauri::AppHandle) -> Result<std::path::PathBuf, String> {
    use tauri::Manager;
    let dir = app.path().app_data_dir().map_err(|e| e.to_string())?;
    Ok(dir.join("folder-sync.bookmark"))
}

/// MAS orchestration: stage in the engine, copy via bookmark, report back.
#[cfg(all(target_os = "macos", feature = "mas"))]
pub(crate) async fn run_folder_sync_mas(
    app: &tauri::AppHandle,
    state: &EngineState,
) -> Result<serde_json::Value, String> {
    let client = crate::engine_client(state)?;
    let started = now_millis();

    let stage = client
        .call("folder_sync.stage", None)
        .await
        .map_err(|e| e.to_string())?;
    if stage.get("skipped").and_then(|v| v.as_bool()).unwrap_or(false) {
        return Ok(serde_json::json!({ "skipped": true, "files_copied": 0, "message": "", "error": "" }));
    }
    let staging = stage
        .get("staging_dir")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    let files: Vec<String> = stage
        .get("files")
        .and_then(|v| v.as_array())
        .map(|a| a.iter().filter_map(|x| x.as_str().map(String::from)).collect())
        .unwrap_or_default();

    let store = bookmark_store_path(app)?;
    let bookmark = std::fs::read(&store)
        .map_err(|_| "폴더 접근 권한을 잃었습니다. 다시 선택하세요".to_string())?;
    let staging_pb = std::path::PathBuf::from(&staging);

    let outcome = crate::macos_bookmarks::with_scoped_access(&bookmark, |target| {
        copy_files(&staging_pb, target, &files)
    });
    let (ok, copied, errmsg) = match outcome {
        Ok(Ok(n)) => (true, n, String::new()),
        Ok(Err(e)) => (false, 0usize, e),
        Err(e) => (false, 0usize, e),
    };

    let _ = client
        .call(
            "folder_sync.report",
            Some(serde_json::json!({
                "started_at": started,
                "finished_at": now_millis(),
                "ok": ok,
                "files_copied": copied,
                "error": errmsg,
            })),
        )
        .await;

    Ok(serde_json::json!({ "skipped": false, "files_copied": copied, "message": "", "error": errmsg }))
}

#[cfg(test)]
mod tests {
    use super::copy_files;

    #[test]
    fn copies_named_files() {
        let staging = tempdir();
        let target = tempdir();
        std::fs::write(staging.join("a.md"), b"hello").unwrap();
        std::fs::write(staging.join("b.md"), b"world").unwrap();
        let files = vec!["a.md".to_string(), "b.md".to_string()];
        let n = copy_files(&staging, &target, &files).unwrap();
        assert_eq!(n, 2);
        assert_eq!(std::fs::read_to_string(target.join("a.md")).unwrap(), "hello");
        assert_eq!(std::fs::read_to_string(target.join("b.md")).unwrap(), "world");
    }

    fn tempdir() -> std::path::PathBuf {
        let mut p = std::env::temp_dir();
        use std::sync::atomic::{AtomicU64, Ordering};
        static C: AtomicU64 = AtomicU64::new(0);
        let n = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        p.push(format!("linetta-fs-test-{}-{}", n, C.fetch_add(1, Ordering::Relaxed)));
        std::fs::create_dir_all(&p).unwrap();
        p
    }
}
