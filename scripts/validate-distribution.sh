#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="$(awk -F'"' '/"version":/ {print $4; exit}' "${ROOT}/apps/desktop/package.json")"

fail() {
  echo "distribution validation failed: $*" >&2
  exit 1
}

require_file() {
  local path="$1"
  [[ -f "${ROOT}/${path}" ]] || fail "missing ${path}"
}

require_executable() {
  local path="$1"
  [[ -x "${ROOT}/${path}" ]] || fail "missing executable ${path}"
}

require_contains() {
  local path="$1"
  local needle="$2"
  require_file "${path}"
  grep -Fq -- "${needle}" "${ROOT}/${path}" || fail "${path} does not contain: ${needle}"
}

require_file ".github/workflows/build.yml"
require_file ".github/workflows/mobile-engine.yml"
require_file ".github/workflows/mobile-release.yml"
require_file "apps/desktop/src-tauri/tauri.windows.conf.json"
require_executable "scripts/patch-tauri-android-signing.sh"
require_executable "scripts/build-android-release-smoke.sh"
require_executable "scripts/smoke-ios-simulator.sh"
require_contains "scripts/build-android-release-smoke.sh" "pnpm tauri android build --aab"
require_contains ".github/workflows/build.yml" "bundles: appimage,deb,rpm"
require_contains ".github/workflows/build.yml" "bundles: nsis,msi"
require_contains ".github/workflows/build.yml" "rpm"
require_contains ".github/workflows/build.yml" "cp rpm/*.rpm dist/"
require_contains ".github/workflows/build.yml" "cp msi/*.msi dist/"
require_contains ".github/workflows/build.yml" "Linetta-winget-manifests.tar.gz"
require_contains ".github/workflows/build.yml" "SHA256SUMS"
require_contains ".github/workflows/build.yml" "tauri.windows.conf.json"
require_contains "apps/desktop/src-tauri/build.rs" "build_windows_engine"
require_contains "apps/desktop/src-tauri/build.rs" "-buildmode=c-shared"
require_contains "apps/desktop/src-tauri/build.rs" "linetta_engine_ffi.dll"
require_contains "apps/desktop/src-tauri/src/ffi.rs" "libloading::Library"
require_contains "apps/desktop/src-tauri/src/ffi.rs" "windows_engine_library_path"
require_contains "apps/desktop/src-tauri/tauri.windows.conf.json" "linetta_engine_ffi.dll"
require_contains ".github/workflows/mobile-engine.yml" "go test -tags mobile ./..."
require_contains ".github/workflows/mobile-engine.yml" "build-engine-ios.sh"
require_contains ".github/workflows/mobile-engine.yml" "build-engine-android.sh"
require_contains ".github/workflows/mobile-engine.yml" "pnpm tauri android build --debug --apk --target aarch64 --ci"
require_contains ".github/workflows/mobile-release.yml" "pnpm tauri ios build"
require_contains ".github/workflows/mobile-release.yml" "pnpm tauri android build"
require_contains ".github/workflows/mobile-release.yml" "ANDROID_KEY_BASE64"
require_contains ".github/workflows/mobile-release.yml" "patch-tauri-android-signing.sh"

require_file "packaging/README.md"
require_file "packaging/flathub/com.devlikebear.linetta.yml"
require_contains "packaging/flathub/com.devlikebear.linetta.yml" "app-id: com.devlikebear.linetta"
require_contains "packaging/flathub/com.devlikebear.linetta.yml" "command: linetta-desktop"
require_contains "packaging/flathub/com.devlikebear.linetta.yml" "tag: v${VERSION}"

require_file "packaging/winget/Devlikebear.Linetta.yaml.template"
require_file "packaging/winget/Devlikebear.Linetta.installer.yaml.template"
require_file "packaging/winget/Devlikebear.Linetta.locale.en-US.yaml.template"
require_executable "scripts/render-winget-manifest.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

"${ROOT}/scripts/render-winget-manifest.sh" \
  "0.4.0" \
  "https://github.com/devlikebear/linetta/releases/download/v0.4.0" \
  "Linetta_0.4.0_x64-setup.exe" \
  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" \
  "${tmp}"

require_rendered_contains() {
  local path="$1"
  local needle="$2"
  [[ -f "${path}" ]] || fail "rendered file missing: ${path}"
  grep -Fq "${needle}" "${path}" || fail "${path} does not contain: ${needle}"
}

require_rendered_contains "${tmp}/Devlikebear.Linetta.yaml" "PackageIdentifier: Devlikebear.Linetta"
require_rendered_contains "${tmp}/Devlikebear.Linetta.yaml" "PackageVersion: 0.4.0"
require_rendered_contains "${tmp}/Devlikebear.Linetta.installer.yaml" "InstallerType: nullsoft"
require_rendered_contains "${tmp}/Devlikebear.Linetta.installer.yaml" "InstallerUrl: https://github.com/devlikebear/linetta/releases/download/v0.4.0/Linetta_0.4.0_x64-setup.exe"
require_rendered_contains "${tmp}/Devlikebear.Linetta.installer.yaml" "InstallerSha256: 0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"
require_rendered_contains "${tmp}/Devlikebear.Linetta.locale.en-US.yaml" "PackageName: Linetta"

echo "distribution metadata ok"
