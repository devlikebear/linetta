fn main() {
    let is_mas = std::env::var("CARGO_FEATURE_MAS").is_ok();
    let target_os = std::env::var("CARGO_CFG_TARGET_OS").unwrap_or_default();
    if is_mas && target_os == "macos" {
        cc::Build::new()
            .file("macos/bookmarks.m")
            .flag("-fobjc-arc")
            .compile("linetta_bookmarks");
        println!("cargo:rustc-link-lib=framework=Foundation");
        println!("cargo:rerun-if-changed=macos/bookmarks.m");
    }
    tauri_build::build();
}
