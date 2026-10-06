#!/usr/bin/env bash
# Render latest.json, the manifest the in-app updater reads from
# releases/latest/download/latest.json.
#
# Each platform key is "<os>-<arch>-<installer>": an installed copy only
# takes the kind of package it came from. An asset is listed only when its
# .sig sits next to it, and a release with no signed asset is an error —
# without the manifest every installed copy fails its update check.
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "Usage: $0 <version> <release-url-base> <asset-dir>" >&2
  exit 2
fi

VERSION="$1" URL_BASE="${2%/}" ASSET_DIR="$3" node <<'NODE'
const fs = require("node:fs");
const path = require("node:path");
const { VERSION, URL_BASE, ASSET_DIR } = process.env;

// The macOS build is Apple Silicon only; Windows and Linux are x86_64.
const targets = [
  ["darwin-aarch64-app", (name) => name === "Linetta-macos.app.tar.gz"],
  ["linux-x86_64-appimage", (name) => name.endsWith(".AppImage")],
  ["linux-x86_64-deb", (name) => name.endsWith(".deb")],
  ["linux-x86_64-rpm", (name) => name.endsWith(".rpm")],
  ["windows-x86_64-nsis", (name) => name.endsWith("-setup.exe")],
  ["windows-x86_64-msi", (name) => name.endsWith(".msi")],
];

const names = fs.readdirSync(ASSET_DIR).sort();
const platforms = {};
for (const [target, matches] of targets) {
  const asset = names.find((name) => matches(name) && names.includes(`${name}.sig`));
  if (!asset) {
    console.error(`updater manifest: no signed asset for ${target}`);
    continue;
  }
  platforms[target] = {
    signature: fs.readFileSync(path.join(ASSET_DIR, `${asset}.sig`), "utf8").trim(),
    url: `${URL_BASE}/${encodeURIComponent(asset)}`,
  };
}

if (Object.keys(platforms).length === 0) {
  console.error("updater manifest: no signed assets; is TAURI_SIGNING_PRIVATE_KEY set?");
  process.exit(1);
}

const manifest = {
  version: VERSION,
  notes: `https://github.com/devlikebear/linetta/releases/tag/v${VERSION}`,
  pub_date: new Date().toISOString(),
  platforms,
};
process.stdout.write(JSON.stringify(manifest, null, 2) + "\n");
NODE
