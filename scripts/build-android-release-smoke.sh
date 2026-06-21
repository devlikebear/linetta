#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ANDROID_GEN="${ROOT}/apps/desktop/src-tauri/gen/android"
KEYSTORE_PROPS="${ANDROID_GEN}/keystore.properties"

if [[ -z "${ANDROID_HOME:-}" ]]; then
  echo "ANDROID_HOME is required for the Android release smoke build." >&2
  exit 2
fi

if [[ -z "${ANDROID_NDK_HOME:-}" ]]; then
  echo "ANDROID_NDK_HOME is required for the Android release smoke build." >&2
  exit 2
fi

if [[ -n "${JAVA_HOME:-}" && -x "${JAVA_HOME}/bin/keytool" ]]; then
  KEYTOOL="${JAVA_HOME}/bin/keytool"
elif command -v keytool >/dev/null 2>&1; then
  KEYTOOL="$(command -v keytool)"
else
  echo "keytool is required. Set JAVA_HOME to a JDK, for example Android Studio's bundled JBR." >&2
  exit 2
fi

if [[ -n "${JAVA_HOME:-}" && -x "${JAVA_HOME}/bin/jarsigner" ]]; then
  JARSIGNER="${JAVA_HOME}/bin/jarsigner"
elif command -v jarsigner >/dev/null 2>&1; then
  JARSIGNER="$(command -v jarsigner)"
else
  echo "jarsigner is required. Set JAVA_HOME to a JDK, for example Android Studio's bundled JBR." >&2
  exit 2
fi

tmp="$(mktemp -d)"
backup=""
cleanup() {
  if [[ -n "${backup}" && -f "${backup}" ]]; then
    cp "${backup}" "${KEYSTORE_PROPS}"
  else
    rm -f "${KEYSTORE_PROPS}"
  fi
  rm -rf "${tmp}"
}
trap cleanup EXIT

if [[ -f "${KEYSTORE_PROPS}" ]]; then
  backup="${tmp}/keystore.properties.backup"
  cp "${KEYSTORE_PROPS}" "${backup}"
fi

if [[ ! -f "${ANDROID_GEN}/app/build.gradle.kts" ]]; then
  (cd "${ROOT}/apps/desktop" && pnpm tauri android init --ci --skip-targets-install)
fi

password="linetta-smoke-password"
alias="linetta-smoke"
keystore="${tmp}/linetta-upload-smoke.jks"

"${KEYTOOL}" \
  -genkeypair \
  -keystore "${keystore}" \
  -storepass "${password}" \
  -keypass "${password}" \
  -alias "${alias}" \
  -keyalg RSA \
  -keysize 2048 \
  -validity 365 \
  -dname "CN=Linetta Android Smoke, OU=Linetta, O=Devlikebear, L=Seoul, ST=Seoul, C=KR" \
  >/dev/null

mkdir -p "${ANDROID_GEN}"
cat > "${KEYSTORE_PROPS}" <<EOF
keyAlias=${alias}
password=${password}
storeFile=${keystore}
EOF

bash "${ROOT}/scripts/patch-tauri-android-signing.sh"

build_config='{"bundle":{"android":{"versionCode":1}}}'

(cd "${ROOT}/apps/desktop" && pnpm tauri android build --apk --target aarch64 --ci --config "${build_config}")

apk_artifact="$(find "${ANDROID_GEN}" -type f -path '*/release/*.apk' -print -quit)"
if [[ -z "${apk_artifact}" ]]; then
  echo "Android release smoke build did not produce a release APK." >&2
  exit 1
fi

apk_listing="$(unzip -Z1 "${apk_artifact}")"
if ! grep -Fq "lib/arm64-v8a/liblinetta.so" <<<"${apk_listing}"; then
  echo "Android release smoke APK is missing liblinetta.so." >&2
  exit 1
fi

if ! grep -Fq "lib/arm64-v8a/liblinetta_desktop_lib.so" <<<"${apk_listing}"; then
  echo "Android release smoke APK is missing liblinetta_desktop_lib.so." >&2
  exit 1
fi

apksigner="$(find "${ANDROID_HOME}/build-tools" -type f -name apksigner -print 2>/dev/null | sort | tail -n 1)"
if [[ -z "${apksigner}" ]]; then
  echo "apksigner was not found under ANDROID_HOME/build-tools." >&2
  exit 2
fi

signature_report="$("${apksigner}" verify --verbose "${apk_artifact}")"
if ! grep -Fq "Verified using v2 scheme (APK Signature Scheme v2): true" <<<"${signature_report}"; then
  echo "Android release smoke APK did not verify with APK Signature Scheme v2." >&2
  echo "${signature_report}" >&2
  exit 1
fi

(cd "${ROOT}/apps/desktop" && pnpm tauri android build --aab --target aarch64 --ci --config "${build_config}")

aab_artifact="$(find "${ANDROID_GEN}" -type f -path '*/outputs/bundle/*Release/*.aab' -print -quit)"
if [[ -z "${aab_artifact}" ]]; then
  echo "Android release smoke build did not produce a release AAB." >&2
  exit 1
fi

aab_listing="$(unzip -Z1 "${aab_artifact}")"
if ! grep -Fq "base/lib/arm64-v8a/liblinetta.so" <<<"${aab_listing}"; then
  echo "Android release smoke AAB is missing liblinetta.so." >&2
  exit 1
fi

if ! grep -Fq "base/lib/arm64-v8a/liblinetta_desktop_lib.so" <<<"${aab_listing}"; then
  echo "Android release smoke AAB is missing liblinetta_desktop_lib.so." >&2
  exit 1
fi

"${JARSIGNER}" -verify "${aab_artifact}" >/dev/null

echo "Android release smoke APK: ${apk_artifact}"
echo "Android release smoke AAB: ${aab_artifact}"
