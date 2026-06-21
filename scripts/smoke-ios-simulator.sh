#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_PATH="${ROOT}/apps/desktop/src-tauri/gen/apple/build/arm64-sim/Linetta.app"
SCREENSHOT_PATH="${LINETTA_IOS_SIM_SCREENSHOT:-/tmp/linetta-ios-sim-smoke.png}"

require_tool() {
  local name="$1"
  command -v "${name}" >/dev/null 2>&1 || {
    echo "${name} is required for the iOS simulator smoke test." >&2
    exit 2
  }
}

require_tool xcrun
require_tool nm
require_tool plutil

pick_device() {
  if [[ -n "${LINETTA_IOS_SIM_UDID:-}" ]]; then
    printf '%s\n' "${LINETTA_IOS_SIM_UDID}"
    return
  fi

  xcrun simctl list devices available | awk -F '[()]' '/iPhone/ && $0 !~ /unavailable/ { print $2; exit }'
}

device="$(pick_device)"
if [[ -z "${device}" ]]; then
  echo "No available iPhone simulator was found. Install a matching iOS simulator runtime first." >&2
  exit 2
fi

(cd "${ROOT}" && make build-mobile-ios-sim)

if [[ ! -d "${APP_PATH}" ]]; then
  echo "Expected iOS simulator app bundle was not produced: ${APP_PATH}" >&2
  exit 1
fi

info_plist="${APP_PATH}/Info.plist"
bundle_id="$(plutil -extract CFBundleIdentifier raw -o - "${info_plist}")"
executable_name="$(plutil -extract CFBundleExecutable raw -o - "${info_plist}")"
executable="${APP_PATH}/${executable_name}"
symbols="$(nm -gU "${executable}" 2>/dev/null)"

for symbol in LinettaEngineStart LinettaEngineCall LinettaEngineStop LinettaEngineFreeCString LinettaEngineSetNotifyCallback; do
  if ! grep -Fq "_${symbol}" <<<"${symbols}"; then
    echo "Missing embedded engine symbol in iOS app executable: ${symbol}" >&2
    exit 1
  fi
done

was_booted=0
if xcrun simctl list devices | grep -F "${device}" | grep -Fq "(Booted)"; then
  was_booted=1
fi

cleanup() {
  if [[ "${was_booted}" -eq 0 && "${LINETTA_IOS_SIM_KEEP_BOOTED:-0}" != "1" ]]; then
    xcrun simctl shutdown "${device}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

xcrun simctl boot "${device}" >/dev/null 2>&1 || true
xcrun simctl bootstatus "${device}" -b >/dev/null
xcrun simctl uninstall "${device}" "${bundle_id}" >/dev/null 2>&1 || true
xcrun simctl install "${device}" "${APP_PATH}"
xcrun simctl launch "${device}" "${bundle_id}" >/tmp/linetta-ios-sim-launch.txt

container="$(xcrun simctl get_app_container "${device}" "${bundle_id}" data)"
db_path="${container}/Library/Application Support/${bundle_id}/library.db"

for _ in $(seq 1 30); do
  if [[ -s "${db_path}" ]]; then
    break
  fi
  sleep 1
done

if [[ ! -s "${db_path}" ]]; then
  echo "iOS simulator app launched but did not create library.db at ${db_path}" >&2
  exit 1
fi

xcrun simctl io "${device}" screenshot "${SCREENSHOT_PATH}" >/dev/null

echo "iOS simulator smoke ok"
echo "device: ${device}"
echo "bundle: ${bundle_id}"
echo "database: ${db_path}"
echo "screenshot: ${SCREENSHOT_PATH}"
