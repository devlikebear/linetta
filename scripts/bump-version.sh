#!/usr/bin/env bash
# Keep desktop, Tauri shell, Cargo metadata, release packaging, and engine
# diagnostics versions aligned.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <semver>" >&2
  exit 2
fi

VERSION="$1"
if [[ ! "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-.+][0-9A-Za-z.-]+)?$ ]]; then
  echo "Version must look like semver, for example 0.1.0" >&2
  exit 2
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export ROOT VERSION

node <<'NODE'
const fs = require("node:fs");
const version = process.env.VERSION;
process.chdir(process.env.ROOT);
for (const file of [
  "apps/desktop/package.json",
  "apps/desktop/src-tauri/tauri.conf.json",
]) {
  const data = JSON.parse(fs.readFileSync(file, "utf8"));
  data.version = version;
  fs.writeFileSync(file, JSON.stringify(data, null, 2) + "\n");
}
NODE

perl -0pi -e 's/(^\[package\]\nname = "linetta-desktop"\nversion = ")[^"]+(")/$1$ENV{VERSION}$2/m' \
  "${ROOT}/apps/desktop/src-tauri/Cargo.toml"
perl -0pi -e 's/(name = "linetta-desktop"\nversion = ")[^"]+(")/$1$ENV{VERSION}$2/m' \
  "${ROOT}/apps/desktop/src-tauri/Cargo.lock"
perl -0pi -e 's/(const engineVersion = ")[^"]+(")/$1$ENV{VERSION}$2/' \
  "${ROOT}/engine/cmd/linetta-engine/main.go"
perl -0pi -e 's/(^\s*tag: v)[0-9]+\.[0-9]+\.[0-9]+([-.+][0-9A-Za-z.-]+)?/$1$ENV{VERSION}/m' \
  "${ROOT}/packaging/flathub/com.devlikebear.linetta.yml"

echo "Version bumped to ${VERSION}"
