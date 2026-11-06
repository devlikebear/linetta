fn main() {
    build_go_engine();
    tauri_build::build();
}

/// Builds `engine/cmd/clio-ffi` as a c-archive and links it statically.
/// macOS and Linux only; Clio does not ship Windows or mobile builds yet.
fn build_go_engine() {
    use std::{
        env, fs,
        path::{Path, PathBuf},
        process::Command,
    };

    let out_dir = PathBuf::from(env::var("OUT_DIR").expect("OUT_DIR"));
    let manifest = PathBuf::from(env::var("CARGO_MANIFEST_DIR").expect("CARGO_MANIFEST_DIR"));
    let engine_dir = manifest.join("../../../engine");
    let target_os = env::var("CARGO_CFG_TARGET_OS").unwrap_or_default();

    assert!(
        matches!(target_os.as_str(), "macos" | "linux"),
        "Clio builds only for macOS and Linux (target_os = {target_os})"
    );

    for file in ["go.mod", "go.sum"] {
        let path = engine_dir.join(file);
        if path.exists() {
            println!("cargo:rerun-if-changed={}", path.display());
        }
    }
    for dir in ["cmd/clio-ffi", "internal"] {
        emit_go_file_rerun_paths(&engine_dir.join(dir));
    }

    let archive = out_dir.join("libclio_engine_ffi.a");
    let status = Command::new("go")
        .current_dir(&engine_dir)
        .arg("build")
        .env("CGO_ENABLED", "1")
        .arg("-buildmode=c-archive")
        .arg("-o")
        .arg(&archive)
        .arg("./cmd/clio-ffi")
        .status()
        .expect("spawn go build");
    assert!(status.success(), "go c-archive build failed");

    println!("cargo:rustc-link-search=native={}", out_dir.display());
    println!("cargo:rustc-link-lib=static=clio_engine_ffi");
    if target_os == "macos" {
        println!("cargo:rustc-link-lib=framework=Security");
        println!("cargo:rustc-link-lib=framework=CoreFoundation");
    }

    fn emit_go_file_rerun_paths(path: &Path) {
        if path.is_file() {
            if path.extension().and_then(|ext| ext.to_str()) == Some("go") {
                println!("cargo:rerun-if-changed={}", path.display());
            }
            return;
        }
        let entries = match fs::read_dir(path) {
            Ok(entries) => entries,
            Err(_) => return,
        };
        let mut children: Vec<PathBuf> = entries
            .filter_map(|entry| entry.ok().map(|entry| entry.path()))
            .collect();
        children.sort();
        for child in children {
            emit_go_file_rerun_paths(&child);
        }
    }
}
