#!/usr/bin/env bash
# Checks that Linetta Works sync blobs sealed by CryptoKit (the iOS build) and
# by Go's AES-GCM (every other build) open on either side. Needs macOS with
# Xcode: it builds engine/cryptokit for this Mac and links it into the vault
# tests.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

ARCH="$(uname -m)"
SDK="$(xcrun --sdk macosx --show-sdk-path)"
TOOLCHAIN_LIB="$(dirname "$(xcrun --find swiftc)")/../lib/swift/macosx"

(cd "$WORK" && xcrun --sdk macosx swiftc -parse-as-library -emit-library -static \
  -module-name LinettaCryptoKit -target "${ARCH}-apple-macos13.0" -O \
  -o "$WORK/liblinetta_cryptokit.a" "$ROOT/engine/cryptokit/LinettaCryptoKit.swift")

cd "$ROOT/engine"
CGO_ENABLED=1 \
CGO_LDFLAGS="-L$WORK -llinetta_cryptokit -L$SDK/usr/lib/swift -L$TOOLCHAIN_LIB" \
  go test -count=1 -tags cryptokit ./internal/vault/ "$@"
