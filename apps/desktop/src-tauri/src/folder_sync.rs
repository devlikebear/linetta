use std::path::Path;
use std::sync::atomic::{AtomicU64, Ordering};

use crate::EngineState;

static TEMP_ID: AtomicU64 = AtomicU64::new(0);

#[derive(Debug)]
struct CopyError {
    #[allow(dead_code)]
    copied: usize,
    message: String,
}

impl std::fmt::Display for CopyError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.message)
    }
}

/// Copy each named file from `staging` into `target`. Returns the count copied.
/// Used by the MAS orchestration path and by tests; unused in plain non-MAS builds.
#[allow(dead_code)]
fn copy_files(staging: &Path, target: &Path, files: &[String]) -> Result<usize, CopyError> {
    let mut n = 0usize;
    for f in files {
        let src = staging.join(f);
        let dst = target.join(f);
        if Path::new(f).file_name().and_then(|name| name.to_str()) != Some(f.as_str()) {
            return Err(CopyError {
                copied: n,
                message: format!("invalid staged filename: {f}"),
            });
        }
        copy_file_atomic(&src, &dst).map_err(|e| CopyError {
            copied: n,
            message: format!("copy {f}: {e}"),
        })?;
        n += 1;
    }
    Ok(n)
}

fn copy_file_atomic(src: &Path, dst: &Path) -> Result<(), String> {
    let name = dst
        .file_name()
        .and_then(|value| value.to_str())
        .ok_or_else(|| "destination filename is not valid UTF-8".to_string())?;
    let tmp = dst.with_file_name(format!(
        ".{name}.linetta-tmp-{}-{}",
        std::process::id(),
        TEMP_ID.fetch_add(1, Ordering::Relaxed)
    ));
    let result = (|| {
        std::fs::copy(src, &tmp).map_err(|e| e.to_string())?;
        std::fs::File::open(&tmp)
            .and_then(|file| file.sync_all())
            .map_err(|e| e.to_string())?;
        std::fs::rename(&tmp, dst).map_err(|e| e.to_string())
    })();
    if result.is_err() {
        let _ = std::fs::remove_file(&tmp);
    }
    result
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
    let engine = crate::engine_handle(state.inner())?;
    #[cfg(all(target_os = "macos", feature = "mas"))]
    {
        let bookmark = crate::macos_bookmarks::create_bookmark(std::path::Path::new(&path))?;
        let store = bookmark_store_path(&app)?;
        if let Some(parent) = store.parent() {
            std::fs::create_dir_all(parent).map_err(|e| e.to_string())?;
        }
        let previous = std::fs::read(&store).ok();
        write_bytes_atomic(&store, &bookmark)?;
        if let Err(error) = crate::call_engine(
            engine,
            "settings.set".to_string(),
            Some(serde_json::json!({ "folder_sync_dir": path })),
        )
        .await
        {
            match previous {
                Some(bytes) => {
                    let _ = write_bytes_atomic(&store, &bytes);
                }
                None => {
                    let _ = std::fs::remove_file(&store);
                }
            }
            return Err(error);
        }
    }
    #[cfg(not(all(target_os = "macos", feature = "mas")))]
    {
        let _ = &app;
        crate::call_engine(
            engine,
            "settings.set".to_string(),
            Some(serde_json::json!({ "folder_sync_dir": path })),
        )
        .await?;
    }
    Ok(())
}

#[cfg(all(target_os = "macos", feature = "mas"))]
fn write_bytes_atomic(path: &Path, bytes: &[u8]) -> Result<(), String> {
    let tmp = path.with_extension(format!(
        "tmp-{}-{}",
        std::process::id(),
        TEMP_ID.fetch_add(1, Ordering::Relaxed)
    ));
    let result = (|| {
        std::fs::write(&tmp, bytes).map_err(|e| e.to_string())?;
        std::fs::File::open(&tmp)
            .and_then(|file| file.sync_all())
            .map_err(|e| e.to_string())?;
        std::fs::rename(&tmp, path).map_err(|e| e.to_string())
    })();
    if result.is_err() {
        let _ = std::fs::remove_file(&tmp);
    }
    result
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
        let engine = crate::engine_handle(state.inner())?;
        crate::call_engine(engine, "folder_sync.run".to_string(), None).await
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
    let engine = crate::engine_handle(state)?;
    let started = now_millis();

    let stage = crate::call_engine(engine.clone(), "folder_sync.stage".to_string(), None).await?;
    if stage
        .get("skipped")
        .and_then(|v| v.as_bool())
        .unwrap_or(false)
    {
        return Ok(
            serde_json::json!({ "skipped": true, "files_copied": 0, "message": "", "error": "" }),
        );
    }
    let staging = stage
        .get("staging_dir")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    let files: Vec<String> = stage
        .get("files")
        .and_then(|v| v.as_array())
        .map(|a| {
            a.iter()
                .filter_map(|x| x.as_str().map(String::from))
                .collect()
        })
        .unwrap_or_default();

    let store = bookmark_store_path(app)?;
    let bookmark = std::fs::read(&store)
        .map_err(|_| "폴더 접근 권한을 잃었습니다. 다시 선택하세요".to_string())?;
    let staging_pb = std::path::PathBuf::from(&staging);

    let partial_copied = std::cell::Cell::new(0usize);
    let outcome = crate::macos_bookmarks::with_scoped_access(&bookmark, |target| {
        copy_files(&staging_pb, target, &files).map_err(|e| {
            partial_copied.set(e.copied);
            e.to_string()
        })
    });
    let (ok, copied, errmsg) = match outcome {
        Ok(Ok(n)) => (true, n, String::new()),
        Ok(Err(e)) => (false, partial_copied.get(), e),
        Err(e) => (false, 0usize, e),
    };

    let _ = crate::call_engine(
        engine,
        "folder_sync.report".to_string(),
        Some(serde_json::json!({
            "started_at": started,
            "finished_at": now_millis(),
            "ok": ok,
            "files_copied": copied,
            "error": errmsg,
        })),
    )
    .await;

    Ok(
        serde_json::json!({ "skipped": false, "files_copied": copied, "message": "", "error": errmsg }),
    )
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
        assert_eq!(
            std::fs::read_to_string(target.join("a.md")).unwrap(),
            "hello"
        );
        assert_eq!(
            std::fs::read_to_string(target.join("b.md")).unwrap(),
            "world"
        );
    }

    #[test]
    fn reports_the_number_copied_before_a_failure() {
        let staging = tempdir();
        let target = tempdir();
        std::fs::write(staging.join("a.md"), b"hello").unwrap();
        std::fs::write(staging.join("b.md"), b"world").unwrap();
        std::fs::create_dir(target.join("b.md")).unwrap();
        let files = vec!["a.md".to_string(), "b.md".to_string()];

        let err = copy_files(&staging, &target, &files).unwrap_err();

        assert_eq!(err.copied, 1);
        assert_eq!(
            std::fs::read_to_string(target.join("a.md")).unwrap(),
            "hello"
        );
    }

    fn tempdir() -> std::path::PathBuf {
        let mut p = std::env::temp_dir();
        use std::sync::atomic::{AtomicU64, Ordering};
        static C: AtomicU64 = AtomicU64::new(0);
        let n = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        p.push(format!(
            "linetta-fs-test-{}-{}",
            n,
            C.fetch_add(1, Ordering::Relaxed)
        ));
        std::fs::create_dir_all(&p).unwrap();
        p
    }
}
