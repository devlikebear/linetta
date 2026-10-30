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

/// A work's picture folder, as the engine names it: one path component
/// ending in `.images`.
fn is_image_dir(name: &str) -> bool {
    name.ends_with(".images") && Path::new(name).file_name().and_then(|n| n.to_str()) == Some(name)
}

/// The only staged names accepted: a plain file name, or `<work>.images/<file>`
/// for a picture. Anything else (`..`, an absolute path, deeper nesting) is
/// refused before it can reach outside the target folder.
fn valid_staged_name(f: &str) -> bool {
    let plain = |s: &str| Path::new(s).file_name().and_then(|n| n.to_str()) == Some(s);
    match f.split_once('/') {
        None => plain(f),
        Some((dir, name)) => is_image_dir(dir) && plain(name),
    }
}

/// Copy each named file from `staging` into `target`. Returns the count copied.
/// A picture already in the target under the same name and size is not
/// copied again: its bytes never change under its id, and rewriting it on
/// every sync would only churn the writer's folder.
/// Used by the MAS orchestration path and by tests; unused in plain non-MAS builds.
#[allow(dead_code)]
fn copy_files(staging: &Path, target: &Path, files: &[String]) -> Result<usize, CopyError> {
    let mut n = 0usize;
    for f in files {
        if !valid_staged_name(f) {
            return Err(CopyError {
                copied: n,
                message: format!("invalid staged filename: {f}"),
            });
        }
        let src = staging.join(f);
        let dst = target.join(f);
        if let Some((dir, _)) = f.split_once('/') {
            std::fs::create_dir_all(target.join(dir)).map_err(|e| CopyError {
                copied: n,
                message: format!("create {dir}: {e}"),
            })?;
            let same = match (std::fs::metadata(&src), std::fs::metadata(&dst)) {
                (Ok(a), Ok(b)) => b.is_file() && a.len() == b.len(),
                _ => false,
            };
            if same {
                n += 1;
                continue;
            }
        }
        copy_file_atomic(&src, &dst).map_err(|e| CopyError {
            copied: n,
            message: format!("copy {f}: {e}"),
        })?;
        n += 1;
    }
    Ok(n)
}

/// Remove, from each work's picture folder in `target`, the pictures `files`
/// no longer lists, and the folder itself once it is empty. Only names in
/// `image_dirs` are touched, and only files directly inside them.
#[allow(dead_code)]
fn prune_image_dirs(target: &Path, image_dirs: &[String], files: &[String]) -> Result<(), String> {
    for dir in image_dirs {
        if !is_image_dir(dir) {
            return Err(format!("invalid image folder: {dir}"));
        }
        let path = target.join(dir);
        let entries = match std::fs::read_dir(&path) {
            Ok(entries) => entries,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => continue,
            Err(e) => return Err(format!("read {dir}: {e}")),
        };
        let mut remaining = 0usize;
        for entry in entries {
            let entry = entry.map_err(|e| e.to_string())?;
            let name = entry.file_name().to_string_lossy().into_owned();
            let is_file = entry.file_type().map(|t| t.is_file()).unwrap_or(false);
            if !is_file || files.iter().any(|f| f == &format!("{dir}/{name}")) {
                remaining += 1;
                continue;
            }
            std::fs::remove_file(entry.path()).map_err(|e| format!("remove {dir}/{name}: {e}"))?;
        }
        if remaining == 0 {
            std::fs::remove_dir(&path).map_err(|e| format!("remove {dir}: {e}"))?;
        }
    }
    Ok(())
}

