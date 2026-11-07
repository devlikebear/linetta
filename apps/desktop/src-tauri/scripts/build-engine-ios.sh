#!/bin/sh
set -e

ENGINE_DIR="$(cd "$(dirname "$0")/../../../../engine" && pwd)"
OUT="${1:-/tmp/linetta-ios}"
BUILD_DIR="$OUT/build"
INCLUDE_DIR="$OUT/include"
mkdir -p "$OUT/device" "$OUT/sim" "$BUILD_DIR" "$INCLUDE_DIR"

mk_wrap() {
  sdk="$1"
  minflag="$2"
  wrapper="$BUILD_DIR/clangwrap-$sdk.sh"
  cat > "$wrapper" <<EOF
#!/bin/sh
exec "\$(xcrun --sdk $sdk --find clang)" -isysroot "\$(xcrun --sdk $sdk --show-sdk-path)" -arch arm64 $minflag "\$@"
EOF
  chmod +x "$wrapper"
  echo "$wrapper"
}

DEV_CC="$(mk_wrap iphoneos "-miphoneos-version-min=15.0")"
SIM_CC="$(mk_wrap iphonesimulator "-mios-simulator-version-min=15.0")"

# The engine seals Linetta Works sync blobs with CryptoKit on iOS
# (engine/cryptokit). Its objects go into the same archive, so the framework
# links on its own.
add_cryptokit() {
  sdk="$1"
  target="$2"
  archive="$3"
  (cd "$BUILD_DIR" && xcrun --sdk "$sdk" swiftc -parse-as-library -emit-library -static \
    -module-name LinettaCryptoKit -target "$target" -O \
    -o "$BUILD_DIR/libcryptokit-$sdk.a" "$ENGINE_DIR/cryptokit/LinettaCryptoKit.swift")
  libtool -static -o "$archive.merged" "$archive" "$BUILD_DIR/libcryptokit-$sdk.a"
  mv "$archive.merged" "$archive"
}

cd "$ENGINE_DIR"
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 CC="$DEV_CC" \
  go build -tags mobile -buildmode=c-archive -o "$OUT/device/liblinetta.a" ./cmd/linetta-ffi
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 CC="$SIM_CC" \
  go build -tags mobile -buildmode=c-archive -o "$OUT/sim/liblinetta.a" ./cmd/linetta-ffi
add_cryptokit iphoneos arm64-apple-ios15.0 "$OUT/device/liblinetta.a"
add_cryptokit iphonesimulator arm64-apple-ios15.0-simulator "$OUT/sim/liblinetta.a"
cp "$OUT/device/liblinetta.h" "$OUT/"
cp "$OUT/device/liblinetta.h" "$INCLUDE_DIR/"

rm -rf "$OUT/LinettaEngine.xcframework"
xcodebuild -create-xcframework \
  -library "$OUT/device/liblinetta.a" -headers "$INCLUDE_DIR" \
  -library "$OUT/sim/liblinetta.a" -headers "$INCLUDE_DIR" \
  -output "$OUT/LinettaEngine.xcframework"
echo "built: $OUT/LinettaEngine.xcframework"
