#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PRINT_DEVICE_UDID=0
if [[ "${1:-}" == "--print-device-udid" ]]; then
  PRINT_DEVICE_UDID=1
  shift
fi
SIM_NAME="${1:-${IOS_SIM:-iPad Pro 11-inch (M4)}}"
if [[ "${2:-}" == "--print-device-udid" ]]; then
  PRINT_DEVICE_UDID=1
fi

require_tool() {
  local name="$1"
  command -v "${name}" >/dev/null 2>&1 || {
    echo "${name} is required for iOS simulator development." >&2
    exit 2
  }
}

require_tool node
require_tool pnpm
require_tool xcrun

device_udid="$(
  xcrun simctl list devices available --json |
    IOS_SIM_TARGET="${SIM_NAME}" node -e '
const fs = require("node:fs");
const target = process.env.IOS_SIM_TARGET;
const data = JSON.parse(fs.readFileSync(0, "utf8"));
const matches = [];

function runtimeVersion(runtime) {
  const match = runtime.match(/iOS-(\d+(?:-\d+)*)$/);
  return match ? match[1].split("-").map(Number) : [0];
}

for (const [runtime, devices] of Object.entries(data.devices)) {
  const version = runtimeVersion(runtime);
  for (const device of devices) {
    if (device.isAvailable && (device.name === target || device.udid === target)) {
      matches.push({ udid: device.udid, version });
    }
  }
}

matches.sort((left, right) => {
  const count = Math.max(left.version.length, right.version.length);
  for (let index = 0; index < count; index += 1) {
    const delta = (left.version[index] ?? 0) - (right.version[index] ?? 0);
    if (delta !== 0) {
      return delta;
    }
  }
  return left.udid.localeCompare(right.udid);
});

const chosen = matches.at(-1)?.udid ?? "";

if (chosen) {
  process.stdout.write(chosen);
}
'
)"

if [[ -z "${device_udid}" ]]; then
  echo "No available iOS simulator matched '${SIM_NAME}'." >&2
  echo 'Run `xcrun simctl list devices available` or override with IOS_SIM="iPhone 16".' >&2
  exit 2
fi

if [[ "${PRINT_DEVICE_UDID}" -eq 1 ]]; then
  printf '%s\n' "${device_udid}"
  exit 0
fi

echo "Preparing iOS simulator ${SIM_NAME} (${device_udid})..."
xcrun simctl boot "${device_udid}" >/dev/null 2>&1 || true
xcrun simctl bootstatus "${device_udid}" -b >/dev/null

cd "${ROOT}/apps/desktop"
exec pnpm tauri ios dev "${SIM_NAME}"
