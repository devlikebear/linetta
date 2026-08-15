fn main() {
    build_go_engine();
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
    let target = env::var("TARGET").unwrap_or_default();

    emit_go_engine_rerun_paths(&engine_dir);

    let mut tags: Vec<&str> = Vec::new();
    if env::var("CARGO_FEATURE_MAS").is_ok() {
        tags.push("mas");
    }
    if matches!(target_os.as_str(), "android" | "ios") {
        tags.push("mobile");
    }

    match target_os.as_str() {
        "android" => build_android_engine(&engine_dir, &manifest, &out_dir, &tags),
        "ios" => build_apple_engine(&engine_dir, &out_dir, &tags, true),
        "windows" => build_windows_engine(&engine_dir, &manifest, &out_dir, &target, &tags),
        _ => build_apple_engine(&engine_dir, &out_dir, &tags, false),
    }

    if matches!(target_os.as_str(), "macos" | "ios") {
        println!("cargo:rustc-link-lib=framework=Security");
        println!("cargo:rustc-link-lib=framework=CoreFoundation");
    }
    println!("cargo:rerun-if-env-changed=ANDROID_HOME");
    println!("cargo:rerun-if-env-changed=ANDROID_NDK_HOME");
    println!("cargo:rerun-if-env-changed=NDK_HOME");
    println!("cargo:rerun-if-env-changed=SDKROOT");

    fn emit_go_engine_rerun_paths(engine_dir: &Path) {
        for file in ["go.mod", "go.sum"] {
            let path = engine_dir.join(file);
            if path.exists() {
                println!("cargo:rerun-if-changed={}", path.display());
            }
        }
        for dir in ["cmd/linetta-ffi", "internal"] {
            emit_go_file_rerun_paths(&engine_dir.join(dir));
        }
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

    fn build_apple_engine(engine_dir: &Path, out_dir: &Path, tags: &[&str], ios: bool) {
        let archive = out_dir.join("liblinetta_engine_ffi.a");
        let mut cmd = go_build_command(engine_dir, tags);
        cmd.env("CGO_ENABLED", "1")
            .arg("-buildmode=c-archive")
            .arg("-o")
            .arg(&archive)
            .arg("./cmd/linetta-ffi");

        if ios {
            let target = env::var("TARGET").unwrap_or_default();
            let sdk = if target.contains("ios-sim")
                || env::var("SDKROOT")
                    .map(|v| v.to_ascii_lowercase().contains("iphonesimulator"))
                    .unwrap_or(false)
            {
                "iphonesimulator"
            } else {
                "iphoneos"
            };
            let sdkroot = env::var("SDKROOT").unwrap_or_else(|_| xcrun(sdk, &["--show-sdk-path"]));
            let clang = xcrun(sdk, &["--find", "clang"]);
            let minflag = if sdk == "iphonesimulator" {
                "-mios-simulator-version-min=14.0"
            } else {
                "-miphoneos-version-min=14.0"
            };
            let wrapper = out_dir.join(format!("linetta-{sdk}-clangwrap.sh"));
            fs::write(
                &wrapper,
                format!(
                    "#!/bin/sh\nexec \"{clang}\" -isysroot \"{sdkroot}\" -arch arm64 {minflag} \"$@\"\n"
                ),
            )
            .expect("write iOS clang wrapper");
            make_executable(&wrapper);
            cmd.env("GOOS", "ios")
                .env("GOARCH", "arm64")
                .env("CC", &wrapper);
        }

        run_go_build(&mut cmd, "go c-archive build failed");
        println!("cargo:rustc-link-search=native={}", out_dir.display());
        println!("cargo:rustc-link-lib=static=linetta_engine_ffi");
    }

    fn build_android_engine(engine_dir: &Path, manifest: &Path, out_dir: &Path, tags: &[&str]) {
        let target = env::var("TARGET").unwrap_or_default();
        let (abi, goarch, goarm, clang) = android_target(&target);
        let ndk = find_android_ndk();
        let host_tag = android_host_tag(&ndk);
        let cc = ndk
            .join("toolchains/llvm/prebuilt")
            .join(host_tag)
            .join("bin")
            .join(clang);
        assert!(cc.exists(), "Android clang not found: {}", cc.display());

        let lib = out_dir.join("liblinetta.so");
        let mut cmd = go_build_command(engine_dir, tags);
        cmd.env("CGO_ENABLED", "1")
            .env("GOOS", "android")
            .env("GOARCH", goarch)
            .env("CC", &cc)
            .arg("-buildmode=c-shared")
            .arg("-o")
            .arg(&lib)
            .arg("./cmd/linetta-ffi");
        if let Some(goarm) = goarm {
            cmd.env("GOARM", goarm);
        }
        run_go_build(&mut cmd, "go c-shared build failed");

        let jni_dir = manifest.join("gen/android/app/src/main/jniLibs").join(abi);
        fs::create_dir_all(&jni_dir).expect("create Android jniLibs dir");
        fs::copy(&lib, jni_dir.join("liblinetta.so")).expect("copy Android engine .so");

        println!("cargo:rustc-link-search=native={}", out_dir.display());
        println!("cargo:rustc-link-lib=dylib=linetta");
    }

    fn build_windows_engine(
        engine_dir: &Path,
        manifest: &Path,
        out_dir: &Path,
        target: &str,
        tags: &[&str],
    ) {
        let dll = out_dir.join("linetta_engine_ffi.dll");
        let mut cmd = go_build_command(engine_dir, tags);
        cmd.env("CGO_ENABLED", "1")
            .env("GOOS", "windows")
            .env("GOARCH", windows_goarch(target))
            .arg("-buildmode=c-shared")
            .arg("-o")
            .arg(&dll)
            .arg("./cmd/linetta-ffi");
        run_go_build(&mut cmd, "go c-shared build failed");

        let profile_dir = cargo_profile_dir(out_dir);
        let profile_dll = profile_dir.join("linetta_engine_ffi.dll");
        fs::copy(&dll, &profile_dll).unwrap_or_else(|e| {
            panic!(
                "copy Windows engine DLL to {} failed: {e}",
                profile_dll.display()
            )
        });
        // The bundle resource path in `tauri.windows.conf.json` has to be a
        // fixed, profile-independent location: Tauri merges the platform config
        // for debug builds too, and fails the build script when the resource is
        // missing. Keep a copy under `gen/` (already gitignored) so `cargo
        // check`, `tauri dev`, and `tauri build` all resolve the same path.
        let bundle_dir = manifest.join("gen/windows");
        fs::create_dir_all(&bundle_dir).unwrap_or_else(|e| {
            panic!(
                "create Windows resource dir {} failed: {e}",
                bundle_dir.display()
            )
        });
        let bundle_dll = bundle_dir.join("linetta_engine_ffi.dll");
        fs::copy(&dll, &bundle_dll).unwrap_or_else(|e| {
            panic!(
                "copy Windows engine DLL to {} failed: {e}",
                bundle_dll.display()
            )
        });

        println!(
            "cargo:rustc-env=LINETTA_ENGINE_DLL_BUILD_PATH={}",
            profile_dll.display()
        );
    }

    fn go_build_command(engine_dir: &Path, tags: &[&str]) -> Command {
        let mut cmd = Command::new("go");
        cmd.current_dir(engine_dir).arg("build");
        if !tags.is_empty() {
            cmd.arg(format!("-tags={}", tags.join(",")));
        }
        cmd
    }

    fn cargo_profile_dir(out_dir: &Path) -> PathBuf {
        out_dir
            .ancestors()
            .nth(3)
            .unwrap_or_else(|| panic!("unexpected OUT_DIR layout: {}", out_dir.display()))
            .to_path_buf()
    }

    fn windows_goarch(target: &str) -> &'static str {
        match target {
            "x86_64-pc-windows-msvc" => "amd64",
            "aarch64-pc-windows-msvc" => "arm64",
            "i686-pc-windows-msvc" => "386",
            _ => panic!("unsupported Windows target: {target}"),
        }
    }

    fn run_go_build(cmd: &mut Command, message: &str) {
        assert!(cmd.status().expect("spawn go build").success(), "{message}");
    }

    fn xcrun(sdk: &str, args: &[&str]) -> String {
        let output = Command::new("xcrun")
            .arg("--sdk")
            .arg(sdk)
            .args(args)
            .output()
            .expect("spawn xcrun");
        assert!(output.status.success(), "xcrun {:?} failed", args);
        String::from_utf8(output.stdout)
            .expect("xcrun output utf8")
            .trim()
            .to_string()
    }

    fn android_target(target: &str) -> (&'static str, &'static str, Option<&'static str>, String) {
        let api = env::var("ANDROID_API").unwrap_or_else(|_| "24".to_string());
        match target {
            "aarch64-linux-android" => (
                "arm64-v8a",
                "arm64",
                None,
                format!("aarch64-linux-android{api}-clang"),
            ),
            "armv7-linux-androideabi" => (
                "armeabi-v7a",
                "arm",
                Some("7"),
                format!("armv7a-linux-androideabi{api}-clang"),
            ),
            "x86_64-linux-android" => (
                "x86_64",
                "amd64",
                None,
                format!("x86_64-linux-android{api}-clang"),
            ),
            "i686-linux-android" => ("x86", "386", None, format!("i686-linux-android{api}-clang")),
            _ => panic!("unsupported Android target: {target}"),
        }
    }

    fn find_android_ndk() -> PathBuf {
        for key in ["ANDROID_NDK_HOME", "NDK_HOME"] {
            if let Ok(value) = env::var(key) {
                if !value.is_empty() {
                    return PathBuf::from(value);
                }
            }
        }
        let android_home = env::var("ANDROID_HOME")
            .or_else(|_| env::var("ANDROID_SDK_ROOT"))
            .expect("set ANDROID_NDK_HOME or ANDROID_HOME");
        let ndk_root = PathBuf::from(android_home).join("ndk");
        let mut versions: Vec<PathBuf> = fs::read_dir(&ndk_root)
            .unwrap_or_else(|_| panic!("Android NDK not found under {}", ndk_root.display()))
            .filter_map(|entry| entry.ok().map(|entry| entry.path()))
            .filter(|path| path.is_dir())
            .collect();
        versions.sort();
        versions
            .pop()
            .unwrap_or_else(|| panic!("Android NDK not found under {}", ndk_root.display()))
    }

    fn android_host_tag(ndk: &Path) -> &'static str {
        let prebuilt = ndk.join("toolchains/llvm/prebuilt");
        let candidates = match env::consts::OS {
            "macos" => ["darwin-arm64", "darwin-x86_64", ""],
            "linux" => ["linux-x86_64", "", ""],
            "windows" => ["windows-x86_64", "", ""],
            other => panic!("unsupported Android NDK host: {other}"),
        };
        for candidate in candidates {
            if !candidate.is_empty() && prebuilt.join(candidate).exists() {
                return candidate;
            }
        }
        panic!(
            "Android NDK prebuilt toolchain not found under {}",
            prebuilt.display()
        );
    }

    fn make_executable(_path: &Path) {
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mut perms = fs::metadata(_path)
                .expect("clang wrapper metadata")
                .permissions();
            perms.set_mode(0o755);
            fs::set_permissions(_path, perms).expect("chmod clang wrapper");
        }
    }
}