/// Flush a file's contents to disk.
///
/// The handle has to be writable: on Windows `sync_all` maps to
/// `FlushFileBuffers`, which requires write access and fails with
/// ERROR_ACCESS_DENIED on the read-only handle `File::open` returns. On Unix
/// `fsync` accepts a read-only descriptor, which is why this only broke here.
pub(crate) fn sync_file(path: &Path) -> std::io::Result<()> {
    std::fs::OpenOptions::new()
        .write(true)
        .open(path)?
        .sync_all()
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
        sync_file(&tmp).map_err(|e| e.to_string())?;
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
        sync_file(&tmp).map_err(|e| e.to_string())?;
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

    let image_dirs: Vec<String> = stage
        .get("image_dirs")
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
        let copied = copy_files(&staging_pb, target, &files).map_err(|e| {
            partial_copied.set(e.copied);
            e.to_string()
        })?;
        prune_image_dirs(target, &image_dirs, &files).map_err(|e| {
            partial_copied.set(copied);
            e
        })?;
        Ok(copied)
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
    use super::{copy_files, prune_image_dirs, sync_file};

    // `File::open` hands back a read-only handle, and on Windows sync_all maps
    // to FlushFileBuffers, which needs write access. Every atomic write in this
    // module flushes before renaming, so a read-only handle broke folder sync
    // and backup restore on Windows with ERROR_ACCESS_DENIED.
    #[test]
    fn sync_file_flushes_through_a_writable_handle() {
        let dir = tempdir();
        let path = dir.join("flushed.md");
        std::fs::write(&path, b"contents").unwrap();

        sync_file(&path).expect("sync_file must succeed on every platform");
        assert_eq!(std::fs::read_to_string(&path).unwrap(), "contents");
    }

    #[test]
    fn sync_file_reports_a_missing_path() {
        let dir = tempdir();
        assert!(sync_file(&dir.join("absent.md")).is_err());
    }

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

    #[test]
    fn copies_pictures_into_their_folder_once() {
        let staging = tempdir();
        let target = tempdir();
        std::fs::create_dir(staging.join("work.images")).unwrap();
        std::fs::write(staging.join("work.md"), b"md").unwrap();
        std::fs::write(staging.join("work.images/a.jpg"), b"AAAA").unwrap();
        let files = vec!["work.md".to_string(), "work.images/a.jpg".to_string()];

        assert_eq!(copy_files(&staging, &target, &files).unwrap(), 2);
        assert_eq!(std::fs::read(target.join("work.images/a.jpg")).unwrap(), b"AAAA");

        // A second sync leaves the picture as it is.
        std::fs::write(target.join("work.images/a.jpg"), b"KEEP").unwrap();
        assert_eq!(copy_files(&staging, &target, &files).unwrap(), 2);
        assert_eq!(std::fs::read(target.join("work.images/a.jpg")).unwrap(), b"KEEP");
    }

    #[test]
    fn refuses_names_outside_the_target() {
        let staging = tempdir();
        let target = tempdir();
        for bad in ["../x.md", "/etc/x", "work.images/../x", "other/a.jpg", "a.images/b/c.jpg"] {
            let err = copy_files(&staging, &target, &[bad.to_string()]).unwrap_err();
            assert!(err.message.contains("invalid staged filename"), "{bad}: {}", err.message);
        }
        assert!(prune_image_dirs(&target, &["../work.images".to_string()], &[]).is_err());
    }

    #[test]
    fn prunes_pictures_no_scene_uses() {
        let target = tempdir();
        std::fs::create_dir(target.join("work.images")).unwrap();
        std::fs::create_dir(target.join("gone.images")).unwrap();
        std::fs::write(target.join("work.images/a.jpg"), b"A").unwrap();
        std::fs::write(target.join("work.images/b.jpg"), b"B").unwrap();
        std::fs::write(target.join("gone.images/c.jpg"), b"C").unwrap();
        std::fs::write(target.join("writer-notes.txt"), b"theirs").unwrap();

        prune_image_dirs(
            &target,
            &["work.images".to_string(), "gone.images".to_string()],
            &["work.images/a.jpg".to_string()],
        )
        .unwrap();

        assert!(target.join("work.images/a.jpg").exists());
        assert!(!target.join("work.images/b.jpg").exists());
        assert!(!target.join("gone.images").exists());
        assert!(target.join("writer-notes.txt").exists());
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
