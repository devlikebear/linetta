#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 /path/to/Linetta.app" >&2
  exit 2
fi

APP_PATH="$1"
if [ ! -d "${APP_PATH}" ]; then
  echo "app bundle not found: ${APP_PATH}" >&2
  exit 1
fi

required_env=(
  APPLE_CERTIFICATE
  APPLE_CERTIFICATE_PASSWORD
  APPLE_ID
  APPLE_PASSWORD
  APPLE_TEAM_ID
)

missing_env=()
for name in "${required_env[@]}"; do
  if [ -z "${!name:-}" ]; then
    missing_env+=("${name}")
  fi
done

if [ "${#missing_env[@]}" -gt 0 ]; then
  printf 'missing required environment variables: %s\n' "${missing_env[*]}" >&2
  exit 1
fi

KEYCHAIN_PASSWORD_VALUE="${KEYCHAIN_PASSWORD:-$(uuidgen)}"
KEYCHAIN_PATH="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/linetta-signing.keychain-db"
CERTIFICATE_PATH="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/linetta-developer-id.p12"
NOTARY_ARCHIVE="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/Linetta-notary.zip"

cleanup() {
  rm -f "${CERTIFICATE_PATH}" "${NOTARY_ARCHIVE}"
  if [ -f "${KEYCHAIN_PATH}" ]; then
    security delete-keychain "${KEYCHAIN_PATH}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

decode_certificate() {
  if printf '%s' "${APPLE_CERTIFICATE}" | base64 --decode > "${CERTIFICATE_PATH}" 2>/dev/null; then
    return
  fi

  printf '%s' "${APPLE_CERTIFICATE}" | base64 -D > "${CERTIFICATE_PATH}"
}

echo "Importing Apple Developer ID certificate"
decode_certificate
security create-keychain -p "${KEYCHAIN_PASSWORD_VALUE}" "${KEYCHAIN_PATH}"
security default-keychain -s "${KEYCHAIN_PATH}"
security unlock-keychain -p "${KEYCHAIN_PASSWORD_VALUE}" "${KEYCHAIN_PATH}"
security set-keychain-settings -lut 21600 "${KEYCHAIN_PATH}"
security import "${CERTIFICATE_PATH}" \
  -k "${KEYCHAIN_PATH}" \
  -P "${APPLE_CERTIFICATE_PASSWORD}" \
  -T /usr/bin/codesign \
  -T /usr/bin/security
security set-key-partition-list \
  -S apple-tool:,apple:,codesign: \
  -s \
  -k "${KEYCHAIN_PASSWORD_VALUE}" \
  "${KEYCHAIN_PATH}"

SIGNING_IDENTITY="${APPLE_SIGNING_IDENTITY:-}"
if [ -z "${SIGNING_IDENTITY}" ]; then
  SIGNING_IDENTITY="$(
    security find-identity -v -p codesigning "${KEYCHAIN_PATH}" \
      | awk -F '"' '/Developer ID Application/ { print $2; exit }'
  )"
fi

if [ -z "${SIGNING_IDENTITY}" ]; then
  echo "Developer ID Application signing identity was not found in the imported certificate" >&2
  security find-identity -v -p codesigning "${KEYCHAIN_PATH}" >&2 || true
  exit 1
fi

echo "Signing nested Mach-O files"
while IFS= read -r -d '' candidate; do
  if file "${candidate}" | grep -q 'Mach-O'; then
    codesign --force --timestamp --options runtime --sign "${SIGNING_IDENTITY}" "${candidate}"
  fi
done < <(find "${APP_PATH}/Contents" -type f -print0)

echo "Signing app bundle"
codesign --force --timestamp --options runtime --sign "${SIGNING_IDENTITY}" "${APP_PATH}"
codesign --verify --deep --strict --verbose=2 "${APP_PATH}"

echo "Submitting app bundle for notarization"
rm -f "${NOTARY_ARCHIVE}"
ditto -c -k --keepParent "${APP_PATH}" "${NOTARY_ARCHIVE}"
xcrun notarytool submit "${NOTARY_ARCHIVE}" \
  --apple-id "${APPLE_ID}" \
  --team-id "${APPLE_TEAM_ID}" \
  --password "${APPLE_PASSWORD}" \
  --wait

echo "Stapling notarization ticket"
xcrun stapler staple "${APP_PATH}"
xcrun stapler validate "${APP_PATH}"
spctl -a -vvv -t exec "${APP_PATH}"
